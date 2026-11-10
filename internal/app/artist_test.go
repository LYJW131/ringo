package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
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
	if a.nowPlaying || a.route != (route{page: pageArtist, id: "12"}) {
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
	if a.route != (route{page: pageArtist, id: "13"}) {
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
	if a.route != (route{page: pageAlbum, id: "8"}) {
		t.Fatalf("the album opened %+v", a.route)
	}
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if _, ok := tt.Find("迷跡波"); !ok {
		t.Error("the tab went back to the songs")
	}
}

// A band's date is when it formed, not when it was born.
func TestGroupFormed(t *testing.T) {
	for _, group := range []bool{false, true} {
		a := artistApp(false)
		a.artists["159260351"].page.Artist.Group = group
		tt := ui.NewTester(a.view, 1180, 1700)
		settled(tt)
		if _, ok := tt.Find("Formed"); ok != group {
			t.Errorf("a group %v reads Formed %v: %q", group, ok, tt.Texts())
		}
	}
}

// With MM_FRAMES set, saves an artist's page headed by their picture
// moving (its still, as tests don't decode video), by their round photo, and
// by nothing, in the light and the dark, wide and narrow, to set beside
// Music on the Mac's (.work/apple-app/artist).
func TestArtistFrames(t *testing.T) {
	dir := framesDir(t)
	heads := []struct {
		name string
		set  func(ar *music.Artist)
	}{
		{"video", func(ar *music.Artist) {
			ar.WideURL, ar.VideoURL = "wide:2:{w}x{h}", "motion"
			ar.Colors = music.Colors{BG: "202a3a", Text1: "ffffff"}
		}},
		{"photo", func(ar *music.Artist) { ar.Colors = music.Colors{BG: "6a4a35", Text1: "ffffff"} }},
		{"bare", func(ar *music.Artist) { ar.ImageURL, ar.Bio = "", "" }},
	}
	for _, head := range heads {
		for _, w := range []int{1180, 860} {
			for _, dark := range []bool{false, true} {
				a := artistApp(false)
				head.set(&a.artists["159260351"].page.Artist)
				shelfArt(a)
				tt := ui.NewTester(a.view, w, 900)
				tt.SetDark(dark)
				settled(tt)
				name := fmt.Sprintf("artist-%s-%d", head.name, w)
				if dark {
					name += "-dark"
				}
				saveFrame(t, dir, name+".png", tt.Image())
			}
		}
	}
}
