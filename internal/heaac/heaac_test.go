package heaac

import (
	"math"
	"os"
	"testing"

	"github.com/egoist/mygo/audio/mp4"
)

// testdata/tone-he.m4a is three seconds of a tone made here, 440 Hz on the
// left and 660 Hz on the right at 48 kHz, encoded as HE-AAC by macOS's own
// encoder, 13 KB:
//
//	afconvert -f m4af -d aach -b 48000 tone.wav tone-he.m4a

// Apple's AudioSpecificConfigs say SBR in the sync extension after an AAC-LC
// one, which a decoder of AAC-LC alone skips and plays the core at half the
// rate; AAC-LC's own say nothing more, and stay with MyGo's decoder.
func TestParseASC(t *testing.T) {
	for _, c := range []struct {
		asc  []byte
		want config
	}{
		{[]byte{0x13, 0x10, 0x56, 0xe5, 0x98}, config{coreRate: 24000, rate: 48000, sbr: true}},
		{[]byte{0x13, 0x90, 0x56, 0xe5, 0xa0}, config{coreRate: 22050, rate: 44100, sbr: true}},
		{[]byte{0x11, 0x90}, config{coreRate: 48000, rate: 48000}},
		{[]byte{0x12, 0x10}, config{coreRate: 44100, rate: 44100}},
		{[]byte{0x2b, 0x11, 0x88}, config{coreRate: 24000, rate: 48000, sbr: true}}, // object type 5 first
	} {
		got, ok := parseASC(c.asc)
		if !ok || got != c.want {
			t.Errorf("%x: %+v %v, want %+v", c.asc, got, ok, c.want)
		}
	}
	if _, ok := parseASC([]byte{0x13}); ok {
		t.Error("read a config cut short")
	}
	if c, err := open([]byte{0x11, 0x90}); err != nil {
		t.Fatal(err)
	} else if _, wasm := c.(*codec); wasm {
		t.Error("AAC-LC went to the WebAssembly decoder, not MyGo's")
	}
}

// crossings counts how often a channel of interleaved stereo crosses zero
// upwards: its frequency, in a second of it.
func crossings(pcm []float32, ch int) int {
	n := 0
	for i := ch + 2; i < len(pcm); i += 2 {
		if pcm[i-2] < 0 && pcm[i] >= 0 {
			n++
		}
	}
	return n
}

// HE-AAC in MP4 decodes through MyGo's decoder to the rate SBR doubles it
// to, each channel its own tone, for as long as it was encoded; and from
// the middle of it, sought to, the same.
func TestDecodesHEAAC(t *testing.T) {
	f, err := os.Open("testdata/tone-he.m4a")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d, err := mp4.NewDecoder(f)
	if err != nil {
		t.Fatal(err)
	}
	if d.SampleRate() != 48000 {
		t.Fatalf("decodes at %d Hz, want SBR's 48000", d.SampleRate())
	}
	// The encoder's priming and padding are in a tag of its own, which MyGo
	// doesn't read, rather than an edit list: a tenth of a second more.
	if secs := float64(d.Len()) / 48000; secs < 3 || secs > 3.2 {
		t.Errorf("%.2f s long, want 3 and the encoder's padding", secs)
	}
	check := func(when string, pcm []float32) {
		t.Helper()
		peak := float32(0)
		for _, v := range pcm {
			peak = max(peak, v, -v)
		}
		left, right := crossings(pcm, 0), crossings(pcm, 1)
		if peak < 0.2 || peak > 0.6 || math.Abs(float64(left-440)) > 3 || math.Abs(float64(right-660)) > 3 {
			t.Errorf("%s: peak %.2f, %d Hz left and %d Hz right, want 440 and 660", when, peak, left, right)
		}
	}
	second := func() []float32 {
		pcm := make([]float32, 0, 48000*2)
		buf := make([]float32, 4096)
		for len(pcm) < 48000*2 {
			n, err := d.Read(buf)
			pcm = append(pcm, buf[:n]...)
			if err != nil {
				t.Fatalf("after %d frames: %v", len(pcm)/2, err)
			}
		}
		return pcm[:48000*2]
	}
	second() // the encoder's start
	check("the second second", second())
	if err := d.SetPosition(48000 * 3 / 2); err != nil {
		t.Fatal(err)
	}
	check("from the middle", second())
}
