package main

import (
	"bytes"
	"code.google.com/p/draw2d/draw2d"
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
		Mouse1:   res.Button_released.Enum(),
		Mouse2:   res.Button_released.Enum(),
		KeyUp:    res.Button_released.Enum(),
		KeyDown:  res.Button_released.Enum(),
		KeyLeft:  res.Button_released.Enum(),
		KeyRight: res.Button_released.Enum(),
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
						Type: res.Type_Input.Enum(),
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
							Type: res.Type_FullState.Enum(),
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
							}
						}
						if err != nil {
							go Send(write, &res.Packet{
								Type: res.Type_FullState.Enum(),
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
						KeyUp: res.Button_pressed.Enum(),
					})
				case wde.KeyS, wde.KeyPadDown, wde.KeyDownArrow:
					sendInput(&res.Packet{
						KeyDown: res.Button_pressed.Enum(),
					})
				case wde.KeyA, wde.KeyPadLeft, wde.KeyLeftArrow:
					sendInput(&res.Packet{
						KeyLeft: res.Button_pressed.Enum(),
					})
				case wde.KeyD, wde.KeyPadRight, wde.KeyRightArrow:
					sendInput(&res.Packet{
						KeyRight: res.Button_pressed.Enum(),
					})
				case wde.KeyF1:
					go Send(write, &res.Packet{
						Type: res.Type_SelectMan.Enum(),
						Man:  res.Man_Whip.Enum(),
					})
				case wde.KeyF2:
					go Send(write, &res.Packet{
						Type: res.Type_SelectMan.Enum(),
						Man:  res.Man_Density.Enum(),
					})
				case wde.KeyF3:
					go Send(write, &res.Packet{
						Type: res.Type_SelectMan.Enum(),
						Man:  res.Man_Vacuum.Enum(),
					})
				case wde.KeyF4:
					go Send(write, &res.Packet{
						Type: res.Type_SelectMan.Enum(),
						Man:  res.Man_Normal.Enum(),
					})
				}
			case wde.KeyTypedEvent:
				// TODO
			case wde.KeyUpEvent:
				switch e.Key {
				case wde.KeyW, wde.KeyPadUp, wde.KeyUpArrow, wde.KeySpace:
					sendInput(&res.Packet{
						KeyUp: res.Button_released.Enum(),
					})
				case wde.KeyS, wde.KeyPadDown, wde.KeyDownArrow:
					sendInput(&res.Packet{
						KeyDown: res.Button_released.Enum(),
					})
				case wde.KeyA, wde.KeyPadLeft, wde.KeyLeftArrow:
					sendInput(&res.Packet{
						KeyLeft: res.Button_released.Enum(),
					})
				case wde.KeyD, wde.KeyPadRight, wde.KeyRightArrow:
					sendInput(&res.Packet{
						KeyRight: res.Button_released.Enum(),
					})
				}
			case wde.MouseDownEvent:
				switch e.Which {
				case wde.LeftButton:
					sendInput(&res.Packet{
						Mouse1: res.Button_pressed.Enum(),
					})
				case wde.RightButton:
					sendInput(&res.Packet{
						Mouse2: res.Button_pressed.Enum(),
					})
				}
			case wde.MouseUpEvent:
				switch e.Which {
				case wde.LeftButton:
					sendInput(&res.Packet{
						Mouse1: res.Button_released.Enum(),
					})
				case wde.RightButton:
					sendInput(&res.Packet{
						Mouse2: res.Button_released.Enum(),
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

var sprites [res.Man_count]*image.RGBA
var terrain []*image.RGBA

func init() {
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
	for x := dst.Rect.Min.X; x < dst.Rect.Max.X; x += dst.Rect.Dy() {
		terrain = append(terrain, dst.SubImage(image.Rect(x, dst.Rect.Min.Y, x+dst.Rect.Dy(), dst.Rect.Max.Y)).(*image.RGBA))
	}
}

func Render(w wde.Window, me res.Man, state State) {
	if state.World == nil {
		return
	}

	img := image.NewRGBA(w.Screen().Bounds())
	gc := draw2d.NewGraphicContext(img)

	offX := int64(img.Rect.Dx()/2) - state.Mans[me].Position.X/PixelSize
	offY := int64(img.Rect.Dy()/2) - state.Mans[me].Position.Y/PixelSize

	size := int64(terrain[0].Rect.Dy())

	min, max := Coord{-size, -size}, Coord{int64(img.Rect.Dx()) + size, int64(img.Rect.Dy()) + size}
	min = min.Sub(Coord{offX, offY}).Floor(size)
	max = max.Sub(Coord{offX, offY}).Floor(size)

	for x := min.X; x < max.X; x += size {
		for y := min.Y; y < max.Y; y += size {
			t := terrain[state.World.Tile(x/size, y/size)]
			r := image.Rect(int(x+offX), int(y+offY), int(x+offX+size), int(y+offY+size))
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
			draw.DrawMask(img, r, sprites[i], sprites[i].Rect.Min, image.NewUniform(color.Alpha16{uint16(0xffff * (VelocityClones + 1 - j) / (VelocityClones + 1))}), image.ZP, draw.Over)

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

					draw.DrawMask(img, r, sprites[i], sprites[i].Rect.Min, image.NewUniform(color.Alpha{0x40}), image.ZP, draw.Over)
				}

				target := state.Mans[i].Target
				draw.Draw(img, image.Rect(0, 0, 1, 1).Add(image.Point{
					X: int(target.X/PixelSize + offX),
					Y: int(target.Y/PixelSize + offY),
				}), sprites[i], sprites[i].Rect.Min, draw.Over)

				switch res.Man(i) {
				case res.Man_Whip:
					if state.WhipStop != 0 && state.WhipEnd != (Coord{}) {
						gc.SetStrokeColor(color.Black)
						gc.MoveTo(float64(pos.X/PixelSize+offX), float64(pos.Y/PixelSize+offY))
						gc.LineTo(float64(state.WhipEnd.X/PixelSize+offX), float64(state.WhipEnd.Y/PixelSize+offY))
						gc.Stroke()
					}
				}
			}
		}
	}

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
