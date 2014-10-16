package main

import (
	"code.google.com/p/goprotobuf/proto"
	"encoding/binary"
	"github.com/Rnoadm/wdvn/res"
	"io"
	"log"
	"math"
	"net"
	"sort"
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

func (c Coord) Add(o Coord) Coord {
	return Coord{c.X + o.X, c.Y + o.Y}
}

func (c Coord) Sub(o Coord) Coord {
	return Coord{c.X - o.X, c.Y - o.Y}
}

func (c Coord) Hull() (min, max Coord) {
	// avoid rounding off odd coordinates
	max = Coord{c.X / 2, c.Y / 2}
	min = Coord{max.X - c.X, max.Y - c.Y}
	return
}

type Unit struct {
	Position     Coord
	Velocity     Coord
	Acceleration Coord
	Target       Coord
	Size         Coord
}

func (u *Unit) OnGround(state *State) bool {
	size := u.Size
	if size == (Coord{}) {
		size = DefaultSize
	}
	tr := state.Trace(u.Position, u.Position.Add(Coord{0, 1}), size, true)
	return tr.HitWorld
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

	size := u.Size
	if size == (Coord{}) {
		size = DefaultSize
	}
	tr := state.Trace(u.Position, u.Position.Add(Coord{u.Velocity.X / TicksPerSecond, u.Velocity.Y / TicksPerSecond}), size, true)
	if tr.End == u.Position {
		stuck := state.Trace(u.Position.Add(Coord{0, -size.Y}), u.Position, size, true)
		if stuck.End != u.Position.Add(Coord{0, -size.Y}) {
			tr = stuck
		}
	}
	u.Position = tr.End
}

type State struct {
	Tick      uint64
	Mans      [res.Man_count]Unit
	WhipStart uint64
	WhipStop  uint64
	WhipEnd   Coord
	WhipPull  bool
}

func (state *State) Update(input *[res.Man_count]res.Packet) {
	state.Tick++

	for i := range state.Mans {
		state.Mans[i].UpdateMan(state, &(*input)[i])
	}

	if state.WhipStop != 0 && state.WhipStop-state.WhipStart < state.Tick-state.WhipStop {
		state.WhipStart, state.WhipStop, state.WhipEnd = 0, 0, Coord{}
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
			delta := stop.Sub(start)

			dist1 := math.Hypot(float64(delta.X), float64(delta.Y))
			if state.WhipStart < state.WhipStop-WhipTime {
				state.WhipStart = state.WhipStop - WhipTime
			}
			dist2 := float64(state.WhipStop-state.WhipStart) * 128 * PixelSize / WhipTime
			if dist2 >= 16*PixelSize {
				stop.X = start.X + int64(float64(delta.X)*dist2/dist1)
				stop.Y = start.Y + int64(float64(delta.Y)*dist2/dist1)

				tr := state.Trace(start, stop, Coord{}, false)
				var u *Unit
				ex, ey := start.X, start.Y
				if tr.End != (Coord{}) {
					ex, ey = tr.End.X, tr.End.Y
				}
				for i := len(tr.Units) - 1; i >= 0; i-- {
					if tr.Units[i].Unit == &state.Mans[res.Man_Whip] {
						continue
					}
					u = tr.Units[i].Unit
					if !tr.HitWorld {
						ex, ey = u.Position.X, u.Position.Y
					}
					break
				}

				state.WhipEnd = Coord{ex, ey}

				// TODO: damage enemy

				dx, dy := start.X-ex, start.Y-ey
				dist := math.Hypot(float64(dx), float64(dy))
				if dist > 0 && (u != nil || tr.HitWorld) {
					if state.WhipPull {
						state.Mans[res.Man_Whip].Velocity.X += int64(float64(-dx) / dist * 256 * PixelSize)
						state.Mans[res.Man_Whip].Velocity.Y += int64(float64(-dy) / dist * 256 * PixelSize)
					} else if u != nil {
						u.Velocity.X += int64(float64(dx) / dist * 256 * PixelSize)
						u.Velocity.Y += int64(float64(dy) / dist * 256 * PixelSize)
					}
				}
			}
		}
	}
}

