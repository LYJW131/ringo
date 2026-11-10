package applemusic

import (
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"mygo-music/internal/music"
)

// What favoriting, keeping and autoplay ask Apple, signed in: the method,
// the path, the query and the body, as the web player's script has them.
func TestWrites(t *testing.T) {
	type call struct {
		method, path, query string
		body                map[string]any
	}
	var (
		mu    sync.Mutex
		calls []call
	)
	inLibrary := regexp.MustCompile(`^/v1/catalog/us/(songs|albums)/(\d+)$`)
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		if r.Method == "HEAD" || r.URL.Hostname() != "amp-api.music.apple.com" {
			return answer(404, "")
		}
		mu.Lock()
		calls = append(calls, call{r.Method, r.URL.Path, r.URL.RawQuery, readBody(r)})
		mu.Unlock()
		switch {
		case r.URL.Path == "/v1/me/stations/continuous":
			// As Apple answered, cut short.
			return answer(200, map[string]any{"results": map[string]any{
				"station": map[string]any{"id": "ra.cp-1", "type": "stations", "attributes": map[string]any{"kind": "albumSeeded"}},
				"tracks": []any{map[string]any{"id": "11", "type": "songs",
					"attributes": map[string]any{"name": "Next", "playParams": map[string]any{"id": "11", "kind": "song"}}}}}})
		case r.URL.Path == "/v1/me/library/albums/l.own/tracks":
			return answer(200, map[string]any{"data": []any{
				map[string]any{"id": "i.one", "type": "library-songs"},
				map[string]any{"id": "i.two", "type": "library-songs"}}})
		case inLibrary.MatchString(r.URL.Path) && r.Method == "GET":
			m := inLibrary.FindStringSubmatch(r.URL.Path)
			own := map[string]string{"songs": "i.own", "albums": "l.own"}[m[1]]
			return answer(200, map[string]any{"data": []any{map[string]any{"id": m[2], "type": m[1],
				"relationships": map[string]any{"library": map[string]any{"data": []any{map[string]any{"id": own}}}}}}})
		}
		return answer(204, nil)
	})
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "us")
	since := func() []call {
		mu.Lock()
		defer mu.Unlock()
		out := slices.Clone(calls)
		calls = nil
		return out
	}
	writes := func() []call {
		var out []call
		for _, c := range since() {
			if c.method != "GET" {
				out = append(out, c)
			}
		}
		return out
	}
	check := func(name string, got []call, want ...call) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("%s: %+v", name, got)
			return
		}
		for i := range want {
			if got[i].method != want[i].method || got[i].path != want[i].path || got[i].query != want[i].query {
				t.Errorf("%s: %+v, want %+v", name, got[i], want[i])
			}
		}
	}

	since()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(ts.Favorite(music.ItemAlbum, "100", true))
	check("favorite an album", writes(), call{method: "POST", path: "/v1/me/favorites", query: "ids%5Balbums%5D=100"})
	must(ts.Favorite(music.ItemPlaylist, "pl.1", false))
	check("unfavorite a playlist", writes(), call{method: "DELETE", path: "/v1/me/favorites", query: "ids%5Bplaylists%5D=pl.1"})
	must(ts.Favorite(music.ItemArtist, "200", true))
	check("favorite an artist", writes(), call{method: "POST", path: "/v1/me/favorites", query: "ids%5Bartists%5D=200"})
	// A song's favorite is its rating, as Like has it.
	must(ts.Favorite(music.ItemSong, "300", true))
	got := writes()
	if len(got) == 0 || got[0].method != "PUT" || got[0].path != "/v1/me/ratings/songs/300" {
		t.Errorf("favorite a song: %+v", got)
	}

	must(ts.AddToLibrary(music.ItemSong, "400", true))
	check("add a song", writes(), call{method: "POST", path: "/v1/me/library", query: "ids%5Bsongs%5D=400&representation=ids"})
	must(ts.AddToLibrary(music.ItemSong, "400", false))
	check("take a song out", writes(), call{method: "DELETE", path: "/v1/me/library/songs/i.own"})
	must(ts.AddToLibrary(music.ItemAlbum, "500", false))
	// Its songs go first: Apple lists an album for as long as the library
	// holds songs of it, whatever it answered to taking the album out.
	check("take an album out", writes(),
		call{method: "DELETE", path: "/v1/me/library/songs/i.one"},
		call{method: "DELETE", path: "/v1/me/library/songs/i.two"},
		call{method: "DELETE", path: "/v1/me/library/albums/l.own"})

	next, err := ts.Autoplay([]string{"10", "i.upload"})
	must(err)
	got = writes()
	check("autoplay", got, call{method: "POST", path: "/v1/me/stations/continuous", query: "limit%5Bresults%3Atracks%5D=10&with=tracks"})
	if len(got) == 1 {
		data, _ := got[0].body["data"].([]any)
		if len(data) != 1 || data[0].(map[string]any)["id"] != "10" || data[0].(map[string]any)["type"] != "songs" {
			t.Errorf("autoplay of %v", got[0].body)
		}
	}
	if len(next) != 1 || next[0].ID != "11" || next[0].Name != "Next" {
		t.Errorf("autoplay: %+v", next)
	}
}

// Whether songs are in the library is asked a hundred at a time, of the
// catalog with nothing but inLibrary, as the web player asks; a song that
// isn't may come without the attribute, and one the storefront doesn't have
// doesn't come at all.
func TestInLibraryOf(t *testing.T) {
	var (
		mu      sync.Mutex
		queries []string
	)
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		if r.URL.Hostname() != "amp-api.music.apple.com" || r.URL.Path != "/v1/catalog/us" {
			return answer(404, "")
		}
		q := r.URL.Query()
		mu.Lock()
		queries = append(queries, q.Get("fields[songs]"))
		mu.Unlock()
		var data []any
		for _, id := range strings.Split(q.Get("ids[songs]"), ",") {
			switch id {
			case "999":
			case "1", "150":
				data = append(data, map[string]any{"id": id, "type": "songs", "attributes": map[string]any{"inLibrary": true}})
			default:
				data = append(data, map[string]any{"id": id, "type": "songs", "attributes": map[string]any{}})
			}
		}
		return answer(200, map[string]any{"data": data})
	})
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "us")
	var ids []string
	for i := 1; i <= 150; i++ {
		ids = append(ids, strconv.Itoa(i))
	}
	in, err := ts.InLibrary(music.ItemSong, append(ids, "999", "i.own"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for id, on := range in {
		if on {
			got = append(got, id)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"1", "150", "i.own"}) {
		t.Errorf("in the library: %v", got)
	}
	if len(queries) != 2 || queries[0] != "inLibrary" {
		t.Errorf("asked %d times, for %q", len(queries), queries)
	}
}
