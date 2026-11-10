package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// The menu behind a song's dots in Now Playing copies its title, with its
// artists, and the link its source gave it.
func TestSongMenuCopies(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	tr := song("1859", "宝船", "DOUDOU", "小明")
	tr.URL = "https://example.com/song/1859"
	a.pl.Cue([]music.Track{tr})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	for _, c := range []struct{ item, want string }{
		{"Copy Title", "宝船"},
		{"Copy Title and Artists", "宝船 - DOUDOU, 小明"},
		{"Copy Link", "https://example.com/song/1859"},
	} {
		if err := tt.Click("More"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("Share", c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
		if copied != c.want {
			t.Errorf("%s copied %q, want %q", c.item, copied, c.want)
		}
	}
}

// A right click on a song's row opens its menu, without playing it, and a song
// with no page at its source offers no link to copy.
func TestSongMenuOnRightClick(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	linked := song("2", "宝船", "DOUDOU")
	linked.URL = "https://example.com/song/2"
	a.playlists["42"] = &music.Playlist{ID: "42", Name: "飙升榜", Tracks: []music.Track{song("1", "一张相片", "林宥嘉"), linked}}
	a.route = route{page: pagePlaylist, id: "42"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("一张相片"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Share", "Copy Link"); err == nil {
		t.Error("offered the link of a song with none")
	}
	tt.CloseMenu()

	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Share", "Copy Link"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if copied != linked.URL {
		t.Errorf("copied %q, want %q", copied, linked.URL)
	}
	if a.pl.Current() != nil {
		t.Error("the right click played the song")
	}
}

// A song's menu goes to its artist's and album's pages.
func TestSongMenuGoesToPages(t *testing.T) {
	a := newTestApp()
	tr := music.Track{ID: "1", Name: "宝船", Artists: []music.Artist{{ID: "12", Name: "DOUDOU"}}, Album: music.Album{ID: "8", Name: "迷跡波"}, Duration: 200000}
	a.playlists["42"] = &music.Playlist{ID: "42", Name: "飙升榜", Tracks: []music.Track{tr}}
	a.route = route{page: pagePlaylist, id: "42"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Go to Artist: DOUDOU"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	if a.route != (route{page: pageArtist, id: "12"}) {
		t.Fatalf("went to %+v", a.route)
	}
	tt.Key(0, ui.KeyBack)
	tt.Frame()

	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Show Album"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	if a.route != (route{page: pageAlbum, id: "8"}) {
		t.Errorf("went to %+v", a.route)
	}
}
