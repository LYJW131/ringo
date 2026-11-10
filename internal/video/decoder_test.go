package video

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// testdata/clip.mp4 is FFmpeg's own test pattern, made here and free of any
// rights, laid out as Apple's moving covers are: H.264 High with B-frames,
// BT.709 in TV range, in fragmented MP4. 36 frames of 160×160 at 30 a
// second, made with
//
//	ffmpeg -f lavfi -i testsrc2=size=160x160:rate=30:duration=1.2 \
//	  -c:v libx264 -profile:v high -pix_fmt yuv420p -bf 2 -g 12 -crf 30 -preset slow \
//	  -color_range tv -colorspace bt709 -color_primaries bt709 -color_trc bt709 \
//	  -movflags frag_keyframe+empty_moov+default_base_moof -an clip.mp4
func clip(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// frames decodes what's left of d, keeping a copy of the first frame, its
// rows packed.
func frames(t *testing.T, d ui.VideoDecoder) (int, []byte) {
	t.Helper()
	n := 0
	var first []byte
	for {
		f, err := d.Next()
		if err == io.EOF {
			return n, first
		}
		if err != nil {
			t.Fatalf("frame %d: %v", n, err)
		}
		img := f.Image.(*image.RGBA)
		if img.Rect.Dx() != 160 || img.Rect.Dy() != 160 || f.Duration != time.Second/30 {
			t.Fatalf("frame %d: %v for %v", n, img.Rect, f.Duration)
		}
		if n == 0 {
			for y := range 160 {
				first = append(first, img.Pix[y*img.Stride:y*img.Stride+160*4]...)
			}
		}
		n++
	}
}

// playsAndRewinds checks that d gives every frame of testdata/clip.mp4, and
// gives them again from the same first frame once rewound; and that the
// first frame's colors are FFmpeg's own for it, testdata/clip-first.png,
// made with ffmpeg -i clip.mp4 -frames:v 1 -sws_flags
// accurate_rnd+full_chroma_int, as a cover read with the wrong matrix or
// range isn't. Edges differ a little, as chroma is spread differently.
func playsAndRewinds(t *testing.T, d ui.VideoDecoder) {
	t.Helper()
	n, first := frames(t, d)
	if n != 36 {
		t.Errorf("%d frames, want 36", n)
	}
	if _, err := d.Next(); err != io.EOF {
		t.Errorf("after the end: %v", err)
	}
	if err := d.Rewind(); err != nil {
		t.Fatal(err)
	}
	again, first2 := frames(t, d)
	if again != 36 || !bytes.Equal(first, first2) {
		t.Errorf("played again: %d frames, the same first %v", again, bytes.Equal(first, first2))
	}
	f, err := os.Open("testdata/clip-first.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ref, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	var sum, count int
	for y := range 160 {
		for x := range 160 {
			r, g, b, _ := ref.At(x, y).RGBA()
			i := (y*160 + x) * 4
			for c, v := range []uint32{r >> 8, g >> 8, b >> 8} {
				d := int(v) - int(first[i+c])
				sum += max(d, -d)
				count++
			}
		}
	}
	if mean := float64(sum) / float64(count); mean > 6 {
		t.Errorf("first frame off FFmpeg's by %.1f on average", mean)
	}
}

// Every frame comes out, the B-frames the decoder holds back too, and
// Rewind plays it again from the same first frame.
func TestDecoderPlaysAndRewinds(t *testing.T) {
	if !Supported() {
		t.Skip("no compiler for this machine")
	}
	d, err := Open(clip(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	playsAndRewinds(t, d)
}

// The Mac's own decoder, which plays covers there, gives the same frames
// in the same colors, and lets go of the file it reads from once closed.
func TestHardwarePlaysAndRewinds(t *testing.T) {
	if !Hardware() {
		t.Skip("no hardware decoder here")
	}
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

// What isn't a fragmented MP4 of H.264 doesn't open.
func TestDecoderRefuses(t *testing.T) {
	if _, err := Open([]byte("not a video")); err == nil {
		t.Error("opened text")
	}
	if _, err := Open(nil); err == nil {
		t.Error("opened nothing")
	}
}