type TraceUnit struct {
	*Unit
	Dist int64 // distance squared
}

type Trace struct {
	End      Coord
	Units    []TraceUnit
	HitWorld bool
}

func (t Trace) Len() int           { return len(t.Units) }
func (t Trace) Swap(i, j int)      { t.Units[i], t.Units[j] = t.Units[j], t.Units[i] }
func (t Trace) Less(i, j int) bool { return t.Units[i].Dist < t.Units[j].Dist }

func (state *State) Trace(start, end, hull Coord, worldOnly bool) *Trace {
	min, max := hull.Hull()
	min = min.Add(start)
	max = max.Add(start)
	delta := end.Sub(start)
	maxDist := delta.X*delta.X + delta.Y*delta.Y

	traceAABB := func(mins, maxs Coord) int64 {
		if max.X >= mins.X && min.X <= maxs.X && max.Y >= mins.Y && min.Y <= maxs.Y {
			return 0
		}

		if delta.X == 0 && delta.Y == 0 {
			return -1
		}

		var xEnter, xExit float64
		if delta.X > 0 {
			xEnter = float64(mins.X-max.X) / float64(delta.X)
			xExit = float64(maxs.X-min.X) / float64(delta.X)
		} else if delta.X < 0 {
			xEnter = float64(maxs.X-min.X) / float64(delta.X)
			xExit = float64(mins.X-max.X) / float64(delta.X)
		} else {
			xEnter = 0
			xExit = 1
		}

		var yEnter, yExit float64
		if delta.Y > 0 {
			yEnter = float64(mins.Y-max.Y) / float64(delta.Y)
			yExit = float64(maxs.Y-min.Y) / float64(delta.Y)
		} else if delta.Y < 0 {
			yEnter = float64(maxs.Y-min.Y) / float64(delta.Y)
			yExit = float64(mins.Y-max.Y) / float64(delta.Y)
		} else {
			yEnter = 0
			yExit = 1
		}

		if (xEnter < 0 && yEnter < 0) || xEnter > 1 || yEnter > 1 {
			return -1
		}

		enter := math.Max(xEnter, yEnter)
		exit := math.Min(xExit, yExit)

		if enter > exit {
			return -1
		}

		var x, y int64
		if xEnter < yEnter {
			if delta.X < 0 {
				x = maxs.X - min.X
			} else {
				x = max.X - mins.X
			}
			y = x * delta.Y / delta.X
		} else {
			if delta.Y < 0 {
				y = maxs.Y - min.Y
			} else {
				y = max.Y - mins.Y
			}
			x = y * delta.Y / delta.X
		}

		return x*x + y*y
	}

	traceUnit := func(u *Unit) int64 {
		size := u.Size
		if size == (Coord{}) {
			size = DefaultSize
		}
		mins, maxs := size.Hull()
		mins = mins.Add(u.Position)
		maxs = maxs.Add(u.Position)

		return traceAABB(mins, maxs)
	}

	tr := &Trace{}

	tr.End = end

	// TODO: make this work on more interesting maps
	if max.Y > 0 {
		tr.HitWorld, tr.End = true, start
		return tr
	}
	if max.Y+delta.Y > 0 {
		dx := start.Y * delta.X / delta.Y
		maxDist = max.Y*max.Y + dx*dx

		tr.HitWorld, tr.End = true, Coord{start.X - dx, -hull.Y / 2}
	}

	if !worldOnly {
		for i := range state.Mans {
			u := &state.Mans[i]
			if d := traceUnit(u); d >= 0 && d <= maxDist {
				tr.Units = append(tr.Units, TraceUnit{u, d})
			}
		}

		sort.Sort(tr)
	}
	return tr
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
