package applemusic

import (
	"math"
	"os"
	"strings"
	"testing"
)

// Every song of an album comes with the album's gain besides its own, and
// the album's is the same for all of them, so that an album played through
// keeps its songs' balance and doesn't jump between them, while their own
// gains differ. Signed in, reading only: APPLE_MUSIC_TOKEN_FILE.
func TestLiveAlbumLoudness(t *testing.T) {
	file := os.Getenv("APPLE_MUSIC_TOKEN_FILE")
	if file == "" {
		t.Skip("APPLE_MUSIC_TOKEN_FILE names a file holding a media-user-token")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	if w := os.Getenv("APPLE_MUSIC_WRAPPER"); w != "" {
		s.settings.set("wrapper", w)
	}
	// The albums of a quiet piano song and a loud rock one: Satie by Pascal
	// Rogé, and Metallica.
	for _, song := range []string{"1440721398", "1440736288"} {
		b, err := s.catalog("/songs/"+song, params{"include": "albums"})
		if err != nil {
			t.Fatal(err)
		}
		r, err := first(b)
		if err != nil {
			t.Fatal(err)
		}
		album := albumIDOf(r)
		page, err := s.Album(album)
		if err != nil {
			t.Fatal(err)
		}
		var albumGains, gains []float64
		for _, tr := range page.Tracks {
			st, err := s.Stream(tr.ID, "high")
			if err != nil {
				t.Fatalf("%s: %v", tr.ID, err)
			}
			t.Logf("album %s song %s: own gain %+6.2f dB, peak %6.2f dBTP; album gain %+6.2f dB, peak %6.2f dBTP",
				album, tr.ID, st.Gain, 20*math.Log10(st.Peak), st.AlbumGain, 20*math.Log10(st.AlbumPeak))
			if st.AlbumPeak <= 0 {
				t.Errorf("%s: no album loudness", tr.ID)
			}
			albumGains, gains = append(albumGains, st.AlbumGain), append(gains, st.Gain)
		}
		spread := func(v []float64) float64 {
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, x := range v {
				lo, hi = min(lo, x), max(hi, x)
			}
			return hi - lo
		}
		if d := spread(albumGains); d != 0 {
			t.Errorf("album %s: its songs' album gains differ by %.2f dB", album, d)
		}
		if d := spread(gains); d < 1 {
			t.Errorf("album %s: its songs' own gains differ by only %.2f dB", album, d)
		}
	}
}
