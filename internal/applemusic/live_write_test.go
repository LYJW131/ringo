package applemusic

import (
	"os"
	"strings"
	"testing"
	"time"

	"mygo-music/internal/music"
)

// Writing to a real account, so run only with APPLE_MUSIC_WRITE_TOKEN_FILE,
// the token of an account kept for testing, apart from the read-only
// APPLE_MUSIC_TOKEN_FILE: favorites, the library and autoplay, each read
// back, and everything put back as it was. Nothing is reported as played.
// Reading back waits up to settleWithin for Apple to show a change.

const settleWithin = 2 * time.Minute

func TestLiveWrites(t *testing.T) {
	file := os.Getenv("APPLE_MUSIC_WRITE_TOKEN_FILE")
	if file == "" {
		t.Skip("APPLE_MUSIC_WRITE_TOKEN_FILE names the token of an account that may be written to")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	found, err := s.Browse(music.PageSearch + "taylor swift")
	if err != nil {
		t.Fatal(err)
	}
	// waitFor reads back until ok, or fails once settleWithin is up.
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		start := time.Now()
		for !ok() {
			if time.Since(start) > settleWithin {
				t.Errorf("%s: not so after %v", what, settleWithin)
				return
			}
			time.Sleep(10 * time.Second)
		}
		t.Logf("%s, after %v", what, time.Since(start).Round(time.Second))
	}
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

	song := firstItem(found, music.ItemSong)
	if _, lib := state("songs", song); lib {
		t.Fatalf("the song is in the library already")
	}
	if err := s.AddToLibrary(music.ItemSong, song, true); err != nil {
		t.Fatalf("adding a song: %v", err)
	}
	waitFor("the song in the library", func() bool { _, lib := state("songs", song); return lib })
	added, err := s.Browse(music.PageLibrarySongs)
	if err == nil {
		t.Logf("library songs: %s", shape(added))
	}
	if err := s.AddToLibrary(music.ItemSong, song, false); err != nil {
		t.Errorf("taking the song out: %v", err)
	}
	waitFor("the song out of the library", func() bool { _, lib := state("songs", song); return !lib })

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
