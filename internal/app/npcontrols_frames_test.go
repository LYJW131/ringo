package app

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"ringo/internal/i18n"
	"ringo/internal/music"
)

// controlsShot is Now Playing with a song under way, for a look at the
// controls under its cover: over the colors of a cover from one color down
// to another, with a stream's badge if st is set and a station's name if
// radio is.
type controlsShot struct {
	from, to ui.Color
	title    string
	st       *music.Stream
	radio    string
	paused   bool
	stray    bool // not in the library: the plus, not the star
	w, h     int
}

// controlsApp is the app of a shot, and its window at twice the size, come
// to rest.
func controlsApp(s controlsShot) (*app, *ui.Tester) {
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	if s.title == "" {
		s.title = "打上花火"
	}
	tr := song("7", s.title, "米津玄师")
	if s.stray {
		tr.Saved = false
	}
	tr.Album = music.Album{ID: "8", Name: "BOOTLEG", ImageURL: "cover:shot"}
	tr.Duration = 259000
	giveCover(a, tr.Album.ImageURL, coverImage(s.from, s.to))
	a.pl.Cue([]music.Track{tr, song("8", "LOSER", "米津玄师")})
	a.pl.SetLyrics(nil)
	a.pl.SetStream(s.st)
	a.pl.Hold(61 * time.Second)
	if s.radio != "" {
		a.pl.AttachRadio(radioName(&music.Station{ID: "st", Name: s.radio}), func(func([]music.Track)) {})
	}
	a.pl.SetVolume(0.72)
	a.volume = 0.72
	a.nowPlaying = true
	if s.w == 0 {
		s.w, s.h = 1440, 900
	}
	tt := ui.NewTester(a.view, s.w, s.h)
	tt.SetScale(2)
	tt.SetDark(true)
	tt.Advance(2 * time.Second)
	tt.Frame()
	return a, tt
}

// controlsCrop is the part of the window's picture under the cover, with
// the cover's foot over it and a margin at each side.
func controlsCrop(t *testing.T, tt *ui.Tester) image.Image {
	t.Helper()
	cover, ok := tt.Find(T("Album Cover"))
	if !ok {
		t.Fatal("no cover")
	}
	img := tt.Image()
	const s = 2 // the window's scale
	x0, x1 := int((cover.X-44)*s), int((cover.X+max(cover.W, 280)+44)*s)
	y0 := int((cover.Y + cover.H - 28) * s)
	return img.SubImage(image.Rect(x0, y0, x1, img.Bounds().Max.Y))
}

