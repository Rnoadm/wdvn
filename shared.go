package main

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"encoding/binary"
	"encoding/gob"
	"github.com/Rnoadm/wdvn/res"
	"io"
	"log"
	"math"
	"net"
	"sort"
)

const (
	VelocityClones          = 2
	TileSize                = 16
	PixelSize               = 64
	Gravity                 = PixelSize * 9                              // per tick
	MinimumVelocity         = PixelSize * TicksPerSecond / 2             // unit stops moving if on ground
	TerminalVelocity        = 10 * TileSize * PixelSize * TicksPerSecond // unit cannot move faster on x or y than this
	Friction                = 100                                        // 1/Friction of the velocity is removed per tick
	TicksPerSecond          = 100
	WhipTimeMin             = 0.2 * TicksPerSecond
	WhipTimeMax             = 1.5 * TicksPerSecond
	WhipDamageMin           = 1
	WhipDamageMax           = 5
	WhipSpeedMin            = 64 * PixelSize
	WhipSpeedMax            = 512 * PixelSize
	WhipDistance            = 10 * TileSize * PixelSize
	WhipAntiGravityDuration = TicksPerSecond / 2
	DefaultLives            = 100
	DefaultHealth           = 10
	RespawnTime             = 10 * TicksPerSecond
)

var (
	ManSize    = Coord{30 * PixelSize, 46 * PixelSize}
	CrouchSize = Coord{30 * PixelSize, 30 * PixelSize}
)

var (
	Type_Ping      = res.Type_Ping.Enum()
	Type_SelectMan = res.Type_SelectMan.Enum()
	Type_Input     = res.Type_Input.Enum()
	Type_StateDiff = res.Type_StateDiff.Enum()
	Type_FullState = res.Type_FullState.Enum()
	Type_World     = res.Type_World.Enum()

	Man_Whip    = res.Man_Whip.Enum()
	Man_Density = res.Man_Density.Enum()
	Man_Vacuum  = res.Man_Vacuum.Enum()
	Man_Normal  = res.Man_Normal.Enum()

	Button_released = res.Button_released.Enum()
	Button_pressed  = res.Button_pressed.Enum()
)

type Coord struct{ X, Y int64 }

func (c Coord) Add(o Coord) Coord {
	return Coord{c.X + o.X, c.Y + o.Y}
}

func (c Coord) Sub(o Coord) Coord {
	return Coord{c.X - o.X, c.Y - o.Y}
}

func (c Coord) Hull() (min, max Coord) {
	// avoid rounding off odd coordinates
	max = Coord{c.X / 2, 0}
	min = Coord{max.X - c.X, max.Y - c.Y}
	return
}

func (c Coord) Floor(i int64) Coord {
	x := (c.X%i + i) % i
	y := (c.Y%i + i) % i
	return Coord{c.X - x, c.Y - y}
}

func (c Coord) Zero() bool {
	return c == Coord{}
}

type Unit struct {
	Position     Coord
	Velocity     Coord
	Acceleration Coord
	Target       Coord
	Size         Coord
	Gravity      int64
	Health       int64
}

func (u *Unit) OnGround(state *State) bool {
	tr := state.Trace(u.Position, u.Position.Add(Coord{0, 1}), u.Size, false)
	tr.Collide(u)
	return tr.End == u.Position
}

func (u *Unit) Hurt(state *State, by *Unit, amount int64) {
	u.Health -= amount
}

func (u *Unit) IsMan(state *State) bool {
	for i := range state.Mans {
		if u == &state.Mans[i] {
			return true
		}
	}
	return false
}

