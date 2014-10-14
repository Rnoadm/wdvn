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
	"time"
)

func Client(conn net.Conn, in <-chan *res.Packet, out chan<- *res.Packet) {
	defer conn.Close()

	write := make(chan *res.Packet)
	go Read(conn, out)
	go Write(conn, write)

	for {
		select {
		case p := <-in:
			write <- p
		}
	}
}

type State struct {
	Me   res.Man
	Mans [res.Man_count]struct {
		X, Y int64
	}
}

func GUI(in <-chan *res.Packet, out chan<- *res.Packet) {
	defer wde.Stop()

	w, err := wde.NewWindow(800, 300)
	if err != nil {
		panic(err)
	}

	w.Show()

	var state State
	var mouse *image.Point

	tick := time.Tick(time.Second / 100)

	for {
		select {
		case <-tick:
			if mouse != nil {
				width, height := w.Size()
				x, y := int64(mouse.X-width/2), int64(mouse.Y-height/2)
				x += state.Mans[state.Me].X
				y += state.Mans[state.Me].Y
				out <- &res.Packet{
					Type: res.Type_MoveMan.Enum(),
					X:    proto.Int64(x),
					Y:    proto.Int64(y),
				}
			}

		case <-repaintch:
			Render(w, state)

		case p := <-in:
			switch p.GetType() {
			case res.Type_SelectMan:
				state.Me = p.GetMan()

			case res.Type_MoveMan:
				state.Mans[p.GetMan()].X = p.GetX()
				state.Mans[p.GetMan()].Y = p.GetY()
			}
			Repaint()

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
				mouse = nil
			case wde.MouseMovedEvent:
				mouse = &e.Where
			case wde.MouseDraggedEvent:
				mouse = &e.Where
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

	offX := int64(img.Rect.Dx()/2-sprites[0].Rect.Dx()/2) - state.Mans[state.Me].X/256
	offY := int64(img.Rect.Dy()/2-sprites[0].Rect.Dy()/2) - state.Mans[state.Me].Y/256

	for i := res.Man(0); i < res.Man_count; i++ {
		draw.Draw(img, sprites[i].Rect.Add(image.Pt(int(state.Mans[i].X/256+offX), int(state.Mans[i].Y/256+offY))), sprites[i], image.ZP, draw.Over)
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
