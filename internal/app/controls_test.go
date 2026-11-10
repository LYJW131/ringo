package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
	"ringo/internal/player"
)

// controlsOpen is Now Playing open on the first of three songs, 50 seconds
// in, its pane pane, and its window at rest.
func controlsOpen(pane int) (*app, *ui.Tester) {
	a := newTestApp()
	a.pl.Cue([]music.Track{song("1", "一"), song("2", "二"), song("3", "三")})
	a.pl.SetLyrics(nil)
	a.pl.Hold(50 * time.Second)
	a.nowPlaying, a.npPane = true, pane
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	return a, tt
}

// The times beside the progress bar say where it is dragged to while it
// is, in each pane of Now Playing, and where the song is again once it is
// let go: they stayed where the song played, and the drag showed only on
// the bar.
func TestTimesFollowTheDrag(t *testing.T) {
	for _, c := range []struct {
		name          string
		pane          int
		dragged, left string
	}{
		{"lyrics", paneLyrics, "2:00", "-1:20"},
		{"stage", paneStage, "2:00 / 3:20", ""},
		{"wall", paneWall, "2:00", "-1:20"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, tt := controlsOpen(c.pane)
			bar, ok := tt.Find("Playback Progress")
			if !ok {
				t.Fatal("no progress bar")
			}
			y := bar.Y + bar.H/2
			tt.Press(bar.X+bar.W*0.25, y)
			tt.Frame()
			tt.Move(bar.X+bar.W*0.6, y)
			tt.Frame()
			tt.Frame()
			if !tt.HasText(c.dragged) || c.left != "" && !tt.HasText(c.left) {
				t.Errorf("dragged to 60%% of 3:20, the times are not %q and %q: %q", c.dragged, c.left, tt.Texts())
			}
			if pos := a.pl.Position(); pos != 50*time.Second {
				t.Errorf("seeked to %v while dragging", pos)
			}
			tt.Release(bar.X+bar.W*0.6, y)
			tt.Frame()
			tt.Frame()
			if pos := a.pl.Position(); pos < 115*time.Second || pos > 125*time.Second {
				t.Errorf("let go at 60%%, at %v", pos)
			}
		})
	}
}

// The buttons under the cover do what they say, where they are on glass:
// shuffle and repeat turn on and off, the ones beside the play button go
// from song to song, and the play button pauses.
func TestNowPlayingTransport(t *testing.T) {
	for _, pane := range []int{paneLyrics, paneStage, paneWall} {
		a, tt := controlsOpen(pane)
		click := func(label string) {
			t.Helper()
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
		}
		click("Shuffle")
		if !a.pl.Shuffle() {
			t.Errorf("pane %d: Shuffle did not shuffle", pane)
		}
		click("Shuffle")
		if a.pl.Shuffle() {
			t.Errorf("pane %d: Shuffle stayed on", pane)
		}
		click("Repeat")
		click("Repeat")
		if a.pl.Repeat() != player.RepeatOne {
			t.Errorf("pane %d: Repeat twice repeats %v", pane, a.pl.Repeat())
		}
		click("Repeat")
		if a.pl.Repeat() != player.RepeatOff {
			t.Errorf("pane %d: Repeat a third time repeats %v", pane, a.pl.Repeat())
		}
		click("Next")
		if id := a.pl.Current().ID; id != "2" {
			t.Errorf("pane %d: Next plays %s", pane, id)
		}
		click("Previous")
		if id := a.pl.Current().ID; id != "1" {
			t.Errorf("pane %d: Previous plays %s", pane, id)
		}
		click("Play/Pause")
		if a.pl.Playing() {
			t.Errorf("pane %d: the play button did not pause", pane)
		}
	}
}

// The volume under the cover is set where its bar is pressed, and the
// speaker at its quiet end mutes it and brings it back.
func TestNowPlayingVolume(t *testing.T) {
	a, tt := controlsOpen(paneLyrics)
	a.volume = 0.5
	tt.Frame()
	bar, ok := tt.Find("Volume")
	if !ok {
		t.Fatal("no volume")
	}
	tt.ClickAt(bar.X+bar.W*0.8, bar.Y+bar.H/2)
	tt.Frame()
	if a.volume < 0.75 || a.volume > 0.85 {
		t.Fatalf("pressed at 80%%, the volume is %v", a.volume)
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.volume != 0 {
		t.Errorf("muted, the volume is %v", a.volume)
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.volume < 0.75 {
		t.Errorf("unmuted, the volume is %v", a.volume)
	}
}

// AutoPlay's slip of glass, under the songs of Up Next, turns AutoPlay on
// and off.
func TestAutoPlayPill(t *testing.T) {
	a, tt := controlsOpen(paneQueue)
	for _, want := range []bool{true, false} {
		if err := tt.Click("AutoPlay"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.Autoplay != want {
			t.Fatalf("clicked, AutoPlay is %v", a.settings.Autoplay)
		}
	}
}
