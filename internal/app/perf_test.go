package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/lyrics"
	"ringo/internal/music"
)

// playingApp is an app playing a track with lyrics, at a position the test
// sets with pl.Seek, and the number of times its view was built.
func playingApp() (*app, *ui.Tester, *int) {
	a := newTestApp()
	a.pl.Cue([]music.Track{song("7", "宝船", "DOUDOU")})
	// A line every 10 seconds, sung over the first 5 of them.
	var sung []music.LyricLine
	for i := range 12 {
		sung = append(sung, music.LyricLine{Start: int64(i) * 10000, End: int64(i)*10000 + 5000, Text: "一二三四五六"})
	}
	lines := lyrics.FromSource(&music.Lyrics{Lines: sung})
	a.pl.SetLyrics(lines)
	a.pl.Hold(0)
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) { builds++; a.view(c) }, 1200, 800)
	return a, tt, &builds
}

// framesAt counts the frames the view asks for over a second, at 60 a
// second, with playback held at pos.
func framesAt(a *app, tt *ui.Tester, pos time.Duration) int {
	a.pl.Seek(pos)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	asked := 0
	for range 60 {
		tt.Advance(time.Second / 60)
		if tt.FramePending() {
			asked++
			tt.Frame()
		}
	}
	return asked
}

// Now Playing asks for frames only as fast as what it shows changes. The
// Tester runs timers as it settles a frame, so this counts only what does
// not change for a while: a frame log of the app (MYGO_FRAME_STATS) measures
// the rest.
func TestNowPlayingFrameBudget(t *testing.T) {
	a, tt, _ := playingApp()
	if n := framesAt(a, tt, 12*time.Second); n > 3 {
		t.Errorf("a page under the player: %d frames a second", n)
	}
	// Its rows still, the wall asks for no more than the lyrics beside it
	// do: it moves only as it is moved.
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	a.nowPlaying, a.npPane = true, paneLyrics
	// The lyrics between two lines, that is: a line sung asks for the
	// frames that move the light across it.
	lyricsFrames := framesAt(a, tt, 16*time.Second)
	a.npPane = paneWall
	if n := framesAt(a, tt, 12*time.Second); n > lyricsFrames+3 {
		t.Errorf("the wall, still: %d frames a second, the lyrics %d", n, lyricsFrames)
	}
	tt.SetPreferences(ui.Preferences{})
	a.npPane = paneStage
	a.settings.StageSparks = true
	for _, s := range stageStyles {
		a.settings.StageStyle = s.id
		if n := framesAt(a, tt, 18*time.Second); n > 12 {
			t.Errorf("the stage between lines, in %s: %d frames a second", s.label, n)
		}
	}
}

// The lyrics, between a line sung to its last word and the next, build the
// view no more than the times shown ask for; and while a line's words
// light, the frames that move the light only paint. Counted as builds,
// which FramePending misses for a frame waiting on After: the words asked
// to be lit every 40 ms after the last of them was, and then, lit by the
// view, built the whole of Now Playing 25 times a second.
func TestLyricsBuildBudget(t *testing.T) {
	a, tt, builds := playingApp()
	a.nowPlaying, a.npPane = true, paneLyrics
	count := func(pos time.Duration) int {
		a.pl.Seek(pos)
		tt.Frame()
		// Opened, and come to rest: the backdrop fades in from the first.
		tt.Advance(2 * time.Second)
		n := *builds
		tt.Advance(time.Second)
		return *builds - n
	}
	// Sung over its first 5 seconds, the second line rests from 15 s to 20 s.
	if n := count(16 * time.Second); n > 3 {
		t.Errorf("the lyrics between lines: %d builds a second", n)
	}
	if tt.FramePending() {
		t.Error("the lyrics ask for frames between lines")
	}
	// While its words light, the line asks for the frames, as it should.
	if n := count(11 * time.Second); n > 3 {
		t.Errorf("the lyrics lighting a line's words: %d builds a second", n)
	}
	if !tt.FramePending() {
		t.Error("the line sung asks for no frames as its words light")
	}
}

// Home with a source's own page, its feature card at rest while nothing plays,
// asks for no frames once shown.
func TestHomeFrameBudget(t *testing.T) {
	a := newTestApp()
	a.home = &music.Browse{Sections: []music.Section{
		{Title: "For You", Tracks: []music.Track{song("1", "一"), song("2", "二")}},
		{Title: "New Albums", Albums: []music.Album{{ID: "8", Name: "迷跡波"}}},
	}}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(2 * time.Second)
	tt.Frame()
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Error("Home asks for frames while idle")
	}
}

// Another song's backdrop fades in over the last one's, which is then let
// go, so that the backdrop stops asking for frames.
func TestBackdropFadeEnds(t *testing.T) {
	a, tt, _ := coverApp(t)
	a.art.art[sized("a", 600)+"#backdrop"] = &artwork{tint: ui.RGB(200, 40, 40)}
	a.art.art[sized("b", 600)+"#backdrop"] = &artwork{tint: ui.RGB(40, 40, 200)}
	q := a.pl.Queue()
	q[1].Album.ImageURL, q[2].Album.ImageURL = "a", "b"
	tt.Frame()
	a.pl.Next()
	tt.Frame()
	if a.backdropCache.before == nil {
		t.Fatal("the backdrop changed songs without fading")
	}
	tt.Advance(backdropFade + 100*time.Millisecond)
	tt.Frame()
	if a.backdropCache.before != nil {
		t.Error("the last song's backdrop still fades after the fade")
	}
}
