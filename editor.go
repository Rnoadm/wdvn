package main

import (
	"encoding/gob"
	"fmt"
	"github.com/skelterjohn/go.wde"
	"image"
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
		world      World
		offX, offY int64
	)

	func() {
		f, err := os.Open(filename)
		if err == nil {
			defer f.Close()
			err = gob.NewDecoder(f).Decode(&world)
		}
		if err != nil || len(world.Tiles) < 3 {
			world.Min = Coord{0, 0}
			world.Max = Coord{0, 1}
			world.Tiles = make([]WorldTile, 3)
			world.Tiles[0].Solid = false
			world.Tiles[1].Solid = true
		}
	}()

	render := func(offX, offY int64) {
		img := image.NewRGBA(w.Screen().Bounds())

		draw.Draw(img, img.Rect, image.White, image.ZP, draw.Src)

		offX = int64(img.Rect.Dx()/2) - offX
		offY = int64(img.Rect.Dy()/2) - offY

		world.Render(img, offX, offY)

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

			switch e.Which {
			case wde.LeftButton:
				world.Tiles[i].Tile += 1
				world.Tiles[i].Tile %= len(terrain)
			case wde.MiddleButton:
				world.Tiles[i].Tile += len(terrain) - 1
				world.Tiles[i].Tile %= len(terrain)
			case wde.RightButton:
				world.Tiles[i].Solid = !world.Tiles[i].Solid
			}

			world.shrink()

			world.mtx.Lock()
			world.rendered = nil
			world.mtx.Unlock()
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
