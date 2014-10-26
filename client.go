package main

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"encoding/gob"
	"fmt"
	"github.com/BenLubar/bindiff"
	"github.com/Rnoadm/wdvn/res"
	"github.com/skelterjohn/go.wde"
	"image"
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

	var (
		read   = make(chan *res.Packet)
		write  = make(chan *res.Packet)
		errors = make(chan error, 2)
	)
	go Reconnect(addr, read, write, errors)
	defer Disconnect(read, write, errors)

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

	var (
		renderResize = make(chan struct{}, 1)
		renderMan    = make(chan res.Man, 1)
		renderState  = make(chan State, 1)
		renderError  = make(chan error, 1)
	)
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
