package player

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"ringo/internal/music"
)

// decodeAll decodes the whole of a song from its start.
func decodeAll(t *testing.T, b []byte) []float32 {
	t.Helper()
	s, err := openMP4Stream(downloaded(b))
	if err != nil {
		t.Fatal(err)
	}
	var out []float32
	buf := make([]float32, 4096)
	for {
		n, err := s.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil || n == 0 {
			return out
		}
	}
}

// maxDiff returns how far apart a and b are at most, over the length of
// the shorter.
func maxDiff(a, b []float32) float64 {
	d := 0.0
	for i := range min(len(a), len(b)) {
		d = math.Max(d, math.Abs(float64(a[i]-b[i])))
	}
	return d
}

// A song plays as it streams into a file of the cache, rather than the
// app's memory, and the cache keeps it once it has all come in; one cut
// short leaves no file behind.
func TestStreamIntoCache(t *testing.T) {
	b, err := os.ReadFile("testdata/seek-aac-frag.m4a")
	if err != nil {
		t.Fatal(err)
	}
	half := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cut" {
			w.Header().Set("Content-Length", strconv.Itoa(len(b)))
			w.Write(b[:len(b)/2])
			w.(http.Flusher).Flush()
			<-half
			return
		}
		w.Write(b)
	}))
	defer srv.Close()
	defer close(half)
	dir := t.TempDir()
	c := NewCache(dir, 1<<30)
	ref := decodeAll(t, b)

	st := &music.Stream{URL: srv.URL + "/whole", Size: int64(len(b)), Quality: "high", Type: "m4a"}
	s, err := openMP4Stream(c.download("1", "high", st))
	if err != nil {
		t.Fatal(err)
	}
	var got []float32
	buf := make([]float32, 4096)
	for {
		n, err := s.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil || n == 0 {
			break
		}
	}
	s.Close()
	if d := maxDiff(got, ref); len(got) != len(ref) || d != 0 {
		t.Errorf("played %d of %d samples, off by up to %.4f", len(got), len(ref), d)
	}
	if dl, _, ok := c.lookup("1", "high"); !ok {
		t.Error("not kept")
	} else {
		dl.Close()
	}

	cut := c.download("2", "high", &music.Stream{URL: srv.URL + "/cut", Size: int64(len(b)), Quality: "high", Type: "m4a"})
	cut.waitFor(int64(len(b) / 2))
	cut.Close()
	for start := time.Now(); time.Since(start) < 2*time.Second; time.Sleep(10 * time.Millisecond) {
		if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) == 0 {
			break
		}
	}
	if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) != 0 {
		t.Errorf("left %v", parts)
	}
	if _, _, ok := c.lookup("2", "high"); ok {
		t.Error("kept a song cut short")
	}
}

// A stream whose type would lead out of the cache's folder is played but not
// kept, nor is one asked for at a quality that would. They name the file, taken
// as the source sent them, so a type of "/../../x" wrote the song and its JSON
// beside the cache instead.
func TestCacheKeepsOnlyKnownNames(t *testing.T) {
	b, err := os.ReadFile("testdata/seek-aac-frag.m4a")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer srv.Close()
	root := t.TempDir()
	dir := filepath.Join(root, "cache")
	c := NewCache(dir, 1<<30)

	for _, tc := range []struct{ quality, typ string }{{"/../../pwned", "m4a"}, {"high", "/../../pwned"}} {
		dl := c.download("1", tc.quality, &music.Stream{URL: srv.URL, Size: int64(len(b)), Type: tc.typ})
		dl.waitFor(int64(len(b)))
		dl.Close()
		for start := time.Now(); time.Since(start) < 2*time.Second; time.Sleep(10 * time.Millisecond) {
			if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) == 0 {
				break
			}
		}
	}
	var outside []string
	filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && filepath.Dir(path) != dir {
			outside = append(outside, path)
		}
		return nil
	})
	if len(outside) != 0 {
		t.Errorf("wrote %v outside the cache", outside)
	}
	if songs := c.songs(); len(songs) != 0 {
		t.Errorf("kept %v", songs)
	}
}

// A song the cache won't keep sets aside no more memory than maxPrealloc,
// whatever size the API says it is, which it set aside in full before.
func TestStreamSizeIsNotTrusted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("short")) }))
	defer srv.Close()
	dl := startDownload(srv.URL, 1<<40, nil, nil)
	defer dl.Close()
	dl.mu.Lock()
	n := cap(dl.data)
	dl.mu.Unlock()
	if n > maxPrealloc {
		t.Errorf("set aside %d bytes", n)
	}
}

// The spectrum keeps the last seconds decoded, however long the audio: hours
// in, it measures what plays, forgets what played long ago, and measures again
// after a seek back.
func TestSpectrumKeepsRecentChunks(t *testing.T) {
	const rate = 44100
	s := newSpectrum(rate)
	if keep := time.Duration(len(s.ring)) * fftSize * time.Second / rate; keep < spectrumKeep || keep > spectrumKeep+time.Second {
		t.Fatalf("keeps %v of chunks, want about %v", keep, spectrumKeep)
	}
	tone := func(at int64, frames int) {
		stereo := make([]float32, frames*2)
		for i := range frames {
			v := float32(0.5 * math.Sin(2*math.Pi*440*float64(at+int64(i))/rate))
			stereo[2*i], stereo[2*i+1] = v, v
		}
		s.feed(at, stereo)
	}
	measured := func(at time.Duration) bool {
		_, ok := s.at(int64(at.Seconds() * rate))
		return ok
	}

	// Ten hours in, as an audiobook seeks to where it was left.
	start := int64(10 * 60 * 60 * rate)
	tone(start, 30*rate)
	end := time.Duration(start/rate)*time.Second + 30*time.Second
	if !measured(end - time.Second) {
		t.Error("a second before the end decoded isn't measured")
	}
	if measured(end - 25*time.Second) {
		t.Error("25 seconds before the end decoded is still kept")
	}
	if measured(end + time.Second) {
		t.Error("what isn't decoded yet is measured")
	}
	if !measured(end - spectrumKeep + time.Second) {
		t.Errorf("%v before the end decoded isn't kept", spectrumKeep-time.Second)
	}

	// A seek back decodes, and measures, again.
	back := time.Duration(start/rate)*time.Second + 2*time.Second
	tone(int64(back.Seconds()*rate), rate)
	if !measured(back + 500*time.Millisecond) {
		t.Error("what plays after a seek back isn't measured")
	}
}
