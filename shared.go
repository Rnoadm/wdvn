package main

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"encoding/binary"
	"encoding/gob"
	"github.com/Rnoadm/wdvn/res"
	"io"
	"math"
	"net"
	"sort"
)

const (
	VelocityClones   = 4
	TileSize         = 16
	PixelSize        = 64
	Gravity          = PixelSize * 9              // per tick
	MinimumVelocity  = PixelSize * 20             // unit stops moving if on ground
	TerminalVelocity = 100 * TileSize * PixelSize // unit cannot move faster on x or y than this
	Friction         = 100                        // 1/Friction of the velocity is removed per tick
	TicksPerSecond   = 100
	WhipTimeMin      = 0.2 * TicksPerSecond
	WhipTimeMax      = 1.5 * TicksPerSecond
	WhipDamageMin    = 10
	WhipDamageMax    = 5000
	WhipSpeedMin     = 64 * PixelSize
	WhipSpeedMax     = 512 * PixelSize
	WhipDistance     = 10 * TileSize * PixelSize
	DefaultLives     = 100
	DefaultHealth    = 10000
	RespawnTime      = 2 * TicksPerSecond
)

type Side uint8

const (
	SideTop Side = iota
	SideBottom
	SideLeft
	SideRight
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

type State struct {
	Tick       uint64
	Lives      uint64
	Mans       [res.Man_count]Unit
	SpawnPoint Coord

	world *World
}

func (state *State) EachUnit(f func(*Unit)) {
	for i := range state.Mans {
		f(&state.Mans[i])
	}
}

func (state *State) Update(input *[res.Man_count]res.Packet, world *World) {
	state.Tick++
	state.world = world

	for i := range state.Mans {
		state.Mans[i].UnitData.(Man).Input(&(*input)[i])
	}

	state.EachUnit(func(u *Unit) {
		u.Update(state)
	})
}

type TraceUnit struct {
	*Unit
	Dist int64 // distance squared
	X, Y int64
	Side
}

type Trace struct {
	End      Coord
	Units    []TraceUnit
	HitWorld bool
	Special  SpecialTile
	Side
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
		t.Side = t.Units[i].Side
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

	traceAABB := func(mins, maxs Coord) (dist, x, y int64, side Side) {
		if delta.X >= 0 && (min.X >= maxs.X || max.X+delta.X <= mins.X) {
			return -1, 0, 0, 0
		}
		if delta.X <= 0 && (min.X+delta.X >= maxs.X || max.X <= mins.X) {
			return -1, 0, 0, 0
		}
		if delta.Y >= 0 && (min.Y >= maxs.Y || max.Y+delta.Y <= mins.Y) {
			return -1, 0, 0, 0
		}
		if delta.Y <= 0 && (min.Y+delta.Y >= maxs.Y || max.Y <= mins.Y) {
			return -1, 0, 0, 0
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
			return -1, 0, 0, 0
		}

		if xEnter > yEnter {
			if delta.X > 0 {
				side = SideLeft
			} else {
				side = SideRight
			}
		} else {
			if delta.Y > 0 {
				side = SideTop
			} else {
				side = SideBottom
			}
		}

		x = int64(enter * float64(delta.X))
		y = int64(enter * float64(delta.Y))

		dist = x*x + y*y
		if dist < 0 {
			dist = 0
		}

		return
	}

	traceUnit := func(u *Unit) (dist, x, y int64, side Side) {
		if u.Health <= 0 {
			return -1, 0, 0, 0
		}

		mins, maxs := u.Size.Hull()
		mins = mins.Add(u.Position)
		maxs = maxs.Add(u.Position)

		dist, x, y, side = traceAABB(mins, maxs)
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
				dist, dx, dy, side := traceAABB(Coord{x, y}, Coord{x + TileSize*PixelSize, y + TileSize*PixelSize})
				if dist >= 0 && (dist < maxDist || (dist == maxDist && tr.Special == SpecialTile_None)) {
					maxDist = dist
					tr.HitWorld = true
					tr.End = start.Add(Coord{dx, dy})
					tr.Special = state.world.Special(x/TileSize/PixelSize, y/TileSize/PixelSize)
					tr.Side = side
				}
			}
		}
	}

	if !worldOnly {
		state.EachUnit(func(u *Unit) {
			if dist, x, y, side := traceUnit(u); dist >= 0 && dist <= maxDist {
				tr.Units = append(tr.Units, TraceUnit{Unit: u, Dist: dist, X: start.X + x, Y: start.Y + y, Side: side})
			}
		})

		sort.Sort(tr)
	}
	return tr
}

func Read(conn net.Conn, packets chan<- *res.Packet, errors chan<- error) {
	var l [64 / 8]byte
	for {
		_, err := io.ReadFull(conn, l[:])
		if err != nil {
			errors <- err
			return
		}

		b := make([]byte, binary.LittleEndian.Uint64(l[:]))
		_, err = io.ReadFull(conn, b)
		if err != nil {
			errors <- err
			return
		}

		p := new(res.Packet)
		err = proto.Unmarshal(b, p)
		if err != nil {
			errors <- err
			return
		}

		packets <- p
	}
}

func Write(conn net.Conn, packets <-chan *res.Packet, errors chan<- error) {
	var l [64 / 8]byte
	for p := range packets {
		b, err := proto.Marshal(p)
		if err != nil {
			errors <- err
			return
		}

		binary.LittleEndian.PutUint64(l[:], uint64(len(b)))

		n, err := conn.Write(l[:])
		if err == nil && n != len(l) {
			err = io.ErrShortWrite
		}
		if err != nil {
			errors <- err
			return
		}

		n, err = conn.Write(b)
		if err == nil && n != len(b) {
			err = io.ErrShortWrite
		}
		if err != nil {
			errors <- err
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
