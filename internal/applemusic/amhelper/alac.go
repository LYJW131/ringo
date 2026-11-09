package amhelper

// plausibleALAC reports whether sample starts as an ALAC packet does: a
// channel pair element (or a single channel's, for mono), instance 0, then
// twelve unused bits, which are zero. A sample decrypted with the wrong key
// is noise, which starts so about once in 130,000 tries; a few in a row
// settle it. The white-box gives no error of its own for a wrong key.
func plausibleALAC(sample []byte) bool {
	if len(sample) < 3 {
		return false
	}
	element := sample[0] >> 5
	instance := (sample[0] >> 1) & 0xF
	unused := uint16(sample[0]&1)<<11 | uint16(sample[1])<<3 | uint16(sample[2]>>5)
	return (element == 0 || element == 1) && instance == 0 && unused == 0
}

// samplesChecked is how many samples a key is checked on, the first time
// it's used.
const samplesChecked = 8

// decryptsALAC reports whether the samples, just decrypted, look like ALAC.
func decryptsALAC(samples [][]byte) bool {
	checked := 0
	for _, s := range samples {
		if checked == samplesChecked {
			break
		}
		if len(s) < 16 {
			continue // too short to be encrypted at all
		}
		if !plausibleALAC(s) {
			return false
		}
		checked++
	}
	return true
}
