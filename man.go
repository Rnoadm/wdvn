package main

import (
	"encoding/gob"
	"github.com/Rnoadm/wdvn/res"
	"image"
	"image/color"
	"math"
)

func Scale(delta Coord, distance float64) Coord {
	if delta.Zero() {
		return delta
	}
	actual := math.Hypot(float64(delta.X), float64(delta.Y))
	delta.X = int64(float64(delta.X) * distance / actual)
	delta.Y = int64(float64(delta.Y) * distance / actual)
	return delta
}

func Lerp(min, max int64, tmin, tmax, t uint64) int64 {
	return min + (max-min)*int64(t)/int64(tmax-tmin)
}

type Man interface {
	UnitData
	DoRespawn(*State, *Unit)
	Man() res.Man
	Lives() int64
	Respawn() uint64
	Target() Coord
	Input(*res.Packet)
	Checkpoint() *Coord
	Crouching() bool
}

func init() {
	gob.Register((*WhipMan)(nil))
	gob.Register((*DensityMan)(nil))
	gob.Register((*VacuumMan)(nil))
	gob.Register((*Lemon)(nil))
	gob.Register((*NormalMan)(nil))
}

type ManUnitData struct {
	Man_        res.Man
	Target_     Coord
	Crouching_  bool
	Input_      *res.Packet
	Respawn_    uint64
	Lives_      int64
	Checkpoint_ Coord
}

func (m *ManUnitData) UpdateDead(state *State, u *Unit) {
	if m.Respawn_ == 0 {
		m.Respawn_ = state.Tick + RespawnTime
	}
	if m.Respawn_ <= state.Tick {
		if m.Lives_ > 0 {
			m.DoRespawn(state, u)
		} else if m.Man_ == res.Man_Whip {
			allowRespawn := true
			maxLives := m.Lives_
			for i := range state.Mans {
				uu := &state.Mans[i]
				mm := uu.UnitData.(Man)
				if mm.Respawn() == 0 || mm.Respawn() > state.Tick {
					allowRespawn = false
					break
				}
				if l := mm.Lives(); l > maxLives {
					maxLives = l
				}
			}

			if allowRespawn {
				for i := range state.Mans {
					uu := &state.Mans[i]
					mm := uu.UnitData.(Man)

					if mm.Lives() == maxLives {
						mm.DoRespawn(state, uu)
					}
				}
			}
		}
	}
}

func (m *ManUnitData) DoRespawn(state *State, u *Unit) {
	m.Lives_--
	u.Health = ManHealth
	u.Size = Coord{}
	m.Crouching_ = false
	u.Position = state.FindSpawnPosition(ManSize)
	u.Size = ManSize
	u.Gravity = 0
	u.Velocity = Coord{}
	u.Acceleration = Coord{}
	m.Respawn_ = 0
}