func (u *Unit) UpdateMan(state *State, input *res.Packet, man res.Man) {
	if u.Health <= 0 {
		if state.Respawn[man] == 0 {
			state.Respawn[man] = state.Tick + RespawnTime
		}
		if state.Respawn[man] <= state.Tick && state.Lives > 0 {
			state.Lives--
			u.Health = DefaultHealth
			u.Position = state.SpawnPoint
			u.Gravity = 0
			u.Velocity = Coord{}
			u.Acceleration = Coord{}
			state.Respawn[man] = 0
		}
		return
	}

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
			u.Acceleration.Y = -350 * PixelSize
		}
		if man == res.Man_Whip {
			u.Gravity = 0
		}
	} else {
		u.Acceleration.Y = 0
	}
	if man == res.Man_Whip {
		if state.WhipStop != 0 && state.WhipStop-state.WhipStart < state.Tick-state.WhipStop {
			state.WhipStart, state.WhipStop, state.WhipEnd = 0, 0, Coord{}
		}
		if state.WhipStop == 0 && state.Mans[res.Man_Whip].Gravity != 0 {
			state.Mans[res.Man_Whip].Gravity += Gravity / WhipAntiGravityDuration
			if state.Mans[res.Man_Whip].Gravity > 0 {
				state.Mans[res.Man_Whip].Gravity = 0
			}
		}
		m1, m2 := input.GetMouse1() == res.Button_pressed, input.GetMouse2() == res.Button_pressed
		if m1 || m2 {
			state.WhipPull = m2
			if state.WhipStart == 0 {
				state.WhipStart = state.Tick
			}
		} else if state.WhipStart != 0 {
			if state.WhipStop == 0 {
				state.WhipStop = state.Tick
				start, stop := state.Mans[res.Man_Whip].Position, state.Mans[res.Man_Whip].Target
				start.Y -= ManSize.Y / 2
				delta := stop.Sub(start)

				dist := math.Hypot(float64(delta.X), float64(delta.Y))
				if state.WhipStart < state.WhipStop-WhipTimeMax {
					state.WhipStart = state.WhipStop - WhipTimeMax
				}
				state.WhipEnd = Coord{}
				if state.WhipStart < state.WhipStop-WhipTimeMin {
					stop.X = start.X + int64(float64(delta.X)*WhipDistance/dist)
					stop.Y = start.Y + int64(float64(delta.Y)*WhipDistance/dist)

					tr := state.Trace(start, stop, Coord{1, 1}, false)
					u := tr.Collide(&state.Mans[res.Man_Whip])
					state.WhipEnd = tr.End

					if u != nil && !u.IsMan(state) {
						damage := int64(WhipDamageMin + (WhipDamageMax-WhipDamageMin)*(state.WhipStop-state.WhipStart)/(WhipTimeMax-WhipTimeMin))
						u.Hurt(state, &state.Mans[res.Man_Whip], damage)
					}

					dx, dy := start.X-tr.End.X, start.Y-tr.End.Y
					dist = math.Hypot(float64(dx), float64(dy))
					if dist > 0 && (u != nil || tr.HitWorld) {
						speed := float64(WhipSpeedMin+(WhipSpeedMax-WhipSpeedMin)*(state.WhipStop-state.WhipStart)/(WhipTimeMax-WhipTimeMin)) / dist
						if state.WhipPull {
							state.Mans[res.Man_Whip].Velocity.X += int64(float64(-dx) * speed)
							state.Mans[res.Man_Whip].Velocity.Y += int64(float64(-dy) * speed)
							state.Mans[res.Man_Whip].Gravity = -Gravity
						} else if u != nil {
							u.Velocity.X += int64(float64(dx) * speed)
							u.Velocity.Y += int64(float64(dy) * speed)
						}
					}
				}
			}
		}
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

	tr := state.Trace(u.Position, u.Position.Add(Coord{u.Velocity.X / TicksPerSecond, u.Velocity.Y / TicksPerSecond}), u.Size, false)
	collide := tr.Collide(u)
	if tr.End == u.Position {
		if u.Velocity.Zero() {
			stuck := state.Trace(tr.End.Add(Coord{0, -u.Size.Y}), tr.End, u.Size, false)
			collide2 := stuck.Collide(u)
			if stuck.End != tr.End {
				tr, collide = stuck, collide2
			}
		} else {
			stuck := state.Trace(u.Position, u.Position.Add(Coord{u.Velocity.X / TicksPerSecond, 0}), u.Size, false)
			collide2 := stuck.Collide(u)
			if stuck.End != tr.End {
				tr, collide = stuck, collide2
			} else {
				stuck = state.Trace(u.Position, u.Position.Add(Coord{0, u.Velocity.Y / TicksPerSecond}), u.Size, false)
				collide2 = stuck.Collide(u)
				if stuck.End != tr.End {
					tr, collide = stuck, collide2
				} else {
					stuck = state.Trace(tr.End.Add(Coord{0, -u.Size.Y}), tr.End, u.Size, false)
					collide2 = stuck.Collide(u)
					if stuck.End != tr.End {
						tr, collide = stuck, collide2
					}
				}
			}
		}
	}
	if collide == nil {
		switch tr.Special {
		case SpecialTile_None:
			// do nothing
		case SpecialTile_Bounce:
			u.Velocity.X, u.Velocity.Y = -u.Velocity.X*2, -u.Velocity.Y*2
		default:
			panic("unimplemented special tile type: " + specialTile_names[tr.Special])
		}
	}
	u.Position = tr.End
	if collide != nil {
		if !u.IsMan(state) || !collide.IsMan(state) {
			u.Hurt(state, collide, 1)
			collide.Hurt(state, u, 1)
			u.Velocity.X, u.Velocity.Y = u.Velocity.X*2, u.Velocity.Y*2
			collide.Velocity.X, collide.Velocity.Y = collide.Velocity.X*2, collide.Velocity.Y*2
		}
		collide.Velocity.X, u.Velocity.X = u.Velocity.X*2/3+collide.Velocity.X/3, collide.Velocity.X*2/3+u.Velocity.X/3
		collide.Velocity.Y, u.Velocity.Y = u.Velocity.Y*2/3+collide.Velocity.Y/3, collide.Velocity.Y*2/3+u.Velocity.Y/3
	}
	if pos := u.Position.Floor(PixelSize * TileSize); state.world.Outside(pos.X/TileSize/PixelSize, pos.Y/TileSize/PixelSize) > 100 {
		u.Hurt(state, nil, u.Health)
	}
}

