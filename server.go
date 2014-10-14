package main

import (
	"code.google.com/p/goprotobuf/proto"
	"github.com/Rnoadm/wdvn/res"
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
	go Manager(ch, state, broadcast)

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
			X:    proto.Int64(state.Mans[i].Position.X),
			Y:    proto.Int64(state.Mans[i].Position.Y),
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

func Manager(in <-chan *res.Packet, out chan<- State, broadcast chan<- *res.Packet) {
	var state State
	var input [res.Man_count]res.Packet

	tick := time.Tick(time.Second / 100)

	for {
		select {
		case p := <-in:
			switch p.GetType() {
			//case res.Type_MoveMan:
			//	state.Mans[p.GetMan()].Position.X = p.GetX()
			//	state.Mans[p.GetMan()].Position.Y = p.GetY()

			case res.Type_Input:
				proto.Merge(&input[p.GetMan()], p)
			}

		case out <- state:

		case <-tick:
			prev := state

			for i := range state.Mans {
				// TODO: check for ground
				onGround := false
				if state.Mans[i].Position.Y >= 0 {
					onGround = true
				}

				if onGround {
					// TODO: deal physics damage based on velocity
					state.Mans[i].Velocity.Y = 0
				}

				if input[i].GetKeyLeft() == res.Button_pressed {
					if input[i].GetKeyRight() == res.Button_pressed {
						state.Mans[i].Acceleration.X = 0
					} else {
						state.Mans[i].Acceleration.X = -2 * PixelSize
					}
				} else {
					if input[i].GetKeyRight() == res.Button_pressed {
						state.Mans[i].Acceleration.X = 2 * PixelSize
					} else {
						state.Mans[i].Acceleration.X = 0
					}
				}
				if !onGround && res.Man(i) == res.Man_Normal {
					state.Mans[i].Acceleration.X = 0
				}
				if onGround && input[i].GetKeyUp() == res.Button_pressed {
					if res.Man(i) == res.Man_Normal {
						state.Mans[i].Acceleration.Y = -100 * PixelSize
					} else {
						state.Mans[i].Acceleration.Y = -300 * PixelSize
					}
				} else {
					state.Mans[i].Acceleration.Y = 0
				}

				state.Mans[i].Velocity.X -= state.Mans[i].Velocity.X / Friction
				state.Mans[i].Velocity.Y -= state.Mans[i].Velocity.Y / Friction

				state.Mans[i].Velocity.X += state.Mans[i].Acceleration.X
				state.Mans[i].Velocity.Y += state.Mans[i].Acceleration.Y
				if !onGround {
					state.Mans[i].Velocity.Y += Gravity
				}

				if state.Mans[i].Velocity.X > TerminalVelocity {
					state.Mans[i].Velocity.X = TerminalVelocity
				}
				if state.Mans[i].Velocity.X < -TerminalVelocity {
					state.Mans[i].Velocity.X = -TerminalVelocity
				}
				if state.Mans[i].Velocity.Y > TerminalVelocity {
					state.Mans[i].Velocity.Y = TerminalVelocity
				}
				if state.Mans[i].Velocity.Y < -TerminalVelocity {
					state.Mans[i].Velocity.Y = -TerminalVelocity
				}

				if onGround && state.Mans[i].Velocity.X < MinimumVelocity && state.Mans[i].Velocity.X > -MinimumVelocity && state.Mans[i].Velocity.Y < MinimumVelocity && state.Mans[i].Velocity.Y > -MinimumVelocity && input[i].GetKeyUp() == res.Button_released && input[i].GetKeyDown() == res.Button_released && input[i].GetKeyLeft() == res.Button_released && input[i].GetKeyRight() == res.Button_released {
					state.Mans[i].Velocity.X = 0
					state.Mans[i].Velocity.Y = 0
				}

				// TODO: trace to prevent going through stuff
				state.Mans[i].Position.X += state.Mans[i].Velocity.X / PixelSize
				state.Mans[i].Position.Y += state.Mans[i].Velocity.Y / PixelSize
				if state.Mans[i].Position.Y > 0 {
					state.Mans[i].Position.Y = 0
				}

				if state.Mans[i].Position.X/PixelSize != prev.Mans[i].Position.X/PixelSize || state.Mans[i].Position.Y/PixelSize != prev.Mans[i].Position.Y/PixelSize {
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
