package audio

import (
	"math"
	"sync"
	"testing"
	"time"
)

// A holdingDevice is a loggedDevice that says how many frames it holds
// after a write, as the test has it say.
type holdingDevice struct {
	*loggedDevice
	mu     sync.Mutex
	frames int64
	sure   bool
}

func (d *holdingDevice) holds() (int64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.frames, d.sure
}

func (d *holdingDevice) say(frames int64, sure bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.frames, d.sure = frames, sure
}

// positionAt returns the frame p is heard at, and the frames mixed then,
// with the output waiting in a write, so that what it mixed is known.
func positionAt(p *Player, d *loggedDevice) (heard, mixedFrames int) {
	<-d.park
	mixedFrames = (d.count() + 1) * nullFrames(defaultRate)
	heard = int(math.Round(p.Position().Seconds() * defaultRate))
	d.resume <- struct{}{}
	return heard, mixedFrames
}

// A player is heard as far behind what is mixed as the device says it
// holds, and half a write, which it plays while the next waits: it was
// taken to hold what it was opened to, 40 ms here and a quarter of a
// second for music, though the Audio Queue holds some 20 ms more, and the
// position ran that far ahead of what was heard.
func TestPositionIsBehindWhatTheDeviceHolds(t *testing.T) {
	devs := make(chan *holdingDevice, 2)
	outputTo(t, func(int) device {
		d := &holdingDevice{loggedDevice: newLoggedDevice(), frames: 2400, sure: true}
		devs <- d
		return d
	})
	p := NewSound(defaultRate, 1, rising(defaultRate)).NewPlayer()
	p.Play()
	defer p.Close()
	d := waitFor(t, "the output to open", devs)
	d.waitWrites(t, 12)
	heard, mixedFrames := positionAt(p, d.loggedDevice)
	if want := mixedFrames - (2400 + nullFrames(defaultRate)/2); heard != want {
		t.Errorf("heard at frame %d with %d mixed, want %d: 2400 held and half a write behind", heard, mixedFrames, want)
	}
}

// A device not sure yet of what it holds is asked again as it plays, and
// the position follows what it comes to say.
func TestPositionFollowsWhatTheDeviceComesToHold(t *testing.T) {
	devs := make(chan *holdingDevice, 2)
	outputTo(t, func(int) device {
		d := &holdingDevice{loggedDevice: newLoggedDevice(), frames: 1920}
		devs <- d
		return d
	})
	p := NewSound(defaultRate, 1, rising(defaultRate)).NewPlayer()
	p.Play()
	defer p.Close()
	d := waitFor(t, "the output to open", devs)
	d.waitWrites(t, 12)
	half := nullFrames(defaultRate) / 2
	if heard, mixedFrames := positionAt(p, d.loggedDevice); heard != mixedFrames-(1920+half) {
		t.Fatalf("heard at frame %d with %d mixed, want 1920 held and half a write behind", heard, mixedFrames)
	}
	d.say(2880, true)
	d.waitWrites(t, d.count()+3)
	if heard, mixedFrames := positionAt(p, d.loggedDevice); heard != mixedFrames-(2880+half) {
		t.Errorf("heard at frame %d with %d mixed, want 2880 held and half a write behind", heard, mixedFrames)
	}
	// Sure of it, it is asked no more.
	d.say(960, true)
	d.waitWrites(t, d.count()+3)
	if heard, mixedFrames := positionAt(p, d.loggedDevice); heard != mixedFrames-(2880+half) {
		t.Errorf("heard at frame %d with %d mixed, after it was sure of 2880", heard, mixedFrames)
	}
}

// A device's latency is what it reports, in frames at its rate, unless it
// reports none, or more than could be so.
func TestReportedLatency(t *testing.T) {
	for _, c := range []struct {
		frames uint32
		rate   float64
		want   time.Duration
	}{
		{7680, 48000, 160 * time.Millisecond}, // AirPods Max
		{798, 48000, 16625 * time.Microsecond},
		{88200, 44100, 2 * time.Second}, // AirPlay
		{0, 48000, 0},
		{7680, 0, 0},
		{144001, 48000, 0}, // past three seconds
	} {
		if got := reportedLatency(c.frames, c.rate); got != c.want {
			t.Errorf("reportedLatency(%d, %v) = %v, want %v", c.frames, c.rate, got, c.want)
		}
	}
}
