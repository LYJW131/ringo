package video

import (
	"bytes"
	"image"
	"io"
	"os"
	"testing"
	"unsafe"
)

// The Mac's own decoder, which plays covers there, gives the same frames
// in the same colors, and lets go of the file it reads from once closed.
func TestHardwarePlaysAndRewinds(t *testing.T) {
	d, err := openHardware(clip(t))
	if err != nil {
		t.Fatal(err)
	}
	playsAndRewinds(t, d)
	path := d.(*hardware).path
	if err := d.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("its file stayed: %v", err)
	}
}

// bgraFrame is a cover's frame as AVFoundation hands it, 456 square, BGRA,
// its rows padded as a pixel buffer's are.
func bgraFrame() (pix []byte, row int) {
	row = 456*4 + 64
	pix = make([]byte, row*456)
	for i := range pix {
		pix[i] = byte(i*7 + i/row*13)
	}
	return pix, row
}

// Accelerate turns a frame's BGRA into RGBA as Go does, byte for byte.
func TestToRGBAAccelerated(t *testing.T) {
	m := media()
	if m == nil {
		t.Skip("no AVFoundation")
	}
	if m.permute == nil {
		t.Fatal("Accelerate didn't load: every frame is copied in Go")
	}
	pix, row := bgraFrame()
	fast, slow := image.NewRGBA(image.Rect(0, 0, 456, 456)), image.NewRGBA(image.Rect(0, 0, 456, 456))
	toRGBA(m.permute, fast, unsafe.Pointer(&pix[0]), row)
	toRGBA(nil, slow, unsafe.Pointer(&pix[0]), row)
	if !bytes.Equal(fast.Pix, slow.Pix) {
		t.Fatal("Accelerate's frame isn't Go's")
	}
	if slow.Pix[0] != pix[2] || slow.Pix[2] != pix[0] || slow.Pix[3] != pix[3] {
		t.Fatal("red and blue didn't change places")
	}
}

// BenchmarkToRGBA is a cover's frame copied out of AVFoundation's buffer,
// as each frame of a cover that moves is: with Accelerate, and in Go.
func BenchmarkToRGBA(b *testing.B) {
	m := media()
	if m == nil {
		b.Skip("no AVFoundation")
	}
	pix, row := bgraFrame()
	img := image.NewRGBA(image.Rect(0, 0, 456, 456))
	for _, c := range []struct {
		name    string
		permute func(src, dst *vImageBuffer, order *[4]uint8, flags uint32) int
	}{{"accelerate", m.permute}, {"go", nil}} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				toRGBA(c.permute, img, unsafe.Pointer(&pix[0]), row)
			}
		})
	}
}
