package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// The page under Now Playing is not built while Now Playing covers it, and
// shows again where it was scrolled once Now Playing closes.
func TestCoveredPageKeepsItsPlace(t *testing.T) {
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "小明"}
	tracks := make([]music.Track, 200)
	for i := range tracks {
		tracks[i] = music.Track{ID: fmt.Sprint(i + 1), Name: fmt.Sprintf("曲目%d", i+1)}
	}
	a.playlists["42"] = &music.Playlist{ID: "42", Name: "飙升榜", Tracks: tracks}
	// Home, which scrolls otherwise than a list does.
	for i := range 40 {
		a.mine = append(a.mine, music.Playlist{ID: fmt.Sprint(100 + i), Name: fmt.Sprintf("歌单%d", i+1), Mine: true})
	}
	a.mineCollapsed = true
	a.pl.Cue([]music.Track{{ID: "9999", Name: "一张相片"}})
	tt := ui.NewTester(a.view, 1200, 800)

	for _, r := range []route{{page: pagePlaylist, id: "42"}, {page: pageHome}} {
		a.open(r)
		tt.Frame()
		tt.Wheel(700, 400, 0, 2400)
		tt.Advance(time.Second)
		tt.Frame()
		before := shown(tt)

		a.nowPlaying = true
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		if _, ok := tt.Find("Recently Added"); ok {
			t.Errorf("%+v: the sidebar is built under Now Playing", r)
		}

		a.nowPlaying = false
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		if after := shown(tt); after != before {
			t.Errorf("%+v: shows\n%s\nafter Now Playing, was\n%s", r, after, before)
		}
		if before == firstShown(a) {
			t.Errorf("%+v: the wheel did not scroll it", r)
		}
	}
}

// shown returns the texts the window shows, each where it is.
func shown(tt *ui.Tester) string {
	var s string
	for _, text := range tt.Texts() {
		if r, ok := tt.Find(text); ok {
			s += fmt.Sprintf("%s@%.0f,%.0f ", text, r.X, r.Y)
		}
	}
	return s
}

// firstShown returns what a's page shows unscrolled.
func firstShown(a *app) string {
	b := *a
	b.trackList, b.pageScroll = ui.ListState{}, ui.ScrollState{}
	tt := ui.NewTester(b.view, 1200, 800)
	tt.Frame()
	return shown(tt)
}