func (m *ManUnitData) Update(state *State, u *Unit) {
	onGround, _ := u.OnGround(state)

	if m.Input_.GetKeyLeft() == res.Button_pressed {
		if m.Input_.GetKeyRight() == res.Button_pressed {
			u.Acceleration.X = 0
		} else {
			if u.Size == CrouchSize {
				u.Acceleration.X = -1 * PixelSize
			} else {
				u.Acceleration.X = -2 * PixelSize
			}
		}
	} else {
		if m.Input_.GetKeyRight() == res.Button_pressed {
			if u.Size == CrouchSize {
				u.Acceleration.X = 1 * PixelSize
			} else {
				u.Acceleration.X = 2 * PixelSize
			}
		} else {
			u.Acceleration.X = 0
		}
	}
	if m.Input_.GetKeyDown() == res.Button_pressed {
		if !m.Crouching_ {
			u.Position = u.Position.Sub(ManSize.Sub(CrouchSize))
			u.Size = CrouchSize
			m.Crouching_ = true
		}
	} else {
		if m.Crouching_ {
			tr := state.Trace(u.Position, u.Position.Sub(ManSize.Sub(CrouchSize)), CrouchSize, false)
			collide := tr.CollideFunc(func(o *Unit) bool {
				return m.CollideWith(state, u, o)
			})
			if collide == nil && !tr.HitWorld {
				u.Size = ManSize
				m.Crouching_ = false
			}
		}
	}
	if !onGround && m.Man() == res.Man_Normal {
		u.Acceleration.X = 0
	}
	if onGround && u.Velocity.Y == 0 && m.Input_.GetKeyUp() == res.Button_pressed {
		if m.Man() == res.Man_Normal {
			u.Acceleration.Y = -200 * PixelSize
		} else {
			u.Acceleration.Y = -350 * PixelSize
		}
	} else {
		u.Acceleration.Y = 0
	}

	m.Target_.X = u.Position.X + m.Input_.GetX()*PixelSize
	m.Target_.Y = u.Position.Y + m.Input_.GetY()*PixelSize
}
func (m *ManUnitData) Color() color.RGBA {
	return mancolors[m.Man()]
}
func (m *ManUnitData) ShowDamage() bool {
	return true
}
func (m *ManUnitData) Mass(state *State, u *Unit) int64 {
	if m.Crouching_ {
		return 2000
	}
	return 1000
}
func (m *ManUnitData) Man() res.Man {
	return m.Man_
}
func (m *ManUnitData) Respawn() uint64 {
	return m.Respawn_
}
func (m *ManUnitData) Sprite(state *State, u *Unit) *image.RGBA {
	if m.Crouching_ {
		return mansprites[m.Man()][1]
	}
	return mansprites[m.Man()][0]
}
func (m *ManUnitData) Target() Coord {
	return m.Target_
}
func (m *ManUnitData) Input(p *res.Packet) {
	m.Input_ = p
}
func (m *ManUnitData) Lives() int64 {
	return m.Lives_
}
func (m *ManUnitData) Checkpoint() *Coord {
	return &m.Checkpoint_
}
func (m *ManUnitData) Crouching() bool {
	return m.Crouching_
}
func (m *ManUnitData) CollideWith(state *State, u, o *Unit) bool {
	return u != o
}

type WhipMan struct {
	ManUnitData

	WhipStart  uint64
	WhipStop   uint64
	WhipEnd    Coord
	WhipTether Coord
	WhipPull   bool
}

func (m *WhipMan) UpdateDead(state *State, u *Unit) {
	m.ManUnitData.UpdateDead(state, u)

	m.WhipStart = 0
	m.WhipStop = 0
	m.WhipEnd = Coord{}
	m.WhipTether = Coord{}
	m.WhipPull = false
	u.Gravity = 0
}

func (m *WhipMan) Update(state *State, u *Unit) {
	m.ManUnitData.Update(state, u)

	if m.WhipStop != 0 && (m.WhipStop-m.WhipStart)/10 < state.Tick-m.WhipStop {
		m.WhipStart, m.WhipStop, m.WhipEnd = 0, 0, Coord{}
	}
	if !m.WhipTether.Zero() {
		if u.Position.Sub(m.WhipTether).LengthSquared() < WhipDistance*WhipDistance {
			if ok, _ := u.OnGround(state); !ok {
				if u.Position.Y > m.WhipTether.Y {
					u.Gravity = -Gravity * 9 / 10
				} else {
					u.Gravity = 0
				}
				if u.Velocity.LengthSquared() > TileSize*PixelSize*TileSize*PixelSize {
					u.Velocity.X = u.Velocity.X * 19 / 20
					u.Velocity.Y = u.Velocity.Y * 19 / 20
				}
				u.Velocity.X += (m.WhipTether.X - u.Position.X) / 100
				u.Velocity.Y += (m.WhipTether.Y - u.Position.Y) / 100
				u.Acceleration.X /= 4
				u.Acceleration.Y /= 4
			}
		} else if m.WhipStop == 0 {
			u.Gravity = 0
			m.WhipTether = Coord{}
		}
	}
	m1, m2 := m.Input_.GetMouse1() == res.Button_pressed, m.Input_.GetMouse2() == res.Button_pressed
	if m1 || m2 {
		m.WhipPull = m2
		if m.WhipStart == 0 {
			m.WhipStart = state.Tick
		}
	} else if m.WhipStart != 0 {
		if m.WhipStop == 0 {
			m.WhipStop = state.Tick

			if m.WhipStart < m.WhipStop-WhipTimeMax {
				m.WhipStart = m.WhipStop - WhipTimeMax
			}

			u.Gravity, m.WhipTether = 0, Coord{}
			velocity, whipEnd, hurt, collide, hitWorld := m.Whip(state, u)
			m.WhipEnd = whipEnd
			collide.Hurt(state, u, hurt)

			if m.WhipPull {
				u.Velocity = u.Velocity.Sub(velocity)
				if collide == nil && hitWorld {
					m.WhipTether = whipEnd
				}
			} else if collide != nil {
				collide.Velocity = collide.Velocity.Add(velocity)
			}
		}
	}
}

