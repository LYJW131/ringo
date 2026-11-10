package applemusic

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"testing"
)

// favoriteSongs is a stand-in for Favorite Songs as Apple lists them, a
// playlist of the library: a hundred to a request at most, parts by offset,
// and the total in meta, if total is set. A request whose offset is in fail
// is refused.
type favoriteSongs struct {
	n     int
	total bool
	fail  map[int]bool

	mu      sync.Mutex
	offsets []int
}

func (f *favoriteSongs) serve(r *http.Request) *http.Response {
	switch r.URL.Path {
	case "/v1/me/library/playlists":
		return answer(200, map[string]any{"data": []any{map[string]any{
			"id": "p.fav", "type": "library-playlists",
			"attributes": map[string]any{"name": "Favorite Songs", "canDelete": false},
		}}})
	case "/v1/me/library/playlists/p.fav/tracks":
	default:
		return answer(404, "")
	}
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	f.mu.Lock()
	f.offsets = append(f.offsets, offset)
	refused := f.fail[offset]
	f.mu.Unlock()
	if limit > 100 || refused {
		return answer(400, "")
	}
	data := []any{}
	for i := offset; i < min(offset+limit, f.n); i++ {
		data = append(data, map[string]any{"id": fmt.Sprint("i.", i), "type": "library-songs", "attributes": map[string]any{"name": fmt.Sprint("Song ", i)}})
	}
	out := map[string]any{"data": data}
	if offset+limit < f.n {
		out["next"] = fmt.Sprintf("/v1/me/library/playlists/p.fav/tracks?offset=%d", offset+limit)
	}
	if f.total {
		out["meta"] = map[string]any{"total": f.n}
	}
	return answer(200, out)
}

func (f *favoriteSongs) asked() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.offsets)
	slices.Sort(out)
	return out
}

func newFavorites(t *testing.T, f *favoriteSongs) *testSource {
	ts := newTestSource(t, f.serve)
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "us")
	return ts
}

// The favorite songs go on with a few parts asked for together, and say how
// many there are in all, from the first part's answer: the parts come joined,
// none is asked for past the end, and the first is asked for alone.
func TestLikedGoesOnInParts(t *testing.T) {
	f := &favoriteSongs{n: 250, total: true}
	ts := newFavorites(t, f)
	first, err := ts.Liked(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Tracks) != 100 || first.Total != 250 || !first.More {
		t.Fatalf("the first part has %d songs, of %d in all, more %v", len(first.Tracks), first.Total, first.More)
	}
	if got := f.asked(); !slices.Equal(got, []int{0}) {
		t.Fatalf("the first part was asked for with %v", got)
	}
	rest, err := ts.Liked(100)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(rest.Tracks); n != 150 || rest.Tracks[0].Name != "Song 100" || rest.Tracks[149].Name != "Song 249" || rest.More || rest.Total != 250 {
		t.Fatalf("the rest has %d songs, more %v, of %d in all", n, rest.More, rest.Total)
	}
	if got := f.asked(); !slices.Equal(got, []int{0, 100, 200}) {
		t.Errorf("asked for the parts at %v, want 0, 100 and 200, and none at 300 past the end", got)
	}
}

// Without a total, parts past the end are asked for and come empty, and the
// list ends where the first part of it that did.
func TestLikedWithoutTotal(t *testing.T) {
	f := &favoriteSongs{n: 130}
	ts := newFavorites(t, f)
	first, err := ts.Liked(0)
	if err != nil || first.Total != 0 || !first.More {
		t.Fatalf("the first part: %+v %v", first, err)
	}
	rest, err := ts.Liked(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Tracks) != 30 || rest.More {
		t.Errorf("the rest has %d songs, more %v", len(rest.Tracks), rest.More)
	}
}

// A part that fails keeps back only itself and those after: what came before
// is shown, with more to come, and the next ask begins at the part that failed.
func TestLikedPartFails(t *testing.T) {
	f := &favoriteSongs{n: 1000, total: true, fail: map[int]bool{300: true}}
	ts := newFavorites(t, f)
	if _, err := ts.Liked(0); err != nil {
		t.Fatal(err)
	}
	rest, err := ts.Liked(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Tracks) != 200 || !rest.More {
		t.Fatalf("the rest has %d songs, more %v", len(rest.Tracks), rest.More)
	}
	f.mu.Lock()
	f.fail = nil
	f.mu.Unlock()
	last, err := ts.Liked(300)
	if err != nil || len(last.Tracks) != 300 || last.Tracks[0].Name != "Song 300" || !last.More {
		t.Fatalf("going on from it: %+v %v", last, err)
	}
}
