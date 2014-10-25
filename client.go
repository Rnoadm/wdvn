package main

import (
	"bytes"
	"code.google.com/p/draw2d/draw2d"
	"code.google.com/p/freetype-go/freetype/truetype"
	"code.google.com/p/goprotobuf/proto"
	"encoding/gob"
	"fmt"
	"github.com/BenLubar/bindiff"
	"github.com/Rnoadm/wdvn/res"
	"github.com/skelterjohn/go.wde"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"
)

func Client(addr string) {
	defer quitWait.Done()
	defer wde.Stop()

	w, err := wde.NewWindow(*flagWidth, *flagHeight)
	if err != nil {
		panic(err)
	}
	defer w.Close()

	w.Show()

	graphicsInit()

	read, write, errors := make(chan *res.Packet), make(chan *res.Packet), make(chan error, 2)
	defer func() {
		close(write)
		for {
			if read == nil && errors == nil {
				return
			}

			select {
			case _, ok := <-read:
				if !ok {
					read = nil
				}

			case _, ok := <-errors:
				if !ok {
					errors = nil
				}
			}
		}
	}()
	go Reconnect(addr, read, write, errors)

	var (
		me        res.Man
		state     State
		lastState []byte
		lastTick  uint64
		input     = make(chan *res.Packet, 1)
		noState   = true
		world     *World
		mouse     image.Point
	)
	defer close(input)
	go func() {
		var p *res.Packet

		for {
			out := write
			if p == nil {
				out = nil
			}

			select {
			case v, ok := <-input:
				if !ok {
					return
				}

				if p == nil {
					p = &res.Packet{
						Type: Type_Input,
					}
				}
				if v == nil {
					proto.Merge(p, ReleaseAll)
				} else {
					proto.Merge(p, v)
				}

			case out <- p:
				p = nil
			}
		}
	}()

	sendMouse := func() {
		width, height := w.Size()
		if *flagSplitScreen {
			width /= 2
			height /= 2

			var them res.Man
			if mouse.Y >= height {
				them |= 1
			}
			if mouse.X >= width {
				them |= 2
			}

			if me&1 == 1 {
				mouse.Y -= height
			}
			if me&2 == 2 {
				mouse.X -= width
			}

			delta := state.Mans[them].Position.Sub(state.Mans[me].Position)
			mouse.X += int(delta.X / PixelSize)
			mouse.Y += int(delta.Y / PixelSize)
		}
		mouse.X -= width / 2
		mouse.Y -= height / 2
		input <- &res.Packet{
			X: proto.Int64(int64(mouse.X)),
			Y: proto.Int64(int64(mouse.Y)),
		}

	}

	renderResize, renderMan, renderState, renderError := make(chan struct{}, 1), make(chan res.Man, 1), make(chan State, 1), make(chan error, 1)
	go RenderThread(w, renderResize, renderMan, renderState, renderError)

	for {
		select {
		case err := <-errors:
			select {
			case <-quitRequest:
				return
			default:
			}

			world = nil
			state = State{}
			noState = true
			for {
				select {
				case renderState <- state:
				case <-renderState:
					continue
				}
				break
			}
			for {
				select {
				case renderError <- err:
				case <-renderError:
					continue
				}
				break
			}

		case p := <-read:
			switch p.GetType() {
			case res.Type_Ping:
				go Send(write, p)

			case res.Type_SelectMan:
				me = p.GetMan()
				for {
					select {
					case renderMan <- me:
					case <-renderMan:
						continue
					}
					break
				}

			case res.Type_StateDiff:
				if !noState {
					if lastTick < p.GetTick() {
						go Send(write, &res.Packet{
							Type: Type_FullState,
						})
						noState = true
					} else if lastTick == p.GetTick() {
						var err error
						lastState, err = bindiff.Forward(lastState, p.GetData())
						if err == nil {
							var newState State
							err = gob.NewDecoder(bytes.NewReader(lastState)).Decode(&newState)
							if err == nil {
								state = newState
								state.world = world
								lastTick = state.Tick
								for {
									select {
									case renderState <- state:
									case <-renderState:
										continue
									}
									break
								}
							}
						}
						if err != nil {
							go Send(write, &res.Packet{
								Type: Type_FullState,
							})
							noState = true
						}
					}
				}

			case res.Type_FullState:
				state = State{}
				err := gob.NewDecoder(bytes.NewReader(p.GetData())).Decode(&state)
				if err != nil {
					panic(err)
				}
				state.world = world
				lastState, lastTick, noState = p.GetData(), state.Tick, false
				for {
					select {
					case renderState <- state:
					case <-renderState:
						continue
					}
					break
				}

			case res.Type_World:
				world = new(World)
				err := gob.NewDecoder(bytes.NewReader(p.GetData())).Decode(&world)
				if err != nil {
					panic(err)
				}
				state.world = world
				for {
					select {
					case renderState <- state:
					case <-renderState:
						continue
					}
					break
				}
			}

		case event := <-w.EventChan():
			switch e := event.(type) {
			case wde.CloseEvent:
				close(quitRequest)
			case wde.ResizeEvent:
				select {
				case renderResize <- struct{}{}:
				default:
				}
			case wde.KeyDownEvent:
				switch e.Key {
				case wde.KeyW, wde.KeyPadUp, wde.KeyUpArrow, wde.KeySpace:
					input <- &res.Packet{
						KeyUp: Button_pressed,
					}

				case wde.KeyS, wde.KeyPadDown, wde.KeyDownArrow:
					input <- &res.Packet{
						KeyDown: Button_pressed,
					}

				case wde.KeyA, wde.KeyPadLeft, wde.KeyLeftArrow:
					input <- &res.Packet{
						KeyLeft: Button_pressed,
					}

				case wde.KeyD, wde.KeyPadRight, wde.KeyRightArrow:
					input <- &res.Packet{
						KeyRight: Button_pressed,
					}

				case wde.KeyF1:
					go Send(write, &res.Packet{
						Type: Type_SelectMan,
						Man:  Man_Whip,
					})
				case wde.KeyF2:
					go Send(write, &res.Packet{
						Type: Type_SelectMan,
						Man:  Man_Density,
					})
				case wde.KeyF3:
					go Send(write, &res.Packet{
						Type: Type_SelectMan,
						Man:  Man_Vacuum,
					})
				case wde.KeyF4:
					go Send(write, &res.Packet{
						Type: Type_SelectMan,
						Man:  Man_Normal,
					})
				}
			case wde.KeyTypedEvent:
				// TODO
			case wde.KeyUpEvent:
				switch e.Key {
				case wde.KeyW, wde.KeyPadUp, wde.KeyUpArrow, wde.KeySpace:
					input <- &res.Packet{
						KeyUp: Button_released,
					}

				case wde.KeyS, wde.KeyPadDown, wde.KeyDownArrow:
					input <- &res.Packet{
						KeyDown: Button_released,
					}

				case wde.KeyA, wde.KeyPadLeft, wde.KeyLeftArrow:
					input <- &res.Packet{
						KeyLeft: Button_released,
					}

				case wde.KeyD, wde.KeyPadRight, wde.KeyRightArrow:
					input <- &res.Packet{
						KeyRight: Button_released,
					}
				}
			case wde.MouseDownEvent:
				mouse = e.Where
				sendMouse()
				switch e.Which {
				case wde.LeftButton:
					input <- &res.Packet{
						Mouse1: Button_pressed,
					}

				case wde.RightButton:
					input <- &res.Packet{
						Mouse2: Button_pressed,
					}
				}
			case wde.MouseUpEvent:
				mouse = e.Where
				sendMouse()
				switch e.Which {
				case wde.LeftButton:
					input <- &res.Packet{
						Mouse1: Button_released,
					}

				case wde.RightButton:
					input <- &res.Packet{
						Mouse2: Button_released,
					}
				}
			case wde.MouseEnteredEvent:
				// TODO
			case wde.MouseExitedEvent:
				input <- nil
			case wde.MouseMovedEvent:
				mouse = e.Where
				sendMouse()
			case wde.MouseDraggedEvent:
				mouse = e.Where
				sendMouse()
			default:
				panic(fmt.Errorf("unexpected event type %T in %#v", event, event))
			}

		case <-quitRequest:
			return
		}
	}
}

