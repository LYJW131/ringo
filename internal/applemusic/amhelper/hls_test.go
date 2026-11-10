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
		tr, err := resolveTrack(context.Background(), srv.Client(), srv.URL+"/a/master.m3u8", kindALAC, c.maxRate, c.maxBits, &timings{})
		if c.want == 0 {
			if !errors.Is(err, errNoAudio) {
				t.Errorf("limits %d/%d: %v, want no ALAC", c.maxRate, c.maxBits, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if tr.SampleRate != c.want || tr.BitDepth != 24 || tr.Codec != "alac" {
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
	if _, err := resolveTrack(context.Background(), srv.Client(), srv.URL+"/a/master.m3u8", kindALAC, 0, 0, &timings{}); !errors.Is(err, errNoAudio) {
		t.Fatalf("%v, want no ALAC", err)
	}
}

// The groups of a song with a Dolby Atmos version, as Apple's master
// playlists list them, in their order.
const atmosMaster = `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-stereo-256-downmix",NAME="songEnhanced",AUTOSELECT=YES,CHANNELS="2/-/DOWNMIX"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-stereo-256",NAME="songEnhanced",AUTOSELECT=YES,CHANNELS="2"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-stereo-256-binaural",NAME="songEnhanced",AUTOSELECT=YES,CHANNELS="2/-/BINAURAL"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-atmos-2768",NAME="songEnhanced",AUTOSELECT=YES,CHANNELS="16/JOC"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-alac-stereo-44100-24",NAME="songEnhanced",AUTOSELECT=YES,CHANNELS="2",SAMPLE-RATE="44100",BIT-DEPTH="24"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-stereo-128-binaural",NAME="songEnhanced",AUTOSELECT=YES,CHANNELS="2/-/BINAURAL"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-HE-stereo-64",NAME="songEnhanced",AUTOSELECT=YES,CHANNELS="2"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-HE-stereo-64-binaural",NAME="songEnhanced",AUTOSELECT=YES,CHANNELS="2/-/BINAURAL"
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=261490,BANDWIDTH=279389,CODECS="mp4a.40.2",AUDIO="audio-stereo-256-downmix"
downmix256.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=261817,BANDWIDTH=278324,CODECS="mp4a.40.2",AUDIO="audio-stereo-256"
aac.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=259790,BANDWIDTH=277076,CODECS="mp4a.40.2",AUDIO="audio-stereo-256-binaural"
binaural256.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=770129,BANDWIDTH=770601,CODECS="ec-3",AUDIO="audio-atmos-2768"
atmos.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=1447038,BANDWIDTH=1545171,CODECS="alac",AUDIO="audio-alac-stereo-44100-24"
alac44.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=133258,BANDWIDTH=141473,CODECS="mp4a.40.2",AUDIO="audio-stereo-128-binaural"
binaural128.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=73900,BANDWIDTH=84698,CODECS="mp4a.40.5",AUDIO="audio-HE-stereo-64"
he64.m3u8
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=72814,BANDWIDTH=84937,CODECS="mp4a.40.5",AUDIO="audio-HE-stereo-64-binaural"
hebinaural64.m3u8
`

// Each kind is its own group: the binaural and downmix versions of the
// Dolby Atmos mix at 256 kbps rather than 128, never the plain AAC or the
// E-AC-3 itself, HE-AAC without a binaural one, and ALAC as before.
func TestPickVariantByKind(t *testing.T) {
	variants := parseMaster(atmosMaster, "https://aod.example/a/master.m3u8")
	for kind, want := range map[string]string{
		kindALAC:     "audio-alac-stereo-44100-24",
		kindBinaural: "audio-stereo-256-binaural",
		kindDownmix:  "audio-stereo-256-downmix",
		kindHE:       "audio-HE-stereo-64",
	} {
		if v := pickVariant(variants, kind, 0, 0); v == nil || v.audio != want {
			t.Errorf("kind %q: %+v, want %s", kind, v, want)
		}
	}
	// A song without a Dolby Atmos version has neither of its versions, and
	// an older one, AAC at 64 kbps where others have HE-AAC.
	plain := parseMaster(master+"#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=75107,BANDWIDTH=80330,CODECS=\"mp4a.40.2\",AUDIO=\"audio-stereo-64\"\naac64.m3u8\n", "https://aod.example/a/master.m3u8")
	for _, kind := range []string{kindBinaural, kindDownmix} {
		if v := pickVariant(plain, kind, 0, 0); v != nil {
			t.Errorf("kind %q of a song without Dolby Atmos: %s", kind, v.audio)
		}
	}
	if v := pickVariant(plain, kindHE, 0, 0); v == nil || v.audio != "audio-stereo-64" {
		t.Errorf("HE-AAC of an older song: %+v", v)
	}
}

// The AAC kinds come with what Apple names them for, and their samples
// are checked as AAC, not ALAC, whose packets start otherwise.
func TestResolveTrackOfAAC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a/master.m3u8":
			_, _ = w.Write([]byte(atmosMaster))
		case "/a/binaural256.m3u8":
			_, _ = w.Write([]byte(media))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	tr, err := resolveTrack(context.Background(), srv.Client(), srv.URL+"/a/master.m3u8", kindBinaural, 0, 0, &timings{})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Codec != "aac" || tr.BitRate != 256000 || tr.SampleRate != 0 {
		t.Errorf("%+v, want AAC at 256 kbps", tr)
	}
	if !slices.Equal(tr.Keys, []string{prefetchKey, "skd://itunes.apple.com/p1480208481/c6"}) {
		t.Errorf("keys %q", tr.Keys)
	}
}

// A key is checked on samples as their codec starts them: Apple's AAC with
// a channel pair element, which ALAC's check would take for noise, and the
// other way round; noise passes neither.
func TestDecryptChecksByCodec(t *testing.T) {
	sample := func(head ...byte) []byte { return append(head, make([]byte, 32)...) }
	aac := [][]byte{sample(0x21, 0x0c), sample(0x21, 0x2c), sample(0x21, 0x4e)}
	alac := [][]byte{sample(0x20, 0x00, 0x13), sample(0x20, 0x00, 0x12)}
	noise := [][]byte{sample(0xc2, 0x58), sample(0x94, 0xe0), sample(0x40, 0xfc)}
	if !decrypts("aac")(aac) || decrypts("aac")(noise) {
		t.Error("AAC's check")
	}
	if !decrypts("alac")(alac) || decrypts("alac")(aac) || decrypts("alac")(noise) {
		t.Error("ALAC's check")
	}
}
