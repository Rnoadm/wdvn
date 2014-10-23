package main

import (
	"github.com/Rnoadm/wdvn/res"
	"github.com/dustin/go-humanize"
	"image"
	"image/color"
	"math/rand"
)

type Unit struct {
	Position     Coord
	Velocity     Coord
	Acceleration Coord
	Size         Coord
	Gravity      int64
	Health       int64
	UnitData
}

type UnitData interface {
	Update(*State, *Unit)
	UpdateDead(*State, *Unit)
	Sprite(*State, *Unit) *image.RGBA
	Mass(*State, *Unit) int64
	ShowDamage() bool
	Color() color.RGBA
}

func (u *Unit) OnGround(state *State) (bool, SpecialTile) {
	tr := state.Trace(u.Position, u.Position.Add(Coord{0, 1}), u.Size, false)
	tr.Collide(u)
	return tr.End == u.Position, tr.Special
}

func (u *Unit) Hurt(state *State, by *Unit, amount int64) {
	if amount <= 0 || u.Health <= 0 {
		return
	}
	if u.ShowDamage() {
		c := color.RGBA{96, 96, 96, 255}
		if by != nil {
			c = by.Color()
		}
		state.Floaters = append(state.Floaters, Floater{
			S:  humanize.Comma(amount),
			Fg: u.Color(),
			Bg: c,
			X:  u.Position.X - u.Size.X/2 + rand.Int63n(u.Size.X),
			Y:  u.Position.Y - rand.Int63n(u.Size.Y),
			T:  state.Tick,
		})
	}
	if u.Health < amount {
		amount = u.Health
	}
	u.Health -= amount
}

func (u *Unit) IsMan() bool {
	_, ok := u.UnitData.(Man)
	return ok
}

