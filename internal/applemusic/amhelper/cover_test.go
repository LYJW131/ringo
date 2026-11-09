package amhelper

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestShrinkAverages(t *testing.T) {
	// Stripes a pixel wide, black and white, average to grey.
	src := image.NewRGBA(image.Rect(0, 0, 1200, 900))
	for y := range 900 {
		for x := range 1200 {
			v := uint8(0)
			if x%2 == 1 {
				v = 255
			}
			src.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	out := shrink(src, 600)
	if b := out.Bounds(); b.Dx() != 600 || b.Dy() != 450 {
		t.Fatalf("%v, want 600x450", b)
	}
	for _, p := range []image.Point{{0, 0}, {299, 200}, {599, 449}} {
		r, _, _, _ := out.At(p.X, p.Y).RGBA()
		if v := r >> 8; v < 126 || v > 129 {
			t.Errorf("pixel %v is %d, want grey", p, v)
		}
	}
	// A width that doesn't divide evenly still covers the whole line.
	for _, s := range spans(1044, 600) {
		sum := 0.0
		for _, w := range s {
			sum += w.w
		}
		if sum < 0.999 || sum > 1.001 {
			t.Fatalf("weights add up to %f", sum)
		}
	}
}

func TestKeepCover(t *testing.T) {
	var pic bytes.Buffer
	if err := jpeg.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 1044, 1044)), nil); err != nil {
		t.Fatal(err)
	}
	const link = "https://store-035.blobstore.apple.com/sq/f2/f2caaca1-5627-2a0f-be07-24002f215bc3/image?sig=1"
	dir := t.TempDir()
	c := &covers{dir: dir, http: &http.Client{Transport: fakeCDN{link: pic.Bytes()}}}
	const name = "f2caaca1-5627-2a0f-be07-24002f215bc3"
	if err := c.keep(context.Background(), name, link); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(dir, "cover-"+name+".jpg"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := jpeg.DecodeConfig(f)
	if err != nil || cfg.Width != coverWidth || cfg.Height != coverWidth {
		t.Fatalf("kept %dx%d (%v), want %d wide", cfg.Width, cfg.Height, err, coverWidth)
	}
	if err := c.keep(context.Background(), name, link+"&gone"); err == nil {
		t.Fatal("a cover Apple doesn't have was kept")
	}
	if !blobstoreURL(link) || blobstoreURL("https://blobstore.apple.com.evil.example/x") || blobstoreURL("http://a.blobstore.apple.com/x") {
		t.Fatal("blobstoreURL takes the wrong links")
	}
	if coverName.MatchString("../" + name[3:]) {
		t.Fatal("a path passed for a cover's name")
	}
}
