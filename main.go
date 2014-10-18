package main

import (
	"encoding/gob"
	"flag"
	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
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
		go http.ListenAndServe(*flagProfile, nil)
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
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			panic(err)
		}

		l, err := net.Listen("tcp", addrs[0].String()+":0")
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
