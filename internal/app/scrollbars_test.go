package app

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// barsApp is signed in, with more playlists than its sidebar has room for,
// and a playlist longer than its page.
func barsApp() *app {
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	for i := range 40 {
		a.mine = append(a.mine, music.Playlist{ID: fmt.Sprint(100 + i), Name: fmt.Sprint("Mix ", i), Mine: true})
	}
	tracks := make([]music.Track, 200)
	for i := range tracks {
		tracks[i] = music.Track{ID: fmt.Sprint(i + 1), Name: fmt.Sprint("Song ", i+1)}
	}
	a.playlists["42"] = &music.Playlist{ID: "42", Name: "Long", Tracks: tracks}
	return a
}

// wheel turns the wheel at (x, y), and builds the frames in which what's
// there glides after it.
func wheel(tt *ui.Tester, x, y, dy float32) {
	tt.Wheel(x, y, 0, dy)
	for range 4 {
		tt.Frame()
		tt.Advance(16 * time.Millisecond)
	}
	tt.Frame()
}

// barShows reports whether the container name has its scroll bars showing.
func barShows(a *app, tt *ui.Tester, name string) bool {
	b := a.bars.each[name]
	return b != nil && b.until.After(tt.Now())
}

// The sidebar's scroll bar doesn't show for the pointer being over the
// sidebar, which is when MyGo shows one: it shows as the sidebar is
// scrolled, stays while the pointer is on it, and goes a moment after.
func TestScrollBarsShowOnlyAsScrolled(t *testing.T) {
	a := barsApp()
	tt := ui.NewTester(a.view, 1180, 600)
	settled(tt)
	tt.Move(100, 300)
	settled(tt)
	if barShows(a, tt, "nav") {
		t.Fatal("the sidebar's bar shows for the pointer being over it")
	}
	wheel(tt, 100, 300, 120)
	if !barShows(a, tt, "nav") {
		t.Fatal("the sidebar's bar doesn't show as it's scrolled")
	}
	tt.Advance(barLinger + barFade + 100*time.Millisecond)
	tt.Frame()
	if barShows(a, tt, "nav") {
		t.Error("the sidebar's bar stays after the scrolling")
	}

	// On the bar, the pointer keeps it.
	wheel(tt, 100, 300, 120)
	// The list reaches to 8 points from the sidebar's edge.
	tt.Move(sidebarWidth-14, 300)
	for range 6 {
		tt.Advance(barLinger / 2)
		tt.Frame()
	}
	if !barShows(a, tt, "nav") {
		t.Error("the sidebar's bar goes from under the pointer")
	}
}

// A page's scroll bar doesn't show as the page opens, or is gone back to
// where it was left, which puts it somewhere without the listener scrolling;
// it shows as a list of songs is scrolled, and as a page of shelves is.
func TestPageScrollBarsNotForOpening(t *testing.T) {
	a := barsApp()
	tt := ui.NewTester(a.view, 1180, 780)
	tt.Move(700, 400)
	for _, r := range []route{{page: pagePlaylist, id: "42"}, {page: pageHome}} {
		a.open(r)
		settled(tt)
		settled(tt)
		if barShows(a, tt, "page") {
			t.Fatalf("%+v: its bar shows as it opens", r)
		}
	}
	a.open(route{page: pagePlaylist, id: "42"})
	settled(tt)
	wheel(tt, 700, 400, 30)
	if !barShows(a, tt, "page") {
		t.Fatal("a list's bar doesn't show as it's scrolled a little")
	}
	tt.Wheel(700, 400, 0, 2400)
	settled(tt)
	settled(tt)
	a.open(route{page: pageHome})
	settled(tt)
	settled(tt)
	a.back()
	tt.Frame()
	tt.Frame()
	tt.Frame()
	if barShows(a, tt, "page") {
		t.Error("the bar shows as the page comes back where it was left")
	}
}

// Lyrics that follow the song move by themselves, which shows no scroll
// bar; the listener scrolling them does.
func TestLyricsFollowingShowNoScrollBar(t *testing.T) {
	a, tt, _ := playingApp()
	a.nowPlaying, a.npPane = true, paneLyrics
	tt.Move(900, 400)
	for _, pos := range []time.Duration{2 * time.Second, 16 * time.Second, 22 * time.Second, 40 * time.Second} {
		a.pl.Seek(pos)
		for range 30 {
			tt.Advance(50 * time.Millisecond)
			tt.Frame()
			if barShows(a, tt, "lyrics") {
				t.Fatalf("at %v the lyrics' bar shows for following the song", pos)
			}
		}
	}
	wheel(tt, 900, 400, 200)
	if !barShows(a, tt, "lyrics") {
		t.Error("the lyrics' bar doesn't show as they're scrolled")
	}
}

// With MM_FRAMES set, saves the sidebar with the pointer over it, as it's
// scrolled, and after: a bar in the second only.
func TestScrollBarFrames(t *testing.T) {
	dir := framesDir(t)
	a := barsApp()
	tt := ui.NewTester(a.view, 1180, 600)
	tt.SetDark(true)
	settled(tt)
	side := func(name string) {
		img := tt.Image()
		b := img.Bounds()
		saveFrame(t, dir, name, img.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Min.X+b.Dx()*(sidebarWidth+60)/1180, b.Max.Y)))
	}
	tt.Move(100, 300)
	settled(tt)
	side("bars-1-hover.png")
	wheel(tt, 100, 300, 120)
	// The glide after the wheel comes to rest.
	for range 20 {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	side("bars-2-scrolled.png")
	tt.Advance(a.bars.each["nav"].until.Sub(tt.Now()) + barFade/2)
	tt.Frame()
	tt.Frame()
	side("bars-3-fading.png")
	tt.Advance(barFade)
	tt.Frame()
	tt.Frame()
	side("bars-4-gone.png")
}
