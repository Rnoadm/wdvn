package main

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"encoding/gob"
	"github.com/BenLubar/bindiff"
	"github.com/Rnoadm/wdvn/res"
	"log"
	"net"
	"sync/atomic"
	"time"
)

func Listen(l net.Listener, world *World) {
	defer l.Close()

	broadcast, register, unregister := make(chan *res.Packet), make(chan chan<- *res.Packet), make(chan chan<- *res.Packet)
	go Multicast(broadcast, register, unregister)

	input := make(chan *res.Packet)
	state := make(chan State)
	go Manager(input, state, broadcast, world)

	var connected [res.Man_count]uint64

	for {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		ch := make(chan *res.Packet)
		register <- ch

		go Serve(conn, ch, broadcast, state, input, &connected, func() {
			go func() {
				for _ = range ch {
					// do nothing
				}
			}()
			unregister <- ch
		})
	}
}

func Multicast(broadcast <-chan *res.Packet, register, unregister <-chan chan<- *res.Packet) {
	writers := make(map[chan<- *res.Packet]struct{})

	for {
		select {
		case ch := <-register:
			writers[ch] = struct{}{}

		case ch := <-unregister:
			delete(writers, ch)
			close(ch)

		case packet := <-broadcast:
			for ch := range writers {
				ch <- packet
			}
		}
	}
}

func Serve(conn net.Conn, in <-chan *res.Packet, out chan<- *res.Packet, statech <-chan State, input chan<- *res.Packet, connected *[res.Man_count]uint64, disconnect func()) {
	defer disconnect()
	defer conn.Close()

	read, write := make(chan *res.Packet), make(chan *res.Packet)
	go Read(conn, read)
	go Write(conn, write)

	var man res.Man
	// check for an empty slot
	for man = 0; man < res.Man_count; man++ {
		if atomic.CompareAndSwapUint64(&((*connected)[man]), 0, 1) {
			break
		}
	}
	// multiple people control normal man as a last resort
	if man == res.Man_count {
		man = res.Man_Normal
		atomic.AddUint64(&((*connected)[man]), 1)
	}
	// leave the character when we disconnect
	defer func() {
		atomic.AddUint64(&((*connected)[man]), ^uint64(0))
	}()

	log.Println(conn.RemoteAddr(), "connected for", man)

	// tell the client which man they are
	write <- &res.Packet{
		Type: res.Type_SelectMan.Enum(),
		Man:  man.Enum(),
	}

	// send full state to the client
	{
		state := <-statech
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

		case p, ok := <-read:
			if !ok {
				log.Println(conn.RemoteAddr(), man, "disconnected: read error")
				return
			}

			switch p.GetType() {
			case res.Type_Ping:
				lastPing = time.Now()

			case res.Type_SelectMan:
				if atomic.CompareAndSwapUint64(&((*connected)[p.GetMan()]), 0, 1) {
					log.Println(conn.RemoteAddr(), "switched from", man, "to", p.GetMan())

					atomic.AddUint64(&((*connected)[man]), ^uint64(0))
					man = p.GetMan()
					go Send(write, p)
				}

			case res.Type_Input:
				p.Man = man.Enum()
				go Send(input, p)

			case res.Type_FullState:
				log.Println(conn.RemoteAddr(), "requested full state update")

				state := <-statech

				var buf bytes.Buffer

				err := gob.NewEncoder(&buf).Encode(&state)
				if err != nil {
					panic(err)
				}

				go Send(write, &res.Packet{
					Type: res.Type_FullState.Enum(),
					Data: buf.Bytes(),
				})
			}

		case <-ping.C:
			go Send(out, &res.Packet{
				Type: res.Type_Ping.Enum(),
			})

			if time.Since(lastPing) > time.Second*5 {
				log.Println(conn.RemoteAddr(), man, "disconnected: ping timeout")
				return
			}
		}
	}
}

func Manager(in <-chan *res.Packet, out chan<- State, broadcast chan<- *res.Packet, world *World) {
	var state State
	var input [res.Man_count]res.Packet

	state.Lives = DefaultLives
	state.World = world
	for i := range state.Mans {
		state.Mans[i].Health = DefaultHealth
	}

	tick := time.Tick(time.Second / TicksPerSecond)

	var prev []byte

	for {
		select {
		case p := <-in:
			switch p.GetType() {
			case res.Type_Input:
				proto.Merge(&input[p.GetMan()], p)
			}

		case out <- state:

		case <-tick:
			t := state.Tick

			state.Update(&input)

			var buf bytes.Buffer
			err := gob.NewEncoder(&buf).Encode(&state)
			if err != nil {
				panic(err)
			}
			diff := bindiff.Diff(prev, buf.Bytes(), 5)
			prev = buf.Bytes()

			go Send(broadcast, &res.Packet{
				Type: res.Type_StateDiff.Enum(),
				Tick: proto.Uint64(t),
				Data: diff,
			})
		}
	}
}
