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
)

func Client(conn net.Conn) {
	defer conn.Close()

	read, write := make(chan *res.Packet), make(chan *res.Packet)
	go Read(conn, read)
	go Write(conn, write)

	defer wde.Stop()

	w, err := wde.NewWindow(800, 300)
	if err != nil {
		panic(err)
	}
	defer w.Close()

	w.Show()

	var (
		me        res.Man
		state     State
		lastState []byte
		lastTick  uint64
		input     [res.Man_count]res.Packet
		inputch   = make(chan *res.Packet, 1)
		noState   = true
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

	for {
		select {
		case <-repaintch:
			Render(w, me, state)

		case p, ok := <-read:
			if !ok {
				return
			}

			switch p.GetType() {
			case res.Type_Ping:
				go Send(write, p)

			case res.Type_SelectMan:
				me = p.GetMan()
				Repaint()

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
								lastTick = state.Tick
								Repaint()
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
				lastState, lastTick, noState = p.GetData(), state.Tick, false
				Repaint()
			}

		case event := <-w.EventChan():
			switch e := event.(type) {
			case wde.CloseEvent:
				return
			case wde.ResizeEvent:
				Repaint()
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
	sprites  [res.Man_count]*image.RGBA
	terrain  []*image.RGBA
	fade     [VelocityClones + 1]*image.Uniform
	deadfade *image.Uniform
	deadhaze *image.Uniform
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

	y := dst.Rect.Dy() / len(sprites)
	r := image.Rect(dst.Rect.Min.X, dst.Rect.Min.Y, dst.Rect.Max.X, dst.Rect.Min.Y+y)

	for i := range sprites {
		sprites[i] = dst.SubImage(r.Add(image.Pt(0, y*i))).(*image.RGBA)
	}

	src, err = png.Decode(bytes.NewReader(res.TilePng))
	if err != nil {
		panic(err)
	}
	dst = image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Rect, src, dst.Rect.Min, draw.Src)
	if dst.Rect.Dy() != TileSize {
		panic("tile size mismatch")
	}
	for x := dst.Rect.Min.X; x < dst.Rect.Max.X; x += dst.Rect.Dy() {
		terrain = append(terrain, dst.SubImage(image.Rect(x, dst.Rect.Min.Y, x+dst.Rect.Dy(), dst.Rect.Max.Y)).(*image.RGBA))
	}

	for i := range fade {
		fade[i] = image.NewUniform(color.Alpha16{uint16(0xffff * (len(fade) - i) / len(fade))})
	}

	deadfade = image.NewUniform(color.Alpha{0x40})
	deadhaze = image.NewUniform(color.RGBA{64, 64, 64, 64})
}

func Render(w wde.Window, me res.Man, state State) {
	if state.World == nil {
		return
	}

	img := image.NewRGBA(w.Screen().Bounds())
	gc := draw2d.NewGraphicContext(img)

	offX := int64(img.Rect.Dx()/2) - state.Mans[me].Position.X/PixelSize
	offY := int64(img.Rect.Dy()/2) - state.Mans[me].Position.Y/PixelSize

	min, max := Coord{-TileSize, -TileSize}, Coord{int64(img.Rect.Dx()) + TileSize, int64(img.Rect.Dy()) + TileSize}
	min = min.Sub(Coord{offX, offY}).Floor(TileSize)
	max = max.Sub(Coord{offX, offY}).Floor(TileSize)

	for x := min.X; x < max.X; x += TileSize {
		for y := min.Y; y < max.Y; y += TileSize {
			t := terrain[state.World.Tile(x/TileSize, y/TileSize)]
			r := image.Rect(int(x+offX), int(y+offY), int(x+offX+TileSize), int(y+offY+TileSize))
			draw.Draw(img, r, t, t.Rect.Min, draw.Src)
		}
	}

	for j := VelocityClones; j >= 0; j-- {
		for i := range state.Mans {
			pos := state.Mans[i].Position
			pos.X -= state.Mans[i].Velocity.X * int64(j) / TicksPerSecond
			pos.Y -= state.Mans[i].Velocity.Y * int64(j) / TicksPerSecond
			r := sprites[i].Rect.Sub(sprites[i].Rect.Min).Add(image.Point{
				X: int(pos.X/PixelSize+offX) - sprites[i].Rect.Dx()/2,
				Y: int(pos.Y/PixelSize+offY) - sprites[i].Rect.Dy()/2,
			})
			if state.Respawn[i] != 0 {
				r.Min.Y += r.Dy() - r.Dy()*int(state.Respawn[i]-state.Tick)/RespawnTime
			}
			draw.DrawMask(img, r, sprites[i], sprites[i].Rect.Min, fade[j], image.ZP, draw.Over)

			if j == 0 {
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

					draw.DrawMask(img, r, sprites[i], sprites[i].Rect.Min, deadfade, image.ZP, draw.Over)
				}

				target := state.Mans[i].Target
				draw.Draw(img, image.Rect(0, 0, 1, 1).Add(image.Point{
					X: int(target.X/PixelSize + offX),
					Y: int(target.Y/PixelSize + offY),
				}), sprites[i], sprites[i].Rect.Min, draw.Over)

				switch res.Man(i) {
				case res.Man_Whip:
					if state.WhipStop != 0 && !state.WhipEnd.Zero() {
						gc.SetStrokeColor(color.Black)
						gc.MoveTo(float64(pos.X/PixelSize+offX), float64(pos.Y/PixelSize+offY))
						gc.LineTo(float64(state.WhipEnd.X/PixelSize+offX), float64(state.WhipEnd.Y/PixelSize+offY))
						gc.Stroke()
					}
				}
			}
		}
	}

	if state.Respawn[me] != 0 {
		draw.Draw(img, img.Rect, deadhaze, image.ZP, draw.Over)
	}

	var lives string
	if state.Lives > 1 {
		lives = fmt.Sprintf("%d Mans", state.Lives)
	} else if state.Lives == 1 {
		lives = "1 Man!"
	} else {
		lives = "No Mans!!"
	}
	left, top, _, _ := gc.GetStringBounds(lives)
	gc.FillStringAt(lives, 2-left, 2-top)

	w.Screen().CopyRGBA(img, img.Rect)
	w.FlushImage(img.Rect)
}

var repaintch = make(chan struct{}, 1)

func Repaint() {
	select {
	case repaintch <- struct{}{}:
	default:
	}
}
