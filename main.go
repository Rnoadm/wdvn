package main

import (
	"flag"
	"github.com/skelterjohn/go.wde"
	_ "github.com/skelterjohn/go.wde/init"
	"net"
)

var (
	flagHost    = flag.String("host", "", "host this game, like \":7777\"")
	flagAddress = flag.String("addr", "", "address to connect to, like \"192.168.1.100:7777\"")
	flagEditor  = flag.String("edit", "", "filename of level to edit")
)

func main() {
	flag.Parse()

	if *flagEditor != "" {
		go Editor(*flagEditor)

		wde.Run()
		return
	}

	if *flagHost != "" {
		l, err := net.Listen("tcp", *flagHost)
		if err != nil {
			panic(err)
		}

		go Listen(l)
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
