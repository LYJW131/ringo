package applemusic

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"ringo/internal/music"
	vid "ringo/internal/video"
)

// With APPLE_MUSIC_TOKEN_FILE, reading only: a card of Home's picks and an
// album's page each have a moving cover that fetches and decodes, its
// variant the size the app asks for. What's printed is the videos' sizes
// and rates, not what they are of.
func TestLiveMotion(t *testing.T) {
	file := os.Getenv("APPLE_MUSIC_TOKEN_FILE")
	if file == "" {
		t.Skip("APPLE_MUSIC_TOKEN_FILE names a file holding a media-user-token")
	}
	if !vid.Supported() {
		t.Skip("no decoder on this machine")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))

	var tall string
	home, err := s.Home()
	if err != nil {
		t.Fatal(err)
	}
	for _, sec := range home.Sections {
		for _, it := range sec.Items {
			if sec.Style == music.ShelfPortrait && it.TallVideoURL != "" && tall == "" {
				tall = it.TallVideoURL
			}
		}
	}
	// An album with one in China's catalog and the US's, whichever the
	// account is in.
	var square string
	var page *music.AlbumPage
	for _, id := range []string{"1889992111", "1440935467"} {
		if p, err := s.Album(id); err == nil && p.Album.VideoURL != "" {
			square, page = p.Album.VideoURL, p
			break
		}
	}
	// What the app asks of a cover a page didn't say moves: by the album,
	// and by one of its songs, whose album most answers leave out.
	if page != nil && len(page.Tracks) > 0 {
		for _, c := range []struct{ kind, id string }{{music.ItemAlbum, page.Album.ID}, {music.ItemSong, page.Tracks[0].ID}} {
			m, err := s.Motion(c.kind, []string{c.id})
			if err != nil || m[c.id] != square {
				t.Errorf("Motion of a %s: %v, %d answered, same video %v", c.kind, err, len(m), m[c.id] == square)
			}
		}
	}
	for _, c := range []struct {
		name, url string
		w, h      int
		max       [2]int
	}{{"a card of Home", tall, 600, 800, [2]int{436, 582}}, {"an album's head", square, 464, 464, [2]int{486, 486}}} {
		if c.url == "" {
			t.Errorf("%s: no moving cover", c.name)
			continue
		}
		start := time.Now()
		data, err := vid.Fetch(c.url, c.w, c.h, c.max[0], c.max[1], music.Fetch)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		fetched := time.Since(start)
		d, err := vid.Open(data)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		n := 0
		var size string
		var frame time.Duration
		for n < 90 {
			f, err := d.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("%s: frame %d: %v", c.name, n, err)
				break
			}
			size, frame = f.Image.Bounds().Size().String(), f.Duration
			n++
		}
		d.Close()
		if n < 90 {
			t.Errorf("%s: %d frames", c.name, n)
		}
		t.Logf("%s: %d KB fetched in %v, %d frames of %s at %.2f a second", c.name, len(data)>>10, fetched.Round(time.Millisecond), n, size, float64(time.Second)/float64(frame))
	}
}
