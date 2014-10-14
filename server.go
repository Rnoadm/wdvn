package main

import (
	"code.google.com/p/goprotobuf/proto"
	"github.com/Rnoadm/wdvn/res"
	"net"
)

func Listen(l net.Listener) {
	defer l.Close()

	broadcast, register := make(chan *res.Packet), make(chan chan<- *res.Packet)
	go Multicast(broadcast, register)

	ch := make(chan *res.Packet)
	register <- ch
	state := make(chan State)
	go Manager(ch, state)

	var next_man = res.Man(0)

	for next_man < res.Man_count {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		ch := make(chan *res.Packet)
		register <- ch

		go Serve(conn, ch, broadcast, <-state, next_man)
		next_man++
	}
}

func Multicast(broadcast <-chan *res.Packet, register <-chan chan<- *res.Packet) {
	var writers []chan<- *res.Packet

	for {
		select {
		case ch := <-register:
			writers = append(writers, ch)

		case packet := <-broadcast:
			for _, ch := range writers {
				ch <- packet
			}
		}
	}
}

func Serve(conn net.Conn, in <-chan *res.Packet, out chan<- *res.Packet, state State, man res.Man) {
	defer conn.Close()

	read, write := make(chan *res.Packet), make(chan *res.Packet)
	go Read(conn, read)
	go Write(conn, write)

	write <- &res.Packet{
		Type: res.Type_SelectMan.Enum(),
		Man:  man.Enum(),
	}

	for i := range state.Mans {
		write <- &res.Packet{
			Type: res.Type_MoveMan.Enum(),
			Man:  res.Man(i).Enum(),
			X:    proto.Int64(state.Mans[i].X),
			Y:    proto.Int64(state.Mans[i].Y),
		}
	}

	for {
		select {
		case p := <-in:
			write <- p

		case p := <-read:
			switch p.GetType() {
			case res.Type_MoveMan:
				go func(p *res.Packet) {
					out <- p
				}(&res.Packet{
					Type: res.Type_MoveMan.Enum(),
					Man:  man.Enum(),
					X:    p.X,
					Y:    p.Y,
				})
			}
		}
	}
}

func Manager(in <-chan *res.Packet, out chan<- State) {
	var state State

	for {
		select {
		case p := <-in:
			switch p.GetType() {
			case res.Type_MoveMan:
				state.Mans[p.GetMan()].X = p.GetX()
				state.Mans[p.GetMan()].Y = p.GetY()
			}

		case out <- state:
		}
	}
}