var (
	graphicsOnce sync.Once
	mancolors    [res.Man_count]color.RGBA = [...]color.RGBA{
		res.Man_Whip:    color.RGBA{192, 0, 0, 255},
		res.Man_Density: color.RGBA{128, 128, 0, 255},
		res.Man_Vacuum:  color.RGBA{0, 128, 0, 255},
		res.Man_Normal:  color.RGBA{0, 0, 192, 255},
	}
	mansprites    [res.Man_count][2]*image.RGBA
	manfills      [res.Man_count]*image.Uniform
	terrain       []*image.RGBA
	tilemask      [1 << 10]*image.Alpha
	fade          [VelocityClones + 1]*image.Uniform
	offscreenfade *image.Uniform
	deadhaze      *image.Uniform
	parallax      [2]*image.RGBA
	lemonsprite   *image.RGBA
	grubsprite    *image.RGBA
)

func graphicsInit() {
	graphicsOnce.Do(func() {
		font, err := truetype.Parse(res.LuxisrTtf)
		if err != nil {
			panic(err)
		}
		draw2d.RegisterFont(draw2d.FontData{
			Name:   "luxi",
			Family: draw2d.FontFamilySans,
			Style:  draw2d.FontStyleNormal,
		}, font)

		src, err := png.Decode(bytes.NewReader(res.MansPng))
		if err != nil {
			panic(err)
		}
		dst := image.NewRGBA(src.Bounds())
		draw.Draw(dst, dst.Rect, src, dst.Rect.Min, draw.Src)
		if ManSize.Y < CrouchSize.Y || dst.Rect.Dy() != len(mansprites)*int(ManSize.Y/PixelSize) || dst.Rect.Dx() != int(ManSize.X/PixelSize+CrouchSize.X/PixelSize) {
			panic("man size mismatch")
		}
		r1 := image.Rect(0, 0, int(ManSize.X/PixelSize), int(ManSize.Y/PixelSize)).Add(dst.Rect.Min)
		r2 := image.Rect(int(ManSize.X/PixelSize), int(ManSize.X-CrouchSize.Y), int(ManSize.X/PixelSize+CrouchSize.X/PixelSize), int(ManSize.Y/PixelSize)).Add(dst.Rect.Min)
		for i := range mansprites {
			mansprites[i][0] = dst.SubImage(r1.Add(image.Pt(0, i*int(ManSize.Y/PixelSize)))).(*image.RGBA)
			mansprites[i][1] = dst.SubImage(r2.Add(image.Pt(0, i*int(ManSize.Y/PixelSize)))).(*image.RGBA)
		}
		for i := range manfills {
			manfills[i] = image.NewUniform(mancolors[i])
		}

		src, err = png.Decode(bytes.NewReader(res.TerrainPng))
		if err != nil {
			panic(err)
		}
		dst = image.NewRGBA(src.Bounds())
		draw.Draw(dst, dst.Rect, src, dst.Rect.Min, draw.Src)
		if dst.Rect.Dx()%TileSize != 0 || dst.Rect.Dy() != TileSize {
			panic("tile size mismatch")
		}
		for x := dst.Rect.Min.X; x < dst.Rect.Max.X; x += TileSize {
			terrain = append(terrain, dst.SubImage(image.Rect(x, dst.Rect.Min.Y, x+TileSize, dst.Rect.Max.Y)).(*image.RGBA))
		}

		src, err = png.Decode(bytes.NewReader(res.TileSidePng))
		if err != nil {
			panic(err)
		}
		tileSide := image.NewGray(src.Bounds())
		draw.Draw(tileSide, tileSide.Rect, src, tileSide.Rect.Min, draw.Src)
		if tileSide.Rect.Dy() != TileSize {
			panic("tile size mismatch")
		}
		src, err = png.Decode(bytes.NewReader(res.TileCornerInnerPng))
		if err != nil {
			panic(err)
		}
		tileCornerInner := image.NewGray(src.Bounds())
		draw.Draw(tileCornerInner, tileCornerInner.Rect, src, tileCornerInner.Rect.Min, draw.Src)
		if tileCornerInner.Rect.Dx() != tileCornerInner.Rect.Dy() || tileSide.Rect.Dx() != tileCornerInner.Rect.Dx() {
			panic("tile size mismatch")
		}
		src, err = png.Decode(bytes.NewReader(res.TileCornerOuterPng))
		if err != nil {
			panic(err)
		}
		tileCornerOuter := image.NewGray(src.Bounds())
		draw.Draw(tileCornerOuter, tileCornerOuter.Rect, src, tileCornerOuter.Rect.Min, draw.Src)
		if tileCornerInner.Rect.Dx() != tileCornerOuter.Rect.Dx() || tileCornerInner.Rect.Dy() != tileCornerOuter.Rect.Dy() {
			panic("tile size mismatch")
		}
		{
			r := image.Rect(0, 0, TileSize, TileSize)

			for i := range tilemask {
				tilemask[i] = image.NewAlpha(r)
				if i&(1<<0) == 0 {
					continue
				}
				draw.Draw(tilemask[i], r, image.NewUniform(color.Opaque), image.ZP, draw.Src)
				drawRotated := func(img *image.Gray, f func(int, int) (int, int)) {
					for x := 0; x < img.Rect.Dx(); x++ {
						for y := 0; y < img.Rect.Dy(); y++ {
							tilemask[i].Pix[tilemask[i].PixOffset(f(x, y))] = img.Pix[img.PixOffset(x, y)]
						}
					}
				}
				if i&(1<<1) == 0 {
					drawRotated(tileSide, func(x, y int) (int, int) { return x, y })
				}
				if i&(1<<3) == 0 {
					drawRotated(tileSide, func(x, y int) (int, int) { return TileSize - 1 - y, x })
				}
				if i&(1<<5) == 0 {
					drawRotated(tileSide, func(x, y int) (int, int) { return TileSize - 1 - x, TileSize - 1 - y })
				}
				if i&(1<<7) == 0 {
					drawRotated(tileSide, func(x, y int) (int, int) { return y, TileSize - 1 - x })
				}
				if i&(1<<1|1<<3) == 0 {
					drawRotated(tileCornerOuter, func(x, y int) (int, int) { return x, y })
				}
				if i&(1<<3|1<<5) == 0 {
					drawRotated(tileCornerOuter, func(x, y int) (int, int) { return TileSize - 1 - y, x })
				}
				if i&(1<<5|1<<7) == 0 {
					drawRotated(tileCornerOuter, func(x, y int) (int, int) { return TileSize - 1 - x, TileSize - 1 - y })
				}
				if i&(1<<7|1<<1) == 0 {
					drawRotated(tileCornerOuter, func(x, y int) (int, int) { return y, TileSize - 1 - x })
				}
				if i&(1<<1|1<<2|1<<3) == 1<<1|1<<3 {
					drawRotated(tileCornerInner, func(x, y int) (int, int) { return x, y })
				}
				if i&(1<<3|1<<4|1<<5) == 1<<3|1<<5 {
					drawRotated(tileCornerInner, func(x, y int) (int, int) { return TileSize - 1 - y, x })
				}
				if i&(1<<5|1<<6|1<<7) == 1<<5|1<<7 {
					drawRotated(tileCornerInner, func(x, y int) (int, int) { return TileSize - 1 - x, TileSize - 1 - y })
				}
				if i&(1<<7|1<<8|1<<1) == 1<<7|1<<1 {
					drawRotated(tileCornerInner, func(x, y int) (int, int) { return y, TileSize - 1 - x })
				}
			}
		}

		for i := range fade {
			fade[i] = image.NewUniform(color.Alpha16{uint16(0xffff * (len(fade) - i) / len(fade) * (len(fade) - i) / len(fade))})
		}

		offscreenfade = image.NewUniform(color.Alpha{0x40})
		deadhaze = image.NewUniform(color.RGBA{64, 64, 64, 64})

		src, err = png.Decode(bytes.NewReader(res.Parallax0Png))
		if err != nil {
			panic(err)
		}
		parallax[0] = image.NewRGBA(src.Bounds())
		draw.Draw(parallax[0], parallax[0].Rect, src, parallax[0].Rect.Min, draw.Src)

		src, err = png.Decode(bytes.NewReader(res.Parallax1Png))
		if err != nil {
			panic(err)
		}
		parallax[1] = image.NewRGBA(src.Bounds())
		draw.Draw(parallax[1], parallax[1].Rect, src, parallax[1].Rect.Min, draw.Src)

		src, err = png.Decode(bytes.NewReader(res.LemonPng))
		if err != nil {
			panic(err)
		}
		lemonsprite = image.NewRGBA(src.Bounds())
		draw.Draw(lemonsprite, lemonsprite.Rect, src, lemonsprite.Rect.Min, draw.Src)

		src, err = png.Decode(bytes.NewReader(res.GrubPng))
		if err != nil {
			panic(err)
		}
		grubsprite = image.NewRGBA(src.Bounds())
		draw.Draw(grubsprite, grubsprite.Rect, src, grubsprite.Rect.Min, draw.Src)
	})
}

func RenderThread(w wde.Window, repaint <-chan struct{}, man <-chan res.Man, state <-chan State, err <-chan error) {
	defer quitWait.Done()

	img := image.NewRGBA(w.Screen().Bounds())
	var m res.Man
	var s State
	var e error
	for {
		if img.Rect != w.Screen().Bounds() {
			img = image.NewRGBA(w.Screen().Bounds())
		}
		Render(img, m, &s, e)
		w.Screen().CopyRGBA(img, img.Rect)
		w.FlushImage(img.Rect)
		select {
		case m = <-man:
		case s = <-state:
		case e = <-err:
		case <-repaint:
		case <-quitRequest:
			return
		}
	}
}
