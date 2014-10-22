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
	"net"
	"time"
)

func ClientNet(addr string, read chan<- *res.Packet, write <-chan *res.Packet, errors chan<- error) {
	for {
		func() {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				time.Sleep(time.Second)
				return
			}
			defer conn.Close()

			readch, writech, errorsch := make(chan *res.Packet), make(chan *res.Packet), make(chan error, 2)
			defer close(writech)
			go Read(conn, readch, errorsch)
			go Write(conn, writech, errorsch)

			for {
				select {
				case p := <-write:
					writech <- p

				case p, ok := <-readch:
					if !ok {
						readch = nil
						continue
					}
					read <- p

				case err := <-errorsch:
					errors <- err
					return
				}
			}
		}()
	}
}

func Client(addr string) {
	defer wde.Stop()

	w, err := wde.NewWindow(800, 300)
	if err != nil {
		panic(err)
	}
	defer w.Close()

	w.Show()

	read, write, errors := make(chan *res.Packet), make(chan *res.Packet), make(chan error, 2)
	go ClientNet(addr, read, write, errors)

	var (
		me        res.Man
		state     State
		lastState []byte
		lastTick  uint64
		input     [res.Man_count]res.Packet
		inputch   = make(chan *res.Packet, 1)
		noState   = true
		world     *World
	)
	defer close(inputch)
	releaseAll := &res.Packet{
		Mouse1:   Button_released,
		Mouse2:   Button_released,
		KeyUp:    Button_released,
		KeyDown:  Button_released,
		KeyLeft:  Button_released,
		KeyRight: Button_released,
	}

	sendInput := func(p *res.Packet) {
		inputch <- p
	}
	go func() {
		var p *res.Packet

		for {
			out := write
			if p == nil {
				out = nil
			}

			select {
			case v, ok := <-inputch:
				if !ok {
					return
				}

				if p == nil {
					p = &res.Packet{
						Type: Type_Input,
					}
				}
				if v == nil {
					proto.Merge(p, releaseAll)
				} else {
					proto.Merge(p, v)
				}

			case out <- p:
				p = nil
			}
		}
	}()

	renderResize, renderMan, renderState, renderError := make(chan struct{}, 1), make(chan res.Man, 1), make(chan State, 1), make(chan error, 1)
	go RenderThread(w, renderResize, renderMan, renderState, renderError)

	for {
		select {
		case err := <-errors:
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

			case res.Type_Input:
				proto.Merge(&input[p.GetMan()], p)

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
				return
			case wde.ResizeEvent:
				select {
				case renderResize <- struct{}{}:
				default:
				}
			case wde.KeyDownEvent:
				switch e.Key {
				case wde.KeyW, wde.KeyPadUp, wde.KeyUpArrow, wde.KeySpace:
					sendInput(&res.Packet{
						KeyUp: Button_pressed,
					})
				case wde.KeyS, wde.KeyPadDown, wde.KeyDownArrow:
					sendInput(&res.Packet{
						KeyDown: Button_pressed,
					})
				case wde.KeyA, wde.KeyPadLeft, wde.KeyLeftArrow:
					sendInput(&res.Packet{
						KeyLeft: Button_pressed,
					})
				case wde.KeyD, wde.KeyPadRight, wde.KeyRightArrow:
					sendInput(&res.Packet{
						KeyRight: Button_pressed,
					})
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
					sendInput(&res.Packet{
						KeyUp: Button_released,
					})
				case wde.KeyS, wde.KeyPadDown, wde.KeyDownArrow:
					sendInput(&res.Packet{
						KeyDown: Button_released,
					})
				case wde.KeyA, wde.KeyPadLeft, wde.KeyLeftArrow:
					sendInput(&res.Packet{
						KeyLeft: Button_released,
					})
				case wde.KeyD, wde.KeyPadRight, wde.KeyRightArrow:
					sendInput(&res.Packet{
						KeyRight: Button_released,
					})
				}
			case wde.MouseDownEvent:
				switch e.Which {
				case wde.LeftButton:
					sendInput(&res.Packet{
						Mouse1: Button_pressed,
					})
				case wde.RightButton:
					sendInput(&res.Packet{
						Mouse2: Button_pressed,
					})
				}
			case wde.MouseUpEvent:
				switch e.Which {
				case wde.LeftButton:
					sendInput(&res.Packet{
						Mouse1: Button_released,
					})
				case wde.RightButton:
					sendInput(&res.Packet{
						Mouse2: Button_released,
					})
				}
			case wde.MouseEnteredEvent:
				// TODO
			case wde.MouseExitedEvent:
				sendInput(nil)
			case wde.MouseMovedEvent:
				width, height := w.Size()
				sendInput(&res.Packet{
					X: proto.Int64(int64(e.Where.X - width/2)),
					Y: proto.Int64(int64(e.Where.Y - height/2)),
				})
			case wde.MouseDraggedEvent:
				width, height := w.Size()
				sendInput(&res.Packet{
					X: proto.Int64(int64(e.Where.X - width/2)),
					Y: proto.Int64(int64(e.Where.Y - height/2)),
				})
			default:
				panic(fmt.Errorf("unexpected event type %T in %#v", event, event))
			}
		}
	}
}

