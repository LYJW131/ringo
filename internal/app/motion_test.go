package app

import (
	"fmt"
	"os"
	"sync"
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
		box.Children(func() {
			a.motionOver(c, box, movingVideo("https://example.test/hls/master.m3u8"), 200, 200, motionHead, 8)
		})
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
	m := &motionArt{clips: map[string]*motionClip{}, load: openClip(&loads), post: func(fn func()) { ch <- fn }, players: motionPlayers}
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

// A page's head or Now Playing's cover takes a player from a row's cover
// that shows, where all are taken: with a page of songs moving, the cover
// the listener opened stayed still.
func TestMotionPrimeTakesAPlayer(t *testing.T) {
	if !video.Supported() {
		t.Skip("no decoder on this machine")
	}
	var loads atomic.Int32
	ch := make(chan func(), 16)
	m := &motionArt{clips: map[string]*motionClip{}, load: openClip(&loads), post: func(fn func()) { ch <- fn }, players: 2}
	now := time.Now()
	for i := range 2 {
		m.player(now, fmt.Sprint("https://example.test/row", i), 80, 80, motionSquare, true)
	}
	runPost(t, ch)
	runPost(t, ch)
	// The rows show on, and another row finds no player free.
	later := now.Add(100 * time.Millisecond)
	for i := range 2 {
		m.player(later, fmt.Sprint("https://example.test/row", i), 80, 80, motionSquare, true)
	}
	m.player(later, "https://example.test/row2", 80, 80, motionSquare, true)
	if len(m.clips) != 2 {
		t.Fatalf("a third row took a player: %v", m.clips)
	}
	m.player(later, "https://example.test/stage", 900, 900, motionStageMac, true)
	if _, ok := m.clips["https://example.test/stage|960x960"]; !ok || len(m.clips) != 2 {
		t.Errorf("Now Playing's cover found no player: %v", m.clips)
	}
	runPost(t, ch)
	for _, cl := range m.clips {
		if cl.player != nil {
			cl.player.Close()
		}
	}
}

// moverSource is a fakeSource whose covers move, as motion answers.
type moverSource struct {
	*fakeSource
	motion func(kind string, ids []string) (map[string]string, error)
}

func (s *moverSource) Motion(kind string, ids []string) (map[string]string, error) {
	return s.motion(kind, ids)
}

// moverApp is an app whose source answers that song 7's album moves, with
// the kinds and IDs it was asked about.
func moverApp() (*app, *[]string) {
	a := newTestApp()
	var asked []string
	var mu sync.Mutex
	a.library.src = &moverSource{fake(a), func(kind string, ids []string) (map[string]string, error) {
		mu.Lock()
		defer mu.Unlock()
		out := map[string]string{}
		for _, id := range ids {
			asked = append(asked, kind+"|"+id)
			if kind == music.ItemSong && id == "7" {
				out[id] = "https://example.test/hls/polar.m3u8"
			}
		}
		return out, nil
	}}
	return a, &asked
}

// A song's cover that its page didn't say moves is asked about as it
// shows, once however many places show it and however often, and then
// plays in Now Playing: it stayed still there, its album's video never
// looked for.
func TestMotionAskedAsShown(t *testing.T) {
	if !video.Supported() {
		t.Skip("no decoder on this machine")
	}
	a, asked := moverApp()
	ch := posts(a)
	var loads atomic.Int32
	a.motionArt().load = openClip(&loads)
	a.pl.Cue([]music.Track{coveredSong(a)})
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1180, 780)
	tt.Frame()
	a.askLikes()
	// The source's answer, then the video it named.
	runPost(t, ch)
	for range 3 {
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
		a.askLikes()
	}
	for a.motion != nil && loading(a.motion) {
		runPost(t, ch)
	}
	if len(*asked) != 1 || (*asked)[0] != "song|7" {
		t.Errorf("asked %v", *asked)
	}
	key := "https://example.test/hls/polar.m3u8|" + fmt.Sprintf("%dx%d", stageMotion().w, stageMotion().h)
	if cl := a.motion.clips[key]; cl == nil || cl.player == nil {
		t.Errorf("Now Playing's cover doesn't move: %v", a.motion.clips)
	}
	a.motion.stop()
}

// A song's cover moves where it shows large, in Now Playing, and stays
// still where it is small, in the player bar and the notch: it moved in
// those too, two windows building one player, which then stood still in
// both, and took players from the large ones.
func TestMotionOnlyLarge(t *testing.T) {
	if !video.Supported() {
		t.Skip("no decoder on this machine")
	}
	a := newTestApp()
	ch := posts(a)
	var loads atomic.Int32
	a.motionArt().load = openClip(&loads)
	tr := coveredSong(a)
	tr.Album.VideoURL = "https://example.test/hls/polar.m3u8"
	a.pl.Cue([]music.Track{tr})
	// A display with a notch, which shows the song that plays beside it.
	a.settings.Notch, a.settings.NotchActivity = true, true
	n := &notch{a: a, real: true, w0: 185, h0: 32, winW: notchOpenW + 60, winH: 240}
	a.notch = n
	main := ui.NewTester(a.view, 1180, 780)
	side := ui.NewTester(n.view, int(n.winW), int(n.winH))
	for range 3 {
		main.Advance(100 * time.Millisecond)
		side.Advance(100 * time.Millisecond)
		main.Frame()
		side.Frame()
	}
	if loads.Load() != 0 {
		t.Fatalf("the bar or the notch fetched %d videos", loads.Load())
	}
	a.nowPlaying = true
	main.Frame()
	runPost(t, ch)
	main.Frame()
	key := fmt.Sprintf("%s|%dx%d", tr.Album.VideoURL, stageMotion().w, stageMotion().h)
	if cl := a.motion.clips[key]; cl == nil || cl.player == nil || len(a.motion.clips) != 1 {
		t.Errorf("Now Playing's cover: clips %v", a.motion.clips)
	}
	a.motion.stop()
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

	// Now Playing, its cover moving as its page never said: the source is
	// asked, and answers before the videos load.
	a, _ = moverApp()
	ch = posts(a)
	a.motionArt().load = openClip(&loads)
	a.pl.Cue([]music.Track{coveredSong(a)})
	a.nowPlaying, a.npPane = true, paneQueue
	tt = ui.NewTester(func(c *ui.Context) { a.view(c); a.askLikes() }, 1180, 780)
	tt.SetDark(true)
	settled(tt)
	runPost(t, ch)
	motionFrames(t, dir, "motion-nowplaying", a, ch, tt)
}
