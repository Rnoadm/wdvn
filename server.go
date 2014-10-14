package main

import (
	"code.google.com/p/goprotobuf/proto"
	"github.com/Rnoadm/wdvn/res"
	"image"
	"net"
	"time"
)

func Listen(l net.Listener) {
	defer l.Close()

	broadcast, register, unregister := make(chan *res.Packet), make(chan chan<- *res.Packet), make(chan chan<- *res.Packet)
	go Multicast(broadcast, register, unregister)

	ch := make(chan *res.Packet)
	register <- ch
	state := make(chan State)
	go Manager(ch, state)

	disconnect := make(chan res.Man, res.Man_count)

	for next_man := res.Man(0); next_man < res.Man_count; next_man++ {
		disconnect <- next_man
	}

	for next_man := range disconnect {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		ch := make(chan *res.Packet)
		register <- ch

		go Serve(conn, ch, broadcast, state, next_man, func(man res.Man) func() {
			return func() {
				disconnect <- man
				go func() {
					for _ = range ch {
						// do nothing
					}
				}()
				unregister <- ch
			}
		}(next_man))
	}
}

func Multicast(broadcast <-chan *res.Packet, register, unregister <-chan chan<- *res.Packet) {
	var writers []chan<- *res.Packet

	for {
		select {
		case ch := <-register:
			writers = append(writers, ch)

		case ch := <-unregister:
			for i, ch2 := range writers {
				if ch == ch2 {
					writers = append(writers[:i], writers[i+1:]...)
					close(ch)
					break
				}
			}

		case packet := <-broadcast:
			for _, ch := range writers {
				ch <- packet
			}
		}
	}
}

func Serve(conn net.Conn, in <-chan *res.Packet, out chan<- *res.Packet, statech <-chan State, man res.Man, disconnect func()) {
	defer disconnect()
	defer conn.Close()

	read, write := make(chan *res.Packet), make(chan *res.Packet)
	go Read(conn, read)
	go Write(conn, write)

	write <- &res.Packet{
		Type: res.Type_SelectMan.Enum(),
		Man:  man.Enum(),
	}

	state := <-statech
	for i := range state.Mans {
		write <- &res.Packet{
			Type: res.Type_MoveMan.Enum(),
			Man:  res.Man(i).Enum(),
			X:    proto.Int64(state.Mans[i].X),
			Y:    proto.Int64(state.Mans[i].Y),
		}
	}

	ping := time.NewTicker(time.Second)
	defer ping.Stop()
	lastPing := time.Now()

	tick := time.NewTicker(time.Second / 100)
	defer tick.Stop()
	var mouse *image.Point

	for {
		select {
		case p := <-in:
			write <- p

		case p := <-read:
			switch p.GetType() {
			case res.Type_Ping:
				lastPing = time.Now()

			case res.Type_MoveMan:
				go Send(out, &res.Packet{
					Type: res.Type_MoveMan.Enum(),
					Man:  man.Enum(),
					X:    p.X,
					Y:    p.Y,
				})

			case res.Type_Mouse:
				if p.X == nil {
					mouse = nil
				} else {
					mouse = &image.Point{
						int(p.GetX()),
						int(p.GetY()),
					}
				}
			}

		case <-tick.C:
			if mouse != nil {
				state := <-statech
				go Send(out, &res.Packet{
					Type: res.Type_MoveMan.Enum(),
					Man:  man.Enum(),
					X:    proto.Int64(state.Mans[man].X + int64(mouse.X)),
					Y:    proto.Int64(state.Mans[man].Y + int64(mouse.Y)),
				})
			}

		case <-ping.C:
			go Send(out, &res.Packet{
				Type: res.Type_Ping.Enum(),
			})

			if time.Since(lastPing) > time.Second*5 {
				return
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
