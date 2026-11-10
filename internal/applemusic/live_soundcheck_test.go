package applemusic

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"

	mygomp4 "github.com/egoist/mygo/audio/mp4"

	"ringo/internal/music"
)

// Every song comes with the gain Sound Check plays it at, of the very file
// it plays: without a wrapper the web player's AAC, and with one every
// level and both of a Dolby Atmos version's stereo kinds, measured apart.
// A quiet song is turned up more than a loud one, and a song's peak is the
// one its audio has. Signed in, reading only: APPLE_MUSIC_TOKEN_FILE, and
// APPLE_MUSIC_WRAPPER for what comes through a wrapper.
func TestLiveSoundCheck(t *testing.T) {
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
	asks := []string{"high"}
	if w := os.Getenv("APPLE_MUSIC_WRAPPER"); w != "" {
		s.settings.set("wrapper", w)
		asks = append(asks, "efficient", "lossless", "hires",
			music.Ask("high", music.AtmosHeadphones), music.Ask("high", music.AtmosSpeakers))
	}
	const quiet, loud = "1440721398", "1440736288" // Satie by Pascal Rogé; Metallica
	gains := map[string]float64{}
	for _, id := range []string{"1783644604", "1472563571", "1555813962", quiet, loud} {
		for _, ask := range asks {
			st, err := s.Stream(id, ask)
			if err != nil {
				t.Fatalf("%s as %s: %v", id, ask, err)
			}
			if st.NoAtmos && ask != st.Quality {
				continue // the same as the level, without Dolby Atmos
			}
			t.Logf("%s as %-18s %-15s gain %+6.2f dB, peak %6.2f dBTP", id, ask, st.Quality, st.Gain, 20*math.Log10(st.Peak))
			if st.Peak <= 0 || st.Peak > 2 {
				t.Errorf("%s as %s: no loudness (gain %v, peak %v)", id, ask, st.Gain, st.Peak)
			}
			if ask == "high" {
				gains[id] = st.Gain
			}
		}
	}
	if gains[quiet] < gains[loud]+6 {
		t.Errorf("the quiet song's gain %.2f dB is not well above the loud one's %.2f dB", gains[quiet], gains[loud])
	}

	// The peak the box gives is the true peak of the master Apple encoded
	// from; the AAC decoded, whole, peaks near it.
	st, err := s.Stream("1555813962", "high")
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(st.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	d, err := mygomp4.NewDecoder(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]float32, 8192)
	peak := float32(0)
	for {
		n, err := d.Read(pcm)
		for _, v := range pcm[:n] {
			peak = max(peak, v, -v)
		}
		if err != nil {
			break
		}
	}
	got, said := 20*math.Log10(float64(peak)), 20*math.Log10(st.Peak)
	t.Logf("1555813962 decoded whole peaks at %.2f dBFS; its box says %.2f dBTP", got, said)
	if math.Abs(got-said) > 1 {
		t.Errorf("decoded peak %.2f dBFS, %.2f dB from the box's", got, got-said)
	}
}
