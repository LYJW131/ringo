package applemusic

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/egoist/mygo/audio/aac" // AAC in MP4, as the player has it
	"github.com/egoist/mygo/audio/mp4"

	"mygo-music/internal/music"
)

// With APPLE_MUSIC_TOKEN_FILE, the library's albums the catalog doesn't
// have, of two kinds: those the listener uploaded, whose covers are kept and
// whose songs play through amhelper, starting within 8 seconds and coming
// at more than twice the speed they play at, and read through the player's
// decoder from the start and from the middle; and those the catalog no
// longer has, greyed out, saying why. Nothing is written, and no name, link or count that says whose the
// library is is printed, beyond how many albums of each kind.
func TestLiveUploads(t *testing.T) {
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
	if err := s.needAccount(); err != nil {
		t.Fatal(err)
	}

	// The albums without one of the catalog's, each page asked for with the
	// catalog's, as the link to the next page leaves it out.
	var orphans []*resource
	total := 0
	for off := 0; ; off += 100 {
		b, err := s.me("/library/albums", params{"limit": "100", "include": "catalog", "offset": strconv.Itoa(off)}, reqOpts{})
		if err != nil {
			t.Fatal(err)
		}
		total += len(b.Data)
		for _, a := range b.Data {
			if a != nil && len(a.related("catalog").Data) == 0 {
				orphans = append(orphans, a)
			}
		}
		if b.Next == "" || len(b.Data) == 0 {
			break
		}
	}

	var uploads, gone, elsewhere []*music.AlbumPage
	for _, a := range orphans {
		page, err := s.Album(a.ID)
		if err != nil {
			t.Fatalf("an album's page: %v", clean(err))
		}
		up, greyed := 0, 0
		for _, tr := range page.Tracks {
			if tr.Uploaded {
				up++
			}
			if tr.Unavailable {
				greyed++
			}
		}
		switch {
		case up > 0:
			uploads = append(uploads, page)
			if up+greyed != len(page.Tracks) {
				t.Logf("an uploaded album has %d of its %d songs from the catalog", len(page.Tracks)-up-greyed, len(page.Tracks))
			}
		case greyed == len(page.Tracks):
			gone = append(gone, page)
		default:
			elsewhere = append(elsewhere, page)
		}
	}
	t.Logf("%d albums, %d without the catalog's: %d uploaded, %d no longer in the catalog, %d of songs the catalog has by ID but not here",
		total, len(orphans), len(uploads), len(gone), len(elsewhere))
	if len(uploads) == 0 {
		t.Fatal("no uploaded album")
	}

	// Covers: each uploaded album's, kept by amhelper, then read back.
	for _, p := range uploads {
		s.keptArt(&artwork{URL: imageOf(t, s, p.Album.ID)})
	}
	shown := 0
	deadline := time.Now().Add(5 * time.Minute)
	for _, p := range uploads {
		var u string
		for u == "" && time.Now().Before(deadline) {
			if u = s.keptArt(&artwork{URL: imageOf(t, s, p.Album.ID)}); u == "" {
				time.Sleep(2 * time.Second)
			}
		}
		path, ok := music.PathOf(u)
		if !ok {
			t.Errorf("a cover wasn't kept in 5 minutes")
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			t.Error(err)
			continue
		}
		cfg, format, err := image.DecodeConfig(f)
		f.Close()
		if err != nil {
			t.Errorf("a kept cover: %v", err)
			continue
		}
		shown++
		t.Logf("a cover kept: %s, %dx%d", format, cfg.Width, cfg.Height)
	}
	t.Logf("%d of %d uploaded albums' covers kept and read", shown, len(uploads))

	// Uploaded songs play, at Lossless too, as AAC through amhelper.
	var songs []music.Track
	for _, p := range uploads {
		for _, tr := range p.Tracks {
			if tr.Uploaded {
				songs = append(songs, tr)
			}
		}
	}
	if len(songs) < 2 {
		t.Fatal("fewer than two uploaded songs")
	}

	// One, read as the player reads it, from the start to the end.
	t0 := time.Now()
	st, err := s.Stream(songs[0].ID, "lossless")
	if err != nil {
		t.Fatalf("an uploaded song: %v", clean(err))
	}
	t.Logf("stream: %s, quality %s, %d bit/s, %.1f MB, through amhelper: %v", st.Type, st.Quality, st.BitRate, float64(st.Size)/1e6, strings.HasPrefix(st.URL, "http://127.0.0.1:"))
	res, err := http.Get(st.URL)
	if err != nil {
		t.Fatalf("its file: %v", clean(err))
	}
	var got int64
	var first time.Duration
	chunk := make([]byte, 64<<10)
	for {
		n, err := res.Body.Read(chunk)
		got += int64(n)
		if first == 0 && got >= 256<<10 {
			first = time.Since(t0)
		}
		if err != nil {
			break
		}
	}
	res.Body.Close()
	all := time.Since(t0)
	speed := float64(got) / 1024 / all.Seconds()
	t.Logf("the first 256 KB in %.1f s; all %.1f MB in %.0f s, at %.0f KB/s", first.Seconds(), float64(got)/1e6, all.Seconds(), speed)
	if got != st.Size {
		t.Errorf("%d bytes of %d", got, st.Size)
	}
	if first == 0 || first > 8*time.Second {
		t.Error("its start took more than 8 seconds")
	}
	if speed < 64 {
		t.Error("it came at less than twice the speed it plays at")
	}
	// Asked for again, it's kept.
	t0 = time.Now()
	if st, err = s.Stream(songs[0].ID, "high"); err == nil {
		if res, err := http.Get(st.URL); err == nil {
			n, _ := io.Copy(io.Discard, res.Body)
			res.Body.Close()
			t.Logf("again: %.1f MB in %.1f s", float64(n)/1e6, time.Since(t0).Seconds())
		}
	}

	// Another, read through the player's decoder, from the start and from
	// the middle, before the rest of it has come.
	st, err = s.Stream(songs[1].ID, "high")
	if err != nil {
		t.Fatalf("another uploaded song: %v", clean(err))
	}
	r := &rangeFile{url: st.URL}
	head := make([]byte, 64<<10)
	if _, err := r.ReadAt(head, 0); err != nil {
		t.Fatalf("its start: %v", clean(err))
	}
	t.Logf("boxes at the start: %s", topBoxes(head))
	d, err := mp4.NewDecoder(r)
	if err != nil {
		t.Fatalf("the player's decoder: %v", clean(err))
	}
	buf := make([]float32, 2*d.SampleRate())
	loud := func(seconds int) (int, float64) {
		frames, peak := 0, 0.0
		for frames < seconds*d.SampleRate() {
			n, err := d.Read(buf)
			frames += n / 2
			for _, v := range buf[:n] {
				peak = math.Max(peak, math.Abs(float64(v)))
			}
			if err != nil {
				break
			}
		}
		return frames, peak
	}
	frames, peak := loud(3)
	t.Logf("from the start: %.2f s decoded at %d Hz, peak %.2f; %.0f s long", float64(frames)/float64(d.SampleRate()), d.SampleRate(), peak, float64(d.Len())/float64(d.SampleRate()))
	if frames < 2*d.SampleRate() || peak == 0 {
		t.Error("no sound from the start")
	}
	t0 = time.Now()
	if err := d.SetPosition(d.Len() / 2); err != nil {
		t.Fatalf("seeking to the middle: %v", clean(err))
	}
	frames, peak = loud(2)
	t.Logf("from the middle, %.1f s after seeking: %.2f s decoded, peak %.2f", time.Since(t0).Seconds(), float64(frames)/float64(d.SampleRate()), peak)
	if frames < d.SampleRate() || peak == 0 {
		t.Error("no sound from the middle")
	}

	// What the catalog no longer has is greyed out, and asked for, says why.
	if len(gone) > 0 {
		_, err := s.Stream(gone[0].Tracks[0].ID, "high")
		t.Logf("a song no longer in the catalog: greyed out; played: %v", clean(err))
		if !errors.Is(err, music.ErrUnavailable) || !strings.Contains(err.Error(), "country or region") {
			t.Error("it doesn't say why it doesn't play")
		}
	}
	// What the catalog has by ID but not in this storefront: what the web
	// player's playback answers today, without asking for a key.
	if len(elsewhere) > 0 {
		for _, tr := range elsewhere[0].Tracks {
			if isCatalogID(tr.ID) {
				_, err := s.webPlayback(tr.ID)
				t.Logf("a song the catalog has by ID but not here: greyed out %v; its playback: %v", tr.Unavailable, clean(err))
				break
			}
		}
	}
}

