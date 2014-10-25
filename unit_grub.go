package main

import (
	"encoding/gob"
	"image"
	"image/color"
	"math/rand"
)

type Grub struct {
	LastMoved uint64
}

func init() {
	gob.Register((*Grub)(nil))
}

func (g *Grub) Update(state *State, u *Unit) {
	if state.Tick > 1*TicksPerSecond && g.LastMoved < state.Tick-1*TicksPerSecond {
		if rand.Intn(5) == 0 {
			u.Acceleration.Y = -10 * Gravity
		}
		if rand.Intn(2) == 0 {
			u.Acceleration.X = 50 * PixelSize
		} else {
			u.Acceleration.X = -50 * PixelSize
		}
		g.LastMoved = state.Tick
	} else {
		u.Acceleration.X, u.Acceleration.Y = 0, 0
	}
}
func (g *Grub) UpdateDead(state *State, u *Unit) {
	for i, o := range state.Units {
		if o == u {
			delete(state.Units, i)
			return
		}
	}
}
func (g *Grub) CollideWith(state *State, u, o *Unit) bool {
	return u != o
}
func (g *Grub) Sprite(state *State, u *Unit) *image.RGBA {
	return grubsprite
}
func (g *Grub) Mass(state *State, u *Unit) int64 {
	return 200
}
func (g *Grub) ShowDamage() bool {
	return true
}
func (g *Grub) Color() color.RGBA {
	return color.RGBA{0, 0, 0, 255}
}
