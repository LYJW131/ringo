package audio

import (
	"math"
	"sync"
	"testing"
	"time"
)

// A 96 kHz song played at the output's rate comes out as decoded, frame for
// frame: before, everything was resampled to 48 kHz, without a low-pass
// filter, so that its 30 kHz was heard at 18 kHz.
func TestPlaysAtItsOwnRate(t *testing.T) {
	setup(t)
	const rate = 96000
	mu.Lock()
	sampleRate, rampFrames = rate, rate/200
	mu.Unlock()
	samples := make([]float32, rate/10)
	for i := range samples {
		samples[i] = float32(0.5 * math.Sin(2*math.Pi*30000*float64(i)/rate))
	}
	NewSound(rate, 1, samples).Play()
	out := render(len(samples))
	for i, s := range samples {
		if out[2*i] != s || out[2*i+1] != s {
			t.Fatalf("frame %d = %v %v, want %v", i, out[2*i], out[2*i+1], s)
		}
	}
}

// A song of another rate opens the output again at its rate, once what the
// device held of the one before is heard, and starts from its start there.
func TestOpensAgainAtANewRate(t *testing.T) {
	setup(t)
	type opened struct {
		rate int
		dev  *loggedDevice
	}
	opens := make(chan opened, 8)
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		d := &loggedDevice{}
		opens <- opened{rate, d}
		return d, nullFrames(rate), nil
	}
	defer func() { openOutput = openDevice }()
	mu.Lock()
	testMode = false
	mu.Unlock()
	defer func() {
		mu.Lock()
		testMode = true
		mu.Unlock()
	}()

	a := NewSound(defaultRate, 1, make([]float32, defaultRate)).NewPlayer().SetLoop(true)
	a.Play()
	first := waitFor(t, "the output to open", opens)
	if first.rate != defaultRate {
		t.Fatalf("opened at %d, want %d", first.rate, defaultRate)
	}
	a.Close()
	first.dev.waitWrites(t, first.dev.count()+2)

	const rate = 96000
	samples := ramp(rate / 10)
	before := first.dev.count()
	NewSound(rate, 1, samples).Play()
	second := waitFor(t, "the output to open again", opens)
	if second.rate != rate {
		t.Fatalf("opened again at %d, want %d", second.rate, rate)
	}
	// 40 ms of silence at 10 ms a write, besides one mixed as it started.
	if n := first.dev.closedAfter() - before; n < 4 {
		t.Errorf("the device closed %d writes after the song started, want what it held heard first", n)
	}
	second.dev.waitWrites(t, 1)
	got := second.dev.first()
	for i := range len(got) / 2 {
		if got[2*i] != samples[i] {
			t.Fatalf("frame %d at %d Hz = %v, want %v", i, rate, got[2*i], samples[i])
		}
	}
}

// A loggedDevice keeps what it is given and counts its writes, a little at a time.
type loggedDevice struct {
	mu     sync.Mutex
	writes [][]float32
	closed int // the writes before it closed, or -1
}

func (d *loggedDevice) write(s []float32) error {
	time.Sleep(time.Millisecond)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writes = append(d.writes, append([]float32(nil), s...))
	return nil
}

func (d *loggedDevice) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = len(d.writes)
}

func (d *loggedDevice) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.writes)
}

func (d *loggedDevice) closedAfter() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

func (d *loggedDevice) first() []float32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writes[0]
}

func (d *loggedDevice) waitWrites(t *testing.T, n int) {
	t.Helper()
	for start := time.Now(); d.count() < n; time.Sleep(time.Millisecond) {
		if time.Since(start) > time.Minute {
			t.Fatalf("%d writes never came", n)
		}
	}
}
