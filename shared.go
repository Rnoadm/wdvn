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

func (c Coord) Floor(i int64) Coord {
	x := (c.X%i + i) % i
	y := (c.Y%i + i) % i
	return Coord{c.X - x, c.Y - y}
}

type Unit struct {
	Position     Coord
	Velocity     Coord
	Acceleration Coord
	Target       Coord
	Size         Coord
	Gravity      int64
}

func (u *Unit) OnGround(state *State) bool {
	size := u.Size
	if size == (Coord{}) {
		size = DefaultSize
	}
	tr := state.Trace(u.Position, u.Position.Add(Coord{0, 1}), size, false)
	tr.Collide(u)
	return tr.End == u.Position
}

func (u *Unit) UpdateMan(state *State, input *res.Packet, man res.Man) {
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
	if !onGround && man == res.Man_Normal {
		u.Acceleration.X = 0
	}
	if onGround && u.Velocity.Y == 0 && input.GetKeyUp() == res.Button_pressed {
		if man == res.Man_Normal {
			u.Acceleration.Y = -200 * PixelSize
		} else {
			u.Acceleration.Y = -300 * PixelSize
		}
	} else {
		u.Acceleration.Y = 0
	}
	if man == res.Man_Density {
		if input.GetMouse1() == res.Button_pressed {
			u.Gravity++
		}
		if input.GetMouse2() == res.Button_pressed {
			u.Gravity--
		}
		if u.Gravity < -Gravity {
			u.Gravity = -Gravity
		}
		if u.Gravity > Gravity {
			u.Gravity = Gravity
		}
	}

	u.UpdatePhysics(state)

	u.Target.X = u.Position.X + input.GetX()*PixelSize
	u.Target.Y = u.Position.Y + input.GetY()*PixelSize
}

func (u *Unit) UpdatePhysics(state *State) {
	onGround := u.OnGround(state)

	if onGround && u.Velocity.Y > 0 {
		// TODO: deal physics damage based on velocity
		u.Velocity.Y = 0
	}

	u.Velocity.X -= u.Velocity.X / Friction
	u.Velocity.Y -= u.Velocity.Y / Friction

	u.Velocity.X += u.Acceleration.X
	u.Velocity.Y += u.Acceleration.Y
	if !onGround {
		u.Velocity.Y += Gravity + u.Gravity
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
	tr := state.Trace(u.Position, u.Position.Add(Coord{u.Velocity.X / TicksPerSecond, u.Velocity.Y / TicksPerSecond}), size, false)
	collide := tr.Collide(u)
	if tr.End == u.Position {
		if u.Velocity == (Coord{}) {
			stuck := state.Trace(tr.End.Add(Coord{0, -size.Y}), tr.End, size, false)
			collide2 := stuck.Collide(u)
			if stuck.End != tr.End {
				tr, collide = stuck, collide2
			}
		} else {
			stuck := state.Trace(u.Position, u.Position.Add(Coord{u.Velocity.X / TicksPerSecond, 0}), size, false)
			collide2 := stuck.Collide(u)
			if stuck.End != tr.End {
				tr, collide = stuck, collide2
			} else {
				stuck = state.Trace(u.Position, u.Position.Add(Coord{0, u.Velocity.Y / TicksPerSecond}), size, false)
				collide2 = stuck.Collide(u)
				if stuck.End != tr.End {
					tr, collide = stuck, collide2
				} else {
					stuck = state.Trace(tr.End.Add(Coord{0, -size.Y}), tr.End, size, false)
					collide2 = stuck.Collide(u)
					if stuck.End != tr.End {
						tr, collide = stuck, collide2
					}
				}
			}
		}
	}
	u.Position = tr.End
	if collide != nil {
		g := Gravity*2 + u.Gravity + collide.Gravity
		ux := collide.Velocity.X * (Gravity + collide.Gravity) / g
		cx := u.Velocity.X * (Gravity + u.Gravity) / g
		if ux < cx {
			ux -= Gravity
			cx += Gravity
		} else {
			ux += Gravity
			cx -= Gravity
		}
		u.Velocity.X, collide.Velocity.X = collide.Velocity.X, u.Velocity.X
		collide.Velocity.Y, u.Velocity.Y = 0, 0
	}
}

type World struct{}

func (w *World) Tile(x, y int64) int {
	if y*2 < x {
		return 0
	}
	if y*2 == x {
		return 4
	}
	if y*2 == x+1 {
		return 2
	}
	if y*2 == x+2 {
		return 7
	}
	return 1
}

func (w *World) Solid(x, y int64) bool {
	return y*2 >= x
}

type State struct {
	Tick      uint64
	Mans      [res.Man_count]Unit
	WhipStart uint64
	WhipStop  uint64
	WhipEnd   Coord
	WhipPull  bool
	World
}

func (state *State) Update(input *[res.Man_count]res.Packet) {
	state.Tick++

	for i := range state.Mans {
		state.Mans[i].UpdateMan(state, &(*input)[i], res.Man(i))
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

				tr := state.Trace(start, stop, Coord{1, 1}, false)
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
						ex, ey = tr.Units[i].X, tr.Units[i].Y
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
						state.Mans[res.Man_Whip].Velocity.Y -= Gravity * 20
					} else if u != nil {
						u.Velocity.X += int64(float64(dx) / dist * 256 * PixelSize)
						u.Velocity.Y += int64(float64(dy) / dist * 256 * PixelSize)
						u.Velocity.Y -= Gravity * 20
					}
				}
			}
		}
	}
}

