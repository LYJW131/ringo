package app

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
	"ringo/internal/video"
)

// openClip opens the clip of internal/video's tests, counting how many
// times it's asked for.
func openClip(loads *atomic.Int32) func(string, int, int, motionSize) (ui.VideoDecoder, error) {
	return func(string, int, int, motionSize) (ui.VideoDecoder, error) {
		loads.Add(1)
		data, err := os.ReadFile("../video/testdata/clip.mp4")
		if err != nil {
			return nil, err
		}
		return video.Open(data)
	}
}

// motionView is a cover of 200×200 with a video over it, once shown is set,
// and the number of videos asked for.
func motionView(a *app) (*ui.Tester, *bool, *atomic.Int32) {
	var loads atomic.Int32
	a.motionArt().load = openClip(&loads)
	shown := false
	tt := ui.NewTester(func(c *ui.Context) {
		if !shown {
			return
		}
		box := ui.Box(c).Size(200, 200).Label("cover")
		box.Children(func() { a.motionOver(c, box, "https://example.test/hls/master.m3u8", 200, 200, motionHead, 8) })
	}, 400, 400)
	return tt, &shown, &loads
}

// While the desktop asks for less motion, or the listener turned Animated
// Art off, no video is fetched and no player made: the cover stays still.
func TestMotionStillWhenAsked(t *testing.T) {
	if !video.Supported() {
		t.Skip("no decoder on this machine")
	}
	for _, c := range []struct {
		name   string
		off    bool
		reduce bool
	}{{"switched off", true, false}, {"less motion", false, true}} {
		a := newTestApp()
		posts(a)
		a.settings.AnimatedArt = !c.off
		tt, shown, loads := motionView(a)
		tt.SetPreferences(ui.Preferences{ReduceMotion: c.reduce})
		*shown = true
		for range 3 {
			tt.Frame()
			tt.Advance(100 * time.Millisecond)
		}
		if n := loads.Load(); n != 0 || len(a.motion.clips) != 0 {
			t.Errorf("%s: %d videos fetched, %d players", c.name, n, len(a.motion.clips))
		}
	}
	// And with neither, it plays.
	a := newTestApp()
	ch := posts(a)
	tt, shown, loads := motionView(a)
	*shown = true
	tt.Frame()
	runPost(t, ch)
	tt.Frame()
	if loads.Load() != 1 || a.motion.clips["https://example.test/hls/master.m3u8|486x486"].player == nil {
		t.Errorf("on: %d fetched, clips %v", loads.Load(), a.motion.clips)
	}
}

// No more covers than motionPlayers decode at once: one more that shows
// waits with its still, until one of those stops showing and lets go of
// its player.
func TestMotionPlayers(t *testing.T) {
	if !video.Supported() {
		t.Skip("no decoder on this machine")
	}
	var loads atomic.Int32
	ch := make(chan func(), 16)
	m := &motionArt{clips: map[string]*motionClip{}, load: openClip(&loads), post: func(fn func()) { ch <- fn }}
	now := time.Now()
	for i := range motionPlayers + 1 {
		m.player(now, fmt.Sprint("https://example.test/", i), 400, 400, motionCard, true)
	}
	if busy := len(m.clips); busy != motionPlayers {
		t.Fatalf("%d covers loading at once", busy)
	}
	for range motionPlayers {
		runPost(t, ch)
	}
	// All but the first show on; the one left out asks again and takes its
	// place.
	later := now.Add(time.Second)
	for i := 1; i < motionPlayers; i++ {
		m.player(later, fmt.Sprint("https://example.test/", i), 400, 400, motionCard, true)
	}
	m.player(later, fmt.Sprint("https://example.test/", motionPlayers), 400, 400, motionCard, true)
	runPost(t, ch)
	if _, ok := m.clips["https://example.test/0|436x582"]; ok || len(m.clips) != motionPlayers {
		t.Errorf("clips %v", m.clips)
	}
	// Out of sight long enough, all let go.
	m.sweep(later.Add(motionKeep))
	if len(m.clips) != 0 {
		t.Errorf("kept %d", len(m.clips))
	}
}

// loading reports whether a cover's video is on its way.
func loading(m *motionArt) bool {
	for _, cl := range m.clips {
		if cl.loading {
			return true
		}
	}
	return false
}

// motionFrames saves what tt shows as the videos over its covers play: the
// stills before they come, then at 0, 0.5 and 1 second of them, the first
// as they start to fade in over the stills.
func motionFrames(t *testing.T, dir, name string, a *app, ch chan func(), tt *ui.Tester) {
	t.Helper()
	settled(tt)
	saveFrame(t, dir, name+"-still.png", tt.Image())
	for a.motion != nil && loading(a.motion) {
		runPost(t, ch)
	}
	tt.Frame()
	saveFrame(t, dir, name+"-0.0s.png", tt.Image())
	// A frame each 30th of a second, as the videos ask, with time for the
	// decoders, which run on the wall clock.
	for i := 1; i <= 30; i++ {
		tt.Advance(time.Second / 30)
		time.Sleep(10 * time.Millisecond)
		tt.Frame()
		switch i {
		case 15:
			saveFrame(t, dir, name+"-0.5s.png", tt.Image())
		case 30:
			saveFrame(t, dir, name+"-1.0s.png", tt.Image())
		}
	}
}

// With MM_FRAMES set, saves an album's page and Home's picks as their
// covers move, testdata's clip for each video.
func TestMotionFrames(t *testing.T) {
	dir := framesDir(t)
	var loads atomic.Int32

	a := albumApp()
	ch := posts(a)
	a.motionArt().load = openClip(&loads)
	a.albums["1440755169"].album.Album.VideoURL = "https://example.test/hls/album.m3u8"
	giveCover(a, "cover:1", coverImage(ui.RGB(40, 90, 170), ui.RGB(220, 120, 160)))
	tt := ui.NewTester(a.view, 1180, 780)
	tt.SetDark(true)
	motionFrames(t, dir, "motion-album", a, ch, tt)

	a = newTestApp()
	ch = posts(a)
	a.motionArt().load = openClip(&loads)
	a.account = &music.Account{ID: "1", Name: "Alex"}
	a.home = homePicks(a)
	// Every card of the picks tall and moving, as Apple's own with videos.
	for i := range a.home.Sections[0].Items {
		it := &a.home.Sections[0].Items[i]
		it.TallURL = fmt.Sprintf("tall:%d:{w}x{h}", i%5)
		it.TallVideoURL = fmt.Sprintf("https://example.test/hls/tall%d.m3u8", i)
	}
	tt = ui.NewTester(a.view, 1180, 780)
	tt.SetDark(true)
	motionFrames(t, dir, "motion-home", a, ch, tt)
}
