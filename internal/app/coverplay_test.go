package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// heldAlbums answers the fake's albums only as the test lets each through,
// saying which was asked for.
func heldAlbums(a *app) (asked chan string, let chan struct{}) {
	asked, let = make(chan string, 8), make(chan struct{}, 8)
	fake(a).album = func(id string) (*music.AlbumPage, error) {
		asked <- id
		<-let
		return &music.AlbumPage{Album: music.Album{ID: id, Name: id}, Tracks: []music.Track{song(id+".1", id+" one"), song(id+".2", id+" two")}}, nil
	}
	return asked, let
}

// An album whose cover the pointer rests on loads before its play button is
// clicked, and a click while it loads plays it as it comes, without asking
// for it again. Passing over the cover loads nothing.
func TestCoverLoadsAheadOfItsPlayButton(t *testing.T) {
	a, _ := shelfApp()
	asked, let := heldAlbums(a)
	ch := posts(a)
	tt := ui.NewTester(a.view, 1180, 3400)
	tt.Frame()
	card, ok := tt.Find("Better Over Time")
	if !ok {
		t.Fatal("no card")
	}
	// A move shows in the next frame.
	tt.Move(card.X+card.W/2, card.Y+card.W/2)
	tt.Frame()
	tt.Advance(aheadAfter / 2)
	tt.Move(card.X+card.W/2, card.Y-200)
	tt.Frame()
	tt.Advance(aheadAfter)
	select {
	case id := <-asked:
		t.Fatalf("passing over the cover asked for %s", id)
	case <-time.After(50 * time.Millisecond):
	}

	tt.Move(card.X+card.W/2, card.Y+card.W/2)
	tt.Frame()
	// The pointer is known to be on it a frame after it came.
	tt.Advance(aheadAfter + 50*time.Millisecond)
	if id := wait(t, asked, "the album"); id != "a2" {
		t.Fatalf("asked for %s", id)
	}
	tt.ClickAt(card.X+card.W-29, card.Y+card.W-29)
	if a.pl.Current() != nil {
		t.Fatal("played before the album came")
	}
	if !a.waitsFor(albumOrigin("a2")) {
		t.Error("the play button doesn't wait for the album")
	}
	let <- struct{}{}
	until(t, ch, func() bool { return a.pl.Current() != nil })
	if tr := a.pl.Current(); tr.ID != "a2.1" || tr.Origin != albumOrigin("a2") {
		t.Errorf("played %s from %+v", tr.ID, tr.Origin)
	}
	select {
	case id := <-asked:
		t.Errorf("asked for %s again", id)
	case <-time.After(50 * time.Millisecond):
	}
}

// Of two albums whose play buttons were clicked as they loaded, the one
// clicked last plays, whichever comes first.
func TestLastCoverClickedPlays(t *testing.T) {
	a := newTestApp()
	asked, let := heldAlbums(a)
	ch := posts(a)
	a.playAlbum("x")
	wait(t, asked, "the first album")
	a.playAlbum("y")
	wait(t, asked, "the second album")
	let <- struct{}{}
	runPost(t, ch)
	if a.pl.Current() != nil {
		t.Fatalf("played %s, clicked before the other", a.pl.Current().ID)
	}
	let <- struct{}{}
	runPost(t, ch)
	if tr := a.pl.Current(); tr == nil || tr.ID != "y.1" {
		t.Fatalf("played %+v", tr)
	}
}

// An album loaded ahead that fails is forgotten without a word, and its page
// asks for it again as it opens.
func TestCoverLoadedAheadFailsQuietly(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	fake(a).album = func(id string) (*music.AlbumPage, error) { return nil, errNotSet }
	a.fetchAlbum("x", true)
	runPost(t, ch)
	if _, ok := a.albums["x"]; ok || a.toast != "" {
		t.Errorf("a failure ahead left %+v and said %q", a.albums["x"], a.toast)
	}
}
