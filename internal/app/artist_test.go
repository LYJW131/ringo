package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// Each artist's name opens their page: from Now Playing, which closes, and
// from a song's row, which doesn't play the song as the row's click would.
func TestArtistNameOpensTheirPage(t *testing.T) {
	a := newTestApp()
	duet := music.Track{ID: "1", Name: "广寒宫", Artists: []music.Artist{{ID: "11", Name: "甲"}, {ID: "12", Name: "乙"}}, Duration: 200000}
	a.pl.Cue([]music.Track{duet})
	a.pl.SetLyrics(nil)
	a.artists["12"] = &artistData{page: &music.ArtistPage{
		Artist: music.Artist{ID: "12", Name: "乙"},
		Tracks: []music.Track{{ID: "2", Name: "起风了", Artists: []music.Artist{{ID: "13", Name: "丙"}}, Duration: 200000}},
	}}
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()

	// The second of two artists credited.
	if err := tt.Click("乙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.nowPlaying || a.route != (route{page: pageArtist, src: "fake", id: "12"}) {
		t.Fatalf("after the click: Now Playing open %v, page %+v", a.nowPlaying, a.route)
	}

	tt.Advance(time.Second)
	tt.Frame()
	// Their name, at the start of the line it fills.
	r, ok := tt.Find("丙")
	if !ok {
		t.Fatal("no artist in the row")
	}
	tt.ClickAt(r.X+4, r.Y+r.H/2)
	tt.Frame()
	if a.route != (route{page: pageArtist, src: "fake", id: "13"}) {
		t.Fatalf("a row's artist opened %+v", a.route)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != duet.ID {
		t.Errorf("a click on a row's artist played %+v", cur)
	}

	// Back goes to the artist before.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route.id != "12" {
		t.Errorf("back went to %+v", a.route)
	}
}

// An artist's Albums tab lists their albums, each opening its page, and is
// still chosen on the way back.
func TestArtistAlbums(t *testing.T) {
	a := newTestApp()
	a.artists["12"] = &artistData{page: &music.ArtistPage{
		Artist: music.Artist{ID: "12", Name: "乙"},
		Tracks: []music.Track{song("1", "老歌")},
		Albums: []music.Album{{ID: "8", Name: "迷跡波"}},
	}}
	a.open(route{page: pageArtist, id: "12"})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("迷跡波"); ok {
		t.Fatal("the albums show before their tab is chosen")
	}
	if err := tt.Click("Albums"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("迷跡波"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageAlbum, src: "fake", id: "8"}) {
		t.Fatalf("the album opened %+v", a.route)
	}
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if _, ok := tt.Find("迷跡波"); !ok {
		t.Error("the tab went back to the songs")
	}
}