func (m *WhipMan) Whip(state *State, u *Unit) (velocity, whipEnd Coord, hurt int64, collide *Unit, hitWorld bool) {
	if m.WhipStart == 0 {
		return
	}
	t := state.Tick - m.WhipStart
	if t > WhipTimeMax {
		t = WhipTimeMax
	}
	if t < WhipTimeMin {
		return
	}

	start := u.Position
	start.Y -= u.Size.Y / 2
	delta := m.Target().Sub(start)
	stop := start.Add(Scale(delta, WhipDistance))

	tr := state.Trace(start, stop, Coord{1, 1}, false)
	collide = tr.Collide(u)
	whipEnd = tr.End
	hitWorld = tr.HitWorld

	if collide != nil && !collide.IsMan() {
		hurt = Lerp(WhipDamageMin, WhipDamageMax, WhipTimeMin, WhipTimeMax, t)
	}

	if start != tr.End && (collide != nil || tr.HitWorld) {
		velocity = Scale(start.Sub(tr.End), float64(Lerp(WhipSpeedMin, WhipSpeedMax, WhipTimeMin, WhipTimeMax, t)))
	}

	return
}

type DensityMan struct {
	ManUnitData
}

func (m *DensityMan) Update(state *State, u *Unit) {
	m.ManUnitData.Update(state, u)

	u.Acceleration.X -= u.Acceleration.X * u.Gravity / Gravity / 5

	if m.Input_.GetMouse1() == res.Button_pressed {
		u.Gravity += 10
	}
	if m.Input_.GetMouse2() == res.Button_pressed {
		u.Gravity -= 10
	}
	if u.Gravity < -Gravity {
		u.Gravity = -Gravity
	}
	if u.Gravity > Gravity*4 {
		u.Gravity = Gravity * 4
	}
}

func (m *DensityMan) Mass(state *State, u *Unit) int64 {
	mass := m.ManUnitData.Mass(state, u)
	return mass + mass*u.Gravity/Gravity
}

type VacuumMan struct {
	ManUnitData
	Held_      uint64
	HeldSince_ uint64
	LastLemon_ uint64
}

func (m *VacuumMan) UpdateDead(state *State, u *Unit) {
	m.ManUnitData.UpdateDead(state, u)

	if h := m.Held(state); h != nil {
		h.Position = u.Position
		h.Velocity = Coord{}
	}

	m.Held_, m.HeldSince_ = 0, 0
}

func (m *VacuumMan) Held(state *State) *Unit {
	if m.Held_ == 0 {
		return nil
	}
	if m.Held_ <= uint64(res.Man_count) {
		return &state.Mans[m.Held_-1]
	}
	return state.Units[m.Held_-uint64(res.Man_count)]
}

