package main

import (
	"encoding/gob"
	"github.com/Rnoadm/wdvn/res"
	"image"
	"math"
)

type Man interface {
	UnitData
	Man() res.Man
	Respawn() uint64
	Target() Coord
	Input(*res.Packet)
}

func init() {
	gob.Register((*WhipMan)(nil))
	gob.Register((*DensityMan)(nil))
	gob.Register((*VacuumMan)(nil))
	gob.Register((*NormalMan)(nil))
}

type ManUnitData struct {
	Man_       res.Man
	Target_    Coord
	Crouching_ bool
	Input_     *res.Packet
	Respawn_   uint64
}

func (m *ManUnitData) UpdateDead(state *State, u *Unit) {
	if m.Respawn_ == 0 {
		m.Respawn_ = state.Tick + RespawnTime
	}
	if m.Respawn_ <= state.Tick && state.Lives > 0 {
		state.Lives--
		u.Health = DefaultHealth
		u.Position = state.SpawnPoint
		u.Gravity = 0
		u.Velocity = Coord{}
		u.Acceleration = Coord{}
		m.Respawn_ = 0
	}
}

func (m *ManUnitData) Update(state *State, u *Unit) {
	onGround := u.OnGround(state)

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
			u.Size = CrouchSize
			if !onGround {
				u.Position.Y += CrouchSize.Y - ManSize.Y
			}
			m.Crouching_ = true
		}
	} else {
		if m.Crouching_ {
			tr := state.Trace(u.Position, u.Position, ManSize, false)
			collide := tr.Collide(u)
			if collide == nil && !tr.HitWorld {
				u.Size = ManSize
				if !onGround {
					u.Velocity.Y += Gravity
				}
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
		if m.Man() == res.Man_Whip {
			u.Gravity = 0
		}
	} else {
		u.Acceleration.Y = 0
	}

	m.Target_.X = u.Position.X + m.Input_.GetX()*PixelSize
	m.Target_.Y = u.Position.Y + m.Input_.GetY()*PixelSize
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

type WhipMan struct {
	ManUnitData

	WhipStart uint64
	WhipStop  uint64
	WhipEnd   Coord
	WhipPull  bool
}

func (m *WhipMan) Update(state *State, u *Unit) {
	m.ManUnitData.Update(state, u)

	if m.WhipStop != 0 && m.WhipStop-m.WhipStart < state.Tick-m.WhipStop {
		m.WhipStart, m.WhipStop, m.WhipEnd = 0, 0, Coord{}
	}
	if m.WhipStop == 0 && u.Gravity != 0 {
		u.Gravity += Gravity / WhipAntiGravityDuration
		if u.Gravity > 0 {
			u.Gravity = 0
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
			start, stop := u.Position, m.Target()
			start.Y -= u.Size.Y / 2
			delta := stop.Sub(start)

			dist := math.Hypot(float64(delta.X), float64(delta.Y))
			if m.WhipStart < m.WhipStop-WhipTimeMax {
				m.WhipStart = m.WhipStop - WhipTimeMax
			}
			m.WhipEnd = Coord{}
			if m.WhipStart < m.WhipStop-WhipTimeMin {
				stop.X = start.X + int64(float64(delta.X)*WhipDistance/dist)
				stop.Y = start.Y + int64(float64(delta.Y)*WhipDistance/dist)

				tr := state.Trace(start, stop, Coord{1, 1}, false)
				collide := tr.Collide(&state.Mans[res.Man_Whip])
				m.WhipEnd = tr.End

				if collide != nil && !collide.IsMan() {
					damage := int64(WhipDamageMin + (WhipDamageMax-WhipDamageMin)*(m.WhipStop-m.WhipStart)/(WhipTimeMax-WhipTimeMin))
					collide.Hurt(state, u, damage)
				}

				dx, dy := start.X-tr.End.X, start.Y-tr.End.Y
				dist = math.Hypot(float64(dx), float64(dy))
				if dist > 0 && (collide != nil || tr.HitWorld) {
					speed := float64(WhipSpeedMin+(WhipSpeedMax-WhipSpeedMin)*(m.WhipStop-m.WhipStart)/(WhipTimeMax-WhipTimeMin)) / dist
					if m.WhipPull {
						u.Velocity.X += int64(float64(-dx) * speed)
						u.Velocity.Y += int64(float64(-dy) * speed)
						u.Gravity = -Gravity
					} else if collide != nil {
						collide.Velocity.X += int64(float64(dx) * speed)
						collide.Velocity.Y += int64(float64(dy) * speed)
					}
				}
			}
		}
	}
}

type DensityMan struct {
	ManUnitData
}

func (m *DensityMan) Update(state *State, u *Unit) {
	m.ManUnitData.Update(state, u)

	if m.Input_.GetMouse1() == res.Button_pressed {
		u.Gravity++
	}
	if m.Input_.GetMouse2() == res.Button_pressed {
		u.Gravity--
	}
	if u.Gravity < -Gravity {
		u.Gravity = -Gravity
	}
	if u.Gravity > Gravity {
		u.Gravity = Gravity
	}
}

type VacuumMan struct {
	ManUnitData
}

type NormalMan struct {
	ManUnitData
}
