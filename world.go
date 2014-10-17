package main

type WorldTile struct {
	Tile  int
	Solid bool
}

type World struct {
	Min, Max Coord
	Tiles    []WorldTile
}

func (w *World) index(x, y int64) (i int, out int64) {
	if x < w.Min.X {
		out += w.Min.X - x
		x = w.Min.X
	} else if x > w.Max.X {
		out += x - w.Max.X
		x = w.Max.X
	}
	if y < w.Min.Y {
		out += w.Min.Y - y
		y = w.Min.Y
	} else if y > w.Max.Y {
		out += y - w.Max.Y
		y = w.Max.Y
	}
	i = int((x-w.Min.X)*(w.Max.Y-w.Min.Y+1) + (y - w.Min.Y))
	return
}

func (w *World) Outside(x, y int64) int64 {
	_, out := w.index(x, y)
	return out
}

func (w *World) Tile(x, y int64) int {
	i, _ := w.index(x, y)
	return w.Tiles[i].Tile
}

func (w *World) Solid(x, y int64) bool {
	i, _ := w.index(x, y)
	return w.Tiles[i].Solid
}

func (w *World) ensureTileExists(x, y int64) {
	newMin, newMax := w.Min, w.Max
	if w.Min.X > x {
		newMin.X = x
	} else if w.Max.X < x {
		newMax.X = x
	}
	if w.Min.Y > y {
		newMin.Y = y
	} else if w.Max.Y < y {
		newMax.Y = y
	}
	w.resize(newMin, newMax)
}

func (w *World) shrink() {
	newMin, newMax := w.Min, w.Max
	for {
		top, bottom, left, right := true, true, true, true
		if newMin.Y != newMax.Y {
			for x := newMin.X; x <= newMax.X; x++ {
				i1, _ := w.index(x, newMin.Y)
				i2, _ := w.index(x, newMin.Y+1)
				i3, _ := w.index(x, newMax.Y-1)
				i4, _ := w.index(x, newMax.Y)
				if w.Tiles[i1] != w.Tiles[i2] {
					top = false
				}
				if w.Tiles[i3] != w.Tiles[i4] {
					bottom = false
				}
			}
		} else {
			top, bottom = false, false
		}
		if newMin.X != newMax.X {
			for y := newMin.Y; y <= newMax.Y; y++ {
				i1, _ := w.index(newMin.X, y)
				i2, _ := w.index(newMin.X+1, y)
				i3, _ := w.index(newMax.X-1, y)
				i4, _ := w.index(newMax.X, y)
				if w.Tiles[i1] != w.Tiles[i2] {
					left = false
				}
				if w.Tiles[i3] != w.Tiles[i4] {
					right = false
				}
			}
		} else {
			left, right = false, false
		}

		if !top && !bottom && !left && !right {
			break
		}
		if left {
			newMin.X++
		}
		if right {
			newMax.X--
		}
		if top {
			newMin.Y++
		}
		if bottom {
			newMax.Y--
		}
	}
	w.resize(newMin, newMax)
}

func (w *World) resize(newMin, newMax Coord) {
	if newMin == w.Min && newMax == w.Max {
		return
	}
	newTiles := make([]WorldTile, (newMax.X-newMin.X+1)*(newMax.Y-newMin.Y+1))
	for x := newMin.X; x <= newMax.X; x++ {
		for y := newMin.Y; y <= newMax.Y; y++ {
			i, _ := w.index(x, y)
			newTiles[(x-newMin.X)*(newMax.Y-newMin.Y+1)+(y-newMin.Y)] = w.Tiles[i]
		}
	}
	w.Min, w.Max, w.Tiles = newMin, newMax, newTiles
}
