package app

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// sourced is tracks of the source called fake, as the player goes by.
func sourced(tracks ...music.Track) []music.Track {
	for i := range tracks {
		tracks[i].Source = "fake"
	}
	return tracks
}

// nightDrive is the station radioApp plays.
var nightDrive = music.Station{ID: "st", Name: "Night Drive"}

// radioApp is signed in, with what the app posts in hand.
func radioApp() (*app, chan func()) {
	a := newTestApp()
	ch := posts(a)
	without(a, "isLiked")
	a.account = &music.Account{ID: "1"}
	return a, ch
}

// Starting a station plays what its source gives, and it goes on asking for
// more as the songs near their end, without the songs it has twice.
func TestStationPlaysAndGoesOn(t *testing.T) {
	asked := make(chan string, 4)
	a, ch := radioApp()
	batches := [][]music.Track{sourced(song("1", "一"), song("2", "二")), sourced(song("2", "二"), song("3", "三"))}
	fake(a).stationNext = func(id string) ([]music.Track, error) {
		asked <- id
		b := batches[0]
		if len(batches) > 1 {
			batches = batches[1:]
		}
		return b, nil
	}
	st := nightDrive
	a.playStation(a.library, &st)
	if id := wait(t, asked, "the first songs"); id != "st" {
		t.Fatalf("asked station %q", id)
	}
	until(t, ch, func() bool { return a.pl.Radio() != "" })
	if a.pl.Radio() != radioName("fake", &nightDrive) || a.pl.Current().ID != "1" || a.radioStarting != "" {
		t.Fatalf("radio %q, playing %+v, starting %q", a.pl.Radio(), a.pl.Current(), a.radioStarting)
	}
	wait(t, asked, "more songs")
	until(t, ch, func() bool { return len(a.pl.Queue()) > 2 })
	if q := a.pl.Queue(); len(q) != 3 || q[2].ID != "3" {
		t.Errorf("the queue is %+v", q)
	}
}

// Playing the station playing goes on with it, not starting it again.
func TestStationPlayingGoesOn(t *testing.T) {
	a, _ := radioApp()
	fake(a).stationNext = func(string) ([]music.Track, error) { t.Error("started it again"); return nil, nil }
	a.pl.Cue(sourced(song("1", "一"), song("2", "二"), song("3", "三")))
	a.pl.AttachRadio(radioName("fake", &nightDrive), func(func([]music.Track)) {})
	a.playStation(a.library, &nightDrive)
	if a.radioStarting != "" {
		t.Errorf("starting %q", a.radioStarting)
	}
}

// A station that only gives what can't be played is asked again a few times,
// then says it can't play, and one that fails says so too.
func TestStationWithNothingToPlay(t *testing.T) {
	asked := 0
	a, ch := radioApp()
	unavailable := song("1", "一")
	unavailable.Unavailable = true
	fake(a).stationNext = func(string) ([]music.Track, error) { asked++; return []music.Track{unavailable}, nil }
	a.playStation(a.library, &nightDrive)
	runPost(t, ch)
	if asked != 3 || a.pl.Radio() != "" || a.pl.Current() != nil || a.toast == "" || a.radioStarting != "" {
		t.Fatalf("asked %d times, radio %q, toast %q, starting %q", asked, a.pl.Radio(), a.toast, a.radioStarting)
	}

	a.toast = ""
	fake(a).stationNext = func(string) ([]music.Track, error) { return nil, errors.New("offline") }
	a.playStation(a.library, &nightDrive)
	runPost(t, ch)
	if a.pl.Radio() != "" || a.toast == "" {
		t.Errorf("radio %q, toast %q", a.pl.Radio(), a.toast)
	}
}

// A station asked for while signed out asks the listener to sign in, not it.
func TestStationSignedOut(t *testing.T) {
	a, _ := radioApp()
	a.account = nil
	fake(a).stationNext = func(string) ([]music.Track, error) { t.Error("asked the station"); return nil, nil }
	a.playStation(a.library, &nightDrive)
	if !a.signIn.open {
		t.Error("didn't ask to sign in")
	}
}

// Now Playing's chip leaves the radio: the song plays on, and those the radio
// queued after it go.
func TestLeaveRadio(t *testing.T) {
	a, _ := radioApp()
	a.pl.Cue(sourced(song("1", "晚风"), song("2", "推荐的歌"), song("3", "下一首")))
	a.pl.SetLyrics(nil)
	a.pl.AttachRadio(radioName("fake", &nightDrive), func(func([]music.Track)) {})
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("Leave Night Drive"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.Radio() != "" || a.pl.Current().ID != "1" || len(a.pl.Queue()) != 1 {
		t.Fatalf("radio %q, current %+v, queue %d", a.pl.Radio(), a.pl.Current(), len(a.pl.Queue()))
	}
	if _, ok := tt.Find("Leave Night Drive"); ok {
		t.Error("the chip stayed")
	}
}

// A radio's name gives back the source, the station and what it's called, and
// something else names none.
func TestRadioName(t *testing.T) {
	src, id, label := radioParts(radioName("fake", &music.Station{ID: "st", Name: "Night Drive"}))
	if src != "fake" || id != "st" || label != "Night Drive" {
		t.Errorf("source %q, station %q, called %q", src, id, label)
	}
	if src, _, _ := radioParts("heart"); src != "" {
		t.Errorf("source %q of something else", src)
	}
}
