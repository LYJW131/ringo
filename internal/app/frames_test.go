package app

import (
	"fmt"
	"image"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/i18n"
	"ringo/internal/lyrics"
	"ringo/internal/music"
)

// framesDir is where MM_FRAMES says to save a look at the views, which a test
// skips without.
func framesDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("MM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	return dir
}

// With MM_FRAMES set, saves Home as it greets a listener with no music yet.
func TestHomeEmptyFrame(t *testing.T) {
	dir := framesDir(t)
	a := newTestApp()
	tt := ui.NewTester(a.view, 1180, 780)
	settled(tt)
	saveFrame(t, dir, "home-empty.png", tt.Image())
}

// framesTracks are songs with covers, by an invented band.
func framesTracks(a *app) []music.Track {
	covers := []struct{ from, to ui.Color }{
		{ui.RGB(40, 90, 170), ui.RGB(220, 120, 160)},
		{ui.RGB(230, 150, 70), ui.RGB(120, 40, 90)},
		{ui.RGB(30, 130, 110), ui.RGB(210, 220, 140)},
	}
	band := music.Artist{ID: "1", Name: "The Polar Lights"}
	var tracks []music.Track
	for i, name := range []string{"Northern Lights", "Slow Tide", "A Long Way North", "Paper Boats", "The Harbor at Night", "Quiet Hours", "Last Ferry Home", "Snowfield"} {
		url := fmt.Sprintf("cover:%d", i%len(covers))
		giveCover(a, url, coverImage(covers[i%len(covers)].from, covers[i%len(covers)].to))
		tracks = append(tracks, music.Track{
			ID: fmt.Sprint(i + 1), Name: name, Artists: []music.Artist{band},
			Album:    music.Album{ID: fmt.Sprint(10 + i%len(covers)), Name: "Polar Night", ImageURL: url},
			Duration: int64(180000 + 21000*i),
		})
	}
	return tracks
}

// With MM_FRAMES set, saves a playlist's page.
func TestPlaylistFrame(t *testing.T) {
	dir := framesDir(t)
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	tracks := framesTracks(a)
	a.mine = []music.Playlist{
		{ID: "10", Name: "Evening Walks", ImageURL: "cover:0", Mine: true},
		{ID: "11", Name: "Focus", ImageURL: "cover:1", Mine: true},
	}
	a.saved = []music.Playlist{{ID: "20", Name: "Staff Picks", ImageURL: "cover:2", Saved: true}}
	a.playlists["10"] = &music.Playlist{ID: "10", Name: "Evening Walks", Owner: "Alex", Description: "For the long way home.", ImageURL: "cover:0", Mine: true, Tracks: tracks}
	a.route = route{page: pagePlaylist, id: "10"}
	tt := ui.NewTester(a.view, 1180, 780)
	settled(tt)
	saveFrame(t, dir, "playlist.png", tt.Image())
}

// With MM_FRAMES set, saves Now Playing with lyrics following a song.
func TestNowPlayingFrame(t *testing.T) {
	dir := framesDir(t)
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	tracks := framesTracks(a)
	a.pl.Cue(tracks)
	a.pl.SetLyrics(lyrics.FromSource(&music.Lyrics{Lines: []music.LyricLine{
		{Start: 4000, Text: "The sky goes green at the edges"},
		{Start: 9000, Text: "And everyone stops to look"},
		{Start: 14000, Text: "Nobody says a word"},
		{Start: 19000, Text: "Nobody needs to"},
		{Start: 24000, Text: "We stand out on the ice"},
		{Start: 29000, Text: "Until the light goes home"},
	}}))
	a.pl.Hold(10 * time.Second)
	a.nowPlaying, a.npPane = true, paneLyrics
	tt := ui.NewTester(a.view, 1180, 780)
	tt.SetDark(true)
	tt.Advance(2 * time.Second)
	tt.Frame()
	saveFrame(t, dir, "now-playing.png", tt.Image())
}

// With MM_FRAMES set, saves Now Playing for a local song with no cover.
func TestNowPlayingNoCoverFrame(t *testing.T) {
	dir := framesDir(t)
	a := newTestApp()
	tracks := framesTracks(a)
	for i := range tracks {
		tracks[i].Album.ImageURL = ""
	}
	a.pl.Cue(tracks)
	a.pl.Hold(10 * time.Second)
	a.nowPlaying, a.npPane = true, paneQueue
	tt := ui.NewTester(a.view, 1180, 780)
	tt.SetDark(true)
	tt.Advance(2 * time.Second)
	tt.Frame()
	saveFrame(t, dir, "now-playing-no-cover.png", tt.Image())
}

