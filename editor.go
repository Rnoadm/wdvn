package main

import (
	"encoding/gob"
	"fmt"
	"github.com/skelterjohn/go.wde"
	"image"
	"image/color"
	"image/draw"
	"os"
)

func Editor(filename string) {
	defer wde.Stop()

	w, err := wde.NewWindow(800, 300)
	if err != nil {
		panic(err)
	}
	defer w.Close()

	w.Show()

	var (
		solidity   bool
		world      World
		offX, offY int64
		solid      = map[bool]*image.Uniform{
			true:  image.NewUniform(color.Gray{64}),
			false: image.NewUniform(color.Gray{192}),
		}
	)

	func() {
		f, err := os.Open(filename)
		if err == nil {
			defer f.Close()
			err = gob.NewDecoder(f).Decode(&world)
		}
		if err != nil || len(world.Tiles) < 3 {
			world.Min = Coord{0, -1}
			world.Max = Coord{0, 1}
			world.Tiles = make([]WorldTile, 3)
			world.Tiles[0].Tile, world.Tiles[0].Solid = 0, false
			world.Tiles[1].Tile, world.Tiles[1].Solid = 2, true
			world.Tiles[2].Tile, world.Tiles[2].Solid = 1, true
		}
	}()

	render := func(offX, offY int64) {
		img := image.NewRGBA(w.Screen().Bounds())

		offX = int64(img.Rect.Dx()/2) - offX
		offY = int64(img.Rect.Dy()/2) - offY

		min, max := Coord{-TileSize, -TileSize}, Coord{int64(img.Rect.Dx()) + TileSize, int64(img.Rect.Dy()) + TileSize}
		min = min.Sub(Coord{offX, offY}).Floor(TileSize)
		max = max.Sub(Coord{offX, offY}).Floor(TileSize)

		for x := min.X; x < max.X; x += TileSize {
			for y := min.Y; y < max.Y; y += TileSize {
				var i image.Image
				if solidity {
					i = solid[world.Solid(x/TileSize, y/TileSize)]
				} else {
					i = terrain[world.Tile(x/TileSize, y/TileSize)]
				}
				r := image.Rect(int(x+offX), int(y+offY), int(x+offX+TileSize), int(y+offY+TileSize))
				draw.Draw(img, r, i, i.Bounds().Min, draw.Src)
			}
		}

		w.Screen().CopyRGBA(img, img.Rect)
		w.FlushImage(img.Rect)
	}

	render(offX, offY)
	for event := range w.EventChan() {
		switch e := event.(type) {
		case wde.CloseEvent:
			f, err := os.Create(filename)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			err = gob.NewEncoder(f).Encode(&world)
			return
		case wde.ResizeEvent:
			// do nothing
		case wde.KeyDownEvent:
			// TODO
		case wde.KeyTypedEvent:
			switch e.Key {
			case wde.KeySpace:
				solidity = !solidity
			case wde.KeyUpArrow:
				offY -= 10
			case wde.KeyDownArrow:
				offY += 10
			case wde.KeyLeftArrow:
				offX -= 10
			case wde.KeyRightArrow:
				offX += 10
			}
		case wde.KeyUpEvent:
			// TODO
		case wde.MouseDownEvent:
			width, height := w.Size()
			c := Coord{offX + int64(e.Where.X-width/2), offY + int64(e.Where.Y-height/2)}
			c = c.Floor(TileSize)
			world.ensureTileExists(c.X/TileSize, c.Y/TileSize)
			i, _ := world.index(c.X/TileSize, c.Y/TileSize)

			if solidity {
				world.Tiles[i].Solid = !world.Tiles[i].Solid
			} else {
				switch e.Which {
				case wde.LeftButton:
					world.Tiles[i].Tile += 1
					world.Tiles[i].Tile %= len(terrain)
				case wde.RightButton:
					world.Tiles[i].Tile += len(terrain) - 1
					world.Tiles[i].Tile %= len(terrain)
				}
			}

			world.shrink()
		case wde.MouseUpEvent:
			// TODO
		case wde.MouseEnteredEvent:
			// TODO
		case wde.MouseExitedEvent:
			// TODO
		case wde.MouseMovedEvent:
			// TODO
		case wde.MouseDraggedEvent:
			// TODO
		default:
			panic(fmt.Errorf("unexpected event type %T in %#v", event, event))
		}
		render(offX, offY)
	}
}
