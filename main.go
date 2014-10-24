package main

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/gob"
	"flag"
	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/pprof"
	"sync"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var (
	flagHost    = flag.String("host", "", "Start a dedicated server on this address. Example: \":7777\"")
	flagAddress = flag.String("addr", "", "address to connect to, like \""+net.JoinHostPort(externalIP(), "7777")+"\"")
	flagEditor  = flag.String("edit", "", "filename of level to edit")

	flagLevel       = flag.String("level", "", "filename of level to play")
	flagWidth       = flag.Int("w", 800, "width")
	flagHeight      = flag.Int("h", 300, "height")
	flagSplitScreen = flag.Bool("ss", false, "split screen")

	flagRecord     = flag.String("record", "", "record a replay to this file")
	flagRender     = flag.String("render", "", "play a replay from this file as YUV4MPEG2 on stdout")
	flagProfile    = flag.String("prof", "", "start a pprof server for developer use")
	flagCPUProfile = flag.Bool("cpuprofile", false, "profile to a file instead of starting a server")
)

var (
	quitRequest = make(chan struct{})
	quitWait    sync.WaitGroup
	replay      chan []byte
)

func main() {
	flag.Parse()

	if *flagHost != "" && *flagAddress != "" || *flagAddress != "" && *flagEditor != "" || *flagEditor != "" && *flagHost != "" {
		flag.Usage()
		os.Exit(1)
	}

	if *flagProfile != "" {
		if *flagCPUProfile {
			f, err := os.Create(*flagProfile)
			if err != nil {
				panic(err)
			}
			defer f.Close()

			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		} else {
			go func() {
				log.Println(http.ListenAndServe(*flagProfile, nil))
			}()
		}
	}

	if *flagRender != "" {
		f, err := os.Open(*flagRender)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		r, err := gzip.NewReader(f)
		if err != nil {
			log.Fatal(err)
		}
		defer r.Close()

		err = EncodeVideo(os.Stdout, r)
		if err != nil {
			log.Fatal(err)
		}

		return
	}

	signalch := make(chan os.Signal)
	signal.Notify(signalch, os.Interrupt)
	go func() {
		<-signalch
		log.Println("Requesting exit. ^C again to terminate immediately.")
		close(quitRequest)
	}()

	level := FooLevel
	if *flagLevel != "" {
		level = &World{}
		func() {
			f, err := os.Open(*flagLevel)
			if err != nil {
				panic(err)
			}
			defer f.Close()

			err = gob.NewDecoder(f).Decode(level)
			if err != nil {
				panic(err)
			}
		}()
	}

	if *flagHost != "" {
		l, err := net.Listen("tcp", *flagHost)
		if err != nil {
			log.Fatal(err)
		}

		quitWait.Add(1)
		go Listen(l, level)
		quitWait.Wait()
		return
	}

	if *flagRecord != "" {
		f, err := os.Create(*flagRecord)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		w, err := gzip.NewWriterLevel(f, gzip.BestCompression)
		if err != nil {
			panic(err)
		}
		defer w.Close()

		replay = make(chan []byte, 64)

		quitWait.Add(1)
		go func() {
			defer quitWait.Done()

			var l [binary.MaxVarintLen64]byte

			i := binary.PutUvarint(l[:], 1)

			n, err := w.Write(l[:i])
			if err == nil && n != i {
				err = io.ErrShortWrite
			}
			if err != nil {
				panic(err)
			}

			for {
				select {
				case b := <-replay:
					i := binary.PutUvarint(l[:], uint64(len(b)))

					n, err = w.Write(l[:i])
					if err == nil && n != i {
						err = io.ErrShortWrite
					}
					if err != nil {
						panic(err)
					}

					n, err = w.Write(b)
					if err == nil && n != len(b) {
						err = io.ErrShortWrite
					}
					if err != nil {
						panic(err)
					}

				case <-quitRequest:
					return
				}
			}
		}()
	}

	if *flagAddress == "" {
		addr := externalIP()

		l, err := net.Listen("tcp", net.JoinHostPort(addr, "0"))
		if err != nil {
			panic(err)
		}

		quitWait.Add(1)
		go Listen(l, level)

		*flagAddress = l.Addr().String()
	}

	quitWait.Add(1)
	go Client(*flagAddress)
	wde.Run()
	quitWait.Wait()
}

// from http://stackoverflow.com/a/23558495/2664560
func externalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			return ip.String()
		}
	}
	// last resort: ipv4 loopback
	return "127.0.0.1"
}
