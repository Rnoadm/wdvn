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
	TicksPerSecond   = 100
)

type Coord struct{ X, Y int64 }

type Unit struct {
	Position     Coord
	Velocity     Coord
	Acceleration Coord
	Target       Coord
}

func (u *Unit) OnGround(state *State) bool {
	// TODO: check for ground
	return u.Position.Y >= 0
}

func (u *Unit) UpdateMan(state *State, input *res.Packet) {
	normalMan := u == &state.Mans[res.Man_Normal]
	onGround := u.OnGround(state)

	if input.GetKeyLeft() == res.Button_pressed {
		if input.GetKeyRight() == res.Button_pressed {
			u.Acceleration.X = 0
		} else {
			u.Acceleration.X = -2 * PixelSize
		}
	} else {
		if input.GetKeyRight() == res.Button_pressed {
			u.Acceleration.X = 2 * PixelSize
		} else {
			u.Acceleration.X = 0
		}
	}
	if !onGround && normalMan {
		u.Acceleration.X = 0
	}
	if onGround && input.GetKeyUp() == res.Button_pressed {
		if normalMan {
			u.Acceleration.Y = -100 * PixelSize
		} else {
			u.Acceleration.Y = -300 * PixelSize
		}
	} else {
		u.Acceleration.Y = 0
	}

	u.UpdatePhysics(state)

	u.Target.X = u.Position.X + input.GetX()
	u.Target.Y = u.Position.Y + input.GetY()
}

func (u *Unit) UpdatePhysics(state *State) {
	onGround := u.OnGround(state)

	if onGround {
		// TODO: deal physics damage based on velocity
		u.Velocity.Y = 0
	}

	u.Velocity.X -= u.Velocity.X / Friction
	u.Velocity.Y -= u.Velocity.Y / Friction

	u.Velocity.X += u.Acceleration.X
	u.Velocity.Y += u.Acceleration.Y
	if !onGround {
		u.Velocity.Y += Gravity
	}

	if u.Velocity.X > TerminalVelocity {
		u.Velocity.X = TerminalVelocity
	}
	if u.Velocity.X < -TerminalVelocity {
		u.Velocity.X = -TerminalVelocity
	}
	if u.Velocity.Y > TerminalVelocity {
		u.Velocity.Y = TerminalVelocity
	}
	if u.Velocity.Y < -TerminalVelocity {
		u.Velocity.Y = -TerminalVelocity
	}

	if onGround &&
		u.Velocity.X < MinimumVelocity &&
		u.Velocity.X > -MinimumVelocity &&
		u.Velocity.Y < MinimumVelocity &&
		u.Velocity.Y > -MinimumVelocity &&
		u.Acceleration.X == 0 &&
		u.Acceleration.Y == 0 {

		u.Velocity.X = 0
		u.Velocity.Y = 0
	}

	// TODO: trace to prevent going through stuff
	u.Position.X += u.Velocity.X / TicksPerSecond
	u.Position.Y += u.Velocity.Y / TicksPerSecond
	if u.Position.Y > 0 {
		u.Position.Y = 0
	}
}

type State struct {
	Mans [res.Man_count]Unit
}

func (state *State) Update(input *[res.Man_count]res.Packet) {
	for i := range state.Mans {
		state.Mans[i].UpdateMan(state, &(*input)[i])
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