// imageOf is the library album's cover link as Apple answers it now.
func imageOf(t *testing.T, s *Source, id string) string {
	t.Helper()
	b, err := s.me("/library/albums/"+id, nil, reqOpts{})
	if err != nil {
		t.Fatal(clean(err))
	}
	item, err := first(b)
	if err != nil || item.attrs().Artwork == nil {
		t.Fatal("no cover")
	}
	return item.attrs().Artwork.URL
}

// clean is err without the signed query of any link in it.
func clean(err error) string {
	if err == nil {
		return "no error"
	}
	words := strings.Fields(err.Error())
	for i, w := range words {
		if j := strings.Index(w, "?"); j >= 0 && strings.Contains(w, "://") {
			words[i] = w[:j] + "?…"
		}
	}
	return strings.Join(words, " ")
}

// topBoxes names the MP4 boxes that begin in b, in order.
func topBoxes(b []byte) string {
	var out []string
	for off := 0; off+8 <= len(b); {
		n := int(binary.BigEndian.Uint32(b[off:]))
		out = append(out, string(b[off+4:off+8]))
		if n < 8 {
			break
		}
		off += n
	}
	return strings.Join(out, ", ")
}

// rangeFile reads a file on the web by ranges, a quarter of a megabyte at a
// time, as a file the decoder can seek in.
type rangeFile struct {
	url    string
	size   int64
	pos    int64
	asked  int
	chunks map[int64][]byte
}

