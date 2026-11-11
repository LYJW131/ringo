package audio

import (
	"math"
	"sync/atomic"
	"testing"
	"time"
)

// A haltingDevice is a loggedDevice that can stop at once: it says how many
// writes it had taken when it did, and when it closes.
type haltingDevice struct {
	*loggedDevice
	flushed chan int
	closed  chan struct{}
}

func newHaltingDevice() *haltingDevice {
	return &haltingDevice{newLoggedDevice(), make(chan int, 8), make(chan struct{}, 1)}
}

func (d *haltingDevice) flush() error {
	d.flushed <- d.count()
	return nil
}

func (d *haltingDevice) close() {
	d.loggedDevice.close()
	d.closed <- struct{}{}
}

// A tracked device counts its closing, for outputTo to wait for.
type tracked struct {
	device
	closes *atomic.Int32
}

func (d tracked) close() {
	d.device.close()
	d.closes.Add(1)
}

// A trackedFlusher is a tracked device that can stop at once.
type trackedFlusher struct{ tracked }

func (d trackedFlusher) flush() error { return d.device.(flusher).flush() }

// outputTo opens the output on the devices open returns, until the test
// ends: then the output is let go of whatever it waits for, and the test
// waits for it to close them, so that the next test's opens its own.
func outputTo(t *testing.T, open func(rate int) device) {
	t.Helper()
	setup(t)
	var opens, closes atomic.Int32
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		opens.Add(1)
		d := tracked{open(rate), &closes}
		if _, ok := d.device.(flusher); ok {
			return trackedFlusher{d}, nullFrames(rate), nil
		}
		return d, nullFrames(rate), nil
	}
	mu.Lock()
	testMode = false
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		testMode = true
		mu.Unlock()
		for start := time.Now(); closes.Load() != opens.Load(); time.Sleep(time.Millisecond) {
			select {
			case wake <- struct{}{}:
			default:
			}
			if time.Since(start) > time.Minute {
				t.Error("the output never closed its device")
				break
			}
		}
		openOutput = openDevice
	})
}

// rising is a sound whose every frame is its own, to tell where one is.
func rising(n int) []float32 {
	s := make([]float32, n)
	for i := range s {
		s[i] = float32(i+1) / float32(n+1)
	}
	return s
}

// A player paused at once stops where it is heard, not where it is mixed:
// the device drops what it held and is written nothing while it waits, the
// position is the frame heard, and playing again goes on from that frame,
// the first thing the device is written. Before, a pause was heard only
// after the quarter of a second the device held, and playing again after as
// much silence.
func TestPausesAtOnce(t *testing.T) {
	devs := make(chan *haltingDevice, 2)
	outputTo(t, func(int) device {
		d := newHaltingDevice()
		devs <- d
		return d
	})
	samples := rising(defaultRate)
	p := NewSound(defaultRate, 1, samples).NewPlayer()
	p.Play()
	defer p.Close()
	d := waitFor(t, "the output to open", devs)
	d.waitWrites(t, 8)
	// The output waits in a write, so that what it mixed is known.
	<-d.park
	mixedFrames := (d.count() + 1) * nullFrames(defaultRate)
	if !p.PauseAtOnce() {
		t.Fatal("PauseAtOnce: the device can't")
	}
	d.resume <- struct{}{}
	at := waitFor(t, "the device to stop at once", d.flushed)
	if want := mixedFrames / nullFrames(defaultRate); at != want {
		t.Errorf("stopped after %d writes, want %d: it mixed on", at, want)
	}
	// 40 ms held, at the interactive latency.
	heard := mixedFrames - defaultRate*40/1000
	if got := int(math.Round(p.Position().Seconds() * defaultRate)); got != heard {
		t.Errorf("paused at frame %d, want the frame heard, %d", got, heard)
	}
	if p.Playing() {
		t.Error("still playing")
	}
	p.Play()
	d.waitWrites(t, at+1)
	// Past the 5 ms it fades in over, it is the song from where it was heard.
	next := d.all()[at]
	for _, i := range []int{rampFrames + 1, rampFrames + 100} {
		if next[2*i] != samples[heard+i] {
			t.Errorf("frame %d written after playing again = %v, want the song's frame %d, %v", i, next[2*i], heard+i, samples[heard+i])
		}
	}
}