func (m *VacuumMan) Update(state *State, u *Unit) {
	m.ManUnitData.Update(state, u)

	if m.Input_.GetMouse2() == res.Button_pressed {
		start := u.Position.Sub(Coord{0, u.Size.Y / 2})
		delta := Scale(m.Target().Sub(start), VacuumDistance)
		tr := state.Trace(start, start.Add(delta), Coord{1, 1}, false)
		if collide := tr.CollideFunc(func(o *Unit) bool {
			return m.CollideWith(state, u, o)
		}); collide != nil {
			if m.Held_ == 0 {
				if collide.Position.Sub(Coord{0, collide.Size.Y / 2}).Sub(start).LengthSquared() < (u.Size.X+collide.Size.X)*(u.Size.X+collide.Size.X) {
					for i := range state.Mans {
						if collide == &state.Mans[i] {
							m.Held_ = uint64(i) + 1
							m.HeldSince_ = state.Tick
							break
						}
					}
					if m.Held_ == 0 {
						for i, c := range state.Units {
							if collide == c {
								m.Held_ = i + uint64(res.Man_count)
								m.HeldSince_ = state.Tick
								break
							}
						}
					}
				}
			}
			collide.Velocity = collide.Velocity.Sub(Coord{delta.X / VacuumSuck, delta.Y / VacuumSuck})
		}
	} else if m.Held_ != 0 {
		if h := m.Held(state); h != nil {
			h.Position = u.Position
			h.Position.Y--
			if m.Target().X > u.Position.X {
				h.Position.X += u.Size.X/2 + h.Size.X/2 + PixelSize
			} else {
				h.Position.X -= u.Size.X/2 + h.Size.X/2 + PixelSize
			}
			h.Velocity = Scale(m.Target().Sub(u.Position).Add(Coord{0, u.Size.Y / 2}), VacuumSpeed*float64(state.Tick-m.HeldSince_)).Add(u.Velocity)
			tr := state.Trace(h.Position, h.Position.Add(h.Velocity.Unit()), h.Size, false)
			collide := tr.CollideFunc(func(o *Unit) bool {
				return o != u && h.CollideWith(state, h, o)
			})
			if collide == nil && !tr.HitWorld {
				m.Held_, m.HeldSince_ = 0, 0
			}
		} else {
			m.Held_, m.HeldSince_ = 0, 0
		}
	} else if m.Input_.GetMouse1() == res.Button_pressed {
		if state.Tick-m.LastLemon_ > LemonTime {
			lemon := &Unit{
				Size:     LemonSize,
				Health:   1,
				UnitData: &Lemon{state.NextUnit},
			}
			lemon.Position = u.Position
			lemon.Position.Y -= u.Size.Y / 2
			if m.Target().X > u.Position.X {
				lemon.Position.X += u.Size.X/2 + LemonSize.X/2 + PixelSize
			} else {
				lemon.Position.X -= u.Size.X/2 + LemonSize.X/2 + PixelSize
			}
			lemon.Velocity = Scale(m.Target().Sub(u.Position).Add(Coord{0, u.Size.Y / 2}), LemonSpeed).Add(u.Velocity)
			tr := state.Trace(lemon.Position, lemon.Position.Add(lemon.Velocity.Unit()), lemon.Size, false)
			collide := tr.CollideFunc(func(o *Unit) bool {
				return lemon.CollideWith(state, lemon, o)
			})
			if collide == nil && !tr.HitWorld {
				state.Units[state.NextUnit] = lemon
				state.NextUnit++
			}
			m.LastLemon_ = state.Tick
		}
	}

	if m.HeldSince_ != 0 {
		u.Hurt(state, m.Held(state), int64(state.Tick-m.HeldSince_)/VacuumHurt)
	}
}

func (m *VacuumMan) Mass(state *State, u *Unit) int64 {
	mass := m.ManUnitData.Mass(state, u)

	if h := m.Held(state); h != nil {
		mass += h.Mass(state, u)
	}

	return mass
}

func (*VacuumMan) CollideWith(state *State, u, o *Unit) bool {
	if u == o {
		return false
	}
	if _, ok := o.UnitData.(*Lemon); ok {
		return false
	}
	return true
}

type Lemon struct {
	ID uint64
}

func (*Lemon) Color() color.RGBA {
	return mancolors[res.Man_Vacuum]
}

func (*Lemon) ShowDamage() bool {
	return false
}

func (*Lemon) Mass(*State, *Unit) int64 {
	return 50
}

func (*Lemon) Sprite(*State, *Unit) *image.RGBA {
	return lemonsprite
}

func (*Lemon) Update(*State, *Unit) {}

func (l *Lemon) UpdateDead(state *State, u *Unit) {
	delete(state.Units, l.ID)
}

func (*Lemon) CollideWith(state *State, u, o *Unit) bool {
	if _, ok := o.UnitData.(*Lemon); ok {
		return false
	}
	if _, ok := o.UnitData.(*VacuumMan); ok {
		return false
	}
	return true
}

type NormalMan struct {
	ManUnitData
}
