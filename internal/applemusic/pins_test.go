package applemusic

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"ringo/internal/music"
)

// Pins come with where each is and the library's ID it's pinned by; a song,
// which the app knows by its catalog ID, is unpinned by its library's, and a
// pin moved to the top is put after "top".
func TestPins(t *testing.T) {
	var mu sync.Mutex
	var asked []string
	pin := func(id, typ, pos string, attrs map[string]any) map[string]any {
		return map[string]any{"id": id, "type": typ, "attributes": attrs, "meta": map[string]any{"libraryPin": map[string]any{"positionUUID": pos, "action": "drillIn"}}}
	}
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		if r.URL.Path == "/v1/me/library/pins" && r.Method == "GET" {
			return answer(200, map[string]any{
				"data": []any{map[string]any{"id": "i.song", "type": "library-songs"}, map[string]any{"id": "l.album", "type": "library-albums"}},
				"resources": map[string]any{
					"library-songs":  map[string]any{"i.song": pin("i.song", "library-songs", "A", map[string]any{"name": "生命線", "playParams": map[string]any{"id": "i.song", "kind": "song", "catalogId": "1542953977", "isLibrary": true}})},
					"library-albums": map[string]any{"l.album": pin("l.album", "library-albums", "B", map[string]any{"name": "HALO - EP"})},
				},
			})
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/me/library/pins/") {
			return answer(404, "")
		}
		mu.Lock()
		asked = append(asked, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		return answer(204, nil)
	})
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "us")

	pins, err := ts.Pins()
	if err != nil || len(pins) != 2 {
		t.Fatalf("pins %+v %v", pins, err)
	}
	song, album := pins[0], pins[1]
	if song.Kind != music.ItemSong || song.ID != "1542953977" || *song.Pin != (music.PinPlace{ID: "i.song", Position: "A"}) {
		t.Errorf("the song is %+v, pinned %+v", song, song.Pin)
	}
	if album.Kind != music.ItemAlbum || *album.Pin != (music.PinPlace{ID: "l.album", Position: "B"}) {
		t.Errorf("the album is %+v, pinned %+v", album, album.Pin)
	}

	if err := ts.Pin(music.ItemSong, "1542953977", false); err != nil {
		t.Fatal(err)
	}
	if err := ts.Pin(music.ItemAlbum, "l.other", true); err != nil {
		t.Fatal(err)
	}
	if err := ts.MovePin("l.album", ""); err != nil {
		t.Fatal(err)
	}
	if err := ts.MovePin("i.song", "B"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"DELETE /v1/me/library/pins/i.song?",
		"POST /v1/me/library/pins/l.other?",
		"POST /v1/me/library/pins/l.album?after=top",
		"POST /v1/me/library/pins/i.song?after=B",
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(asked, want) {
		t.Errorf("asked\n%q\nwant\n%q", asked, want)
	}
}
