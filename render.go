package main

import (
	"code.google.com/p/draw2d/draw2d"
	"code.google.com/p/freetype-go/freetype/truetype"
	"fmt"
	"github.com/Rnoadm/wdvn/res"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"strings"
	"sync"
	"time"
)

func Render(img *image.RGBA, me res.Man, state *State, err error) {
	hx, hy := (img.Rect.Min.X+img.Rect.Max.X)/2, (img.Rect.Min.Y+img.Rect.Max.Y)/2

	draw.Draw(img, img.Rect, image.White, image.ZP, draw.Src)

	if state == nil || state.world == nil || state.Mans[0].UnitData == nil {
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
		x, y := 2, 12+4
		if i&1 == 1 {
			y = img.Rect.Dy() - 12*2 - 4
		}
		if i&2 == 2 {
			x = img.Rect.Dx() - 112
		}
		if state.Mans[i].Health > 0 {
			h := int(state.Mans[i].Health * 110 / ManHealth)
			draw.Draw(img, image.Rect(x, y-11, x+h, y-1), image.White, image.ZP, draw.Src)
			draw.Draw(img, image.Rect(x+1, y-10, x+h-1, y-2), manfills[i], image.ZP, draw.Src)
		} else if m.Respawn() != 0 && m.Lives() > 0 {
			RenderText(img, fmt.Sprintf("Respawn in %s", time.Duration(m.Respawn()-state.Tick)*time.Second/TicksPerSecond), image.Pt(x, y), color.White, ManData[i].Color, false)
		}
		var lives string
		if l := m.Lives(); l > 1 {
			lives = fmt.Sprintf("%d Mans", l)
		} else if l == 1 {
			lives = "1 Man!"
		} else if l == 0 {
			lives = "No Mans!!"
		} else {
			lives = fmt.Sprintf("%d Mans???", l)
		}
		RenderText(img, lives, image.Pt(x, y+12), color.White, ManData[i].Color, false)
		ping := "disconnected"
		if p := m.Ping(); p > 0 {
			if p > time.Millisecond {
				p -= p % time.Millisecond
			}
			ping = p.String()
		}
		RenderText(img, ping, image.Pt(x, y+12*2), color.White, ManData[i].Color, false)
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
			}
		})
	}

	{
		u := &state.Mans[res.Man_Whip]
		r := u.Sprite(state, u).Rect
		mm := u.UnitData.(*WhipMan)
		if mm.WhipStart != 0 && mm.WhipStop == 0 {
			velocity, whipEnd, _, collide, _ := mm.Whip(state, u)

			if !whipEnd.Zero() {
				gc := draw2d.NewGraphicContext(img)
				gc.SetStrokeColor(color.Black)
				gc.SetLineWidth(0.25)
				gc.MoveTo(float64(u.Position.X/PixelSize+offX), float64(u.Position.Y/PixelSize-int64(r.Dy()/2)+offY))
				gc.LineTo(float64(whipEnd.X/PixelSize+offX), float64(whipEnd.Y/PixelSize+offY))
				gc.Stroke()

				if mm.WhipPull {
					velocity = u.Velocity.Sub(velocity)
					sprite := u.Sprite(state, u)
					for j := int64(VelocityClones); j >= 0; j-- {
						pos := u.Position
						pos.X += velocity.X * j / TicksPerSecond / VelocityClones
						pos.Y += velocity.Y * j / TicksPerSecond / VelocityClones

						r := sprite.Rect.Sub(sprite.Rect.Min).Add(image.Point{
							X: int(pos.X/PixelSize+offX) - sprite.Rect.Dx()/2,
							Y: int(pos.Y/PixelSize+offY) - sprite.Rect.Dy(),
						})

						draw.DrawMask(img, r, sprite, sprite.Rect.Min, fade[j], image.ZP, draw.Over)
					}
				} else if collide != nil {
					velocity = velocity.Add(collide.Velocity)
					sprite := collide.Sprite(state, collide)
					for j := int64(VelocityClones); j >= 0; j-- {
						pos := collide.Position
						pos.X += velocity.X * j / TicksPerSecond / VelocityClones
						pos.Y += velocity.Y * j / TicksPerSecond / VelocityClones

						r := sprite.Rect.Sub(sprite.Rect.Min).Add(image.Point{
							X: int(pos.X/PixelSize+offX) - sprite.Rect.Dx()/2,
							Y: int(pos.Y/PixelSize+offY) - sprite.Rect.Dy(),
						})

						draw.DrawMask(img, r, sprite, sprite.Rect.Min, fade[j], image.ZP, draw.Over)
					}
				}
			}
		}
		if mm.WhipStop != 0 && !mm.WhipEnd.Zero() {
			gc := draw2d.NewGraphicContext(img)
			gc.SetStrokeColor(color.Black)
			gc.SetLineWidth(1)
			gc.MoveTo(float64(u.Position.X/PixelSize+offX), float64(u.Position.Y/PixelSize-int64(r.Dy()/2)+offY))
			gc.LineTo(float64(mm.WhipEnd.X/PixelSize+offX), float64(mm.WhipEnd.Y/PixelSize+offY))
			gc.Stroke()
		}
		if mm.WhipStop == 0 && !mm.WhipTether.Zero() {
			gc := draw2d.NewGraphicContext(img)
			gc.SetStrokeColor(color.Black)
			gc.SetLineWidth(1)
			gc.MoveTo(float64(u.Position.X/PixelSize+offX), float64(u.Position.Y/PixelSize-int64(r.Dy()/2)+offY))
			gc.LineTo(float64(mm.WhipTether.X/PixelSize+offX), float64(mm.WhipTether.Y/PixelSize+offY))
			gc.Stroke()
		}
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

var (
	graphicsOnce  sync.Once
	manspritessrc [res.Man_count][2]string = [...][2]string{
		res.Man_Whip:    {res.ManWhipPng, res.ManWhipCrouchPng},
		res.Man_Density: {res.ManDensityPng, res.ManDensityCrouchPng},
		res.Man_Vacuum:  {res.ManVacuumPng, res.ManVacuumCrouchPng},
		res.Man_Normal:  {res.ManNormalPng, res.ManNormalCrouchPng},
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

func readRGBA(s string) *image.RGBA {
	src, err := png.Decode(strings.NewReader(s))
	if err != nil {
		panic(err)
	}
	dst := image.NewRGBA(src.Bounds().Sub(src.Bounds().Min))
	draw.Draw(dst, dst.Rect, src, src.Bounds().Min, draw.Src)
	return dst
}

func graphicsInit() {
	graphicsOnce.Do(func() {
		font, err := truetype.Parse([]byte(res.LuxisrTtf))
		if err != nil {
			panic(err)
		}
		draw2d.RegisterFont(draw2d.FontData{
			Name:   "luxi",
			Family: draw2d.FontFamilySans,
			Style:  draw2d.FontStyleNormal,
		}, font)

		for i, d := range ManData {
			mansprites[i][0] = readRGBA(manspritessrc[i][0])
			if mansprites[i][0].Rect.Dx() != int(d.Size.X/PixelSize) || mansprites[i][0].Rect.Dy() != int(d.Size.Y/PixelSize) {
				log.Panicln("man sprite size mismatch", res.Man(i))
			}
			mansprites[i][1] = readRGBA(manspritessrc[i][1])
			if mansprites[i][1].Rect.Dx() != int(d.SizeCrouch.X/PixelSize) || mansprites[i][1].Rect.Dy() != int(d.SizeCrouch.Y/PixelSize) {
				log.Panicln("man sprite crouch size mismatch", res.Man(i))
			}
			manfills[i] = image.NewUniform(d.Color)
		}

		dst := readRGBA(res.TerrainPng)
		if dst.Rect.Dx()%TileSize != 0 || dst.Rect.Dy() != TileSize {
			log.Panic("tile size mismatch")
		}
		for x := dst.Rect.Min.X; x < dst.Rect.Max.X; x += TileSize {
			terrain = append(terrain, dst.SubImage(image.Rect(x, dst.Rect.Min.Y, x+TileSize, dst.Rect.Max.Y)).(*image.RGBA))
		}

		dst = readRGBA(res.TileSidePng)
		tileSide := image.NewGray(dst.Rect)
		draw.Draw(tileSide, tileSide.Rect, dst, tileSide.Rect.Min, draw.Src)
		if tileSide.Rect.Dy() != TileSize {
			log.Panic("tile size mismatch")
		}
		dst = readRGBA(res.TileCornerInnerPng)
		tileCornerInner := image.NewGray(dst.Rect)
		draw.Draw(tileCornerInner, tileCornerInner.Rect, dst, tileCornerInner.Rect.Min, draw.Src)
		if tileCornerInner.Rect.Dx() != tileCornerInner.Rect.Dy() || tileSide.Rect.Dx() != tileCornerInner.Rect.Dx() {
			log.Panic("tile size mismatch")
		}
		dst = readRGBA(res.TileCornerOuterPng)
		tileCornerOuter := image.NewGray(dst.Rect)
		draw.Draw(tileCornerOuter, tileCornerOuter.Rect, dst, tileCornerOuter.Rect.Min, draw.Src)
		if tileCornerInner.Rect.Dx() != tileCornerOuter.Rect.Dx() || tileCornerInner.Rect.Dy() != tileCornerOuter.Rect.Dy() {
			log.Panic("tile size mismatch")
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
		parallax[0] = readRGBA(res.Parallax0Png)
		parallax[1] = readRGBA(res.Parallax1Png)
		lemonsprite = readRGBA(res.LemonPng)
		grubsprite = readRGBA(res.GrubPng)
	})
}
