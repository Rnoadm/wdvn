package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"github.com/BenLubar/bindiff"
	"github.com/Rnoadm/wdvn/res"
	"image"
	"image/color"
	"io"
)

func EncodeAll(w *bufio.Writer, frames <-chan *image.YCbCr) error {
	frame, ok := <-frames
	if !ok {
		return fmt.Errorf("No frames!")
	}

	_, err := fmt.Fprintf(w, "YUV4MPEG2 W%d H%d F%d:1 Ip A1:1 C444\n", frame.Rect.Dx(), frame.Rect.Dy(), TicksPerSecond)
	if err != nil {
		return err
	}

	for ok {
		_, err := w.WriteString("FRAME\n")
		if err != nil {
			return err
		}

		_, err = w.Write(frame.Y)
		if err != nil {
			return err
		}

		_, err = w.Write(frame.Cb)
		if err != nil {
			return err
		}

		_, err = w.Write(frame.Cr)
		if err != nil {
			return err
		}

		frame, ok = <-frames
	}

	return nil
}

func EncodeVideo(w io.Writer, r io.Reader) error {
	clientInit()

	frames := make(chan *image.YCbCr)
	br := bufio.NewReader(r)
	go func() {
		defer close(frames)

		var (
			world World
			state State
			buf   bytes.Buffer
			old   []byte
			src   = image.NewRGBA(image.Rect(0, 0, *flagWidth, *flagHeight))
		)

		version, err := binary.ReadUvarint(br)
		if err != nil {
			panic(err)
		}
		switch version {
		case 0:
			panic("invalid replay version")
		case 1:
			// do nothing
		default:
			panic("replay from newer version")
		}

		for {
			l, err := binary.ReadUvarint(br)
			if err == io.EOF {
				return
			}
			if err != nil {
				panic(err)
			}

			n, err := io.CopyN(&buf, br, int64(l))
			if err == nil && n != int64(l) {
				err = io.ErrUnexpectedEOF
			}
			if err != nil {
				panic(err)
			}

			t, err := buf.ReadByte()
			if err != nil {
				panic(err)
			}

			switch t {
			case 0:
				world, state = World{}, State{}
				err = gob.NewDecoder(&buf).Decode(&world)
				if err != nil {
					panic(err)
				}

				old = make([]byte, buf.Len())
				copy(old, buf.Bytes())

				err = gob.NewDecoder(&buf).Decode(&state)
				if err != nil {
					panic(err)
				}

			case 1:
				if state.world == nil {
					panic("diff packet came before world")
				}

				old, err = bindiff.Forward(old, buf.Bytes())
				if err != nil {
					panic(err)
				}
				state = State{}
				err = gob.NewDecoder(bytes.NewReader(old)).Decode(&state)
				if err != nil {
					panic(err)
				}
			}

			state.world = &world
			Render(src, res.Man_Whip, &state, nil)
			frames <- toYCbCr(src)
		}
	}()

	bw := bufio.NewWriter(w)
	defer bw.Flush()

	return EncodeAll(bw, frames)
}

func toYCbCr(src *image.RGBA) *image.YCbCr {
	dst := image.NewYCbCr(src.Rect, image.YCbCrSubsampleRatio444)
	i0 := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y)
	i1 := dst.YOffset(dst.Rect.Min.X, dst.Rect.Min.Y)
	i2 := dst.COffset(dst.Rect.Min.X, dst.Rect.Min.Y)
	for y := src.Rect.Min.Y; y < src.Rect.Max.Y; y++ {
		i0x := i0
		i1x := i1
		i2x := i2
		for x := src.Rect.Min.X; x < src.Rect.Max.X; x++ {
			y, cb, cr := color.RGBToYCbCr(src.Pix[i0x], src.Pix[i0x+1], src.Pix[i0x+2])

			dst.Y[i1x] = y
			dst.Cb[i2x] = cb
			dst.Cr[i2x] = cr

			i0x += 4
			i1x++
			i2x++
		}
		i0 += src.Stride
		i1 += dst.YStride
		i2 += dst.CStride
	}
	return dst
}
