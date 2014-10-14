package main

import (
	"code.google.com/p/goprotobuf/proto"
	"encoding/binary"
	"github.com/Rnoadm/wdvn/res"
	"io"
	"net"
)

func Read(conn net.Conn, packets chan<- *res.Packet) {
	for {
		var l uint64
		err := binary.Read(conn, binary.LittleEndian, &l)
		if err != nil {
			panic(err)
		}

		b := make([]byte, l)
		_, err = io.ReadFull(conn, b)
		if err != nil {
			panic(err)
		}

		p := new(res.Packet)
		err = proto.Unmarshal(b, p)
		if err != nil {
			panic(err)
		}

		packets <- p
	}
}

func Write(conn net.Conn, packets <-chan *res.Packet) {
	for p := range packets {
		b, err := proto.Marshal(p)
		if err != nil {
			panic(err)
		}

		l := uint64(len(b))

		err = binary.Write(conn, binary.LittleEndian, &l)
		if err != nil {
			panic(err)
		}
		n, err := conn.Write(b)
		if err == nil && n != len(b) {
			err = io.ErrShortWrite
		}
		if err != nil {
			panic(err)
		}
	}
}