var (
	mansprites    [res.Man_count][2]*image.RGBA
	mancolors     [res.Man_count]*image.Uniform
	terrain       []*image.RGBA
	tilemask      [1 << 10]*image.Alpha
	fade          [VelocityClones + 1]*image.Uniform
	offscreenfade *image.Uniform
	deadhaze      *image.Uniform
	parallax      [2]*image.RGBA
	lemonsprite   *image.RGBA
)

func init() {
	const FontStyleBoldItalic = draw2d.FontStyleBold | draw2d.FontStyleItalic
	for d, b := range map[draw2d.FontData][]byte{
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono, Style: draw2d.FontStyleBold}:    res.LuximbTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono, Style: FontStyleBoldItalic}:     res.LuximbiTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono, Style: draw2d.FontStyleNormal}:  res.LuximrTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilyMono, Style: draw2d.FontStyleItalic}:  res.LuximriTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilySerif, Style: draw2d.FontStyleBold}:   res.LuxirbTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilySerif, Style: FontStyleBoldItalic}:    res.LuxirbiTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilySerif, Style: draw2d.FontStyleNormal}: res.LuxirrTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilySerif, Style: draw2d.FontStyleItalic}: res.LuxirriTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilySans, Style: draw2d.FontStyleBold}:    res.LuxisbTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilySans, Style: FontStyleBoldItalic}:     res.LuxisbiTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilySans, Style: draw2d.FontStyleNormal}:  res.LuxisrTtf,
		draw2d.FontData{Name: "luxi", Family: draw2d.FontFamilySans, Style: draw2d.FontStyleItalic}:  res.LuxisriTtf,
	} {
		font, err := truetype.Parse(b)
		if err != nil {
			panic(err)
		}
		draw2d.RegisterFont(d, font)
	}

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
	mancolors[res.Man_Whip] = image.NewUniform(color.RGBA{192, 0, 0, 255})
	mancolors[res.Man_Density] = image.NewUniform(color.RGBA{192, 192, 0, 255})
	mancolors[res.Man_Vacuum] = image.NewUniform(color.RGBA{0, 192, 0, 255})
	mancolors[res.Man_Normal] = image.NewUniform(color.RGBA{0, 0, 192, 255})

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
}

func RenderThread(w wde.Window, repaint <-chan struct{}, man <-chan res.Man, state <-chan State, err <-chan error) {
	var m res.Man
	var s State
	var e error
	for {
		Render(w, m, s, e)
		select {
		case m = <-man:
		case s = <-state:
		case e = <-err:
		case <-repaint:
		}
	}
}

