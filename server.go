package main

import (
	"code.google.com/p/goprotobuf/proto"
	"github.com/BenLubar/bindiff"
	"github.com/Rnoadm/wdvn/res"
	"log"
	"net"
	"sync/atomic"
	"time"
)

func Listen(l net.Listener, world *World) {
	defer quitWait.Done()
	defer l.Close()

	var (
		broadcast  = make(chan *res.Packet)
		register   = make(chan chan<- *res.Packet)
		unregister = make(chan chan<- *res.Packet)
		input      = make(chan *res.Packet)
		state      = make(chan (<-chan []byte))
		connection = make(chan bool)
		accept     = make(chan net.Conn)
		connected  [res.Man_count]uint64
	)
	defer close(register)
	quitWait.Add(3)
	go Multicast(broadcast, register, unregister)
	go Manager(input, state, connection, broadcast, world)
	go Accept(accept, l)

	worldPacket := &res.Packet{
		Type: Type_World,
		Data: Encode(world),
	}

	for {
		select {
		case conn := <-accept:
			ch := make(chan *res.Packet)
			register <- ch
			connection <- true

			quitWait.Add(1)
			go Serve(conn, ch, broadcast, state, worldPacket, input, &connected, func() {
				go func() {
					for _ = range ch {
						// discard
					}
				}()
				unregister <- ch
				connection <- false
				quitWait.Done()
			})

		case <-quitRequest:
			return
		}
	}
}

func Accept(accept chan<- net.Conn, l net.Listener) {
	defer close(accept)
	defer quitWait.Done()
	for {
		conn, err := l.Accept()
		if err == nil {
			select {
			case accept <- conn:
			case <-quitRequest:
				return
			}
		} else {
			select {
			case <-quitRequest:
				return
			default:
			}
			log.Print(err)
			if ne, ok := err.(net.Error); !ok || !ne.Temporary() {
				return
			}
		}
	}
}

func Multicast(broadcast <-chan *res.Packet, register, unregister <-chan chan<- *res.Packet) {
	defer quitWait.Done()
	writers := make(map[chan<- *res.Packet]struct{})

	for {
		select {
		case ch, ok := <-register:
			if !ok {
				return
			}
			writers[ch] = struct{}{}

		case ch := <-unregister:
			delete(writers, ch)
			close(ch)

		case packet := <-broadcast:
			for ch := range writers {
				go Send(ch, packet)
			}
		}
	}
}

func Serve(conn net.Conn, in <-chan *res.Packet, out chan<- *res.Packet, state <-chan <-chan []byte, world *res.Packet, input chan<- *res.Packet, connected *[res.Man_count]uint64, disconnect func()) {
	defer disconnect()
	defer conn.Close()

	read, write, errors := make(chan *res.Packet), make(chan *res.Packet), make(chan error, 2)
	defer close(write)
	go Read(conn, read, errors)
	go Write(conn, write, errors)

	var man res.Man
	// check for an empty slot
	for man = 0; man < res.Man_count; man++ {
		if atomic.CompareAndSwapUint64(&(*connected)[man], 0, 1) {
			break
		}
	}
	// multiple people control normal man as a last resort
	if man == res.Man_count {
		man = res.Man_Normal
		atomic.AddUint64(&(*connected)[man], 1)
	}
	pman := man.Enum()
	// leave the character when we disconnect
	defer func() {
		release := &res.Packet{Type: Type_Input}
		proto.Merge(release, ReleaseAll)
		release.Man = pman

		go Send(input, release)

		atomic.AddUint64(&(*connected)[man], ^uint64(0))

	}()

	inputCache := new(res.Packet)

	log.Println(conn.RemoteAddr(), "connected for", man)

	// tell the client which man they are
	write <- &res.Packet{
		Type: Type_SelectMan,
		Man:  pman,
	}

	// send the world
	write <- world

	// send full state to the client
	write <- &res.Packet{
		Type: Type_FullState,
		Data: <-<-state,
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
				// we will return from the error channel. stop checking for new packets
				// so we don't busy wait on a closed channel.
				read = nil
				continue
			}

			switch p.GetType() {
			case res.Type_Ping:
				var t time.Time
				err := t.GobDecode(p.GetData())
				if err != nil {
					log.Println("failed to decode ping packet:", err)
					return
				}
				inputCache.Tick = proto.Uint64(uint64(time.Since(t)))
				go Send(input, &res.Packet{
					Type: Type_Input,
					Man:  pman,
					Tick: inputCache.Tick,
				})
				lastPing = time.Now()

			case res.Type_SelectMan:
				if atomic.CompareAndSwapUint64(&(*connected)[p.GetMan()], 0, 1) {
					log.Println(conn.RemoteAddr(), "switched from", man, "to", p.GetMan())

					release, press := &res.Packet{Type: Type_Input}, &res.Packet{Type: Type_Input}
					proto.Merge(release, ReleaseAll)
					proto.Merge(press, inputCache)
					release.Man, press.Man = pman, p.Man

					go Send(input, release)
					go Send(input, press)

					atomic.AddUint64(&(*connected)[man], ^uint64(0))
					man, pman = p.GetMan(), p.Man

					go Send(write, p)
				}

			case res.Type_Input:
				p.Man = pman
				p.Data = nil
				p.Tick = nil
				go Send(input, p)
				proto.Merge(inputCache, p)

			case res.Type_FullState:
				log.Println(conn.RemoteAddr(), "requested full state update")

				go Send(write, &res.Packet{
					Type: Type_FullState,
					Data: <-<-state,
				})
			}

		case <-ping.C:
			b, err := time.Now().GobEncode()
			if err != nil {
				panic(err)
			}
			go Send(out, &res.Packet{
				Type: Type_Ping,
				Data: b,
			})

			if time.Since(lastPing) > time.Second*5 {
				log.Println(conn.RemoteAddr(), man, "disconnected: ping timeout")
				return
			}

		case err := <-errors:
			log.Println(conn.RemoteAddr(), man, "disconnected:", err)
			return
		}
	}
}

func Manager(in <-chan *res.Packet, out chan<- <-chan []byte, connection <-chan bool, broadcast chan<- *res.Packet, world *World) {
	defer quitWait.Done()

	var (
		state            = NewState(world)
		input            [res.Man_count]res.Packet
		connection_count int
		prev             []byte
		tick             = time.NewTicker(time.Second / TicksPerSecond)
		ch               = make(chan []byte)
	)
	defer tick.Stop()

	if replay != nil {
		replay <- append(append([]byte{0}, Encode(world)...), Encode(state)...)
	}

	for {
		if connection_count == 0 {
			select {
			case b := <-connection:
				if b {
					connection_count++
				} else {
					panic("connection count underflow")
				}
			case <-quitRequest:
				return
			}
		}

		select {
		case p := <-in:
			switch p.GetType() {
			case res.Type_Input:
				proto.Merge(&input[p.GetMan()], p)
			}

		case out <- ch:
			ch <- Encode(state)

		case <-tick.C:
			t := state.Tick

			state.Update(&input)

			cur := Encode(state)
			diff := bindiff.Diff(prev, cur, 5)
			if replay != nil {
				replay <- append([]byte{1}, diff...)
			}
			prev = cur

			go Send(broadcast, &res.Packet{
				Type: Type_StateDiff,
				Tick: proto.Uint64(t),
				Data: diff,
			})

		case b := <-connection:
			if b {
				connection_count++
				if connection_count < 0 {
					panic("connection count overflow")
				}
			} else {
				connection_count--
				if connection_count < 0 {
					panic("connection count underflow")
				}
			}

		case <-quitRequest:
			return
		}
	}
}
