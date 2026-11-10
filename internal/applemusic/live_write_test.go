package applemusic

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"ringo/internal/music"
)

// Writing to a real account, so run only with APPLE_MUSIC_WRITE_TOKEN_FILE,
// the token of an account kept for testing, apart from the read-only
// APPLE_MUSIC_TOKEN_FILE: favorites, the library and autoplay, each read
// back, and everything put back as it was. Nothing is reported as played.
// Reading back waits up to settleWithin for Apple to show a change.

const settleWithin = 2 * time.Minute

// writeSource is a source signed in to the account kept for testing, or the
// test is skipped without one.
func writeSource(t *testing.T) *Source {
	t.Helper()
	file := os.Getenv("APPLE_MUSIC_WRITE_TOKEN_FILE")
	if file == "" {
		t.Skip("APPLE_MUSIC_WRITE_TOKEN_FILE names the token of an account that may be written to")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	t.Cleanup(func() { s.Close() })
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	return s
}

// waitFor reads back until ok, or fails once settleWithin is up.
func waitFor(t *testing.T, what string, ok func() bool) bool {
	t.Helper()
	start := time.Now()
	for !ok() {
		if time.Since(start) > settleWithin {
			t.Errorf("%s: not so after %v", what, settleWithin)
			return false
		}
		time.Sleep(10 * time.Second)
	}
	t.Logf("%s, after %v", what, time.Since(start).Round(time.Second))
	return true
}

func TestLiveWrites(t *testing.T) {
	s := writeSource(t)
	found, err := s.Browse(music.PageSearch + "taylor swift")
	if err != nil {
		t.Fatal(err)
	}
	waitFor := func(what string, ok func() bool) { t.Helper(); waitFor(t, what, ok) }
	state := func(typ, id string) (fav, lib bool) {
		t.Helper()
		own, fav, err := s.personal(typ, id)
		if err != nil {
			t.Fatalf("reading back %s %s: %v", typ, id, err)
		}
		return fav, own != nil
	}

	for _, kind := range []string{music.ItemAlbum, music.ItemPlaylist, music.ItemArtist} {
		id := firstItem(found, kind)
		typ := catalogTypes[kind]
		if id == "" {
			t.Errorf("no %s found", kind)
			continue
		}
		wasFav, wasLib := state(typ, id)
		if err := s.Favorite(kind, id, true); err != nil {
			t.Errorf("favorite %s: %v", kind, err)
			continue
		}
		waitFor("a favorite "+kind, func() bool { fav, _ := state(typ, id); return fav })
		if err := s.Favorite(kind, id, wasFav); err != nil {
			t.Errorf("unfavorite %s: %v", kind, err)
		}
		waitFor("the "+kind+"'s favorite as it was", func() bool { fav, _ := state(typ, id); return fav == wasFav })
		// A favorite album or playlist goes into the library too: out again.
		if _, lib := state(typ, id); lib && !wasLib && kind != music.ItemArtist {
			t.Logf("the favorite %s went into the library: taking it out", kind)
			if err := s.AddToLibrary(kind, id, false); err != nil {
				t.Errorf("taking the %s out: %v", kind, err)
			}
			waitFor("the "+kind+" out of the library", func() bool { _, lib := state(typ, id); return !lib })
		}
	}

	// A song is read back as the app asks of the songs it shows (InLibrary).
	song := firstItem(found, music.ItemSong)
	inLib := func() bool {
		t.Helper()
		in, err := s.InLibrary(music.ItemSong, []string{song})
		if err != nil {
			t.Fatalf("asking about the song: %v", err)
		}
		return in[song]
	}
	if inLib() {
		t.Fatalf("the song is in the library already")
	}
	if err := s.AddToLibrary(music.ItemSong, song, true); err != nil {
		t.Fatalf("adding a song: %v", err)
	}
	waitFor("the song in the library", inLib)
	added, err := s.Browse(music.PageLibrarySongs)
	if err == nil {
		t.Logf("library songs: %s", shape(added))
	}
	if err := s.AddToLibrary(music.ItemSong, song, false); err != nil {
		t.Errorf("taking the song out: %v", err)
	}
	waitFor("the song out of the library", func() bool { return !inLib() })

	next, err := s.Autoplay([]string{song})
	if err != nil {
		t.Errorf("autoplay: %v", err)
	} else if len(next) == 0 {
		t.Error("autoplay: no songs")
	} else {
		named := 0
		for _, tr := range next {
			if tr.Name != "" && isCatalogID(tr.ID) {
				named++
			}
		}
		t.Logf("autoplay: %d songs, %d with a name and a catalog ID", len(next), named)
	}
}

// One of the listener's playlists made for the test, edited and deleted:
// three songs added, renamed, the third moved to first, one taken out, each
// read back from Apple itself, past what the source answers of its own
// changes; then deleted. Whatever happens, it's deleted at the end. Only the
// test's playlist is named in what's logged.
func TestLivePlaylistEdits(t *testing.T) {
	s := writeSource(t)
	found, err := s.Browse(music.PageSearch + "taylor swift")
	if err != nil {
		t.Fatal(err)
	}
	var songs []string
	for _, sec := range found.Sections {
		for _, it := range sec.Items {
			if it.Kind == music.ItemSong && isCatalogID(it.ID) && !slices.Contains(songs, it.ID) && len(songs) < 3 {
				songs = append(songs, it.ID)
			}
		}
	}
	if len(songs) < 3 {
		t.Fatalf("found %d songs, not 3", len(songs))
	}

	name := "[Ringo test] playlist edits " + time.Now().UTC().Format("20060102-150405")
	const description = "Made by Ringo's TestLivePlaylistEdits, and deleted by it."
	b, err := s.me("/library/playlists", nil, reqOpts{method: "POST", body: map[string]any{"attributes": map[string]any{"name": name, "description": description}}})
	if err != nil {
		t.Fatal(err)
	}
	made, err := first(b)
	if err != nil {
		t.Fatal(err)
	}
	id := made.ID
	t.Logf("made %q, %s", name, id)
	deleted := false
	defer func() {
		if deleted {
			return
		}
		if _, err := s.me("/library/playlists/"+id, nil, reqOpts{method: "DELETE"}); err != nil {
			t.Errorf("deleting the test's playlist: %v", err)
		} else {
			t.Log("deleted the test's playlist")
		}
	}()

	// What Apple says of it, asked anew, without what the source keeps.
	type said struct {
		name, description string
		songs             []string
	}
	apple := func() (said, bool) {
		t.Helper()
		b, err := s.me("/library/playlists/"+id, nil, reqOpts{})
		if err != nil {
			t.Logf("reading it back: %v", err)
			return said{}, false
		}
		pl, err := first(b)
		if err != nil {
			return said{}, false
		}
		out := said{name: pl.attrs().Name, description: pl.attrs().Description.long()}
		tracks, err := s.allPages("/v1/me/library/playlists/"+id+"/tracks", params{"limit": "100"}, 100, nil)
		if err != nil && !errors.Is(err, music.ErrUnavailable) {
			t.Logf("reading its songs back: %v", err)
			return said{}, false
		}
		for _, tr := range tracks {
			if tr != nil {
				out.songs = append(out.songs, songID(tr))
			}
		}
		return out, true
	}
	hasSongs := func(want ...string) func() bool {
		return func() bool { got, ok := apple(); return ok && slices.Equal(got.songs, want) }
	}

	if err := s.AddToPlaylist(id, songs); err != nil {
		t.Fatalf("adding songs: %v", err)
	}
	if !waitFor(t, "three songs in it", hasSongs(songs...)) {
		t.FailNow()
	}

	renamed := name + " (renamed)"
	if err := s.RenamePlaylist(id, renamed); err != nil {
		t.Fatalf("renaming: %v", err)
	}
	if pl, err := s.Playlist(id); err != nil || pl.Name != renamed {
		t.Errorf("the source doesn't answer with the new name at once (%v)", err)
	}
	waitFor(t, "its new name", func() bool { got, ok := apple(); return ok && got.name == renamed })
	if got, ok := apple(); ok && got.description != description {
		t.Errorf("renaming it changed its description to %q", got.description)
	}

	if err := s.MovePlaylistTrack(id, 2, 0); err != nil {
		t.Fatalf("moving the third song to first: %v", err)
	}
	waitFor(t, "the third song first", hasSongs(songs[2], songs[0], songs[1]))

	if err := s.RemoveFromPlaylist(id, []string{songs[0]}); err != nil {
		t.Fatalf("taking a song out: %v", err)
	}
	waitFor(t, "a song taken out", hasSongs(songs[2], songs[1]))

	if err := s.DeletePlaylist(id); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	deleted = true
	if lists, err := s.Playlists(); err != nil || slices.ContainsFunc(lists, func(p music.Playlist) bool { return p.ID == id }) {
		t.Errorf("the source still lists it at once (%v)", err)
	}
	waitFor(t, "it deleted", func() bool {
		lists, err := s.allPages("/v1/me/library/playlists", params{"limit": "100"}, 500, nil)
		return err == nil && !slices.ContainsFunc(lists, func(p *resource) bool { return p != nil && p.ID == id })
	})
}
