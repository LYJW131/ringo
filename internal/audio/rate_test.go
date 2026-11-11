package audio

import (
	"errors"
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
		d := newLoggedDevice()
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

	const rate = 96000
	samples := ramp(rate / 10)
	before := first.dev.count()
	NewSound(rate, 1, samples).Play()
	second := waitFor(t, "the output to open again", opens)
	if second.rate != rate {
		t.Fatalf("opened again at %d, want %d", second.rate, rate)
	}
	// 40 ms at 10 ms a write, besides one mixed as it started.
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

// A song stopped for one of another rate, which starts at once, still fades
// out, in what the device plays before it closes: before, the output mixed
// nothing more once the rate changed, and cut the song where it was.
func TestFadesOutBeforeANewRate(t *testing.T) {
	setup(t)
	opens := make(chan *loggedDevice, 8)
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		d := newLoggedDevice()
		opens <- d
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
	loud := make([]float32, defaultRate)
	for i := range loud {
		loud[i] = 0.5
	}
	a := NewSound(defaultRate, 1, loud).NewPlayer().SetLoop(true)
	a.Play()
	first := waitFor(t, "the output to open", opens)
	first.waitWrites(t, 2)
	// The output waits in a write while one stops and the other starts, as
	// when the next song is ready the moment this one is skipped.
	<-first.park
	a.Pause()
	b := NewSound(96000, 1, make([]float32, 96000)).NewPlayer()
	b.Play()
	first.resume <- struct{}{}
	defer b.Close()
	waitFor(t, "the output to open again", opens)
	faded := false
	for _, w := range first.all() {
		for i := 0; i < len(w); i += 2 {
			faded = faded || (w[i] > 0.05 && w[i] < 0.45)
		}
	}
	if !faded {
		t.Error("the song stopped was cut, not faded out")
	}
}

// A device silent for as long as it holds closes for a new rate at once,
// rather than after as long again.
func TestOpensAgainAtOnceWhenSilent(t *testing.T) {
	setup(t)
	opens := make(chan *loggedDevice, 8)
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		d := newLoggedDevice()
		opens <- d
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
	a.Close()
	// 40 ms held, at 10 ms a write, and one more that found it closed.
	first.waitWrites(t, first.count()+6)
	<-first.park
	before := first.count()
	b := NewSound(96000, 1, make([]float32, 96000)).NewPlayer()
	b.Play()
	first.resume <- struct{}{}
	defer b.Close()
	waitFor(t, "the output to open again", opens)
	if n := first.closedAfter() - before; n > 1 {
		t.Errorf("the silent device closed %d writes after the song started", n)
	}
}

// A loggedDevice keeps what it is given and counts its writes, a little at a time.
type loggedDevice struct {
	// A test that takes from park keeps the output waiting in its next
	// write until it gives to resume.
	park, resume chan struct{}
	mu           sync.Mutex
	writes       [][]float32
	closed       int // the writes before it closed, or -1
}

func newLoggedDevice() *loggedDevice {
	return &loggedDevice{park: make(chan struct{}), resume: make(chan struct{})}
}

func (d *loggedDevice) write(s []float32) error {
	select {
	case d.park <- struct{}{}:
		<-d.resume
	default:
	}
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

func (d *loggedDevice) all() [][]float32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([][]float32(nil), d.writes...)
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

// A device is set to the music's rate where it takes it, or else to the
// rate that converts to with the least loss.
func TestDeviceRate(t *testing.T) {
	builtIn := [][2]float64{{44100, 44100}, {48000, 48000}, {88200, 88200}, {96000, 96000}}
	upTo48 := [][2]float64{{44100, 44100}, {48000, 48000}}
	ranged := [][2]float64{{8000, 192000}}
	for _, c := range []struct {
		want   int
		ranges [][2]float64
		got    int
	}{
		{96000, builtIn, 96000},
		{44100, builtIn, 44100},
		{192000, builtIn, 96000},
		{176400, builtIn, 88200},
		{88200, upTo48, 44100},
		{96000, upTo48, 48000},
		{22050, upTo48, 44100},
		{352800, ranged, 176400},
		{100000, upTo48, 48000},
		{96000, nil, 0},
	} {
		if got := deviceRate(c.want, c.ranges); got != c.got {
			t.Errorf("deviceRate(%d, %v) = %d, want %d", c.want, c.ranges, got, c.got)
		}
	}
}

// The device's rate is put back once the output closes for being idle,
// and not when it opens again at another rate.
func TestRestoresTheDeviceWhenIdle(t *testing.T) {
	setup(t)
	restored := make(chan struct{}, 8)
	restoreDevice = func() { restored <- struct{}{} }
	defer func() { restoreDevice = restoreDeviceRate }()
	closed, opened := make(chan struct{}, 8), make(chan int, 8)
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		opened <- rate
		return closingDevice{closed}, nullFrames(rate), nil
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
	waitFor(t, "the output to open", opened)
	b := NewSound(96000, 1, make([]float32, 96000)).NewPlayer().SetLoop(true)
	b.Play()
	waitFor(t, "the output to close for another rate", closed)
	waitFor(t, "the output to open again", opened)
	select {
	case <-restored:
		t.Fatal("restored the device for another rate")
	default:
	}
	a.Close()
	b.Close()
	waitFor(t, "the device to be restored", restored)
}

// A closingDevice takes writes at once, and says when it closes.
type closingDevice struct{ closed chan struct{} }

func (closingDevice) write([]float32) error { return nil }
func (d closingDevice) close()              { d.closed <- struct{}{} }

// A device that won't run at the music's rate plays it resampled, at the
// rate the output ran at before, rather than unheard, and isn't asked again
// with each write.
func TestFallsBackToTheRateBefore(t *testing.T) {
	setup(t)
	opened := make(chan int, 64)
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		opened <- rate
		if rate != defaultRate {
			return nil, 0, errors.New("rate refused")
		}
		return newLoggedDevice(), nullFrames(rate), nil
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
	p := NewSound(96000, 1, make([]float32, 96000)).NewPlayer().SetLoop(true)
	p.Play()
	defer p.Close()
	if got := waitFor(t, "the output to open", opened); got != 96000 {
		t.Fatalf("asked for %d first, want 96000", got)
	}
	if got := waitFor(t, "the output to open at the rate before", opened); got != defaultRate {
		t.Fatalf("asked for %d then, want %d", got, defaultRate)
	}
	for start := time.Now(); time.Since(start) < 50*time.Millisecond; time.Sleep(time.Millisecond) {
		select {
		case r := <-opened:
			t.Fatalf("opened again at %d while it plays", r)
		default:
		}
	}
	if Err() != nil {
		t.Errorf("Err = %v", Err())
	}
}

// The output opens again when the system's output device changes, so that
// the new one is set to the music's rate too.
func TestOpensAgainOnANewDevice(t *testing.T) {
	setup(t)
	var devMu sync.Mutex
	dev := uint32(1)
	currentDevice = func() uint32 {
		devMu.Lock()
		defer devMu.Unlock()
		return dev
	}
	defer func() { currentDevice = outputDevice }()
	opened := make(chan int, 8)
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		opened <- rate
		return newLoggedDevice(), nullFrames(rate), nil
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
	p := NewSound(96000, 1, make([]float32, 96000)).NewPlayer().SetLoop(true)
	p.Play()
	defer p.Close()
	waitFor(t, "the output to open", opened)
	devMu.Lock()
	dev = 2
	devMu.Unlock()
	if got := waitFor(t, "the output to open on the new device", opened); got != 96000 {
		t.Errorf("opened again at %d, want 96000", got)
	}
}