func Render(w wde.Window, me res.Man, state State, err error) {
	img := image.NewRGBA(w.Screen().Bounds())
	gc := draw2d.NewGraphicContext(img)

	draw.Draw(img, img.Rect, image.White, image.ZP, draw.Src)

	if state.world == nil || state.Mans[0].UnitData == nil {
		gc.SetFillColor(color.White)
		gc.SetStrokeColor(color.Black)
		gc.SetLineWidth(2)
		const text = "Connecting..."
		left, top, right, bottom := gc.GetStringBounds(text)
		x := float64(img.Rect.Min.X+img.Rect.Dx()/2) + left/2 - right/2
		y := float64(img.Rect.Min.Y+img.Rect.Dy()/2) + top/2 - bottom/2
		gc.StrokeStringAt(text, x, y)
		gc.FillStringAt(text, x, y)

		if err != nil {
			str := err.Error()
			left, _, _, bottom = gc.GetStringBounds(str)
			x, y = float64(img.Rect.Min.X)-left+2, float64(img.Rect.Max.Y)-bottom-2
			gc.StrokeStringAt(str, x, y)
			gc.FillStringAt(str, x, y)
		}

		w.Screen().CopyRGBA(img, img.Rect)
		w.FlushImage(img.Rect)
		return
	}

	offX := int64(img.Rect.Dx()/2) - state.Mans[me].Position.X/PixelSize
	offY := int64(img.Rect.Dy()/2) - state.Mans[me].Position.Y/PixelSize

	for i, p := range parallax {
		for x := img.Rect.Min.X - (int((-offX*int64(1+i)/int64(1+len(parallax)))%int64(p.Rect.Dx()))+p.Rect.Dx())%p.Rect.Dx(); x < img.Rect.Max.X; x += p.Rect.Dx() {
			draw.Draw(img, image.Rect(x, img.Rect.Max.Y-p.Rect.Dy(), img.Rect.Max.X, img.Rect.Max.Y), p, p.Rect.Min, draw.Over)
		}
	}

	state.world.Render(img, offX, offY)

	for i := int64(VelocityClones); i >= 0; i-- {
		state.EachUnit(func(u *Unit) {
			pos := u.Position
			pos.X -= u.Velocity.X * i / TicksPerSecond / VelocityClones
			pos.Y -= u.Velocity.Y * i / TicksPerSecond / VelocityClones

			sprite := u.Sprite(&state, u)
			r := sprite.Rect.Sub(sprite.Rect.Min).Add(image.Point{
				X: int(pos.X/PixelSize+offX) - sprite.Rect.Dx()/2,
				Y: int(pos.Y/PixelSize+offY) - sprite.Rect.Dy(),
			})

			if u.Health <= 0 {
				if m, ok := u.UnitData.(Man); ok {
					r.Min.Y = r.Max.Y - int(m.Respawn()-state.Tick)*r.Dy()/RespawnTime
				} else {
					return
				}
			}

			draw.DrawMask(img, r, sprite, sprite.Rect.Min, fade[i], image.ZP, draw.Over)

			if i == 0 {
				if u.Health > 0 {
					gc.SetStrokeColor(color.RGBA{255, 0, 0, 255})
					gc.SetLineWidth(2)
					gc.MoveTo(float64(pos.X/PixelSize-u.Health*int64(r.Dx())/2/DefaultHealth+offX), float64(pos.Y/PixelSize+1+offY))
					gc.LineTo(float64(pos.X/PixelSize+u.Health*int64(r.Dx())/2/DefaultHealth+offX), float64(pos.Y/PixelSize+1+offY))
					gc.Stroke()
				}

				if m, ok := u.UnitData.(Man); ok {
					if r.Intersect(img.Rect).Empty() {
						if r.Min.X < img.Rect.Min.X {
							r = r.Add(image.Pt(img.Rect.Min.X-r.Min.X, 0))
						}
						if r.Max.X > img.Rect.Max.X {
							r = r.Add(image.Pt(img.Rect.Max.X-r.Max.X, 0))
						}
						if r.Min.Y < img.Rect.Min.Y {
							r = r.Add(image.Pt(0, img.Rect.Min.Y-r.Min.Y))
						}
						if r.Max.Y > img.Rect.Max.Y {
							r = r.Add(image.Pt(0, img.Rect.Max.Y-r.Max.Y))
						}

						draw.DrawMask(img, r, sprite, sprite.Rect.Min, offscreenfade, image.ZP, draw.Over)
					}

					target := m.Target()
					draw.Draw(img, image.Rect(-1, -1, 1, 1).Add(image.Point{
						X: int(target.X/PixelSize + offX),
						Y: int(target.Y/PixelSize + offY),
					}), mancolors[m.Man()], image.ZP, draw.Over)

					switch mm := m.(type) {
					case *WhipMan:
						if mm.WhipStop != 0 && !mm.WhipEnd.Zero() {
							gc.SetStrokeColor(color.Black)
							gc.SetLineWidth(0.5)
							gc.MoveTo(float64(pos.X/PixelSize+offX), float64(pos.Y/PixelSize-int64(r.Dy()/2)+offY))
							gc.LineTo(float64(mm.WhipEnd.X/PixelSize+offX), float64(mm.WhipEnd.Y/PixelSize+offY))
							gc.Stroke()
						}
						if mm.WhipStop == 0 && !mm.WhipTether.Zero() {
							gc.SetStrokeColor(color.Black)
							gc.SetLineWidth(0.5)
							gc.MoveTo(float64(pos.X/PixelSize+offX), float64(pos.Y/PixelSize-int64(r.Dy()/2)+offY))
							gc.LineTo(float64(mm.WhipTether.X/PixelSize+offX), float64(mm.WhipTether.Y/PixelSize+offY))
							gc.Stroke()
						}
					}
				}
			}
		})
	}

	for _, f := range state.Floaters {
		t := state.Tick - f.T
		fg, bg := f.Fg, f.Bg
		gc.SetLineWidth(2)
		left, top, right, bottom := gc.GetStringBounds(f.S)
		x := float64(f.X/PixelSize+offX) + left/2 - right/2
		y := float64(f.Y/PixelSize+offY) + top/2 - bottom/2 - float64(t)
		if t > FloaterFadeStart {
			fg.R -= uint8(uint64(fg.R) * (t - FloaterFadeStart) / (FloaterFadeEnd - FloaterFadeStart))
			fg.G -= uint8(uint64(fg.G) * (t - FloaterFadeStart) / (FloaterFadeEnd - FloaterFadeStart))
			fg.B -= uint8(uint64(fg.B) * (t - FloaterFadeStart) / (FloaterFadeEnd - FloaterFadeStart))
			fg.A -= uint8(uint64(fg.A) * (t - FloaterFadeStart) / (FloaterFadeEnd - FloaterFadeStart))
			bg.R -= uint8(uint64(bg.R) * (t - FloaterFadeStart) / (FloaterFadeEnd - FloaterFadeStart))
			bg.G -= uint8(uint64(bg.G) * (t - FloaterFadeStart) / (FloaterFadeEnd - FloaterFadeStart))
			bg.B -= uint8(uint64(bg.B) * (t - FloaterFadeStart) / (FloaterFadeEnd - FloaterFadeStart))
			bg.A -= uint8(uint64(bg.A) * (t - FloaterFadeStart) / (FloaterFadeEnd - FloaterFadeStart))
		}
		gc.SetFillColor(fg)
		gc.SetStrokeColor(bg)
		gc.StrokeStringAt(f.S, x, y)
		gc.FillStringAt(f.S, x, y)
	}

	if state.Mans[me].UnitData.(Man).Respawn() != 0 {
		draw.Draw(img, img.Rect, deadhaze, image.ZP, draw.Over)
	}

	gc.SetFillColor(color.White)
	gc.SetStrokeColor(color.Black)
	gc.SetLineWidth(2)
	var lives string
	if state.Lives > 1 {
		lives = fmt.Sprintf("%d Mans", state.Lives)
	} else if state.Lives == 1 {
		lives = "1 Man!"
	} else {
		lives = "No Mans!!"
	}
	left, top, _, _ := gc.GetStringBounds(lives)
	gc.StrokeStringAt(lives, 2-left, 2-top)
	gc.FillStringAt(lives, 2-left, 2-top)

	w.Screen().CopyRGBA(img, img.Rect)
	w.FlushImage(img.Rect)
}
