package audio

import (
	"errors"
	"testing"
)

// A failingDevice takes left writes and fails the next.
type failingDevice struct {
	left int
}

var errWontPlay = errors.New("the device won't play")

func (d *failingDevice) write([]float32) error {
	if d.left == 0 {
		return errWontPlay
	}
	d.left--
	return nil
}

func (d *failingDevice) close() {}

// A watchedDevice is a loggedDevice that says when it closes.
type watchedDevice struct {
	*loggedDevice
	closed chan struct{}
}

func (d watchedDevice) close() {
	d.loggedDevice.close()
	close(d.closed)
}

// A device that opens and then plays nothing, failing before it took what
// it holds, is stood in for as one that didn't open, and the player goes
// back to where it was before the device took anything of it. Opened again
// at once, as it was, it failed again with another quarter of a second of
// the song each time: on a Mac with no audio output, whose Audio Queue
// opens and then fails to start, a song played out in a moment.
func TestDeviceThatWontPlayIsStoodInFor(t *testing.T) {
	stood := make(chan watchedDevice, 2)
	standIn = func(retry bool, rate int) device {
		d := watchedDevice{newLoggedDevice(), make(chan struct{})}
		stood <- d
		return d
	}
	var in watchedDevice
	// After outputTo's, which lets the output go of what it holds.
	t.Cleanup(func() {
		if in.loggedDevice != nil {
			<-in.closed
		}
		standIn = func(retry bool, rate int) device { return newNullDevice(retry, rate) }
	})
	opens := make(chan int, 64)
	outputTo(t, func(rate int) device {
		opens <- rate
		// Less than the 40 ms it holds at the interactive latency.
		return &failingDevice{left: 2}
	})
	samples := rising(defaultRate)
	p := NewSound(defaultRate, 1, samples).NewPlayer()
	p.Play()
	defer p.Close()
	waitFor(t, "the output to open", opens)
	in = waitFor(t, "the device to be stood in for", stood)
	in.waitWrites(t, 1)
	select {
	case <-opens:
		t.Error("the device was opened again at once")
	default:
	}
	if Err() != errWontPlay {
		t.Errorf("Err = %v, want why the device won't play", Err())
	}
	// Past the 5 ms it fades in over, it is the song from its start.
	first := in.first()
	if i := rampFrames + 1; first[2*i] != samples[i] {
		t.Errorf("frame %d written to the stand-in = %v, want the song's frame %d, %v", i, first[2*i], i, samples[i])
	}
}
