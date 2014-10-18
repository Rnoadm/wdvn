package main

import (
	"encoding/gob"
	"flag"
	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
)

var (
	flagHost    = flag.String("host", "", "host this game, like \":7777\"")
	flagAddress = flag.String("addr", "", "address to connect to, like \"192.168.1.100:7777\"")
	flagLevel   = flag.String("level", "", "filename of level to play")
	flagEditor  = flag.String("edit", "", "filename of level to edit")
	flagProfile = flag.String("prof", "", "start a pprof server for developer use")
)

func main() {
	flag.Parse()

	if *flagProfile != "" {
		go func() {
			log.Println(http.ListenAndServe(*flagProfile, nil))
		}()
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

		conn, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			panic(err)
		}
		go Client(conn)

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
		conn, err := net.Dial("tcp", *flagAddress)
		if err != nil {
			panic(err)
		}
		go Client(conn)

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