// With MM_FRAMES set, saves Now Playing growing out of the player bar's
// cover, a frame every 50 ms, and shrinking back into it, in a sheet each.
func TestNowPlayingMotionFrames(t *testing.T) {
	dir := framesDir(t)
	a := newTestApp()
	a.pl.Cue(framesTracks(a))
	a.pl.SetLyrics(nil)
	a.pl.Hold(10 * time.Second)
	a.npPane = paneQueue
	tt := ui.NewTester(a.view, 1180, 780)
	tt.SetDark(true)
	settled(tt)
	sheet := func(name string) {
		var shots []image.Image
		for range 12 {
			shots = append(shots, tt.Image())
			tt.Advance(50 * time.Millisecond)
		}
		saveFrame(t, dir, name, contactSheet(shots, 4))
	}
	if err := tt.Click("Open Now Playing"); err != nil {
		t.Fatal(err)
	}
	sheet("np-open.png")
	settled(tt)
	a.nowPlaying = false
	tt.Frame()
	sheet("np-close.png")

	// Pulled down halfway, then let go to fall into the bar.
	a.nowPlaying = true
	settled(tt)
	tt.Press(450, 300)
	tt.Frame()
	for i := 1; i <= 10; i++ {
		tt.Advance(16 * time.Millisecond)
		tt.Move(450, 300+float32(i)*24)
		tt.Frame()
	}
	saveFrame(t, dir, "np-pulled.png", tt.Image())
	tt.Release(450, 540)
	tt.Frame()
	sheet("np-let-go.png")
}

// contactSheet lays shots out in rows of cols, each a third of its size.
func contactSheet(shots []image.Image, cols int) image.Image {
	b := shots[0].Bounds()
	cw, ch := b.Dx()/3, b.Dy()/3
	rows := (len(shots) + cols - 1) / cols
	out := image.NewRGBA(image.Rect(0, 0, cw*cols, ch*rows))
	for i, shot := range shots {
		x0, y0 := (i%cols)*cw, (i/cols)*ch
		for y := range ch {
			for x := range cw {
				out.Set(x0+x, y0+y, shot.At(x*3, y*3))
			}
		}
	}
	return out
}

// With MM_FRAMES set, saves every page of the settings, light and dark at the
// window's size and dark at the smallest it gets, the account page signed in
// and out, the playback page asking for High Quality, for Lossless with no
// wrapper to play it, and for Dolby Atmos for headphones, a search, and
// three pages in Chinese.
func TestSettingsFrame(t *testing.T) {
	dir := framesDir(t)
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	shots := []struct {
		name, page, query string
		signedIn          bool
		lossless          bool
		atmos             string
	}{
		{name: "general", page: "general"},
		{name: "account-in", page: "account", signedIn: true},
		{name: "account-out", page: "account"},
		{name: "playback-high", page: "playback"},
		{name: "playback-lossless", page: "playback", lossless: true},
		{name: "playback-atmos", page: "playback", atmos: music.AtmosHeadphones},
		{name: "advanced", page: "advanced"},
		{name: "search-wrapper", page: "general", query: "wrapper"},
	}
	save := func(name string, shot int, w, h int, dark bool) {
		s := shots[shot]
		a := newTestApp()
		a.updates = true
		a.notch = &notch{a: a}
		src := fake(a)
		// Made up: nothing here is anyone's key.
		src.vals["wrapperKey"] = "not-a-real-key"
		if s.lossless {
			a.settings.Quality = "lossless"
		}
		a.settings.Atmos = s.atmos
		if s.signedIn {
			a.account = &music.Account{ID: "1", Name: "Alex"}
		}
		a.settingsPage, a.settingsQuery = s.page, s.query
		tt := ui.NewTester(a.settingsView, w, h)
		tt.SetDark(dark)
		settled(tt)
		saveFrame(t, dir, name, tt.Image())
	}
	for i, s := range shots {
		save(s.name+"-dark.png", i, 780, 640, true)
		save(s.name+"-min-dark.png", i, 640, 420, true)
		if s.query == "" {
			save(s.name+"-light.png", i, 780, 640, false)
		}
	}
	defer i18n.SetLanguage(i18n.English)
	i18n.SetLanguage(i18n.Chinese)
	for i, s := range shots {
		if s.name == "account-in" || s.name == "playback-lossless" || s.name == "playback-atmos" {
			save(s.name+"-zh-dark.png", i, 780, 640, true)
			save(s.name+"-zh-min-dark.png", i, 640, 420, true)
		}
	}
}