// With MM_FRAMES set, saves the controls of Now Playing at rest and under
// the pointer, pressed and dragged, over a dark cover's colors and a light
// one's, and the frames of the progress bar and a button as the pointer
// comes to them.
func TestNowPlayingControlsFrames(t *testing.T) {
	dir := framesDir(t)
	defer i18n.SetLanguage(i18n.English)
	i18n.SetLanguage(i18n.Chinese)
	hires := &music.Stream{Quality: "lossless", SampleRate: 96000, BitDepth: 24}
	covers := []struct {
		name     string
		from, to ui.Color
	}{
		{"red", ui.RGB(150, 62, 58), ui.RGB(96, 34, 36)},
		{"light", ui.RGB(244, 226, 170), ui.RGB(232, 200, 120)},
		{"blue", ui.RGB(40, 90, 170), ui.RGB(30, 50, 120)},
	}
	center := func(tt *ui.Tester, label string) (float32, float32) {
		t.Helper()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("no %q", label)
		}
		return r.X + r.W/2, r.Y + r.H/2
	}
	rest := func(tt *ui.Tester) {
		tt.Frame()
		tt.Advance(600 * time.Millisecond)
		tt.Frame()
	}
	save := func(name string, tt *ui.Tester) {
		saveFrame(t, dir, "npc-"+name+".png", controlsCrop(t, tt))
	}
	for _, cv := range covers {
		shot := controlsShot{from: cv.from, to: cv.to, st: hires, radio: "深夜电台", stray: true}
		a, tt := controlsApp(shot)
		_ = a
		save(cv.name+"-rest", tt)
		for _, h := range []struct{ name, label string }{
			{"hover-play", T("Play/Pause")}, {"hover-prev", T("Previous")}, {"hover-shuffle", T("Shuffle")},
			{"hover-add", T("Add to Library")}, {"hover-more", T("More")}, {"hover-scrub", T("Playback Progress")},
			{"hover-volume", T("Volume")},
		} {
			x, y := center(tt, h.label)
			tt.Move(x, y)
			rest(tt)
			save(cv.name+"-"+h.name, tt)
		}
		// The chip of the station, under the pointer.
		if r, ok := tt.Find(Tf("Leave %s", shot.radio)); ok {
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
			rest(tt)
			save(cv.name+"-hover-radio", tt)
		}
		// Pressed: the play button, then the one before it.
		for _, p := range []struct{ name, label string }{{"press-play", T("Play/Pause")}, {"press-next", T("Next")}} {
			x, y := center(tt, p.label)
			tt.Move(x, y)
			rest(tt)
			tt.Press(x, y)
			rest(tt)
			save(cv.name+"-"+p.name, tt)
			tt.Move(0, 0)
			tt.CancelPress()
			rest(tt)
		}
		// The progress bar, dragged from where the song is to most of the
		// way; and the volume, dragged down.
		bar, _ := tt.Find(T("Playback Progress"))
		y := bar.Y + bar.H/2
		tt.Move(bar.X+bar.W*0.24, y)
		rest(tt)
		tt.Press(bar.X+bar.W*0.24, y)
		tt.Frame()
		tt.Move(bar.X+bar.W*0.7, y-6)
		rest(tt)
		save(cv.name+"-drag-scrub", tt)
		tt.CancelPress()
		tt.Move(0, 0)
		rest(tt)
		vol, _ := tt.Find(T("Volume"))
		y = vol.Y + vol.H/2
		tt.Move(vol.X+vol.W*0.72, y)
		rest(tt)
		tt.Press(vol.X+vol.W*0.72, y)
		tt.Frame()
		tt.Move(vol.X+vol.W*0.3, y+3)
		rest(tt)
		save(cv.name+"-drag-volume", tt)
		tt.Release(vol.X+vol.W*0.3, y+3)
	}

	// What comes and goes: no station and no badge, each badge, a song
	// shuffled and repeating, a favorite, and a title too long for
	// its line; and a window too short for the cover to be as wide as the
	// controls.
	red := covers[0]
	base := controlsShot{from: red.from, to: red.to}
	_, tt := controlsApp(base)
	save("plain", tt)
	for _, b := range []struct {
		name string
		st   *music.Stream
	}{
		{"badge-lossless", &music.Stream{Quality: "lossless", SampleRate: 44100, BitDepth: 16}},
		{"badge-atmos", &music.Stream{Quality: "atmos-binaural", BitRate: 256000}},
	} {
		s := base
		s.st = b.st
		_, tt := controlsApp(s)
		save(b.name, tt)
	}
	a, tt := controlsApp(controlsShot{from: red.from, to: red.to, st: hires})
	a.pl.ToggleShuffle()
	a.pl.CycleRepeat()
	a.pl.CycleRepeat()
	a.library.liked = map[string]bool{"7": true}
	rest(tt)
	save("shuffled-favorite", tt)
	long := controlsShot{from: red.from, to: red.to, st: hires, radio: "深夜电台",
		title: "Northern Lights Over the Harbor at Night (Live at the Old Lighthouse)"}
	_, tt = controlsApp(long)
	save("long-title", tt)
	short := controlsShot{from: red.from, to: red.to, st: hires, radio: "深夜电台", stray: true, w: 1000, h: 680}
	_, tt = controlsApp(short)
	save("short-window", tt)

	// The same controls where else they are: along the foot of the stage
	// and of the wall, with the wall's song over it, and in the player bar,
	// light and dark, its progress under the pointer.
	for _, p := range []struct {
		name string
		pane int
	}{{"stage", paneStage}, {"wall", paneWall}} {
		a, tt := controlsApp(controlsShot{from: red.from, to: red.to, st: hires, radio: "深夜电台", stray: true})
		a.npPane = p.pane
		a.pl.ToggleShuffle()
		tt.Frame()
		tt.Advance(2 * time.Second)
		bar, _ := tt.Find(T("Playback Progress"))
		tt.Move(bar.X+bar.W*0.4, bar.Y+bar.H/2)
		rest(tt)
		saveFrame(t, dir, "npc-"+p.name+".png", tt.Image())
	}
	for _, dark := range []bool{false, true} {
		a, tt := controlsApp(controlsShot{from: red.from, to: red.to, st: hires})
		a.nowPlaying = false
		a.pl.ToggleShuffle()
		tt.SetDark(dark)
		tt.Frame()
		tt.Advance(2 * time.Second)
		bar, _ := tt.Find(T("Playback Progress"))
		for _, at := range []struct {
			name string
			x, y float32
		}{{"rest", 0, 0}, {"hover", bar.X + bar.W*0.4, bar.Y + bar.H/2}} {
			tt.Move(at.x, at.y)
			rest(tt)
			img := tt.Image()
			b := img.Bounds()
			saveFrame(t, dir, fmt.Sprintf("npc-bar-dark-%v-%s.png", dark, at.name), img.SubImage(image.Rect(b.Min.X+2*sidebarWidth, b.Max.Y-2*(playerBarHeight+16), b.Max.X, b.Max.Y)))
		}
	}

	// Frame by frame: the pointer comes to the progress bar and drags it,
	// and comes to the play button and presses it.
	_, tt = controlsApp(controlsShot{from: red.from, to: red.to, st: hires, radio: "深夜电台", stray: true})
	bar, _ := tt.Find(T("Playback Progress"))
	y := bar.Y + bar.H/2
	frame := 0
	step := func(seq string, n int) {
		for range n {
			tt.Frame()
			save(fmt.Sprintf("seq-%s-%02d", seq, frame), tt)
			frame++
			tt.Advance(time.Second / 60 * 2)
		}
	}
	step("scrub", 1)
	tt.Move(bar.X+bar.W*0.24, y)
	step("scrub", 6)
	tt.Press(bar.X+bar.W*0.24, y)
	step("scrub", 2)
	for i := range 5 {
		tt.Move(bar.X+bar.W*(0.24+0.09*float32(i+1)), y-2)
		step("scrub", 1)
	}
	tt.Release(bar.X+bar.W*0.69, y-2)
	step("scrub", 2)
	tt.Move(0, 0)
	step("scrub", 6)

	// Another window, its song one that plays, for the button to pause.
	a, tt = controlsApp(controlsShot{from: red.from, to: red.to, st: hires, radio: "深夜电台", stray: true})
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()
	frame = 0
	x, y := center(tt, T("Play/Pause"))
	rest(tt)
	step("play", 1)
	tt.Move(x, y)
	step("play", 5)
	tt.Press(x, y)
	step("play", 4)
	tt.Release(x, y)
	step("play", 10)
}
