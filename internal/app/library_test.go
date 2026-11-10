package app

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// A source answering an error as a page loads shows the page's error, with Retry,
// which loads it again, and the page once it answers.
func TestPageErrorRetries(t *testing.T) {
	offline := errors.New("offline")
	for _, c := range []struct {
		name string
		r    route
		set  func(f *fakeSource, fails func() error)
	}{
		{"playlist", route{page: pagePlaylist, id: "7"}, func(f *fakeSource, fails func() error) {
			f.playlist = func(id string) (*music.Playlist, error) {
				if err := fails(); err != nil {
					return nil, err
				}
				return &music.Playlist{ID: id, Name: "Answered"}, nil
			}
		}},
		{"artist", route{page: pageArtist, id: "7"}, func(f *fakeSource, fails func() error) {
			f.artist = func(id string) (*music.ArtistPage, error) {
				if err := fails(); err != nil {
					return nil, err
				}
				return &music.ArtistPage{Artist: music.Artist{ID: id, Name: "Answered"}}, nil
			}
		}},
		{"album", route{page: pageAlbum, id: "7"}, func(f *fakeSource, fails func() error) {
			f.album = func(id string) (*music.AlbumPage, error) {
				if err := fails(); err != nil {
					return nil, err
				}
				return &music.AlbumPage{Album: music.Album{ID: id, Name: "Answered"}}, nil
			}
		}},
		{"browse", route{page: pageBrowse, id: "7"}, func(f *fakeSource, fails func() error) {
			f.browse = func(string) (*music.Browse, error) {
				if err := fails(); err != nil {
					return nil, err
				}
				return &music.Browse{Title: "Answered"}, nil
			}
		}},
		{"liked", route{page: pageLiked}, func(f *fakeSource, fails func() error) {
			f.liked = func(int) (*music.Page, error) {
				if err := fails(); err != nil {
					return nil, err
				}
				return &music.Page{Tracks: []music.Track{song("1", "Answered")}}, nil
			}
		}},
		{"search", route{page: pageSearch, query: "q"}, func(f *fakeSource, fails func() error) {
			f.browse = func(page string) (*music.Browse, error) {
				if err := fails(); err != nil {
					return nil, err
				}
				return &music.Browse{Sections: []music.Section{{Style: music.ShelfList, Items: []music.Item{
					{Kind: music.ItemSong, ID: "1", Title: "Answered", Track: &music.Track{ID: "1", Name: "Answered"}},
				}}}}, nil
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := newTestApp()
			ch := posts(a)
			var tries atomic.Int32
			c.set(fake(a), func() error {
				if tries.Add(1) == 1 {
					return offline
				}
				return nil
			})
			a.open(c.r)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()
			runPost(t, ch)
			tt.Frame()
			tt.Frame()
			if !tt.HasText("offline") {
				t.Errorf("doesn't say what's wrong: %q", tt.Texts())
			}
			if err := tt.Click("Try Again"); err != nil {
				t.Fatalf("%v: %q", err, tt.Texts())
			}
			runPost(t, ch)
			tt.Frame()
			tt.Frame()
			if tries.Load() != 2 {
				t.Errorf("asked %d times", tries.Load())
			}
			if _, ok := tt.Find("Try Again"); ok || !tt.HasText("Answered") {
				t.Errorf("once it answered, shows %q", tt.Texts())
			}
		})
	}
}

// Home that a source fails to answer shows the error, with Retry.
func TestHomeErrorRetries(t *testing.T) {
	var tries atomic.Int32
	a := newTestApp()
	ch := posts(a)
	fake(a).home = func() (*music.Browse, error) {
		if tries.Add(1) == 1 {
			return nil, errors.New("offline")
		}
		return &music.Browse{Sections: []music.Section{{Title: "Shelf", Albums: []music.Album{{ID: "1", Name: "Answered"}}}}}, nil
	}
	a.loadHome()
	runPost(t, ch)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if !tt.HasText("offline") {
		t.Errorf("doesn't say what's wrong: %q", tt.Texts())
	}
	if err := tt.Click("Try Again"); err != nil {
		t.Fatal(err)
	}
	runPost(t, ch)
	tt.Frame()
	if _, ok := tt.Find("Answered"); !ok {
		t.Errorf("once it answered, shows %q", tt.Texts())
	}
}

// A source that answers ErrSignIn opens the dialog once, however often it says
// so after, and the page offers to sign in for when it's put away.
func TestErrSignInAsksOnce(t *testing.T) {
	var tries atomic.Int32
	a := newTestApp()
	ch := posts(a)
	fake(a).playlist = func(string) (*music.Playlist, error) { tries.Add(1); return nil, music.ErrSignIn }
	a.open(route{page: pagePlaylist, id: "7"})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	runPost(t, ch)
	if !a.signIn.open || a.signIn.lib != a.library || !a.needsSignIn {
		t.Fatalf("dialog %+v, needs signing in %v", a.signIn, a.needsSignIn)
	}
	settled(tt)
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	settled(tt)
	if a.signIn.open {
		t.Fatal("Cancel left the dialog open")
	}

	if err := tt.Click("Try Again"); err != nil {
		t.Fatal(err)
	}
	runPost(t, ch)
	settled(tt)
	if a.signIn.open || tries.Load() != 2 {
		t.Fatalf("asked again: dialog open %v, tried %d times", a.signIn.open, tries.Load())
	}

	if err := tt.Click("Sign In"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	if !a.signIn.open {
		t.Error("the page's Sign In opened no dialog")
	}
}

// A source that answers ErrSetup needs setting up, which shows on the page and
// offers the account settings, and goes once a setting is kept.
func TestErrSetupOffersSettings(t *testing.T) {
	kept := make(chan string, 1)
	a := newTestApp()
	ch := posts(a)
	fake(a).playlist = func(string) (*music.Playlist, error) { return nil, music.ErrSetup }
	fake(a).setSetting = func(key, value string) error { kept <- key + "=" + value; return nil }
	a.library.askedSetup = true // opening the window takes a real app
	a.open(route{page: pagePlaylist, id: "7"})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	runPost(t, ch)
	tt.Frame()
	if !a.needsSetup {
		t.Fatal("doesn't need setting up")
	}
	if _, ok := tt.Find("Open Settings"); !ok {
		t.Errorf("the page offers no settings: %q", tt.Texts())
	}

	a.settingsPage = "playback"
	settings := ui.NewTester(a.settingsView, 760, 1400)
	settings.Frame()
	// The field, under the title labelled as it is.
	r, ok := settings.Find("Wrapper Address")
	if !ok {
		t.Fatal("no field")
	}
	settings.ClickAt(r.X+20, r.Y+r.H+18)
	settings.Type("http://wrapper.example")
	settings.Key(0, ui.KeyEnter)
	if got := wait(t, kept, "the setting"); got != "wrapper=http://wrapper.example" {
		t.Fatalf("kept %s", got)
	}
	until(t, ch, func() bool { return !a.needsSetup })
	if a.askedSetup {
		t.Error("it wouldn't ask next time it needs it")
	}
}

// A source that needs setting up says what for, on its page and in the account
// settings, where what it said is what's to fix.
func TestErrSetupSaysWhy(t *testing.T) {
	const why = "Install ffmpeg to play this"
	a := newTestApp()
	ch := posts(a)
	fake(a).playlist = func(string) (*music.Playlist, error) {
		return nil, fmt.Errorf("loading: %w", music.Fail(music.ErrSetup, why))
	}
	a.library.askedSetup = true // opening the window takes a real app
	a.open(route{page: pagePlaylist, id: "7"})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	runPost(t, ch)
	tt.Frame()
	if _, ok := tt.Find(why); !ok {
		t.Errorf("the page doesn't say why: %q", tt.Texts())
	}

	a.settingsPage = "account"
	settings := ui.NewTester(a.settingsView, 760, 1400)
	settings.Frame()
	if _, ok := settings.Find(why); !ok {
		t.Errorf("the settings don't say why: %q", settings.Texts())
	}
}
