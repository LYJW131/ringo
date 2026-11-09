package amhelper

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// The shape of an enhanced-HLS master playlist and an ALAC media playlist.
const master = `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-stereo-256",NAME="AAC",SAMPLE-RATE="44100",BIT-DEPTH="16"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-alac-stereo-48000-24",NAME="ALAC",SAMPLE-RATE="48000",BIT-DEPTH="24"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-alac-stereo-96000-24",NAME="ALAC",SAMPLE-RATE="96000",BIT-DEPTH="24"
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=262000,BANDWIDTH=270000,CODECS="mp4a.40.2",AUDIO="audio-stereo-256"
aac.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=1874567,BANDWIDTH=1976830,CODECS="alac",AUDIO="audio-alac-stereo-48000-24"
alac48.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=3138931,BANDWIDTH=3300000,CODECS="alac",AUDIO="audio-alac-stereo-96000-24"
alac96.m3u8
`

const media = `#EXTM3U
#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://itunes.apple.com/P000000000/s1/e1",KEYFORMAT="com.apple.streamingkeydelivery"
#EXT-X-MAP:URI="song_m.mp4",BYTERANGE="1023@0"
#EXTINF:14.93333,
#EXT-X-BYTERANGE:3287062@1023
song_m.mp4
#EXT-X-KEY:METHOD=SAMPLE-AES,URI="skd://itunes.apple.com/p1480208481/c6",KEYFORMAT="com.apple.streamingkeydelivery"
#EXTINF:14.93333,
song_m.mp4
`

func playlists(t *testing.T, masterBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a/master.m3u8":
			_, _ = w.Write([]byte(masterBody))
		case "/a/alac48.m3u8", "/a/alac96.m3u8":
			_, _ = w.Write([]byte(media))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveTrackPicksTheBestALACWithinLimits(t *testing.T) {
	srv := playlists(t, master)
	for _, c := range []struct{ maxRate, maxBits, want int }{
		{0, 0, 96000},
		{48000, 0, 48000},
		{0, 16, 0},
	} {
		tr, err := resolveTrack(context.Background(), srv.Client(), srv.URL+"/a/master.m3u8", c.maxRate, c.maxBits, &timings{})
		if c.want == 0 {
			if !errors.Is(err, errNoALAC) {
				t.Errorf("limits %d/%d: %v, want no ALAC", c.maxRate, c.maxBits, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if tr.SampleRate != c.want || tr.BitDepth != 24 {
			t.Errorf("limits %d/%d: %d Hz / %d bit, want %d Hz", c.maxRate, c.maxBits, tr.SampleRate, tr.BitDepth, c.want)
		}
		if tr.MediaURL != srv.URL+"/a/song_m.mp4" {
			t.Errorf("media %s", tr.MediaURL)
		}
		if math.Abs(tr.Seconds-29.86666) > 1e-9 {
			t.Errorf("%v s, want the segments' 29.86666", tr.Seconds)
		}
		if !slices.Equal(tr.Keys, []string{prefetchKey, "skd://itunes.apple.com/p1480208481/c6"}) {
			t.Errorf("keys %q", tr.Keys)
		}
	}
}

func TestResolveTrackWithoutALAC(t *testing.T) {
	srv := playlists(t, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,AUDIO=\"audio-stereo-256\"\naac.m3u8\n")
	if _, err := resolveTrack(context.Background(), srv.Client(), srv.URL+"/a/master.m3u8", 0, 0, &timings{}); !errors.Is(err, errNoALAC) {
		t.Fatalf("%v, want no ALAC", err)
	}
}
