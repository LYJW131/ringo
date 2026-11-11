package audio

import "slices"

// On macOS, the output also sets the system's output device to the music's
// rate where it can, as Apple Music does: an Audio Queue at 96 kHz on a
// device at 48 kHz is converted to 48 kHz on its way out. The device is
// put back at the rate it had once the output closes for being idle, or the
// app quits (RestoreDevice): at the rate the listener set it to, if they
// changed it while the music played, and not at all if they changed it
// after the output last did.

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
