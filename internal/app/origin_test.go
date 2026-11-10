package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// origins lists where the queue's songs are played from, in the order they
// were given.
func origins(a *app) []music.Origin {
	var out []music.Origin
	for _, tr := range a.pl.Queue() {
		out = append(out, tr.Origin)
	}
	return out
}

// Whatever starts a queue says where it's from, which Apple's Recently
// Played lists: an album's page plays as the album, a playlist's as the
// playlist, an artist's as the artist, a song picked from some other list as
// nothing, and what's queued from a song's menu keeps to what it is.
func TestPlayedFrom(t *testing.T) {
	// An album's Play button, and a row of it.
	a := albumApp()
	album := music.Origin{Kind: music.FromAlbum, ID: a.route.id}
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	for _, o := range origins(a) {
		if o != album {
			t.Fatalf("the album plays from %+v", o)
		}
	}
	before := len(a.pl.Queue())
	if err := tt.RightClick("Sparks Fly"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Play Last"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if q := a.pl.Queue(); len(q) != before+1 || q[before].Origin != (music.Origin{}) || q[0].Origin != album {
		t.Errorf("a song queued from its menu is from %+v", origins(a))
	}
	a.pl.PlayTrack(a.albums[a.route.id].album.Tracks, 2, a.pageOrigin())
	if a.pl.Current().Origin != album {
		t.Errorf("a row of the album plays from %+v", a.pl.Current().Origin)
	}

	// A playlist's, with what Apple asks of it that its page came with.
	a = newTestApp()
	a.playlists["pl.1"] = &music.Playlist{ID: "pl.1", Name: "Today's Hits", Version: "v1", Tracks: []music.Track{song("1", "Hong Kong")}}
	a.route = route{page: pagePlaylist, src: "fake", id: "pl.1"}
	tt = ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	if got, want := a.pl.Current().Origin, (music.Origin{Kind: music.FromPlaylist, ID: "pl.1", Version: "v1"}); got != want {
		t.Errorf("the playlist plays from %+v", got)
	}

	// An artist's top songs.
	a = artistApp(false)
	tt = ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	if got, want := a.pl.Current().Origin, (music.Origin{Kind: music.FromArtist, ID: "159260351"}); got != want {
		t.Errorf("the artist plays from %+v", got)
	}

	// Anywhere else, a song is played from nothing.
	a = newTestApp()
	a.route = route{page: pageHome, src: "fake"}
	a.playRow([]music.Track{song("1", "Hong Kong")}, 0)
	if got := a.pl.Current().Origin; got != (music.Origin{}) {
		t.Errorf("a song of Home plays from %+v", got)
	}

	// An album's songs queued with Play Next keep to it.
	a = albumApp()
	a.tracksOf(a.library, music.ItemAlbum, a.route.id, func(tracks []music.Track, from music.Origin) {
		if from != album || len(tracks) == 0 {
			t.Errorf("an album's songs queued from %+v", from)
		}
	})
}
