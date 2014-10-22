package main

import (
	"encoding/gob"
	"flag"
	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var (
	flagHost    = flag.String("host", "", "host this game, like \":7777\"")
	flagAddress = flag.String("addr", "", "address to connect to, like \"192.168.1.100:7777\"")

	flagLevel  = flag.String("level", "", "filename of level to play")
	flagEditor = flag.String("edit", "", "filename of level to edit")

	flagSplitScreen = flag.Bool("ss", false, "render split screen")

	flagProfile    = flag.String("prof", "", "start a pprof server for developer use")
	flagCPUProfile = flag.Bool("cpuprofile", false, "profile to a file instead of starting a server")
)

func main() {
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

	if *flagEditor != "" {
		go Editor(*flagEditor)

		wde.Run()
		return
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
		addr, err := externalIP()
		if err != nil {
			panic(err)
		}

		l, err := net.Listen("tcp", addr+":0")
		if err != nil {
			panic(err)
		}

		go Listen(l, level)
		go Client(l.Addr().String())

		wde.Run()
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
	} else if *flagHost != "" {
		select {}
	} else {
		flag.PrintDefaults()
	}
}

// from http://stackoverflow.com/a/23558495/2664560
func externalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
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
			return "", err
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
			return ip.String(), nil
		}
	}
	// last resort: ipv4 loopback
	return "127.0.0.1", nil
}
