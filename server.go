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

	var (
		broadcast  = make(chan *res.Packet)
		register   = make(chan chan<- *res.Packet)
		unregister = make(chan chan<- *res.Packet)
		input      = make(chan *res.Packet)
		state      = make(chan (<-chan []byte))
		connection = make(chan bool)
	)
	go Multicast(broadcast, register, unregister)
	go Manager(input, state, connection, broadcast, world)

	var connected [res.Man_count]uint64

	for {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		ch := make(chan *res.Packet)
		register <- ch
		connection <- true

		go Serve(conn, ch, broadcast, state, world, input, &connected, func() {
			go func() {
				for _ = range ch {
					// do nothing
				}
			}()
			unregister <- ch
			connection <- false
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
				go Send(ch, packet)
			}
		}
	}
}

func Serve(conn net.Conn, in <-chan *res.Packet, out chan<- *res.Packet, state <-chan <-chan []byte, world *World, input chan<- *res.Packet, connected *[res.Man_count]uint64, disconnect func()) {
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
		atomic.AddUint64(&(*connected)[man], ^uint64(0))
	}()

	log.Println(conn.RemoteAddr(), "connected for", man)

	// tell the client which man they are
	write <- &res.Packet{
		Type: Type_SelectMan,
		Man:  pman,
	}

	// send the world
	{
		var buf bytes.Buffer

		err := gob.NewEncoder(&buf).Encode(world)
		if err != nil {
			panic(err)
		}

		write <- &res.Packet{
			Type: Type_World,
			Data: buf.Bytes(),
		}
	}

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
				lastPing = time.Now()

			case res.Type_SelectMan:
				if atomic.CompareAndSwapUint64(&(*connected)[p.GetMan()], 0, 1) {
					log.Println(conn.RemoteAddr(), "switched from", man, "to", p.GetMan())

					atomic.AddUint64(&(*connected)[man], ^uint64(0))
					man, pman = p.GetMan(), p.Man
					go Send(write, p)
				}

			case res.Type_Input:
				p.Man = pman
				go Send(input, p)

			case res.Type_FullState:
				log.Println(conn.RemoteAddr(), "requested full state update")

				go Send(write, &res.Packet{
					Type: Type_FullState,
					Data: <-<-state,
				})
			}

		case <-ping.C:
			go Send(out, &res.Packet{
				Type: Type_Ping,
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
	var (
		state            State
		input            [res.Man_count]res.Packet
		connection_count int
		prev             []byte
		tick             = time.Tick(time.Second / TicksPerSecond)
		ch               = make(chan []byte)
	)
	state.world = world
	for i := range state.Mans {
		state.Mans[i].Position = state.FindSpawnPosition(ManSize)
		state.Mans[i].Size = ManSize
		state.Mans[i].Health = ManHealth
	}
	state.Mans[res.Man_Whip].UnitData = &WhipMan{
		ManUnitData: ManUnitData{
			Man_:        res.Man_Whip,
			Lives_:      ManLives,
			Checkpoint_: state.SpawnPoint,
		},
	}
	state.Mans[res.Man_Density].UnitData = &DensityMan{
		ManUnitData: ManUnitData{
			Man_:        res.Man_Density,
			Lives_:      ManLives,
			Checkpoint_: state.SpawnPoint,
		},
	}
	state.Mans[res.Man_Vacuum].UnitData = &VacuumMan{
		ManUnitData: ManUnitData{
			Man_:        res.Man_Vacuum,
			Lives_:      ManLives,
			Checkpoint_: state.SpawnPoint,
		},
	}
	state.Mans[res.Man_Normal].UnitData = &NormalMan{
		ManUnitData: ManUnitData{
			Man_:        res.Man_Normal,
			Lives_:      ManLives,
			Checkpoint_: state.SpawnPoint,
		},
	}
	state.Units = make(map[uint64]*Unit)

	for {
		if connection_count == 0 {
			if b := <-connection; b {
				connection_count++
			} else {
				panic("connection count underflow")
			}
		}

		select {
		case p := <-in:
			switch p.GetType() {
			case res.Type_Input:
				proto.Merge(&input[p.GetMan()], p)
			}

		case out <- ch:
			var buf bytes.Buffer
			err := gob.NewEncoder(&buf).Encode(&state)
			if err != nil {
				panic(err)
			}
			ch <- buf.Bytes()

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
		}
	}
}
