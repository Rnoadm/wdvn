package main

import (
	"bytes"
	"encoding/gob"
	"github.com/Rnoadm/wdvn/res"
	"image/color"
	"math"
	"math/rand"
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
	WhipSpeedMin     = 200 * PixelSize
	WhipSpeedMax     = 1500 * PixelSize
	WhipDistance     = 10 * TileSize * PixelSize
	ManLives         = 100
	ManHealth        = 10000
	DamageFactor     = TileSize * PixelSize * 100 // momentum/DamageFactor is damage dealt
	RespawnTime      = 2 * TicksPerSecond
	FloaterFadeStart = 0.5 * TicksPerSecond
	FloaterFadeEnd   = 1.5 * TicksPerSecond
	LemonSpeed       = 1000 * PixelSize
	LemonTime        = 0.3 * TicksPerSecond
	VacuumHurt       = TicksPerSecond / 5
	VacuumSpeed      = 100 * PixelSize
	VacuumDistance   = 1000 * PixelSize
	VacuumSuck       = 20
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
	LemonSize  = Coord{16 * PixelSize, 16 * PixelSize}
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

	ReleaseAll = &res.Packet{
		Mouse1:   Button_released,
		Mouse2:   Button_released,
		KeyUp:    Button_released,
		KeyDown:  Button_released,
		KeyLeft:  Button_released,
		KeyRight: Button_released,
	}
)

type State struct {
	Tick       uint64
	Mans       [res.Man_count]Unit
	Floaters   []Floater
	SpawnPoint Coord
	Units      map[uint64]*Unit
	NextUnit   uint64

	world *World
}

func NewState(world *World) *State {
	var state State
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
	return &state
}

type Floater struct {
	S      string
	Fg, Bg color.RGBA
	X, Y   int64
	T      uint64
}

func (state *State) FindSpawnPosition(hull Coord) Coord {
	for i := 0; i < 100; i++ {
		pos := state.SpawnPoint
		pos.X += rand.Int63n(hull.X*10+1) - hull.X*5
		pos.Y += rand.Int63n(hull.Y*10+1) - hull.Y*5
		tr := state.Trace(state.SpawnPoint, pos, hull, false)
		if len(tr.Units) == 0 {
			if tr.End != state.SpawnPoint && tr.HitWorld {
				return tr.End
			}
			pos = tr.End
			tr = state.Trace(pos, pos.Add(Coord{0, hull.Y * 10}), hull, false)
			tr.Collide()
			if tr.HitWorld && tr.End != pos {
				return tr.End
			}
		} else if tr.HitWorld {
			tr = state.Trace(tr.End, tr.End.Sub(Coord{0, hull.Y * 10}), hull, true)
			pos = tr.End
			tr = state.Trace(pos, pos.Add(Coord{0, hull.Y * 10}), hull, false)
			tr.Collide()
			if tr.End != pos {
				return tr.End
			}
		}
	}
	return state.SpawnPoint
}

func (state *State) EachUnit(f func(*Unit)) {
	var ignore *Unit
	if m, ok := state.Mans[res.Man_Vacuum].UnitData.(*VacuumMan); ok {
		ignore = m.Held(state)
	}

	for i := range state.Mans {
		if &state.Mans[i] != ignore {
			f(&state.Mans[i])
		}
	}
	for _, u := range state.Units {
		if u != ignore {
			f(u)
		}
	}
}

func (state *State) Update(input *[res.Man_count]res.Packet) {
	state.Tick++

	for i := range state.Mans {
		state.Mans[i].UnitData.(Man).Input(&(*input)[i])
	}

	state.EachUnit(func(u *Unit) {
		u.Update(state)
	})

	for i, l := 0, len(state.Floaters); i < l; i++ {
		if state.Floaters[i].T < state.Tick-FloaterFadeEnd {
			state.Floaters = append(state.Floaters[:i], state.Floaters[i+1:]...)
			i--
			l--
		}
	}
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
	return t.CollideFunc(func(u *Unit) bool {
		for _, i := range ignore {
			if u == i {
				return false
			}
		}
		return true
	})
}

func (t *Trace) CollideFunc(f func(*Unit) bool) *Unit {
	for i := range t.Units {
		if f(t.Units[i].Unit) {
			t.End = Coord{t.Units[i].X, t.Units[i].Y}
			t.Side = t.Units[i].Side
			return t.Units[i].Unit
		}
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

		if exit < 0 || enter > 1 || enter > exit {
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
		if enter < 0 {
			dist, x, y = 0, 0, 0
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

var FooLevel = LoadWorld(res.FooLevel)

func LoadWorld(b []byte) (w *World) {
	err := gob.NewDecoder(bytes.NewReader(b)).Decode(&w)
	if err != nil {
		panic(err)
	}
	return
}
