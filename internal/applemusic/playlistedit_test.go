package applemusic

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Editing one of the listener's playlists asks Apple what the web player
// asks: a song taken out by its library ID, a catalog song's or an upload's;
// a move PUT as the whole list, music videos where they were; a new name
// PATCHed; the playlist DELETEd. Apple goes on answering with the playlist as
// it was, as it does for a while, and the source answers with the changes
// until editSettles has passed.
func TestPlaylistEdits(t *testing.T) {
	type call struct{ method, path, ids, mode, body string }
	var (
		mu    sync.Mutex
		calls []call
	)
	const tracks = `{"data":[
		{"id":"i.a","type":"library-songs","attributes":{"name":"A","playParams":{"id":"i.a","kind":"song","catalogId":"1"}}},
		{"id":"i.mv","type":"library-music-videos","attributes":{"name":"MV","playParams":{"id":"i.mv","kind":"musicVideo","catalogId":"9"}}},
		{"id":"i.b","type":"library-songs","attributes":{"name":"B","playParams":{"id":"i.b","kind":"song","catalogId":"2"}}},
		{"id":"i.up","type":"library-songs","attributes":{"name":"Up","playParams":{"id":"i.up","kind":"song","reporting":false}}},
		{"id":"i.c","type":"library-songs","attributes":{"name":"C","playParams":{"id":"i.c","kind":"song","catalogId":"3"}}}]}`
	const mine = `{"id":"p.mine","type":"library-playlists","attributes":{"name":"Old","canEdit":true,"canDelete":true}}`
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		if r.URL.Hostname() != "amp-api.music.apple.com" {
			return answer(404, "")
		}
		if r.Method != "GET" {
			b, _ := json.Marshal(readBody(r))
			mu.Lock()
			calls = append(calls, call{r.Method, r.URL.Path, r.URL.Query().Get("ids[library-songs]"), r.URL.Query().Get("mode"), string(b)})
			mu.Unlock()
			return answer(204, nil)
		}
		switch r.URL.Path {
		case "/v1/me/library/playlists/p.mine/tracks":
			return answer(200, tracks)
		case "/v1/me/library/playlists/p.mine":
			return answer(200, `{"data":[`+mine+`]}`)
		case "/v1/me/library/playlists":
			return answer(200, `{"data":[`+mine+`]}`)
		}
		return answer(404, "")
	})
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "us")
	took := func() []call {
		mu.Lock()
		defer mu.Unlock()
		out := calls
		calls = nil
		return out
	}
	songs := func() []string {
		t.Helper()
		pl, err := ts.Playlist("p.mine")
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, tr := range pl.Tracks {
			ids = append(ids, tr.ID)
		}
		return ids
	}

	if err := ts.RemoveFromPlaylist("p.mine", []string{"2"}); err != nil {
		t.Fatal(err)
	}
	if err := ts.RemoveFromPlaylist("p.mine", []string{"i.up"}); err != nil {
		t.Fatal(err)
	}
	if got, want := took(), []call{
		{"DELETE", "/v1/me/library/playlists/p.mine/tracks", "i.b", "all", "null"},
		{"DELETE", "/v1/me/library/playlists/p.mine/tracks", "i.up", "all", "null"},
	}; !slices.Equal(got, want) {
		t.Errorf("taking songs out asked %+v", got)
	}
	if got := songs(); !slices.Equal(got, []string{"1", "3"}) {
		t.Errorf("read back, the songs are %v", got)
	}
	if err := ts.RemoveFromPlaylist("p.mine", []string{"2"}); err == nil {
		t.Error("took out a song no longer there")
	}

	if err := ts.MovePlaylistTrack("p.mine", 1, 0); err != nil {
		t.Fatal(err)
	}
	put := `{"data":[{"id":"i.c","type":"library-songs"},{"id":"i.mv","type":"library-music-videos"},{"id":"i.a","type":"library-songs"}]}`
	if got := took(); !slices.Equal(got, []call{{"PUT", "/v1/me/library/playlists/p.mine/tracks", "", "", put}}) {
		t.Errorf("moving a song asked %+v", got)
	}
	if got := songs(); !slices.Equal(got, []string{"3", "1"}) {
		t.Errorf("read back, the songs are %v", got)
	}

	if err := ts.RenamePlaylist("p.mine", "New"); err != nil {
		t.Fatal(err)
	}
	if got := took(); !slices.Equal(got, []call{{"PATCH", "/v1/me/library/playlists/p.mine", "", "", `{"attributes":{"name":"New"}}`}}) {
		t.Errorf("renaming asked %+v", got)
	}
	lists, err := ts.Playlists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].Name != "New" {
		t.Errorf("read back, the playlists are %+v", lists)
	}

	if err := ts.DeletePlaylist("p.mine"); err != nil {
		t.Fatal(err)
	}
	if got := took(); !slices.Equal(got, []call{{"DELETE", "/v1/me/library/playlists/p.mine", "", "", "null"}}) {
		t.Errorf("deleting asked %+v", got)
	}
	if lists, _ := ts.Playlists(); len(lists) != 0 {
		t.Errorf("read back, a deleted playlist is listed: %+v", lists)
	}
	if _, err := ts.Playlist("p.mine"); err == nil {
		t.Error("read back, a deleted playlist opens")
	}

	// Long after, Apple's word is taken again: changed elsewhere since.
	ts.clk.advance(t, editSettles+time.Minute)
	lists, _ = ts.Playlists()
	if len(lists) != 1 || lists[0].Name != "Old" || !strings.Contains(strings.Join(songs(), ","), "2") {
		t.Errorf("Apple's answers aren't taken once settled: %+v", lists)
	}
}
