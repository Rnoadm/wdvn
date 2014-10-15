package main

import (
	"code.google.com/p/goprotobuf/proto"
	"encoding/binary"
	"github.com/Rnoadm/wdvn/res"
	"io"
	"log"
	"math"
	"net"
)

const (
	PixelSize        = 64
	Gravity          = PixelSize * 9         // per tick
	TerminalVelocity = PixelSize * 1000      // flat
	MinimumVelocity  = PixelSize * PixelSize // unit stops moving if on ground
	Friction         = 100                   // 1/Friction of the velocity is removed per tick
	TicksPerSecond   = 100
	WhipTime         = 0.5 * TicksPerSecond
)

var DefaultSize = Coord{16 * PixelSize, 16 * PixelSize}

type Coord struct{ X, Y int64 }

type Unit struct {
	Position     Coord
	Velocity     Coord
	Acceleration Coord
	Target       Coord
	Size         Coord
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

	u.Target.X = u.Position.X + input.GetX()*PixelSize
	u.Target.Y = u.Position.Y + input.GetY()*PixelSize
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
	Tick      uint64
	Mans      [res.Man_count]Unit
	WhipStart uint64
	WhipStop  uint64
	WhipPull  bool
}

func (state *State) Update(input *[res.Man_count]res.Packet) {
	state.Tick++

	for i := range state.Mans {
		state.Mans[i].UpdateMan(state, &(*input)[i])
	}

	if state.WhipStop != 0 && state.WhipStop-state.WhipStart < state.Tick-state.WhipStop {
		state.WhipStart, state.WhipStop = 0, 0
	}
	m1, m2 := (*input)[res.Man_Whip].GetMouse1() == res.Button_pressed, (*input)[res.Man_Whip].GetMouse2() == res.Button_pressed
	if m1 || m2 {
		state.WhipPull = m2
		if state.WhipStart == 0 {
			state.WhipStart = state.Tick
		}
	} else if state.WhipStart != 0 {
		if state.WhipStop == 0 {
			state.WhipStop = state.Tick
			start, stop := state.Mans[res.Man_Whip].Position, state.Mans[res.Man_Whip].Target
			start.X += 16 * PixelSize / 2
			start.Y -= 16 * PixelSize / 2
			stop.Y -= 16 * PixelSize / 2
			delta := Coord{stop.X - start.X, stop.Y - start.Y}
			dist1 := math.Hypot(float64(delta.X), float64(delta.Y))
			if state.WhipStart < state.WhipStop-WhipTime {
				state.WhipStart = state.WhipStop - WhipTime
			}
			dist2 := float64(state.WhipStop-state.WhipStart) * 128 * PixelSize / WhipTime
			if dist2 >= 4*PixelSize {
				stop.X = start.X + int64(float64(delta.X)*dist2/dist1)
				stop.Y = start.Y + int64(float64(delta.Y)*dist2/dist1)

				ignore := []*Unit{&state.Mans[res.Man_Whip]}
				var trace *Trace
				for {
					tr := state.Trace(start, stop, Coord{}, ignore...)
					if tr == nil {
						break
					}
					if tr.HitWorld {
						if state.WhipPull {
							trace = tr
						}
						break
					}
					ignore = append(ignore, tr.Unit)
					trace = tr
				}
				if trace != nil {
					// TODO: damage enemy

					dx, dy := start.X-trace.Coord.X, start.Y-trace.Coord.Y
					dist := math.Hypot(float64(dx), float64(dy))
					if state.WhipPull {
						state.Mans[res.Man_Whip].Velocity.X += int64(float64(-dx) / dist * 256 * PixelSize)
						state.Mans[res.Man_Whip].Velocity.Y += int64(float64(-dy) / dist * 256 * PixelSize)
					} else if trace.Unit != nil {
						trace.Unit.Velocity.X += int64(float64(dx) / dist * 256 * PixelSize)
						trace.Unit.Velocity.Y += int64(float64(dy) / dist * 256 * PixelSize)
					}
				}
			}
		}
	}
}

type Trace struct {
	Coord    Coord
	HitWorld bool
	Unit     *Unit
}

func (state *State) Trace(start, end, hull Coord, ignore ...*Unit) *Trace {
	step := func(cur *Coord) {
		dx, dy := end.X-cur.X, end.Y-cur.Y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}

		if dx > dy {
			if end.X > cur.X {
				cur.X++
			} else {
				cur.X--
			}
		} else {
			if end.Y > cur.Y {
				cur.Y++
			} else {
				cur.Y--
			}
		}
	}

	traceUnit := func(cur Coord, u *Unit) *Trace {
		for _, i := range ignore {
			if i == u {
				return nil
			}
		}
		size := u.Size
		if size == (Coord{}) {
			size = DefaultSize
		}
		if u.Position.X <= cur.X+hull.X/2 && u.Position.Y-size.Y <= cur.Y+hull.Y/2 &&
			u.Position.X+size.X > cur.X-hull.X/2 && u.Position.Y > cur.Y-hull.Y/2 {
			return &Trace{Coord: cur, Unit: u}
		}
		return nil
	}

	for cur := start; cur != end; step(&cur) {
		// TODO: make this work with actual interesting maps
		if cur.Y+hull.Y/2 >= 0 {
			return &Trace{Coord: cur, HitWorld: true}
		}

		for i := range state.Mans {
			if t := traceUnit(cur, &state.Mans[i]); t != nil {
				return t
			}
		}
	}
	return nil
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
