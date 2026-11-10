package applemusic

import (
	"ringo/internal/applemusic/amhelper"
	"ringo/internal/music"
)

// soundCheckLKFS is how loud Sound Check plays every song: -16 LKFS, by
// ITU-R BS.1770, the level Apple Music's is known to bring songs to since
// it moved to LUFS in 2022, and the measure Apple gives of each song
// (amhelper.Loudness).
const soundCheckLKFS = -16

// soundCheck sets the stream's gain to bring it to soundCheckLKFS, and its
// peak, by the loudness Apple measured of the very file it plays, and the
// same of its album. A song it says nothing of is left as it is.
func soundCheck(st *music.Stream, l *amhelper.Loudness) {
	if l == nil || l.LKFS == 0 {
		return
	}
	st.Gain = soundCheckLKFS - l.LKFS
	st.Peak = l.Peak
	if a := l.Album; a != nil && a.LKFS != 0 {
		st.AlbumGain = soundCheckLKFS - a.LKFS
		st.AlbumPeak = a.Peak
	}
}
