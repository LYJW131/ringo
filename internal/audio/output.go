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

// A flusher is a device that can stop at once: flush fades out what it
// plays over a few tens of milliseconds and drops what it holds, and what
// is written next is heard as soon as it is written. Only macOS's can.
type flusher interface {
	flush() error
}

// A holder is a device that knows how many frames it holds, written and
// not yet heard, just after it took a write, and whether it is sure of
// them: one that measures them is only after a while. A device without it
// is taken to hold what it was opened to, which the Audio Queue holds more
// than: a player's position ran some 20 ms ahead of what was heard.
type holder interface {
	holds() (frames int64, sure bool)
}

// heldBy returns the frames a device holds ahead of what is heard, on
// average, which holds frames after a write of n: the output mixes the
// next n at once, and the device plays as many while that write waits.
func heldBy(holds int64, n int) int64 { return holds + int64(n)/2 }

// idleClose is how long the output stays open with nothing playing.
const idleClose = 5 * time.Second

// deviceCheck is how often the output looks for the system's output device
// changing, to open again on the new one and set it to the music's rate.
const deviceCheck = time.Second

var (
	openOutput    = openDevice // tests replace them
	restoreDevice = restoreDeviceRate
	currentDevice = outputDevice
	// parkFor is the clock a device holding nothing waits by.
	parkFor = func(d time.Duration) <-chan time.Time { return time.After(d) }

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

// The device open, under mu: whether it can stop at once (flusher), and
// whether a player asked it to (PauseAtOnce).
var (
	flushes    bool
	flushAsked bool
)

// setFlushes tells the mixer whether the device open can stop at once.
func setFlushes(can bool) {
	mu.Lock()
	defer mu.Unlock()
	flushes, flushAsked = can, false
}

// takeFlush reports whether a player asked the device to stop at once,
// once for each time one did.
func takeFlush() bool {
	mu.Lock()
	defer mu.Unlock()
	asked := flushAsked
	flushAsked = false
	return asked
}

// parkedFor says what a device holding nothing does next, open at latency
// lat and rate the gen-th time: wait, while no player plays, or open
// again, as mixFor would have it; else it mixes on.
func parkedFor(lat Latency, rate, gen int) (wait, reopen bool) {
	mu.Lock()
	defer mu.Unlock()
	if testMode || latency != lat || reopens != gen {
		return false, true
	}
	for _, v := range voices {
		if v.playing && !v.closed {
			return false, false
		}
	}
	return true, false
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
		}
		// What the device holds, as it says, or else as it was asked to;
		// one not sure yet is asked again as it plays.
		held, sure := int64(lat.ahead())*int64(opened)/int64(time.Second), true
		h, _ := dev.(holder)
		if err == nil {
			setErr(nil)
			if h != nil {
				var frames int64
				frames, sure = h.holds()
				held = heldBy(frames, n)
			}
			setHeld(held)
		}
		buf := make([]float32, n*channels)
		idle, idled, checked := time.Duration(0), false, time.Duration(0)
		per := frames(int64(n), opened)
		f, _ := dev.(flusher)
		setFlushes(f != nil && err == nil)
		// parked is the device holding nothing, after it stopped at once:
		// it is written no silence, so that what plays next is heard as
		// soon as it's mixed, not behind what the device holds ahead.
		// empty is its holding nothing still, until it is written to.
		parked, empty := false, false
		for {
			if takeFlush() && f != nil {
				if f.flush() != nil {
					break
				}
				rewindHeard()
				parked, empty = true, true
			}
			if parked {
				// Until something plays, or the device has been idle
				// for as long as it stays open.
				if wait, reopen := parkedFor(lat, rate, gen); reopen {
					break
				} else if wait {
					start := time.Now()
					select {
					case <-wake:
					case <-parkFor(idleClose - idle):
						idle = idleClose
					}
					if idle += time.Since(start); idle >= idleClose {
						idled = true
						break
					}
					if currentDevice() != seen {
						break
					}
					continue
				}
				parked = false
			}
			busy, why := mixFor(buf, lat, rate, gen)
			if why == rateChanged {
				// What the device holds of the song before, and its fading
				// out if it was stopped for this one, is heard before the
				// device closes, which would drop it. One silent for as
				// long as it holds, or holding nothing, has nothing left
				// to wait for.
				for left := lat.ahead() - idle; left > 0 && !empty; left -= per {
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
			empty = false
			if !sure {
				var frames int64
				if frames, sure = h.holds(); heldBy(frames, n) != held {
					held = heldBy(frames, n)
					changeHeld(held)
				}
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
		setFlushes(false)
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
