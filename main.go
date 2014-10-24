package main

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/gob"
	"flag"
	"fmt"
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
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var (
	groupConnection = flag.NewFlagSet("Connection", flag.ExitOnError)
	flagHost        = groupConnection.String("host", "", "host this game, like \":7777\"")
	flagAddress     = groupConnection.String("addr", "", "address to connect to, like \""+externalIP()+":7777\"")

	groupLevel = flag.NewFlagSet("Level", flag.ExitOnError)
	flagLevel  = groupLevel.String("level", "", "filename of level to play")
	flagEditor = groupLevel.String("edit", "", "filename of level to edit")

	groupRendering  = flag.NewFlagSet("Rendering", flag.ExitOnError)
	flagWidth       = groupRendering.Int("w", 800, "width")
	flagHeight      = groupRendering.Int("h", 300, "height")
	flagSplitScreen = groupRendering.Bool("ss", false, "split screen")

	groupDeveloper = flag.NewFlagSet("Developer", flag.ExitOnError)
	flagRecord     = groupDeveloper.String("record", "", "record a replay to this file")
	flagPlayback   = groupDeveloper.String("replay", "", "play a replay from this file as YUV4MPEG2 on stdout")
	flagProfile    = groupDeveloper.String("prof", "", "start a pprof server for developer use")
	flagCPUProfile = groupDeveloper.Bool("cpuprofile", false, "profile to a file instead of starting a server")
)

var (
	quit   = make(chan struct{})
	replay chan []byte
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])

		fmt.Fprintf(os.Stderr, "\nConnection:\n")
		groupConnection.PrintDefaults()

		fmt.Fprintf(os.Stderr, "\nRendering:\n")
		groupRendering.PrintDefaults()

		fmt.Fprintf(os.Stderr, "\nLevel:\n")
		groupLevel.PrintDefaults()

		fmt.Fprintf(os.Stderr, "\nDeveloper:\n")
		groupDeveloper.PrintDefaults()
	}

	flag.Parse()

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

	signalch := make(chan os.Signal, 1)
	signal.Notify(signalch, os.Interrupt, os.Kill)
	go func() {
		<-signalch
		close(quit)
	}()

	if *flagEditor != "" {
		go Editor(*flagEditor)

		wde.Run()
		<-quit
		return
	}

	if *flagPlayback != "" {
		f, err := os.Open(*flagPlayback)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		r, err := gzip.NewReader(f)
		if err != nil {
			panic(err)
		}
		defer r.Close()

		err = EncodeVideo(os.Stdout, r)
		if err != nil {
			panic(err)
		}

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

		finishReplay := make(chan chan struct{})
		defer func() {
			ch := make(chan struct{})
			finishReplay <- ch
			<-ch
		}()

		replay = make(chan []byte, 64)

		go func() {
			var l [binary.MaxVarintLen64]byte

			for {
				select {
				case b := <-replay:
					i := binary.PutUvarint(l[:], uint64(len(b)))

					n, err := w.Write(l[:i])
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

				case ch := <-finishReplay:
					ch <- struct{}{}
					return
				}
			}
		}()
	}

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

	if *flagHost == "" && *flagAddress == "" {
		addr := externalIP()

		l, err := net.Listen("tcp", addr+":0")
		if err != nil {
			panic(err)
		}

		go Listen(l, level)
		go Client(l.Addr().String())

		wde.Run()
		<-quit
		return
	}

	if *flagHost != "" {
		l, err := net.Listen("tcp", *flagHost)
		if err != nil {
			panic(err)
		}

		go Listen(l, level)
	}
	if *flagAddress != "" {
		go Client(*flagAddress)

		wde.Run()
	}
	<-quit
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
