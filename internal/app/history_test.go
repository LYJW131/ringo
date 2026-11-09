package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// The side buttons of a mouse go back and forward between pages.
func TestBackAndForward(t *testing.T) {
	a := newTestApp()
	a.playlists["42"] = &music.Playlist{ID: "42", Name: "飙升榜", Tracks: []music.Track{{ID: "1", Name: "一张相片"}}}
	a.playlists["43"] = &music.Playlist{ID: "43", Name: "睡前", Tracks: []music.Track{{ID: "2", Name: "宝船"}}}
	tt := ui.NewTester(a.view, 1200, 800)
	other, list := route{page: pagePlaylist, src: "fake", id: "43"}, route{page: pagePlaylist, src: "fake", id: "42"}
	a.open(other)
	a.open(list)
	tt.Frame()

	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route != other {
		t.Fatalf("back went to %+v", a.route)
	}
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != list {
		t.Fatalf("forward went to %+v", a.route)
	}

	// Back closes Now Playing before it leaves the page under it.
	a.pl.Cue([]music.Track{{ID: "1", Name: "一张相片"}})
	a.nowPlaying = true
	tt.Frame()
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.nowPlaying || a.route != list {
		t.Fatalf("back with Now Playing open: open %v, page %+v", a.nowPlaying, a.route)
	}

	// A new page leaves nothing to go forward to.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	home := route{page: pageHome, src: "fake"}
	a.open(home)
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != home {
		t.Fatalf("forward after a new page went to %+v", a.route)
	}
}
