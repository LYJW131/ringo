package applemusic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// A list of the library shown another way asks Apple's library to sort and
// filter it, its next part too, and leaves out what Apple can't sort by; a
// word is looked for in all of the list, all parts of it, as sorted.
func TestLibraryViews(t *testing.T) {
	ts, a := newApple(t)
	ts.storage.set(userTokenKey, "token")
	album := func(i int, name string) string {
		return fmt.Sprintf(`{"id":"l.%d","type":"library-albums","attributes":{"name":%q,"artistName":"Jay Chou","playParams":{"id":"l.%d","kind":"album","isLibrary":true}}}`, i, name, i)
	}
	a.routes = append(a.routes,
		route{regexp.MustCompile(`^/v1/me/storefront$`), func(*http.Request) any { return usStorefront }},
		route{regexp.MustCompile(`^/v1/me/library/albums$`), func(r *http.Request) any {
			if r.URL.Query().Get("offset") == "2" {
				return json.RawMessage(`{"data":[` + album(3, "Fantasy") + `]}`)
			}
			return json.RawMessage(`{"data":[` + album(1, "Jay") + `,` + album(2, "Fantasy Plus") + `],"next":"/v1/me/library/albums?offset=2"}`)
		}},
	)
	b := browse(t, ts, "library:albums?favorites=1&sort=-dateAdded")
	q := a.last(`/v1/me/library/albums$`).Query()
	if q.Get("sort") != "-dateAdded" || q.Get("filter[inFavorites]") != "true" {
		t.Errorf("asked %v", q)
	}
	if !strings.HasPrefix(b.Next, "library:albums?favorites=1&sort=-dateAdded|") {
		t.Fatalf("next %q", b.Next)
	}
	browse(t, ts, b.Next)
	if q := a.last(`/v1/me/library/albums$`).Query(); q.Get("offset") != "2" || q.Get("sort") != "-dateAdded" || q.Get("filter[inFavorites]") != "true" {
		t.Errorf("the next part asked %v", q)
	}
	browse(t, ts, "library:albums?sort=artistName")
	if q := a.last(`/v1/me/library/albums$`).Query(); q.Get("sort") != "name" {
		t.Errorf("a sort Apple refuses was asked: %v", q)
	}
	found := browse(t, ts, "library:albums?sort=-dateAdded&term=fantasy")
	var ids []string
	for _, it := range found.Sections[0].Items {
		ids = append(ids, it.ID)
	}
	if !slices.Equal(ids, []string{"l.2", "l.3"}) || found.Next != "" {
		t.Errorf("found %v, next %q", ids, found.Next)
	}
	if browse(t, ts, "library:albums?term=chou").Sections[0].Items == nil {
		t.Error("an artist's name isn't looked in")
	}
}
