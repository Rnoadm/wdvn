package main

import (
	"image"
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
}

func (u *Unit) OnGround(state *State) bool {
	tr := state.Trace(u.Position, u.Position.Add(Coord{0, 1}), u.Size, false)
	tr.Collide(u)
	return tr.End == u.Position
}

func (u *Unit) Hurt(state *State, by *Unit, amount int64) {
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

	onGround := u.OnGround(state)

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
	if collide == nil && tr.HitWorld {
		switch tr.Special {
		case SpecialTile_None:
			switch speed := (u.Velocity.X*u.Velocity.X + u.Velocity.Y*u.Velocity.Y) / TileSize / PixelSize / TileSize / PixelSize; {
			case speed > 100*100:
				u.Hurt(state, nil, 10)
			case speed > 85*85:
				u.Hurt(state, nil, 8)
			case speed > 70*70:
				u.Hurt(state, nil, 6)
			case speed > 55*55:
				u.Hurt(state, nil, 4)
			case speed > 30*30:
				u.Hurt(state, nil, 1)
			}
		case SpecialTile_Bounce:
			u.Velocity.X, u.Velocity.Y = -u.Velocity.X*3/2, -u.Velocity.Y*3/2
		default:
			panic("unimplemented special tile type: " + specialTile_names[tr.Special])
		}
	}
	u.Position = tr.End
	if collide != nil {
		if !u.IsMan() || !collide.IsMan() {
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
