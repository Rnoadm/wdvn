package main

import (
	"code.google.com/p/draw2d/draw2d"
	"fmt"
	"github.com/Rnoadm/wdvn/res"
	"image"
	"image/color"
	"image/draw"
	"sync"
	"time"
)

func Render(img *image.RGBA, me res.Man, state *State, err error) {
	hx, hy := (img.Rect.Min.X+img.Rect.Max.X)/2, (img.Rect.Min.Y+img.Rect.Max.Y)/2

	draw.Draw(img, img.Rect, image.White, image.ZP, draw.Src)

	if state.world == nil || state.Mans[0].UnitData == nil {
		RenderText(img, "Connecting...", image.Pt(hx, hy), color.Black, color.White, true)

		if err != nil {
			RenderText(img, "Last error: "+err.Error(), image.Pt(img.Rect.Min.X+2, img.Rect.Max.Y-6), color.Black, color.White, false)
		}

		return
	}

	if *flagSplitScreen {
		for i := range state.Mans {
			r := img.Rect
			if i&1 == 0 {
				r.Max.Y = hy
			} else {
				r.Min.Y = hy
			}
			if i&2 == 0 {
				r.Max.X = hx
			} else {
				r.Min.X = hx
			}
			render(img.SubImage(r).(*image.RGBA), res.Man(i), state)
		}
		draw.Draw(img, image.Rect(hx, img.Rect.Min.Y, hx+1, img.Rect.Max.Y), image.Black, image.ZP, draw.Src)
		draw.Draw(img, image.Rect(img.Rect.Min.X, hy, img.Rect.Max.X, hy+1), image.Black, image.ZP, draw.Src)
	} else {
		render(img, me, state)
	}

	for i := range state.Mans {
		m := state.Mans[i].UnitData.(Man)
		x, y := 2, 12
		if i&1 == 1 {
			y = img.Rect.Dy() - 24
		}
		if i&2 == 2 {
			x = img.Rect.Dx() - 112
		}
		if state.Mans[i].Health > 0 {
			draw.Draw(img, image.Rect(x, y-4, x+int(state.Mans[i].Health*110/ManHealth), y), manfills[i], image.ZP, draw.Src)
		} else if m.Respawn() != 0 && m.Lives() > 0 {
			RenderText(img, fmt.Sprintf("Respawn in %s", time.Duration(m.Respawn()-state.Tick)*time.Second/TicksPerSecond), image.Pt(x, y), color.White, mancolors[i], false)
		}
		var lives string
		if l := m.Lives(); l > 1 {
			lives = fmt.Sprintf("%d Mans", l)
		} else if l == 1 {
			lives = "1 Man!"
		} else {
			lives = "No Mans!!"
		}
		RenderText(img, lives, image.Pt(x, y+14), color.White, mancolors[i], false)
	}
}

func render(img *image.RGBA, me res.Man, state *State) {
	img.Rect = img.Rect.Sub(img.Rect.Min)

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

			sprite := u.Sprite(state, u)
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

			if m, ok := u.UnitData.(Man); i == 0 && ok {
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
				}), manfills[m.Man()], image.ZP, draw.Over)

				switch mm := m.(type) {
				case *WhipMan:
					if mm.WhipStop != 0 && !mm.WhipEnd.Zero() {
						gc := draw2d.NewGraphicContext(img)
						gc.SetStrokeColor(color.Black)
						gc.SetLineWidth(0.5)
						gc.MoveTo(float64(pos.X/PixelSize+offX), float64(pos.Y/PixelSize-int64(r.Dy()/2)+offY))
						gc.LineTo(float64(mm.WhipEnd.X/PixelSize+offX), float64(mm.WhipEnd.Y/PixelSize+offY))
						gc.Stroke()
					}
					if mm.WhipStop == 0 && !mm.WhipTether.Zero() {
						gc := draw2d.NewGraphicContext(img)
						gc.SetStrokeColor(color.Black)
						gc.SetLineWidth(0.5)
						gc.MoveTo(float64(pos.X/PixelSize+offX), float64(pos.Y/PixelSize-int64(r.Dy()/2)+offY))
						gc.LineTo(float64(mm.WhipTether.X/PixelSize+offX), float64(mm.WhipTether.Y/PixelSize+offY))
						gc.Stroke()
					}
				}
			}
		})
	}

	for _, f := range state.Floaters {
		t, fg, bg := state.Tick-f.T, f.Fg, f.Bg
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
		RenderText(img, f.S, image.Pt(int(f.X/PixelSize+offX), int(f.Y/PixelSize+offY)-int(t)), fg, bg, true)
	}

	if state.Mans[me].UnitData.(Man).Respawn() != 0 {
		draw.Draw(img, img.Rect, deadhaze, image.ZP, draw.Over)
	}
}

type cachedText struct {
	Fg, Bg   *image.Alpha
	LastUsed time.Time
}

var (
	textCache   = make(map[string]*cachedText)
	textContext = draw2d.NewGraphicContext(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	textLock    sync.Mutex
)

func init() {
	go func() {
		for {
			time.Sleep(time.Minute)

			textLock.Lock()
			for text, cache := range textCache {
				if time.Since(cache.LastUsed) > time.Minute {
					delete(textCache, text)
				}
			}
			textLock.Unlock()
		}
	}()
}

func RenderText(dst *image.RGBA, text string, p image.Point, fg, bg color.Color, centered bool) {
	textLock.Lock()
	defer textLock.Unlock()

	cache, ok := textCache[text]
	if !ok {
		cache = new(cachedText)
		textCache[text] = cache

		left, top, right, bottom := textContext.GetStringBounds(text)
		canvas := image.NewRGBA(image.Rect(int(left)-3, int(top)-3, int(right)+3, int(bottom)+3))
		cache.Fg, cache.Bg = image.NewAlpha(canvas.Rect), image.NewAlpha(canvas.Rect)

		min := canvas.Rect.Min
		canvas.Rect = canvas.Rect.Sub(min)

		gc := draw2d.NewGraphicContext(canvas)
		gc.SetLineWidth(2)

		gc.StrokeStringAt(text, float64(-min.X), float64(-min.Y))
		draw.Draw(cache.Bg, cache.Bg.Rect, canvas, image.ZP, draw.Src)

		draw.Draw(canvas, canvas.Rect, image.Transparent, image.ZP, draw.Src)

		gc.FillStringAt(text, float64(-min.X), float64(-min.Y))
		draw.Draw(cache.Fg, cache.Fg.Rect, canvas, image.ZP, draw.Src)
	}

	if centered {
		p.X -= (cache.Fg.Rect.Min.X + cache.Fg.Rect.Max.X) / 2
		p.Y -= (cache.Fg.Rect.Min.Y + cache.Fg.Rect.Max.Y) / 2
	}

	cache.LastUsed = time.Now()
	draw.DrawMask(dst, cache.Bg.Rect.Add(p), image.NewUniform(bg), image.ZP, cache.Bg, cache.Bg.Rect.Min, draw.Over)
	draw.DrawMask(dst, cache.Fg.Rect.Add(p), image.NewUniform(fg), image.ZP, cache.Fg, cache.Fg.Rect.Min, draw.Over)
}