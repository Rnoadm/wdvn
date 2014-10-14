package main

import (
	"code.google.com/p/goprotobuf/proto"
	"encoding/binary"
	"github.com/Rnoadm/wdvn/res"
	"io"
	"log"
	"net"
)

const (
	PixelSize        = 64
	Gravity          = PixelSize * 9         // per tick
	TerminalVelocity = PixelSize * 1000      // flat
	MinimumVelocity  = PixelSize * PixelSize // unit stops moving if on ground
	Friction         = 100                   // 1/Friction of the velocity is removed per tick
)

type State struct {
	Mans [res.Man_count]struct {
		Position     struct{ X, Y int64 }
		Velocity     struct{ X, Y int64 }
		Acceleration struct{ X, Y int64 }
	}
}

func Read(conn net.Conn, packets chan<- *res.Packet) {
	defer close(packets)

	for {
		var l uint64
		err := binary.Read(conn, binary.LittleEndian, &l)
		if err != nil {
			log.Println(err)
			return
		}

		b := make([]byte, l)
		_, err = io.ReadFull(conn, b)
		if err != nil {
			log.Println(err)
			return
		}

		p := new(res.Packet)
		err = proto.Unmarshal(b, p)
		if err != nil {
			log.Println(err)
			return
		}

		packets <- p
	}
}

func Write(conn net.Conn, packets <-chan *res.Packet) {
	for p := range packets {
		b, err := proto.Marshal(p)
		if err != nil {
			log.Println(err)
			return
		}

		l := uint64(len(b))

		err = binary.Write(conn, binary.LittleEndian, &l)
		if err != nil {
			log.Println(err)
			return
		}
		n, err := conn.Write(b)
		if err == nil && n != len(b) {
			err = io.ErrShortWrite
		}
		if err != nil {
			log.Println(err)
			return
		}
	}
}

func Send(ch chan<- *res.Packet, p *res.Packet) {
	ch <- p
}
