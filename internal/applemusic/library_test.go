package applemusic

import (
	"errors"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"testing"

	"mygo-music/internal/music"
)

// The listener's own playlists, as Apple's Recently Played gives them, by
// their catalog ID.

func TestOwnPlaylistByCatalogID(t *testing.T) {
	// Their own playlist, and one they saved from someone else, in the
	// library and by the catalog IDs they're shared under.
	library := map[string]struct {
		canEdit bool
		catalog string
	}{
		"p.mine":   {true, "pl.u-mine"},
		"p.theirs": {false, "pl.u-theirs"},
		"p.down":   {false, "pl.u-down"}, // the library doesn't answer
	}
	var mu sync.Mutex
	var deleted []string
	libraryPath := regexp.MustCompile(`^/v1/me/library/playlists/(p\.\w+)(/tracks)?$`)
	catalogPath := regexp.MustCompile(`^/v1/catalog/us/playlists/(pl\.[\w-]+)$`)
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		if r.Method == "HEAD" {
			return answer(200, nil)
		}
		if m := libraryPath.FindStringSubmatch(r.URL.Path); m != nil {
			if r.Method == "DELETE" {
				mu.Lock()
				deleted = append(deleted, m[1])
				mu.Unlock()
				return answer(204, nil)
			}
			if m[2] != "" {
				return answer(404, "") // no songs
			}
			if m[1] == "p.down" {
				return answer(503, "")
			}
			return answer(200, map[string]any{"data": []any{map[string]any{"id": m[1], "type": "library-playlists", "attributes": map[string]any{"name": m[1], "canEdit": library[m[1]].canEdit}}}})
		}
		if m := catalogPath.FindStringSubmatch(r.URL.Path); m != nil {
			var lib []any
			for id, p := range library {
				if p.catalog == m[1] {
					lib = append(lib, map[string]any{"id": id, "type": "library-playlists"})
				}
			}
			return answer(200, map[string]any{"data": []any{map[string]any{
				"id": m[1], "type": "playlists", "attributes": map[string]any{"name": m[1]},
				"relationships": map[string]any{"tracks": map[string]any{"data": []any{}}, "library": map[string]any{"data": lib}},
			}}})
		}
		return answer(404, "")
	})
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "us")

	own, err := ts.Playlist("pl.u-mine")
	if err != nil || own.ID != "p.mine" || !own.Mine || own.Saved {
		t.Errorf("their own: %+v %v", own, err)
	}
	saved, err := ts.Playlist("pl.u-theirs")
	if err != nil || saved.ID != "pl.u-theirs" || saved.Mine || !saved.Saved {
		t.Errorf("saved: %+v %v", saved, err)
	}
	// Without an answer from the library, the playlist still opens, from the catalog.
	down, err := ts.Playlist("pl.u-down")
	if err != nil || down.ID != "pl.u-down" || !down.Saved {
		t.Errorf("library down: %+v %v", down, err)
	}

	// Unsaving never deletes a playlist they made.
	for _, id := range []string{"pl.u-mine", "p.mine"} {
		if err := ts.AddToLibrary(music.ItemPlaylist, id, false); !errors.Is(err, music.ErrUnsupported) {
			t.Errorf("unsaving %s: %v", id, err)
		}
	}
	if len(deleted) != 0 {
		t.Errorf("deleted %v", deleted)
	}
	if err := ts.AddToLibrary(music.ItemPlaylist, "pl.u-theirs", false); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(deleted, []string{"p.theirs"}) {
		t.Errorf("deleted %v", deleted)
	}
}