func (u *Unit) Update(state *State) {
	if u.Health > 0 {
		u.UnitData.Update(state, u)
	} else {
		u.UnitData.UpdateDead(state, u)
		u.Acceleration = Coord{}
		u.Gravity = 0
	}

	onGround, special := u.OnGround(state)

	if onGround && u.Velocity.Y > 0 {
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

	if onGround {
		switch special {
		case SpecialTile_Bounce:
			u.Velocity.Y = -100 * Gravity
		case SpecialTile_Checkpoint:
			if m, ok := u.UnitData.(Man); ok && u.Health > 0 {
				pos := u.Position.Floor(TileSize * PixelSize)
				found := false
				for x := int64(-3); x <= 3; x++ {
					for y := int64(-3); y <= 3; y++ {
						if state.world.Special(pos.X/TileSize/PixelSize+x, pos.Y/TileSize/PixelSize+y) == SpecialTile_Checkpoint {
							pos = pos.Add(Coord{x*TileSize*PixelSize + TileSize*PixelSize/2, y*TileSize*PixelSize - TileSize*PixelSize})
							found = true
							break
						}
					}
					if found {
						break
					}
				}
				if !found {
					break
				}
				if pos != *m.Checkpoint() && pos != state.SpawnPoint {
					count := 0
					for i := range state.Mans {
						if pos == *state.Mans[i].UnitData.(Man).Checkpoint() {
							count++
						}
					}
					text := [res.Man_count]string{"CHECKPOINT 25%", "CHECKPOINT 50%", "CHECKPOINT 75%", "CHECKPOINT UNLOCKED"}[count]
					*m.Checkpoint() = pos
					state.Floaters = append(state.Floaters, Floater{
						S:  text,
						Fg: color.RGBA{255, 255, 255, 255},
						Bg: u.Color(),
						X:  pos.X,
						Y:  pos.Y,
						T:  state.Tick,
					})
					if count == int(res.Man_count-1) {
						state.SpawnPoint = pos
					}
				}
			}
		}
	}

	tr := state.Trace(u.Position, u.Position.Add(Coord{u.Velocity.X / TicksPerSecond, u.Velocity.Y / TicksPerSecond}), u.Size, false)
	collide := tr.Collide(u)
	if u.Health > 0 && tr.End == u.Position && !u.Velocity.Zero() {
		stuck := state.Trace(u.Position, u.Position.Add(Coord{u.Velocity.X / TicksPerSecond, 0}), u.Size, false)
		collide2 := stuck.Collide(u)
		if stuck.End != tr.End {
			tr, collide = stuck, collide2
		} else {
			stuck = state.Trace(u.Position, u.Position.Add(Coord{0, u.Velocity.Y / TicksPerSecond}), u.Size, false)
			collide2 = stuck.Collide(u)
			if stuck.End != tr.End {
				tr, collide = stuck, collide2
			}
		}
	}
	if tr.End == u.Position && collide != nil {
		delta := u.Position.Sub(collide.Position)
		delta.X /= TicksPerSecond
		delta.Y /= TicksPerSecond
		if delta.Zero() {
			delta.X += rand.Int63n(PixelSize*2+1) - PixelSize
		}
		stuck := state.Trace(u.Position, u.Position.Add(delta), u.Size, true)
		tr.End = stuck.End
	}
	if collide == nil && tr.HitWorld {
		switch tr.Special {
		case SpecialTile_None:
			switch tr.Side {
			case SideLeft:
				u.Hurt(state, nil, u.Velocity.X*u.Mass(state, u)/DamageFactor)
				u.Velocity.X = 0
			case SideRight:
				u.Hurt(state, nil, -u.Velocity.X*u.Mass(state, u)/DamageFactor)
				u.Velocity.X = 0
			case SideTop:
				u.Hurt(state, nil, u.Velocity.Y*u.Mass(state, u)/DamageFactor)
				u.Velocity.Y = 0
			case SideBottom:
				u.Hurt(state, nil, -u.Velocity.Y*u.Mass(state, u)/DamageFactor)
				u.Velocity.Y = 0
			}
		case SpecialTile_Bounce:
			switch tr.Side {
			case SideLeft:
				u.Velocity.X = -100 * Gravity
			case SideRight:
				u.Velocity.X = 100 * Gravity
			case SideTop:
				u.Velocity.Y = -100 * Gravity
			case SideBottom:
				u.Velocity.Y = 100 * Gravity
			}
		case SpecialTile_Checkpoint:
			// do nothing
		default:
			panic("unimplemented special tile type: " + specialTile_names[tr.Special])
		}
	}
	u.Position = tr.End
	if u.Health > 0 && collide != nil {
		if u.IsMan() != collide.IsMan() {
			u.Velocity.X, u.Velocity.Y = u.Velocity.X*2, u.Velocity.Y*2
			collide.Velocity.X, collide.Velocity.Y = collide.Velocity.X*2, collide.Velocity.Y*2
		}
		weightedSwap := func(vi1, vi2 int64) (v1, v2 int64) {
			m1, m2 := u.Mass(state, u), collide.Mass(state, collide)
			if m1 <= 0 {
				m1 = 1
			}
			if m2 <= 0 {
				m2 = 1
			}
			vm1, vm2 := vi1*m1, vi2*m2
			v1 = (vm1/3 + vm2*2/3) / m1
			v2 = (vm1*2/3 + vm2/3) / m2
			return
		}
		switch tr.Side {
		case SideLeft, SideRight:
			v1, v2 := weightedSwap(u.Velocity.X, collide.Velocity.X)
			u.Velocity.X, collide.Velocity.X = v1, v2
			v1, v2 = v1-v2, v2-v1
			if v1 < 0 {
				v1 = -v1
			}
			if v2 < 0 {
				v2 = -v2
			}
			u.Hurt(state, nil, v1*collide.Mass(state, u)/DamageFactor)
			collide.Hurt(state, nil, v2*u.Mass(state, u)/DamageFactor)
		case SideTop, SideBottom:
			v1, v2 := weightedSwap(u.Velocity.Y, collide.Velocity.Y)
			u.Velocity.Y, collide.Velocity.Y = v1, v2
			v1, v2 = v1-v2, v2-v1
			if v1 < 0 {
				v1 = -v1
			}
			if v2 < 0 {
				v2 = -v2
			}
			u.Hurt(state, nil, v1*collide.Mass(state, u)/DamageFactor)
			collide.Hurt(state, nil, v2*u.Mass(state, u)/DamageFactor)
		}
	}
	if pos := u.Position.Floor(PixelSize * TileSize); state.world.Outside(pos.X/TileSize/PixelSize, pos.Y/TileSize/PixelSize) > 100 {
		u.Hurt(state, nil, u.Health)
	}
}