// A device that can't stop at once pauses nothing at once, nor does no
// device: the caller pauses as before.
func TestPauseAtOnceWhereItCant(t *testing.T) {
	devs := make(chan *loggedDevice, 2)
	outputTo(t, func(int) device {
		d := newLoggedDevice()
		devs <- d
		return d
	})
	p := NewSound(defaultRate, 1, rising(defaultRate)).NewPlayer()
	defer p.Close()
	if p.PauseAtOnce() {
		t.Error("paused at once with nothing playing")
	}
	p.Play()
	waitFor(t, "the output to open", devs).waitWrites(t, 2)
	if p.PauseAtOnce() || !p.Playing() {
		t.Errorf("PauseAtOnce on a device that can't: playing %v", p.Playing())
	}
}

// A device stopped at once still closes once nothing has played for as long
// as it stays open, and the system's device is put back; it stays closed
// until the player plays again, which the next device hears from the frame
// heard. As the paused player stayed among the voices, the output opened
// again at once for it, and played silence ahead of the song.
func TestStoppedDeviceClosesWhenIdle(t *testing.T) {
	restored := make(chan struct{}, 2)
	restoreDevice = func() { restored <- struct{}{} }
	defer func() { restoreDevice = restoreDeviceRate }()
	// Its clock: the time it stays open is over at once.
	parkFor = func(time.Duration) <-chan time.Time {
		c := make(chan time.Time, 1)
		c <- time.Time{}
		return c
	}
	defer func() { parkFor = func(d time.Duration) <-chan time.Time { return time.After(d) } }()
	devs := make(chan *haltingDevice, 2)
	outputTo(t, func(int) device {
		d := newHaltingDevice()
		devs <- d
		return d
	})
	samples := rising(defaultRate)
	p := NewSound(defaultRate, 1, samples).NewPlayer()
	p.Play()
	defer p.Close()
	d := waitFor(t, "the output to open", devs)
	d.waitWrites(t, 8)
	if !p.PauseAtOnce() {
		t.Fatal("PauseAtOnce: the device can't")
	}
	waitFor(t, "the device to close", d.closed)
	waitFor(t, "the system's device to be put back", restored)
	heard := int(math.Round(p.Position().Seconds() * defaultRate))
	p.Play()
	d = waitFor(t, "the output to open again", devs)
	d.waitWrites(t, 1)
	first := d.first()
	if i := rampFrames + 1; first[2*i] != samples[heard+i] {
		t.Errorf("frame %d written to the next device = %v, want the song's frame %d, %v", i, first[2*i], heard+i, samples[heard+i])
	}
}

// A device stopped at once holds nothing, so a song of another rate opens
// the next without waiting for it to play anything out.
func TestStoppedDeviceOpensAgainAtOnce(t *testing.T) {
	type opened struct {
		rate int
		dev  *haltingDevice
	}
	devs := make(chan opened, 2)
	outputTo(t, func(rate int) device {
		d := newHaltingDevice()
		devs <- opened{rate, d}
		return d
	})
	a := NewSound(defaultRate, 1, rising(defaultRate)).NewPlayer()
	a.Play()
	defer a.Close()
	first := waitFor(t, "the output to open", devs)
	first.dev.waitWrites(t, 8)
	if !a.PauseAtOnce() {
		t.Fatal("PauseAtOnce: the device can't")
	}
	at := waitFor(t, "the device to stop at once", first.dev.flushed)
	b := NewSound(96000, 1, rising(96000)).NewPlayer()
	b.Play()
	defer b.Close()
	if second := waitFor(t, "the output to open again", devs); second.rate != 96000 {
		t.Errorf("opened again at %d, want 96000", second.rate)
	}
	if n := first.dev.closedAfter() - at; n != 0 {
		t.Errorf("the device was written %d times after it stopped", n)
	}
}
