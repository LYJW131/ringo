package audio

import (
	"slices"
	"sync/atomic"
	"time"
)

// On macOS, the output also sets the system's output device to the music's
// rate where it can: an Audio Queue at 96 kHz on a device at 48 kHz is
// converted to 48 kHz on its way out. Apple's Music leaves the device as it
// is, by what its listeners say, and they set it in Audio MIDI Setup or
// with a tool that follows the music, as this does. The device is
// put back at the rate it had once the output closes for being idle, or the
// app quits (RestoreDevice): at the rate the listener set it to, if they
// changed it while the music played, and not at all if they changed it
// after the output last did.

// leaveDevice is whether the output leaves the system's output device at
// the rate it has, which the listener may choose: setting it stops other
// apps' sound for a moment.
var leaveDevice atomic.Bool

// SetMatchesDevice sets whether the output sets the system's output device
// to the music's rate, which it does unless told not to, on macOS. Told
// not to, it puts the device back at once; told to again, it opens again,
// to set the device for what plays.
func SetMatchesDevice(on bool) {
	if leaveDevice.Swap(!on) == !on {
		return
	}
	if !on {
		restoreDevice()
		return
	}
	mu.Lock()
	defer mu.Unlock()
	reopens++
}

// A device is heard some time after it is given a frame, which it reports
// in frames at its rate: a few milliseconds for built-in speakers, 160 ms
// for AirPods, two seconds over AirPlay. latencyMost is the most taken at
// its word: past it a device reports nonsense, and is taken to add none,
// as one that reports none.
const latencyMost = 3 * time.Second

// reportedLatency returns how long after a device at rate is given a frame
// it is heard, by the frames of latency it reports in all, or 0 where what
// it reports can't be so.
func reportedLatency(frames uint32, rate float64) time.Duration {
	if frames == 0 || rate <= 0 {
		return 0
	}
	if d := time.Duration(float64(frames) / rate * float64(time.Second)); d <= latencyMost {
		return d
	}
	return 0
}

// commonRates are the rates a device that takes a range of them is tried at,
// besides want itself.
var commonRates = []float64{44100, 48000, 88200, 96000, 176400, 192000, 352800, 384000}

// deviceRate returns the rate to set a device to that takes rates in
// ranges, of [min, max] each, to play music at want: want itself if it can,
// or else one want is a whole multiple of, the highest, as 96 kHz for a
// 192 kHz song on a device of up to 96 kHz, or 44.1 kHz for 88.2 kHz on one
// of 44.1 and 48 kHz, which converts with the least loss; or else a whole
// multiple of want, the lowest; or else the highest it takes. 0 for a
// device that says it takes none.
func deviceRate(want int, ranges [][2]float64) int {
	w := float64(want)
	var rates []float64
	for _, r := range ranges {
		if r[0] <= w && w <= r[1] {
			return want
		}
		rates = append(rates, r[0], r[1])
		for _, c := range commonRates {
			if r[0] < c && c < r[1] {
				rates = append(rates, c)
			}
		}
	}
	if len(rates) == 0 {
		return 0
	}
	slices.Sort(rates)
	whole := func(a, b float64) bool { return b > 0 && a/b == float64(int(a/b)) }
	for _, r := range slices.Backward(rates) {
		if r < w && whole(w, r) {
			return int(r)
		}
	}
	for _, r := range rates {
		if r > w && whole(r, w) {
			return int(r)
		}
	}
	return int(rates[len(rates)-1])
}