type State struct {
	Tick       uint64
	Lives      uint64
	Mans       [res.Man_count]Unit
	Respawn    [res.Man_count]uint64
	WhipStart  uint64
	WhipStop   uint64
	WhipEnd    Coord
	WhipPull   bool
	SpawnPoint Coord

	world *World
}

func (state *State) Update(input *[res.Man_count]res.Packet, world *World) {
	state.Tick++
	state.world = world

	for i := range state.Mans {
		state.Mans[i].UpdateMan(state, &(*input)[i], res.Man(i))
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
	Special  SpecialTile
}

func (t *Trace) Collide(ignore ...*Unit) *Unit {
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
		if u.Health <= 0 {
			return -1, 0, 0
		}

		mins, maxs := u.Size.Hull()
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
	bounds_min = bounds_min.Floor(TileSize * PixelSize)
	bounds_max = bounds_max.Floor(TileSize * PixelSize).Add(Coord{TileSize * PixelSize, TileSize * PixelSize})

	for x := bounds_min.X; x <= bounds_max.X; x += TileSize * PixelSize {
		for y := bounds_min.Y; y <= bounds_max.Y; y += TileSize * PixelSize {
			if state.world.Solid(x/TileSize/PixelSize, y/TileSize/PixelSize) {
				dist, dx, dy := traceAABB(Coord{x, y}, Coord{x + TileSize*PixelSize, y + TileSize*PixelSize})
				if dist >= 0 && (dist < maxDist || (dist == maxDist && tr.Special == SpecialTile_None)) {
					maxDist = dist
					tr.HitWorld = true
					tr.End = start.Add(Coord{dx, dy})
					tr.Special = state.world.Special(x/TileSize/PixelSize, y/TileSize/PixelSize)
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

	var l [64 / 8]byte
	for {
		_, err := io.ReadFull(conn, l[:])
		if err != nil {
			log.Println(err)
			return
		}

		b := make([]byte, binary.LittleEndian.Uint64(l[:]))
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
	var l [64 / 8]byte
	for p := range packets {
		b, err := proto.Marshal(p)
		if err != nil {
			log.Println(err)
			return
		}

		binary.LittleEndian.PutUint64(l[:], uint64(len(b)))

		n, err := conn.Write(l[:])
		if err == nil && n != len(l) {
			err = io.ErrShortWrite
		}
		if err != nil {
			log.Println(err)
			return
		}

		n, err = conn.Write(b)
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

var FooLevel = LoadWorld(res.FooLevel)

func LoadWorld(b []byte) (w *World) {
	err := gob.NewDecoder(bytes.NewReader(b)).Decode(&w)
	if err != nil {
		panic(err)
	}
	return
}
