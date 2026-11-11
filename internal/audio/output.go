package audio

import (
	"errors"
	"runtime"
	"sync"
	"time"
)

// A device is the system's audio output, opened by openDevice to hold
// about ahead of what is heard, and to take frames at rate.
type device interface {
	// write plays interleaved stereo frames after those written before,
	// waiting while the device holds enough already. An error closes the
	// device, and the output opens it again.
	write(samples []float32) error
	close()
}

// idleClose is how long the output stays open with nothing playing.
const idleClose = 5 * time.Second

// deviceCheck is how often the output looks for the system's output device
// changing, to open again on the new one and set it to the music's rate.
const deviceCheck = time.Second

var (
	openOutput    = openDevice // tests replace them
	restoreDevice = restoreDeviceRate
	currentDevice = outputDevice

	wake     = make(chan struct{}, 1)
	runOnce  sync.Once
	testMode bool // tests mix by hand
)

// wakeOutput starts the output, or tells it a voice started. The caller
// holds mu.
func wakeOutput() {
	if testMode {
		return
	}
	runOnce.Do(func() { go run() })
	select {
	case wake <- struct{}{}:
	default:
	}
}

func active() bool {
	mu.Lock()
	defer mu.Unlock()
	return len(voices) > 0 && !testMode
}

// Why mixFor didn't mix.
type reopen int

const (
	mixing      reopen = iota
	stop               // tests mix by hand, or the latency or the device's setting changed
	rateChanged        // the newest player playing is of another rate
)

// reopens counts the times the output was told to open again for a
// setting of its device, under mu.
var reopens int

// mixFor mixes into buf for an output open at latency lat and rate, the
// gen-th time it was told to open, and reports whether anything plays; it
// mixes nothing, and says why, once the output is to open again.
func mixFor(buf []float32, lat Latency, rate, gen int) (busy bool, why reopen) {
	mu.Lock()
	defer mu.Unlock()
	if testMode || latency != lat || reopens != gen {
		return false, stop
	}
	if wantedRate() != rate {
		return false, rateChanged
	}
	return mixLocked(buf), mixing
}

// outputRate returns the rate for the output to open at, and how many
// times it was told to open again.
func outputRate() (rate, gen int) {
	mu.Lock()
	defer mu.Unlock()
	return wantedRate(), reopens
}

// run opens the device while anything plays, and mixes into it.
func run() {
	// Devices that need it, as COM's, stay on one thread.
	runtime.LockOSThread()
	for {
		for !active() {
			<-wake
		}
		// rate is what the music asks for, opened what the device runs at:
		// one that won't run at the music's rate plays it resampled, at
		// the rate it ran at before.
		lat := outputLatency()
		rate, gen := outputRate()
		opened := rate
		dev, n, err := openOutput(lat.ahead(), rate)
		if err != nil && err != errUnsupported && rate != defaultRate {
			opened = defaultRate
			dev, n, err = openOutput(lat.ahead(), opened)
		}
		setRate(opened)
		seen := currentDevice()
		if err != nil {
			setErr(err)
			dev, n = newNullDevice(err != errUnsupported, opened), nullFrames(opened)
			// It holds nothing: what it takes is heard as it takes it.
			setHeld(0)
		} else {
			setErr(nil)
			setHeld(int64(lat.ahead()) * int64(opened) / int64(time.Second))
		}
		buf := make([]float32, n*channels)
		idle, idled, checked := time.Duration(0), false, time.Duration(0)
		per := frames(int64(n), opened)
		for {
			busy, why := mixFor(buf, lat, rate, gen)
			if why == rateChanged {
				// What the device holds of the song before, and its fading
				// out if it was stopped for this one, is heard before the
				// device closes, which would drop it. One silent for as
				// long as it holds has nothing left to wait for.
				for left := lat.ahead() - idle; left > 0; left -= per {
					mixFadingOut(buf)
					if dev.write(buf) != nil {
						break
					}
				}
			}
			if why != mixing {
				break
			}
			if err := dev.write(buf); err != nil {
				// Lost, or a null device trying the real one again.
				break
			}
			if busy {
				idle = 0
			} else if idle += per; idle >= idleClose {
				idled = true
				break
			}
			if checked += per; checked >= deviceCheck {
				checked = 0
				if currentDevice() != seen {
					break
				}
			}
		}
		dev.close()
		if idled {
			restoreDevice()
		}
	}
}

// RestoreDevice puts the system's output device back at the rate it had
// before the output set it to the music's (devicerate.go), as the app
// quits. Only macOS sets it.
func RestoreDevice() { restoreDevice() }

var errUnsupported = errors.New("audio: no audio output on this platform")

// nullFrames returns how many frames a null device takes at a time at
// rate: 10 ms.
func nullFrames(rate int) int { return rate / 100 }

// A nullDevice plays nothing, on time: players still end when they would
// have. One standing in for a device that failed to open fails after a
// while, so that the output tries the device again.
type nullDevice struct {
	rate  int
	next  time.Time
	retry time.Time
	// now and sleep are its clock, which tests replace.
	now   func() time.Time
	sleep func(time.Duration)
}

func newNullDevice(retry bool, rate int) *nullDevice {
	d := &nullDevice{rate: rate, next: time.Now(), now: time.Now, sleep: time.Sleep}
	if retry {
		d.retry = d.next.Add(2 * time.Second)
	}
	return d
}

var errRetry = errors.New("audio: try the device again")

func (d *nullDevice) write(samples []float32) error {
	now := d.now()
	if !d.retry.IsZero() && now.After(d.retry) {
		return errRetry
	}
	if now.Sub(d.next) > 100*time.Millisecond {
		// Behind, as after a sleep: don't hurry to catch up.
		d.next = now
	}
	d.next = d.next.Add(frames(int64(len(samples)/channels), d.rate))
	d.sleep(d.next.Sub(d.now()))
	return nil
}

func (d *nullDevice) close() {}
