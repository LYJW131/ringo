package amhelper

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// The headers in testdata/loudness are the ftyp and moov of Apple's files of
// three songs, as they come: a loud one, its AAC and its ALAC; one with a
// Dolby Atmos version, its stereo, binaural and downmix measured apart; and
// one at Sound Check's level.

// Each file's loudness and peak are read from its tlou, and its album's
// from its alou: its program loudness, and its true peak. The songs out as
// singles are their own albums; 1555813962 is on an album louder than it.
func TestReadLoudness(t *testing.T) {
	for _, c := range []struct {
		file                   string
		lkfs, peakDB           float64
		albumLKFS, albumPeakDB float64
	}{
		{"1783644604-audio-stereo-256.mp4", -5, -0.15625, -5, -0.15625},
		{"1783644604-audio-alac-stereo-48000-24.mp4", -5, -0.15625, -5, -0.15625},
		{"1472563571-audio-stereo-256.mp4", -7.75, -0.4375, -7.75, -0.4375},
		{"1472563571-audio-stereo-256-binaural.mp4", -15.75, -0.15625, -15.75, -0.15625},
		{"1472563571-audio-stereo-256-downmix.mp4", -17.5, -1.15625, -17.5, -1.15625},
		{"1555813962-audio-stereo-256.mp4", -16, -4.4375, -13.75, -2.71875},
	} {
		head, err := os.ReadFile("testdata/loudness/" + c.file)
		if err != nil {
			t.Fatal(err)
		}
		l, ok := readLoudness(head)
		if !ok || l.LKFS != c.lkfs || math.Abs(20*math.Log10(l.Peak)-c.peakDB) > 1e-9 {
			t.Errorf("%s: %+v (%.4f dB), %v; want %v LKFS, %v dB", c.file, l, 20*math.Log10(l.Peak), ok, c.lkfs, c.peakDB)
		}
		if a := l.Album; a == nil || a.LKFS != c.albumLKFS || math.Abs(20*math.Log10(a.Peak)-c.albumPeakDB) > 1e-9 {
			t.Errorf("%s: album %+v; want %v LKFS, %v dB", c.file, a, c.albumLKFS, c.albumPeakDB)
		}
	}
	// A header read without any says so of the album too, to be kept as
	// looked for.
	if l, ok := readLoudness([]byte("\x00\x00\x00\x08ftyp")); ok || l.Album == nil {
		t.Errorf("a header without a moov: %+v, %v", l, ok)
	}
}

// A tlou with a set for a downmix besides the audio as it is reads the
// latter, wherever it comes; one cut short reads as none.
func TestParseTlouSets(t *testing.T) {
	set := func(downmix byte, lkfs byte) []byte {
		// eq, then reserved, downmix and DRC in 16 bits, the peaks, how
		// they were measured, and one measurement.
		return []byte{0, downmix >> 2, downmix << 6, 0x28, 0xa2, 0x85, 0x23, 1, programLoudness, lkfs, 0x23}
	}
	b := append([]byte{1, 0, 0, 0, 2}, set(5, 100)...)
	b = append(b, set(0, 200)...)
	if l, ok := parseTlou(b); !ok || l.LKFS != -7.75 {
		t.Errorf("%+v %v, want the set without a downmix, at -7.75 LKFS", l, ok)
	}
	if _, ok := parseTlou(b[:len(b)-2]); ok {
		t.Error("a tlou cut short was read")
	}
}

// Working out a song reads its file's header, as long as the playlist says
// it is, for its loudness; a file whose header doesn't come still plays,
// without one.
func TestResolveTrackReadsLoudness(t *testing.T) {
	head, err := os.ReadFile("testdata/loudness/1783644604-audio-alac-stereo-48000-24.mp4")
	if err != nil {
		t.Fatal(err)
	}
	var ranges []string
	serveHead := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a/master.m3u8":
			_, _ = w.Write([]byte(master))
		case "/a/alac48.m3u8", "/a/alac96.m3u8":
			_, _ = w.Write([]byte(media))
		case "/a/song_m.mp4":
			ranges = append(ranges, r.Header.Get("Range"))
			if !serveHead {
				http.NotFound(w, r)
				return
			}
			body := append(append([]byte{}, head...), strings.Repeat("\x00", 4096)...)
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(string(body)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	tr, err := resolveTrack(context.Background(), srv.Client(), srv.URL+"/a/master.m3u8", kindALAC, 48000, 0, &timings{})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Loudness == nil || tr.Loudness.LKFS != -5 || tr.Loudness.Album == nil || tr.Loudness.Album.LKFS != -5 || len(ranges) != 1 || ranges[0] != "bytes=0-1022" {
		t.Errorf("loudness %+v, asked for %q", tr.Loudness, ranges)
	}
	serveHead = false
	tr, err = resolveTrack(context.Background(), srv.Client(), srv.URL+"/a/master.m3u8", kindALAC, 48000, 0, &timings{})
	if err != nil || tr.Loudness != nil || tr.MediaURL == "" {
		t.Errorf("without its header: %+v, %v", tr, err)
	}
}
