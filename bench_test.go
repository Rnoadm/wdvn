package main

import (
	"code.google.com/p/goprotobuf/proto"
	"github.com/Rnoadm/wdvn/res"
	"image"
	"math/rand"
	"testing"
)

func makeState() *State {
	rand.Seed(0)

	state := NewState(FooLevel)
	var input [res.Man_count]res.Packet

	input[res.Man_Whip].X = proto.Int64(-80)
	input[res.Man_Whip].Y = proto.Int64(-100)
	input[res.Man_Whip].Mouse2 = Button_pressed

	input[res.Man_Density].X = proto.Int64(-10)
	input[res.Man_Density].Y = proto.Int64(1)
	input[res.Man_Density].Mouse1 = Button_pressed

	input[res.Man_Vacuum].X = proto.Int64(50)
	input[res.Man_Vacuum].Y = proto.Int64(-100)
	input[res.Man_Vacuum].Mouse1 = Button_pressed
	input[res.Man_Vacuum].KeyLeft = Button_pressed

	input[res.Man_Normal].X = proto.Int64(0)
	input[res.Man_Normal].Y = proto.Int64(0)
	input[res.Man_Normal].KeyUp = Button_pressed

	for i := 0; i < 5*TicksPerSecond; i++ {
		state.Update(&input)
	}

	input[res.Man_Density].KeyUp = Button_pressed
	input[res.Man_Vacuum].KeyLeft = Button_released
	input[res.Man_Normal].KeyRight = Button_pressed
	input[res.Man_Normal].KeyDown = Button_pressed

	for i := 0; i < 5*TicksPerSecond; i++ {
		state.Update(&input)
	}

	input[res.Man_Whip].Mouse2 = Button_released
	state.Update(&input)

	return state
}

func init() {
	clientInit()
}

func BenchmarkRender480p(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 720, 480))
	state := makeState()
	*flagSplitScreen = false
	Render(img, res.Man_Whip, state, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Render(img, res.Man_Whip, state, nil)
	}
}

func BenchmarkRender480pSS(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 720, 480))
	state := makeState()
	*flagSplitScreen = true
	Render(img, res.Man_Whip, state, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Render(img, res.Man_Whip, state, nil)
	}
}

func BenchmarkRender720p(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	state := makeState()
	*flagSplitScreen = false
	Render(img, res.Man_Whip, state, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Render(img, res.Man_Whip, state, nil)
	}
}

func BenchmarkRender720pSS(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	state := makeState()
	*flagSplitScreen = true
	Render(img, res.Man_Whip, state, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Render(img, res.Man_Whip, state, nil)
	}
}

func BenchmarkRender1080p(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	state := makeState()
	*flagSplitScreen = false
	Render(img, res.Man_Whip, state, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Render(img, res.Man_Whip, state, nil)
	}
}

func BenchmarkRender1080pSS(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	state := makeState()
	*flagSplitScreen = true
	Render(img, res.Man_Whip, state, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Render(img, res.Man_Whip, state, nil)
	}
}

func BenchmarkRender4K(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 4096, 2160))
	state := makeState()
	*flagSplitScreen = false
	Render(img, res.Man_Whip, state, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Render(img, res.Man_Whip, state, nil)
	}
}

func BenchmarkRender4KSS(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 4096, 2160))
	state := makeState()
	*flagSplitScreen = true
	Render(img, res.Man_Whip, state, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Render(img, res.Man_Whip, state, nil)
	}
}
