package main

import (
	"image"
	"image/draw"
	"sync"
)

const WorldRenderCacheSize = 64

type SpecialTile int

const (
	SpecialTile_None = iota
	SpecialTile_Bounce
	SpecialTile_count
)

var specialTile_names [SpecialTile_count]string = [...]string{
	SpecialTile_None:   "none",
	SpecialTile_Bounce: "bounce",
}

type WorldTile struct {
	Tile  int
	Solid bool
	SpecialTile
}

type World struct {
	Min, Max Coord
	Tiles    []WorldTile

	rendered map[Coord]*image.RGBA
	mtx      sync.Mutex
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

func (w *World) Render(img draw.Image, offX, offY int64) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.rendered == nil {
		w.rendered = make(map[Coord]*image.RGBA)
	}

	min, max := Coord{0, 0}, Coord{int64(img.Bounds().Dx()) + TileSize*WorldRenderCacheSize, int64(img.Bounds().Dy()) + TileSize*WorldRenderCacheSize}
	min = min.Sub(Coord{offX, offY}).Floor(TileSize * WorldRenderCacheSize)
	max = max.Sub(Coord{offX, offY}).Floor(TileSize * WorldRenderCacheSize)
	min.X, min.Y = min.X/TileSize/WorldRenderCacheSize, min.Y/TileSize/WorldRenderCacheSize
	max.X, max.Y = max.X/TileSize/WorldRenderCacheSize, max.Y/TileSize/WorldRenderCacheSize

	for cx := min.X; cx < max.X; cx++ {
		for cy := min.Y; cy < max.Y; cy++ {
			cache, ok := w.rendered[Coord{cx, cy}]
			if !ok {
				cache = image.NewRGBA(image.Rect(0, 0, TileSize*WorldRenderCacheSize, TileSize*WorldRenderCacheSize))
				w.rendered[Coord{cx, cy}] = cache
				for x := 0; x < WorldRenderCacheSize; x++ {
					for y := 0; y < WorldRenderCacheSize; y++ {
						tx, ty := cx*WorldRenderCacheSize+int64(x), cy*WorldRenderCacheSize+int64(y)
						i := 0
						if w.Solid(tx, ty) {
							i |= 1 << 0
						}
						if w.Solid(tx-1, ty) {
							i |= 1 << 1
						}
						if w.Solid(tx-1, ty-1) {
							i |= 1 << 2
						}
						if w.Solid(tx, ty-1) {
							i |= 1 << 3
						}
						if w.Solid(tx+1, ty-1) {
							i |= 1 << 4
						}
						if w.Solid(tx+1, ty) {
							i |= 1 << 5
						}
						if w.Solid(tx+1, ty+1) {
							i |= 1 << 6
						}
						if w.Solid(tx, ty+1) {
							i |= 1 << 7
						}
						if w.Solid(tx-1, ty+1) {
							i |= 1 << 8
						}
						tr := terrain[w.Tile(tx, ty)]
						tm := tilemask[i]
						r := image.Rect(x*TileSize, y*TileSize, x*TileSize+TileSize, y*TileSize+TileSize)
						draw.DrawMask(cache, r, tr, tr.Rect.Min, tm, tm.Rect.Min, draw.Over)
					}
				}
			}
			draw.Draw(img, cache.Rect.Add(img.Bounds().Min).Add(image.Pt(int(offX+cx*TileSize*WorldRenderCacheSize), int(offY+cy*TileSize*WorldRenderCacheSize))), cache, image.ZP, draw.Over)
		}
	}
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

func (w *World) Special(x, y int64) SpecialTile {
	i, _ := w.index(x, y)
	return w.Tiles[i].SpecialTile
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
		if newMin.Y < newMax.Y {
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
		if newMin.X < newMax.X {
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
		if right && newMin.X != newMax.X {
			newMax.X--
		}
		if top {
			newMin.Y++
		}
		if bottom && newMin.Y != newMax.Y {
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
