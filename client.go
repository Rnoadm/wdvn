package main

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"fmt"
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

	var me res.Man
	var state State
	input := make(chan *res.Packet, 1)
	defer close(input)
	go func() {
		releaseAll := &res.Packet{
			Mouse1:   res.Button_released.Enum(),
			Mouse2:   res.Button_released.Enum(),
			KeyUp:    res.Button_released.Enum(),
			KeyDown:  res.Button_released.Enum(),
			KeyLeft:  res.Button_released.Enum(),
			KeyRight: res.Button_released.Enum(),
		}

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

			case res.Type_MoveMan:
				state.Mans[p.GetMan()].Position.X = p.GetX()
				state.Mans[p.GetMan()].Position.Y = p.GetY()
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
				case wde.KeyW, wde.KeyPadUp, wde.KeyUpArrow:
					input <- &res.Packet{
						KeyUp: res.Button_pressed.Enum(),
					}
				case wde.KeyS, wde.KeyPadDown, wde.KeyDownArrow:
					input <- &res.Packet{
						KeyDown: res.Button_pressed.Enum(),
					}
				case wde.KeyA, wde.KeyPadLeft, wde.KeyLeftArrow:
					input <- &res.Packet{
						KeyLeft: res.Button_pressed.Enum(),
					}
				case wde.KeyD, wde.KeyPadRight, wde.KeyRightArrow:
					input <- &res.Packet{
						KeyRight: res.Button_pressed.Enum(),
					}
				}
				// TODO
			case wde.KeyTypedEvent:
				// TODO
			case wde.KeyUpEvent:
				switch e.Key {
				case wde.KeyW, wde.KeyPadUp, wde.KeyUpArrow:
					input <- &res.Packet{
						KeyUp: res.Button_released.Enum(),
					}
				case wde.KeyS, wde.KeyPadDown, wde.KeyDownArrow:
					input <- &res.Packet{
						KeyDown: res.Button_released.Enum(),
					}
				case wde.KeyA, wde.KeyPadLeft, wde.KeyLeftArrow:
					input <- &res.Packet{
						KeyLeft: res.Button_released.Enum(),
					}
				case wde.KeyD, wde.KeyPadRight, wde.KeyRightArrow:
					input <- &res.Packet{
						KeyRight: res.Button_released.Enum(),
					}
				}
				// TODO
			case wde.MouseDownEvent:
				switch e.Which {
				case wde.LeftButton:
					input <- &res.Packet{
						Mouse1: res.Button_pressed.Enum(),
					}
				case wde.RightButton:
					input <- &res.Packet{
						Mouse2: res.Button_pressed.Enum(),
					}
				}
				// TODO
			case wde.MouseUpEvent:
				switch e.Which {
				case wde.LeftButton:
					input <- &res.Packet{
						Mouse1: res.Button_released.Enum(),
					}
				case wde.RightButton:
					input <- &res.Packet{
						Mouse2: res.Button_released.Enum(),
					}
				}
				// TODO
			case wde.MouseEnteredEvent:
				// TODO
			case wde.MouseExitedEvent:
				input <- nil
			case wde.MouseMovedEvent:
				width, height := w.Size()
				input <- &res.Packet{
					X: proto.Int64(int64(e.Where.X - width/2)),
					Y: proto.Int64(int64(e.Where.Y - height/2)),
				}
			case wde.MouseDraggedEvent:
				width, height := w.Size()
				input <- &res.Packet{
					X: proto.Int64(int64(e.Where.X - width/2)),
					Y: proto.Int64(int64(e.Where.Y - height/2)),
				}
			default:
				panic(fmt.Errorf("unexpected event type %T in %#v", event, event))
			}
		}
	}
}

var sprites [res.Man_count]*image.RGBA

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
}

var ground = image.NewUniform(color.RGBA{0, 0, 0, 255})
var sky = image.NewUniform(color.RGBA{255, 255, 255, 255})

func Render(w wde.Window, me res.Man, state State) {
	img := image.NewRGBA(w.Screen().Bounds())

	offX := int64(img.Rect.Dx()/2-sprites[me].Rect.Dx()/2) - state.Mans[me].Position.X/PixelSize
	offY := int64(img.Rect.Dy()/2+sprites[me].Rect.Dy()/2) - state.Mans[me].Position.Y/PixelSize

	draw.Draw(img, image.Rect(img.Rect.Min.X, img.Rect.Min.Y, img.Rect.Max.X, int(offY)), sky, image.ZP, draw.Src)
	draw.Draw(img, image.Rect(img.Rect.Min.X, int(offY), img.Rect.Max.X, img.Rect.Max.Y), ground, image.ZP, draw.Src)

	for i := range state.Mans {
		draw.Draw(img, sprites[i].Rect.Sub(sprites[i].Rect.Min).Add(image.Point{
			X: int(state.Mans[i].Position.X/PixelSize + offX),
			Y: int(state.Mans[i].Position.Y/PixelSize+offY) - sprites[i].Rect.Dy(),
		}), sprites[i], sprites[i].Rect.Min, draw.Over)
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
