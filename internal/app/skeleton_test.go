package app

import (
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// skeletonScenes are the views that load, as they show while they do.
var skeletonScenes = []struct {
	name string
	set  func(a *app)
}{
	{"home", func(a *app) {}},
	{"playlist", func(a *app) {
		a.route = route{page: pagePlaylist, src: "fake", id: "7"}
		a.loadingPl["7"] = true
	}},
	{"browse", func(a *app) {
		a.route = route{page: pageBrowse, src: "fake", id: music.PageNew}
		a.browses[music.PageNew] = &browseData{loading: true}
	}},
	{"lyrics", func(a *app) { a.nowPlaying, a.npPane = true, paneLyrics }},
	{"stage", func(a *app) { a.nowPlaying, a.npPane = true, paneStage }},
}

func skeletonApp(set func(a *app)) *app {
	a := newTestApp()
	a.homeLoading = true
	a.pl.Cue([]music.Track{{ID: "1", Name: "迷星叫"}})
	set(a)
	return a
}

// With MM_FRAMES set, saves each skeleton in the light and the dark, the
// shimmer partway across.
func TestSkeletonFrames(t *testing.T) {
	dir := os.Getenv("MM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, s := range skeletonScenes {
		for _, dark := range []bool{false, true} {
			a := skeletonApp(s.set)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(450 * time.Millisecond)
			tt.Frame()
			name := s.name
			if dark {
				name += "-dark"
			}
			saveFrame(t, dir, "skeleton-"+name+".png", tt.Image())
		}
	}
}