type TraceUnit struct {
	*Unit
	Dist int64 // distance squared
	X, Y int64
}

type Trace struct {
	End      Coord
	Units    []TraceUnit
	HitWorld bool
}

func (t *Trace) Collide(ignore ...*Unit) *Unit {
	log.Println(t)
search:
	for i := range t.Units {
		for _, u := range ignore {
			if u == t.Units[i].Unit {
				continue search
			}
		}
		t.End = Coord{t.Units[i].X, t.Units[i].Y}
		return t.Units[i].Unit
	}
	return nil
}

func (t *Trace) Len() int           { return len(t.Units) }
func (t *Trace) Swap(i, j int)      { t.Units[i], t.Units[j] = t.Units[j], t.Units[i] }
func (t *Trace) Less(i, j int) bool { return t.Units[i].Dist < t.Units[j].Dist }

func (state *State) Trace(start, end, hull Coord, worldOnly bool) *Trace {
	min, max := hull.Hull()
	min = min.Add(start)
	max = max.Add(start)
	delta := end.Sub(start)
	maxDist := int64(1<<63 - 1)

	traceAABB := func(mins, maxs Coord) (dist, x, y int64) {
		if delta.X >= 0 && (min.X >= maxs.X || max.X+delta.X <= mins.X) {
			return -1, 0, 0
		}
		if delta.X <= 0 && (min.X+delta.X >= maxs.X || max.X <= mins.X) {
			return -1, 0, 0
		}
		if delta.Y >= 0 && (min.Y >= maxs.Y || max.Y+delta.Y <= mins.Y) {
			return -1, 0, 0
		}
		if delta.Y <= 0 && (min.Y+delta.Y >= maxs.Y || max.Y <= mins.Y) {
			return -1, 0, 0
		}

		var xEnter, xExit float64
		if delta.X > 0 {
			xEnter = float64(mins.X-max.X) / float64(delta.X)
			xExit = float64(maxs.X-min.X) / float64(delta.X)
		} else if delta.X < 0 {
			xEnter = float64(maxs.X-min.X) / float64(delta.X)
			xExit = float64(mins.X-max.X) / float64(delta.X)
		} else {
			xEnter = math.Inf(-1)
			xExit = math.Inf(1)
		}

		var yEnter, yExit float64
		if delta.Y > 0 {
			yEnter = float64(mins.Y-max.Y) / float64(delta.Y)
			yExit = float64(maxs.Y-min.Y) / float64(delta.Y)
		} else if delta.Y < 0 {
			yEnter = float64(maxs.Y-min.Y) / float64(delta.Y)
			yExit = float64(mins.Y-max.Y) / float64(delta.Y)
		} else {
			yEnter = math.Inf(-1)
			yExit = math.Inf(1)
		}

		enter := math.Max(xEnter, yEnter)
		exit := math.Min(xExit, yExit)

		if enter < 0 || enter > 1 || enter > exit {
			return -1, 0, 0
		}

		x = int64(enter * float64(delta.X))
		y = int64(enter * float64(delta.Y))

		dist = x*x + y*y
		if dist < 0 {
			dist = 0
		}

		return
	}

	traceUnit := func(u *Unit) (dist, x, y int64) {
		size := u.Size
		if size == (Coord{}) {
			size = DefaultSize
		}
		mins, maxs := size.Hull()
		mins = mins.Add(u.Position)
		maxs = maxs.Add(u.Position)

		dist, x, y = traceAABB(mins, maxs)
		return
	}

	tr := &Trace{}

	tr.End = end

	bounds_min, bounds_max := min, max
	if delta.X < 0 {
		bounds_min.X += delta.X
	} else {
		bounds_max.X += delta.X
	}
	if delta.Y < 0 {
		bounds_min.Y += delta.Y
	} else {
		bounds_max.Y += delta.Y
	}
	bounds_min = bounds_min.Floor(16 * PixelSize)
	bounds_max = bounds_max.Floor(16 * PixelSize).Add(Coord{16 * PixelSize, 16 * PixelSize})

	for x := bounds_min.X; x <= bounds_max.X; x += 16 * PixelSize {
		for y := bounds_min.Y; y <= bounds_max.Y; y += 16 * PixelSize {
			if state.World.Solid(x/16/PixelSize, y/16/PixelSize) {
				dist, dx, dy := traceAABB(Coord{x, y}, Coord{x + 16*PixelSize, y + 16*PixelSize})
				if dist >= 0 && dist < maxDist {
					maxDist = dist
					tr.HitWorld = true
					tr.End = start.Add(Coord{dx, dy})
				}
			}
		}
	}

	if !worldOnly {
		for i := range state.Mans {
			u := &state.Mans[i]
			if dist, x, y := traceUnit(u); dist >= 0 && dist <= maxDist {
				tr.Units = append(tr.Units, TraceUnit{Unit: u, Dist: dist, X: start.X + x, Y: start.Y + y})
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
