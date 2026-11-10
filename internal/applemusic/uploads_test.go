package applemusic

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"mygo-music/internal/music"
)

// Songs of the library the catalog doesn't have: one the listener uploaded
// plays by its library ID; one the catalog no longer has doesn't, and says
// why.

// A library song is told apart by its playParams: an upload has its library
// ID and no catalog ID, one the catalog dropped has none, and one the
// catalog has goes by the catalog's ID, as before.
func TestUploadedTracks(t *testing.T) {
	s := &Source{}
	cases := []struct {
		name, json, id   string
		uploaded, greyed bool
	}{
		{"uploaded", `{"id":"i.up","type":"library-songs","attributes":{"name":"x","playParams":{"id":"i.up","isLibrary":true,"kind":"song","reporting":false}}}`, "i.up", true, false},
		{"gone", `{"id":"i.gone","type":"library-songs","attributes":{"name":"x"}}`, "i.gone", false, true},
		{"gone, empty playParams", `{"id":"i.gone","type":"library-songs","attributes":{"name":"x","playParams":{}}}`, "i.gone", false, true},
		{"in the catalog", `{"id":"i.cat","type":"library-songs","attributes":{"name":"x","playParams":{"id":"i.cat","isLibrary":true,"kind":"song","catalogId":"42","reporting":true,"reportingId":"42"}}}`, "42", false, false},
	}
	for _, c := range cases {
		tr := s.toTrack(resourceOf(t, c.json))
		if tr.ID != c.id || tr.Uploaded != c.uploaded || tr.Unavailable != c.greyed {
			t.Errorf("%s: ID %q, Uploaded %v, Unavailable %v; want %q, %v, %v", c.name, tr.ID, tr.Uploaded, tr.Unavailable, c.id, c.uploaded, c.greyed)
		}
	}
}

// The cover of an uploaded album, and of each of its songs, is kept by
// amhelper before the app is given it. Handed the blobstore's link, the app
// gave up on a 4000-pixel one after its 30 seconds, and showed none.
func TestUploadedCovers(t *testing.T) {
	const (
		albumArt = "https://store-033.blobstore.apple.com/sq/a/5194ccf7-8d6a-655b-b9e2-00ef5180ed89/image?X-Amz=1"
		songArt  = "https://store-033.blobstore.apple.com/sq/b/6194ccf7-8d6a-655b-b9e2-00ef5180ed89/image?X-Amz=1"
	)
	var (
		mu   sync.Mutex
		kept []string
	)
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		if u := r.URL.String(); strings.HasPrefix(u, helperBaseURL+"/cover/") {
			mu.Lock()
			kept = append(kept, strings.TrimPrefix(u, helperBaseURL+"/cover/"))
			mu.Unlock()
			return answer(200, `{"ok":true}`)
		}
		return nil
	})
	album := `{"id":"l.up","type":"library-albums","attributes":{"name":"x","artwork":{"url":"` + albumArt + `"}}}`
	song := `{"id":"i.up","type":"library-songs","attributes":{"name":"x","artwork":{"url":"` + songArt + `"},"playParams":{"id":"i.up","kind":"song","reporting":false}}}`
	if got := ts.toAlbum(resourceOf(t, album)).ImageURL; got != "" {
		t.Errorf("before it's kept, the album's cover is %q", got)
	}
	if got := ts.toTrack(resourceOf(t, song)).Album.ImageURL; got != "" {
		t.Errorf("before it's kept, the song's cover is %q", got)
	}
	ts.clk.settle(t)
	if len(kept) != 2 {
		t.Fatalf("kept %v", kept)
	}
	if got := ts.toAlbum(resourceOf(t, album)).ImageURL; !strings.HasPrefix(got, "file://") || !strings.Contains(got, "5194ccf7") {
		t.Errorf("once kept, the album's cover is %q", got)
	}
}

