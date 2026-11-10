package applemusic

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"mygo-music/internal/music"
)

// libraryAlbums is a stand-in for the library's albums as Apple lists them: a
// hundred to a request at most, parts by offset, and the total in meta, if
// total is set. A request whose offset is in fail is refused.
type libraryAlbums struct {
	n     int
	total bool
	fail  map[int]bool

	mu sync.Mutex
	// offsets are where the page's parts asked for were, not the source's own fetching ahead.
	offsets []int
	most    int // the most limit asked for
}

func (l *libraryAlbums) serve(r *http.Request) *http.Response {
	if r.URL.Path != "/v1/me/library/albums" {
		return answer(404, "")
	}
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	l.mu.Lock()
	if q.Has("includeOnly") {
		l.offsets = append(l.offsets, offset)
	}
	l.most = max(l.most, limit)
	l.mu.Unlock()
	if limit > 100 || l.fail[offset] {
		return answer(400, "")
	}
	data := []any{}
	for i := offset; i < min(offset+limit, l.n); i++ {
		data = append(data, map[string]any{"id": fmt.Sprint("l.", i), "type": "library-albums", "attributes": map[string]any{"name": fmt.Sprint("Album ", i)}})
	}
	out := map[string]any{"data": data}
	if offset+limit < l.n {
		out["next"] = fmt.Sprintf("/v1/me/library/albums?l=%s&offset=%d", q.Get("l"), offset+limit)
	}
	if l.total {
		out["meta"] = map[string]any{"total": l.n}
	}
	return answer(200, out)
}

func (l *libraryAlbums) asked() []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := slices.Clone(l.offsets)
	slices.Sort(out)
	return out
}

func newLibrary(t *testing.T, l *libraryAlbums) *testSource {
	ts := newTestSource(t, l.serve)
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "us")
	return ts
}

// A library's list goes on with a few parts asked for together, as Apple
// says where each starts, and what the list holds in all, from the first
// part's answer: the parts come joined, none is asked for past the end, and
// asking for the next went in no more requests than one part did in turn.
func TestLibraryListGoesOnInParts(t *testing.T) {
	l := &libraryAlbums{n: 250, total: true}
	ts := newLibrary(t, l)
	first, err := ts.Browse(music.PageLibraryAlbums)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(first.Sections[0].Items); got != 100 || first.Total != 250 || !strings.Contains(first.Next, "offset=100") {
		t.Fatalf("the first part has %d albums, of %d in all, next %q", got, first.Total, first.Next)
	}
	rest, err := ts.Browse(first.Next)
	if err != nil {
		t.Fatal(err)
	}
	items := rest.Sections[0].Items
	if len(items) != 150 || items[0].Title != "Album 100" || items[149].Title != "Album 249" || rest.Next != "" || rest.Total != 250 {
		t.Fatalf("the rest has %d albums from %q, next %q, of %d in all", len(items), items[0].Title, rest.Next, rest.Total)
	}
	if got := l.asked(); !slices.Equal(got, []int{0, 100, 200}) {
		t.Errorf("asked for the parts at %v, want 0, 100 and 200, and none at 300 past the end", got)
	}
	if l.most > 100 {
		t.Errorf("asked for %d to a request, which Apple refuses", l.most)
	}
}

// Without a total, as for what was added lately, parts past the end are asked
// for and come empty, and the list ends where the first part of it that did.
func TestLibraryListWithoutTotal(t *testing.T) {
	l := &libraryAlbums{n: 130}
	ts := newLibrary(t, l)
	first, err := ts.Browse(music.PageLibraryAlbums)
	if err != nil || first.Total != 0 || first.Next == "" {
		t.Fatalf("the first part: %+v %v", first, err)
	}
	rest, err := ts.Browse(first.Next)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rest.Sections[0].Items); got != 30 || rest.Next != "" {
		t.Errorf("the rest has %d albums, next %q", got, rest.Next)
	}
}

// A part that fails keeps back only itself and those after: what came before
// is shown, and the next ask begins at the part that failed.
func TestLibraryListPartFails(t *testing.T) {
	l := &libraryAlbums{n: 1000, total: true, fail: map[int]bool{300: true}}
	ts := newLibrary(t, l)
	first, err := ts.Browse(music.PageLibraryAlbums)
	if err != nil {
		t.Fatal(err)
	}
	rest, err := ts.Browse(first.Next)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rest.Sections[0].Items); got != 200 || !strings.Contains(rest.Next, "offset=300") || !strings.HasSuffix(rest.Next, "#1000") {
		t.Fatalf("the rest has %d albums, next %q", got, rest.Next)
	}
	l.mu.Lock()
	l.fail = nil
	l.mu.Unlock()
	last, err := ts.Browse(rest.Next)
	if err != nil || len(last.Sections[0].Items) != 300 || !strings.Contains(last.Next, "offset=600") {
		t.Fatalf("going on from it: %+v %v", last, err)
	}
}

// A page of the library is asked for again a minute after, not ten: the
// listener adds to it elsewhere, and the app asks again as it opens.
func TestLibraryPageIsKeptAMinute(t *testing.T) {
	l := &libraryAlbums{n: 10, total: true}
	ts := newLibrary(t, l)
	for range 2 {
		if _, err := ts.Browse(music.PageLibraryAlbums); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(l.asked()); got != 1 {
		t.Fatalf("asked %d times in the same minute, want 1", got)
	}
	ts.clk.minutes(t, 1.5)
	if _, err := ts.Browse(music.PageLibraryAlbums); err != nil {
		t.Fatal(err)
	}
	if got := len(l.asked()); got != 2 {
		t.Errorf("asked %d times in all, want 2 after a minute and a half", got)
	}
}
