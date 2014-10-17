package main

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"encoding/gob"
	"github.com/Rnoadm/wdvn/res"
	"net"
	"time"
)

func Listen(l net.Listener, world *World) {
	defer l.Close()

	broadcast, register, unregister := make(chan *res.Packet), make(chan chan<- *res.Packet), make(chan chan<- *res.Packet)
	go Multicast(broadcast, register, unregister)

	ch := make(chan *res.Packet)
	register <- ch
	state := make(chan State)
	go Manager(ch, state, broadcast, world)

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
	{
		var buf bytes.Buffer

		err := gob.NewEncoder(&buf).Encode(&state)
		if err != nil {
			panic(err)
		}

		write <- &res.Packet{
			Type: res.Type_FullState.Enum(),
			Data: buf.Bytes(),
		}
	}

	ping := time.NewTicker(time.Second)
	defer ping.Stop()
	lastPing := time.Now()

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

			case res.Type_Input:
				p.Man = man.Enum()
				go Send(out, p)
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

func Manager(in <-chan *res.Packet, out chan<- State, broadcast chan<- *res.Packet, world *World) {
	var state State
	var input [res.Man_count]res.Packet

	state.World = world

	tick := time.Tick(time.Second / TicksPerSecond)

	for {
		select {
		case p := <-in:
			switch p.GetType() {
			case res.Type_Input:
				proto.Merge(&input[p.GetMan()], p)
			}

		case out <- state:

		case <-tick:
			prev := state

			state.Update(&input)

			for i := range state.Mans {
				if state.Mans[i].Position.X/PixelSize != prev.Mans[i].Position.X/PixelSize ||
					state.Mans[i].Position.Y/PixelSize != prev.Mans[i].Position.Y/PixelSize {
					go Send(broadcast, &res.Packet{
						Type: res.Type_MoveMan.Enum(),
						Man:  res.Man(i).Enum(),
						X:    proto.Int64(state.Mans[i].Position.X),
						Y:    proto.Int64(state.Mans[i].Position.Y),
					})
				}
			}
		}
	}
}