// With MM_FRAMES set, saves the player bar of windows from the narrowest
// to a wide one, one under another: a Hi-Res song with a short title, then
// one with a long title.
func TestPlayerBarWidthsFrames(t *testing.T) {
	dir := framesDir(t)
	for _, dark := range []bool{true, false} {
		var bars []image.Image
		for _, name := range []string{"Howling", "Northern Lights Over the Harbor at Night (Live at the Old Lighthouse)"} {
			for _, w := range []int{880, 1024, 1181, 1440} {
				a := newTestApp()
				a.account = &music.Account{ID: "1", Name: "Alex"}
				tracks := framesTracks(a)
				tracks[0].Name = name
				a.pl.Cue(tracks)
				a.pl.SetLyrics(nil)
				a.pl.SetStream(&music.Stream{Quality: "lossless", SampleRate: 96000, BitDepth: 24})
				a.pl.Hold(25 * time.Second)
				tt := ui.NewTester(a.view, w, 580)
				tt.SetDark(dark)
				settled(tt)
				img := tt.Image()
				b := img.Bounds()
				bars = append(bars, img.SubImage(image.Rect(b.Min.X, b.Max.Y-100*b.Dx()/w, b.Max.X, b.Max.Y)))
			}
		}
		saveFrame(t, dir, fmt.Sprintf("player-bar-widths-dark-%v.png", dark), stacked(bars))
	}
}

// stacked lays shots out one under another, each at its own width.
func stacked(shots []image.Image) image.Image {
	w, h := 0, 0
	for _, s := range shots {
		w = max(w, s.Bounds().Dx())
		h += s.Bounds().Dy() + 8
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	y := 0
	for _, s := range shots {
		b := s.Bounds()
		for dy := range b.Dy() {
			for dx := range b.Dx() {
				out.Set(dx, y+dy, s.At(b.Min.X+dx, b.Min.Y+dy))
			}
		}
		y += b.Dy() + 8
	}
	return out
}

// With MM_FRAMES set, saves Now Playing with lyrics that have no times, in
// the lyrics pane and on the stage, a minute and a half into the song.
func TestStillLyricsFrames(t *testing.T) {
	dir := framesDir(t)
	for _, pane := range []struct {
		pane int
		name string
	}{{paneLyrics, "lyrics"}, {paneStage, "stage"}} {
		a := newTestApp()
		a.account = &music.Account{ID: "1", Name: "Alex"}
		a.pl.Cue(framesTracks(a))
		a.pl.SetLyrics(stillWords(40))
		a.pl.Hold(90 * time.Second)
		a.nowPlaying, a.npPane = true, pane.pane
		a.settings.LyricsStage = pane.pane == paneStage
		tt := ui.NewTester(a.view, 1180, 780)
		tt.SetDark(true)
		tt.Advance(2 * time.Second)
		tt.Frame()
		saveFrame(t, dir, "still-lyrics-"+pane.name+".png", tt.Image())
	}
}

// With MM_FRAMES set, saves the badge of a Hi-Res stream in Now Playing, and
// of a lossless one in the player bar, light and dark, and of a song's
// Dolby Atmos version in both.
func TestQualityBadgeFrames(t *testing.T) {
	dir := framesDir(t)
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	a.pl.Cue(framesTracks(a))
	a.pl.SetLyrics(nil)
	a.pl.SetStream(&music.Stream{Quality: "lossless", SampleRate: 96000, BitDepth: 24})
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1180, 780)
	tt.SetDark(true)
	settled(tt)
	saveFrame(t, dir, "badge-now-playing.png", tt.Image())
	a.nowPlaying = false
	a.pl.SetStream(&music.Stream{Quality: "lossless", SampleRate: 44100, BitDepth: 16})
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		settled(tt)
		img := tt.Image()
		b := img.Bounds()
		bar := img.SubImage(image.Rect(b.Min.X, b.Max.Y-b.Dy()*120/780, b.Max.X, b.Max.Y))
		saveFrame(t, dir, fmt.Sprintf("badge-player-bar-dark-%v.png", dark), bar)
	}
	a.pl.SetStream(&music.Stream{Quality: "atmos-binaural", BitRate: 256000})
	tt.SetDark(true)
	settled(tt)
	img := tt.Image()
	b := img.Bounds()
	saveFrame(t, dir, "badge-atmos-player-bar.png", img.SubImage(image.Rect(b.Min.X, b.Max.Y-b.Dy()*120/780, b.Max.X, b.Max.Y)))
	a.nowPlaying = true
	settled(tt)
	saveFrame(t, dir, "badge-atmos-now-playing.png", tt.Image())
}