const rangeChunk = 256 << 10

func (r *rangeFile) chunk(i int64) ([]byte, error) {
	if c, ok := r.chunks[i]; ok {
		return c, nil
	}
	req, _ := http.NewRequest("GET", r.url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", i*rangeChunk, (i+1)*rangeChunk-1))
	res, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("HTTP %d for a range", res.StatusCode)
	}
	var total int64
	if _, err := fmt.Sscanf(res.Header.Get("Content-Range")[strings.Index(res.Header.Get("Content-Range"), "/")+1:], "%d", &total); err == nil {
		r.size = total
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	r.asked++
	if r.chunks == nil {
		r.chunks = map[int64][]byte{}
	}
	r.chunks[i] = b
	return b, nil
}

func (r *rangeFile) ReadAt(b []byte, off int64) (int, error) {
	n := 0
	for n < len(b) {
		at := off + int64(n)
		if r.size > 0 && at >= r.size {
			return n, io.EOF
		}
		c, err := r.chunk(at / rangeChunk)
		if err != nil {
			return n, err
		}
		in := int(at % rangeChunk)
		if in >= len(c) {
			return n, io.EOF
		}
		n += copy(b[n:], c[in:])
	}
	return n, nil
}

func (r *rangeFile) Read(b []byte) (int, error) {
	n, err := r.ReadAt(b, r.pos)
	r.pos += int64(n)
	return n, err
}

func (r *rangeFile) Seek(off int64, whence int) (int64, error) {
	if r.size == 0 {
		if _, err := r.chunk(0); err != nil {
			return 0, err
		}
	}
	switch whence {
	case io.SeekCurrent:
		off += r.pos
	case io.SeekEnd:
		off += r.size
	}
	r.pos = off
	return off, nil
}
