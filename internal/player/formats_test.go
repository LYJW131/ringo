package player

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"os"
	"testing"
)

// downloaded returns a download that has all of b in already.
func downloaded(b []byte) *download {
	d := &download{data: b, n: int64(len(b)), size: int64(len(b)), done: true, cancel: func() {}, users: 1}
	d.cond.L = &d.mu
	return d
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// seekLands checks that a seek to each frame plays what decoding straight
// through has there, to within tol.
func seekLands(t *testing.T, b []byte, ref []float32, tol float64, frames ...int64) {
	t.Helper()
	s, err := openMP4Stream(downloaded(b))
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4096)
	for _, frame := range frames {
		if err := s.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		n, err := s.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		if d := maxDiff(buf[:n], ref[frame*2:]); d > tol {
			t.Errorf("seek to %d: off by up to %.4f", frame, d)
		}
	}
}

// snr is how far below ref the difference between got and ref is, in dB.
func snr(got, ref []float32) float64 {
	var sig, noise float64
	for i := range min(len(got), len(ref)) {
		sig += float64(ref[i]) * float64(ref[i])
		d := float64(got[i] - ref[i])
		noise += d * d
	}
	return 10 * math.Log10(sig/noise)
}

// alacSamples is the SHA-256 of seek.m4a's samples, as little-endian float32.
// They were checked once against the FLAC the file was encoded from, which
// they matched to the sample: ALAC is lossless, so any other sum is a decoder
// that has gone wrong.
const alacSamples = "0aa6fa336e09435ce6875404b14a6a55cec1603bc9a8349f8207aefc5c2a1962"

// An ALAC .m4a plays exactly the samples it was encoded from, and lands on
// the frame it's asked for.
func TestALAC(t *testing.T) {
	b := readFixture(t, "seek.m4a")
	got := decodeAll(t, b)
	if len(got) != 2*44100*2 {
		t.Fatalf("decoded %d samples", len(got))
	}
	sum := sha256.New()
	for _, v := range got {
		sum.Write(binary.LittleEndian.AppendUint32(nil, math.Float32bits(v)))
	}
	if h := hex.EncodeToString(sum.Sum(nil)); h != alacSamples {
		t.Fatalf("samples sum to %s", h)
	}
	seekLands(t, b, got, 0, 0, 1, 4095, 4096, 50000, 88199)
}

// An AAC .m4a drops the encoder's priming, as its edit list says, so it lines
// up with what it was encoded from (the ALAC file is its first two seconds)
// and is as long. A seek decodes the packet
// before, whose sound overlaps the one sought to.
func TestAACInMP4(t *testing.T) {
	b := readFixture(t, "seek-aac.m4a")
	s, err := openMP4Stream(downloaded(b))
	if err != nil {
		t.Fatal(err)
	}
	if s.SampleRate() != 44100 || s.Len() != 3*44100 {
		t.Fatalf("rate %d, length %d", s.SampleRate(), s.Len())
	}
	got := decodeAll(t, b)
	if len(got) != 3*44100*2 {
		t.Fatalf("decoded %d samples", len(got))
	}
	if db := snr(got, decodeAll(t, readFixture(t, "seek.m4a"))); db < 20 {
		t.Errorf("%.1f dB from the source", db)
	}
	// Noise the encoder substituted is random each time it's decoded.
	seekLands(t, b, got, 0.05, 0, 1, 4096, 50000, 123457)
}

// An AAC .m4a cut into fragments, as a stream for DASH is, plays what the
// whole one does. Its movie header has no edit list, so the encoder's priming
// plays too: the fragments are what the whole one has after it.
func TestFragmentedAAC(t *testing.T) {
	b := readFixture(t, "seek-aac-frag.m4a")
	got := decodeAll(t, b)
	whole := decodeAll(t, readFixture(t, "seek-aac.m4a"))
	const prime = 1024
	if len(got) < len(whole)+prime*2 {
		t.Fatalf("decoded %d samples, the whole file %d", len(got), len(whole))
	}
	if db := snr(got[prime*2:], whole); db < 20 {
		t.Errorf("%.1f dB from the whole file", db)
	}
	seekLands(t, b, got, 0.05, 0, 1, 4096, 50000, 123457)
}

// AAC's packets each decode to 1024 frames, whatever its container says they
// last: a seek lands where decoding straight through has the frame. Some sites'
// files have a packet near the start a few frames shorter, which put every
// seek past it that much off, as good as other audio for a second after.
func TestAACPacketsOfOtherDurations(t *testing.T) {
	b := bytes.Clone(readFixture(t, "seek-aac-frag.m4a"))
	// The first fragment's packets last 1016 frames, its header says.
	at := bytes.Index(b, []byte("tfhd")) + 4 + 8
	if binary.BigEndian.Uint32(b[at:]) != 1024 {
		t.Fatal("no default duration where expected")
	}
	binary.BigEndian.PutUint32(b[at:], 1016)
	got := decodeAll(t, b)
	for _, frame := range []int64{50000, 100000, 100001} {
		s, err := openMP4Stream(downloaded(b))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		var read []float32
		buf := make([]float32, 4096)
		for len(read) < 44100 {
			n, err := s.Read(buf)
			read = append(read, buf[:n]...)
			if err != nil {
				break
			}
		}
		if db := snr(read, got[frame*2:]); db < 20 {
			t.Errorf("seek to %d: %.1f dB from decoding straight through", frame, db)
		}
	}
}

// A fragmented .m4a starts playing with only its first fragment in, its length
// from its index, as a stream from a site serving DASH comes in.
func TestFragmentedComingIn(t *testing.T) {
	b := readFixture(t, "seek-aac-frag.m4a")
	got := decodeAll(t, b)
	dl := downloaded(b)
	dl.n, dl.done = 10<<10, false
	s, err := openMP4Stream(dl)
	if err != nil {
		t.Fatal(err)
	}
	if s.Len()*2 != int64(len(got)) {
		t.Fatalf("length %d, though %d frames decode", s.Len(), len(got)/2)
	}
	buf := make([]float32, 4096)
	n, err := s.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if d := maxDiff(buf[:n], got); d > 0.05 {
		t.Errorf("off by up to %.4f", d)
	}
}
