package app

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// longHome gives the app enough playlists for its Home to scroll.
func longHome(a *app) {
	for i := range 40 {
		a.mine = append(a.mine, music.Playlist{ID: fmt.Sprint(i + 1), Name: "歌单", Mine: true})
	}
	a.mineCollapsed = true
}

// The edges over a page show while content is under them: at the foot of a
// long page at first, and at its top once it has scrolled to its end.
func TestScrollEdgesFollowContent(t *testing.T) {
	a := newTestApp()
	longHome(a)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if a.underTop {
		t.Error("top edge shows before scrolling")
	}
	if !a.underBottom {
		t.Error("no bottom edge over a long page")
	}
	tt.Scroll(800, 400, 0, 100000)
	tt.Advance(time.Second)
	tt.Frame()
	tt.Frame()
	if !a.underTop {
		t.Error("no top edge after scrolling")
	}
	if a.underBottom {
		t.Error("bottom edge shows at the end of the page")
	}
}

// A page that fits the window shows neither edge.
func TestShortPageHasNoEdges(t *testing.T) {
	a := newTestApp()
	a.mine = []music.Playlist{{ID: "1", Name: "歌单", Mine: true}}
	tt := ui.NewTester(a.view, 1200, 1400)
	tt.Frame()
	if a.underTop || a.underBottom {
		t.Errorf("edges on a page that fits: top %v bottom %v", a.underTop, a.underBottom)
	}
}

// Opening or closing the account menu leaves the player's edge as it was:
// the content's box had no key, so the menu coming or going made it anew,
// and the edge faded in again from nothing, showing the page bare under
// the player until something asked for another frame.
func TestAccountMenuKeepsPlayerEdge(t *testing.T) {
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "小明"}
	longHome(a)
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	// The player's foot, right of where the menu's shadow reaches.
	bar := func() *image.RGBA {
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 1000
		r := image.Rect(int((sidebarWidth+60)*s), int((700-playerBarHeight)*s), img.Bounds().Dx(), img.Bounds().Dy())
		out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
		return out
	}
	want := bar()
	for _, step := range []string{"open", "close"} {
		if err := tt.Click("小明"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !bytes.Equal(bar().Pix, want.Pix) {
			t.Fatalf("the player's edge changed on %s", step)
		}
	}
}