// An album with an upload among its songs isn't deleted from the library,
// as from a card's menu, which doesn't know: nothing of it is, and the
// listener is told why.
func TestAlbumWithUploadsIsntDeleted(t *testing.T) {
	var (
		mu      sync.Mutex
		deleted []string
	)
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		if r.Method == "DELETE" {
			mu.Lock()
			deleted = append(deleted, r.URL.Path)
			mu.Unlock()
			return answer(204, nil)
		}
		if strings.HasSuffix(r.URL.Path, "/library/albums/l.mixed/tracks") {
			return answer(200, `{"data":[
				{"id":"i.cat","type":"library-songs","attributes":{"playParams":{"id":"i.cat","kind":"song","catalogId":"42"}}},
				{"id":"i.up","type":"library-songs","attributes":{"playParams":{"id":"i.up","kind":"song","reporting":false}}}]}`)
		}
		return nil
	})
	ts.storage.set(userTokenKey, "user")
	err := ts.AddToLibrary(music.ItemAlbum, "l.mixed", false)
	if !errors.Is(err, music.ErrUnsupported) || music.Reason(err) == "" {
		t.Errorf("deleting it: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("deleted %v", deleted)
	}
}

// An upload is asked for by its library ID, at any quality, and the link
// Apple answers handed to amhelper, which the player then asks; an
// encrypted .m4p isn't handed over, and a song Apple no longer has says so.
func TestUploadStream(t *testing.T) {
	const link = "https://store-033.blobstore.apple.com/sq-mq-us-033/a/b/song.m4a?sig"
	var (
		mu     sync.Mutex
		bodies []map[string]any
		handed []map[string]any
		other  []string
	)
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		u := r.URL.String()
		if u == helperBaseURL+"/upload/i.up" && r.Method == "POST" {
			mu.Lock()
			handed = append(handed, readBody(r))
			mu.Unlock()
			return answer(200, `{"size":7700000}`)
		}
		if !strings.Contains(u, "webPlayback") {
			mu.Lock()
			other = append(other, u)
			mu.Unlock()
			return nil
		}
		b := readBody(r)
		mu.Lock()
		bodies = append(bodies, b)
		mu.Unlock()
		asset := func(ext, url string) *http.Response {
			return answer(200, map[string]any{"songList": []any{map[string]any{"songId": b["universalLibraryId"],
				"assets": []any{map[string]any{"URL": url, "md5": "x", "metadata": map[string]any{"fileExtension": ext, "bitRate": 256}}}}}})
		}
		switch b["universalLibraryId"] {
		case "i.up":
			return asset("m4a", link)
		case "i.bought":
			return asset("m4p", "https://store-033.blobstore.apple.com/sq-mq-us-033/a/b/song.m4p?sig")
		}
		return answer(200, map[string]any{"failureType": "3077", "customerMessage": "The item you tried to download is no longer available."})
	})
	ts.storage.set(userTokenKey, "user")
	ts.setSettings(map[string]string{"wrapper": "http://127.0.0.1:1"})

	st, err := ts.Stream("i.up", "lossless")
	if err != nil {
		t.Fatal(err)
	}
	want := music.Stream{URL: helperBaseURL + "/upload/i.up.m4a", Type: "m4a", Quality: "high", BitRate: 256000, Size: 7700000}
	if !reflect.DeepEqual(*st, want) {
		t.Errorf("stream %+v, want %+v", *st, want)
	}
	if len(bodies) != 1 || bodies[0]["universalLibraryId"] != "i.up" || bodies[0]["isLibrary"] != true || bodies[0]["salableAdamId"] != nil {
		t.Errorf("asked with %v", bodies)
	}
	if len(handed) != 1 || handed[0]["url"] != link {
		t.Errorf("handed to amhelper: %v", handed)
	}
	if _, err := ts.Stream("i.bought", "high"); !errors.Is(err, music.ErrUnavailable) {
		t.Errorf("an .m4p: %v", err)
	}
	_, err = ts.Stream("i.gone", "high")
	if !errors.Is(err, music.ErrUnavailable) || !strings.Contains(err.Error(), "country or region") {
		t.Errorf("a song no longer available: %v", err)
	}
	ts.clk.settle(t)
	if len(other) != 0 {
		t.Errorf("asked besides: %v", other)
	}
}
