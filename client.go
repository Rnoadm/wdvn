package main

import (
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	"github.com/Rnoadm/wdvn/res"
	"github.com/skelterjohn/go.wde"
	"image"
	"image/color"
	"image/draw"
	"net"
)

type State struct {
	Me   res.Man
	Mans [res.Man_count]struct {
		X, Y int64
	}
}

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

	var state State
	mouse := make(chan *image.Point, 1)
	defer close(mouse)
	go func() {
		var p *res.Packet
		for {
			out := write
			if p == nil {
				out = nil
			}

			select {
			case m, ok := <-mouse:
				if !ok {
					return
				}

				if m == nil {
					p = &res.Packet{
						Type: res.Type_Mouse.Enum(),
					}
				} else {
					width, height := w.Size()
					p = &res.Packet{
						Type: res.Type_Mouse.Enum(),
						X:    proto.Int64(int64(m.X - width/2)),
						Y:    proto.Int64(int64(m.Y - height/2)),
					}
				}

			case out <- p:
				p = nil
			}
		}
	}()

	for {
		select {
		case <-repaintch:
			Render(w, state)

		case p, ok := <-read:
			if !ok {
				return
			}

			switch p.GetType() {
			case res.Type_Ping:
				go Send(write, p)

			case res.Type_SelectMan:
				state.Me = p.GetMan()
				Repaint()

			case res.Type_MoveMan:
				state.Mans[p.GetMan()].X = p.GetX()
				state.Mans[p.GetMan()].Y = p.GetY()
				Repaint()
			}

		case event := <-w.EventChan():
			switch e := event.(type) {
			case wde.CloseEvent:
				return
			case wde.ResizeEvent:
				Repaint()
			case wde.KeyDownEvent:
				// TODO
			case wde.KeyTypedEvent:
				// TODO
			case wde.KeyUpEvent:
				// TODO
			case wde.MouseDownEvent:
				// TODO
			case wde.MouseUpEvent:
				// TODO
			case wde.MouseEnteredEvent:
				// TODO
			case wde.MouseExitedEvent:
				mouse <- nil
			case wde.MouseMovedEvent:
				mouse <- &e.Where
			case wde.MouseDraggedEvent:
				mouse <- &e.Where
			default:
				panic(fmt.Errorf("unexpected event type %T in %#v", event, event))
			}
		}
	}
}

var sprites [res.Man_count]*image.RGBA

func init() {
	r := image.Rect(0, 0, 16, 16)
	for i, c := range []color.RGBA{
		res.Man_Whip:    {255, 0, 0, 255},
		res.Man_Density: {255, 255, 0, 255},
		res.Man_Vacuum:  {0, 255, 0, 255},
		res.Man_Normal:  {0, 0, 255, 255},
	} {
		sprites[i] = image.NewRGBA(r)
		draw.Draw(sprites[i], r, image.NewUniform(c), image.ZP, draw.Src)
	}
}

func Render(w wde.Window, state State) {
	img := image.NewRGBA(w.Screen().Bounds())

	offX := int64(img.Rect.Dx()/2-sprites[0].Rect.Dx()/2) - state.Mans[state.Me].X/64
	offY := int64(img.Rect.Dy()/2-sprites[0].Rect.Dy()/2) - state.Mans[state.Me].Y/64

	for i := res.Man(0); i < res.Man_count; i++ {
		draw.Draw(img, sprites[i].Rect.Add(image.Pt(int(state.Mans[i].X/64+offX), int(state.Mans[i].Y/64+offY))), sprites[i], image.ZP, draw.Over)
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
