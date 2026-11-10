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
	other, list := route{page: pagePlaylist, id: "43"}, route{page: pagePlaylist, id: "42"}
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
	home := route{page: pageHome}
	a.open(home)
	tt.Key(0, ui.KeyForward)
	tt.Frame()
	if a.route != home {
		t.Fatalf("forward after a new page went to %+v", a.route)
	}
}

// A page gone back to shows where it was left, down the page and along its
// shelves, and one opened anew starts at its top. Back used to start every
// page at its top, losing the listener's place.
func TestBackKeepsThePlace(t *testing.T) {
	a, _ := shelfApp()
	a.albums["a1"] = &albumData{album: speakNow()}
	tt := ui.NewTester(a.view, 1180, 780)
	settled(tt)
	tt.Wheel(700, 400, 0, 420)
	settled(tt)
	card, ok := tt.Find("Better Over Time")
	if !ok {
		t.Fatalf("no card to page through among %q", tt.Texts())
	}
	tt.Move(card.X+card.W/2, card.Y+card.H/2)
	settled(tt)
	if err := tt.Click("Next Page"); err != nil {
		t.Fatal(err)
	}
	settled(tt)
	along := func() float32 {
		var x float32
		for _, st := range a.shelves {
			x = max(x, st.X)
		}
		return x
	}
	// Where the card's title is says where the page is scrolled to.
	where := func() float32 {
		r, _ := tt.Find("Young Manhood and Other Donatable Organs")
		return r.Y
	}
	y, x := where(), along()
	if x == 0 {
		t.Fatal("the shelf didn't page")
	}
	newPage := a.route

	a.open(route{page: pageAlbum, id: "a1"})
	settled(tt)
	tt.Key(ui.Cmd, ui.KeyBracketLeft)
	settled(tt)
	if a.route != newPage {
		t.Fatalf("back went to %+v", a.route)
	}
	if got := where(); got != y {
		t.Errorf("back on the page, its shelf is at %v, left at %v", got, y)
	}
	if got := along(); got != x {
		t.Errorf("back on the page, its shelf is scrolled %v along, left at %v", got, x)
	}

	// Forward and back again keeps it too; opened anew, it starts at its top.
	tt.Key(ui.Cmd, ui.KeyBracketRight)
	settled(tt)
	tt.Key(ui.Cmd, ui.KeyBracketLeft)
	settled(tt)
	if got := where(); got != y {
		t.Errorf("back again, its shelf is at %v, left at %v", got, y)
	}
	a.open(route{page: pageAlbum, id: "a1"})
	settled(tt)
	a.open(newPage)
	settled(tt)
	if a.pageScroll.Y != 0 || along() != 0 {
		t.Errorf("opened anew, the page is scrolled %v down and %v along", a.pageScroll.Y, along())
	}
	if f, _ := a.trackList.Visible(); f != 0 {
		t.Errorf("opened anew, the page starts at row %d", f)
	}
}
