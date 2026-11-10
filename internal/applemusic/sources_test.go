package applemusic

import (
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Every way the app gets songs it may play, signed in and lossless, against
// a stand-in for Apple and for amhelper: playing one works out the songs
// after it, and once they're worked out the wrapper is asked nothing, so that
// on Cloudflare it sleeps. The steps go on from each other, as the plugin's did.
func TestSourcesPlayOn(t *testing.T) {
	rec := &recorder{}
	var (
		mu           sync.Mutex
		workedOut    = map[string]bool{} // every song amhelper was asked to work out
		stationBatch = 0
		recentFrom   = 1100
	)
	// A song of the catalog, lossless, as Apple lists it.
	song := func(id string) map[string]any {
		return map[string]any{"id": id, "type": "songs", "attributes": map[string]any{
			"name": "Song " + id, "playParams": map[string]any{"id": id, "kind": "song"},
			"audioTraits":       []string{"lossless"},
			"extendedAssetUrls": map[string]any{"enhancedHls": "https://aod.itunes.apple.com/" + id + ".m3u8"},
		}}
	}
	songs := func(from, n int) []any {
		out := []any{}
		for _, id := range idRange(from, n) {
			out = append(out, song(id))
		}
		return out
	}
	data := func(items ...any) map[string]any { return map[string]any{"data": items} }
	type route struct {
		path   *regexp.Regexp
		answer func(u *url.URL) any
	}
	apple := []route{
		{regexp.MustCompile(`^/v1/catalog/us/charts$`), func(*url.URL) any {
			return map[string]any{"results": map[string]any{"songs": []any{map[string]any{"name": "Top Songs", "data": songs(1000, 30)}}}}
		}},
		// Home: Apple's shelves, of songs here, the second the songs played lately.
		{regexp.MustCompile(`^/v1/me/recommendations$`), func(*url.URL) any {
			mu.Lock()
			defer mu.Unlock()
			shelf := func(id string, songs []any) map[string]any {
				return map[string]any{"id": id, "type": "personal-recommendation",
					"attributes":    map[string]any{"display": map[string]any{"kind": "MusicCoverShelf"}},
					"relationships": map[string]any{"contents": map[string]any{"data": songs}}}
			}
			return data(shelf("top", songs(1000, 30)), shelf("recent", songs(recentFrom, 10)))
		}},
		{regexp.MustCompile(`^/v1/me/recent/played/tracks$`), func(*url.URL) any {
			mu.Lock()
			defer mu.Unlock()
			return map[string]any{"data": songs(recentFrom, 10)}
		}},
		{regexp.MustCompile(`^/v1/catalog/us/albums/(\d+)$`), func(u *url.URL) any {
			id := u.Path[strings.LastIndex(u.Path, "/")+1:]
			from := 1500
			if id == "800" {
				from = 1200
			}
			if u.Query().Get("relate") != "" {
				return data(map[string]any{"id": id, "type": "albums"})
			}
			return data(map[string]any{"id": id, "type": "albums", "attributes": map[string]any{"name": id}, "relationships": map[string]any{"tracks": map[string]any{"data": songs(from, 20)}}})
		}},
		{regexp.MustCompile(`^/v1/catalog/us/artists/900$`), func(*url.URL) any {
			return data(map[string]any{"id": "900", "type": "artists", "attributes": map[string]any{"name": "X"}, "views": map[string]any{"top-songs": map[string]any{"data": songs(1300, 20)}}})
		}},
		{regexp.MustCompile(`^/v1/catalog/us/playlists/pl\.1$`), func(u *url.URL) any {
			if u.Query().Get("relate") != "" {
				return data(map[string]any{"id": "pl.1", "type": "playlists"})
			}
			return data(map[string]any{"id": "pl.1", "type": "playlists", "attributes": map[string]any{"name": "P"}, "relationships": map[string]any{"tracks": map[string]any{"data": songs(1400, 20)}}})
		}},
		{regexp.MustCompile(`^/v1/me/library/playlists$`), func(*url.URL) any {
			return data(map[string]any{"id": "p.fav", "type": "library-playlists", "attributes": map[string]any{"name": "Favorite Songs", "canDelete": false}})
		}},
		{regexp.MustCompile(`^/v1/me/library/playlists/p\.fav/tracks$`), func(*url.URL) any { return map[string]any{"data": songs(1600, 20)} }},
		{regexp.MustCompile(`^/v1/catalog/us/search$`), func(u *url.URL) any {
			offset, _ := strconv.Atoi(u.Query().Get("offset"))
			r := map[string]any{"data": songs(1700+offset, 25)}
			if offset < 25 {
				r["next"] = "/next"
			}
			return map[string]any{"results": map[string]any{"songs": r}}
		}},
		{regexp.MustCompile(`^/v1/me/stations/next-tracks/ra\.1$`), func(*url.URL) any {
			mu.Lock()
			from := 3000 + 10*stationBatch
			stationBatch++
			mu.Unlock()
			return map[string]any{"data": songs(from, 10)}
		}},
		{regexp.MustCompile(`^/v1/me/ratings/songs$`), func(*url.URL) any { return data() }},
		{regexp.MustCompile(`^/v1/me/library/playlists/p\.fav$`), func(*url.URL) any {
			return data(map[string]any{"id": "p.fav", "type": "library-playlists"})
		}},
		{regexp.MustCompile(`^/v1/catalog/us/songs$`), func(u *url.URL) any {
			out := []any{}
			for _, id := range strings.Split(u.Query().Get("ids"), ",") {
				out = append(out, song(id))
			}
			return map[string]any{"data": out}
		}},
	}
	var ts *testSource
	ts = newTestSource(t, func(r *http.Request) *http.Response {
		u := r.URL
		rec.note(u.String(), ts.clk.Now())
		if u.Hostname() == "127.0.0.1" {
			switch {
			case strings.Contains(u.Path, "/prepare/"):
				mu.Lock()
				workedOut[u.Path[strings.LastIndex(u.Path, "/")+1:]] = true
				mu.Unlock()
				return answer(200, map[string]any{"sampleRate": 44100, "bitDepth": 16, "bandwidth": 1000000})
			case strings.HasSuffix(u.Path, "/wake"):
				return answer(200, map[string]any{"woke": true})
			case strings.Contains(u.Path, "/find/"):
				return answer(200, map[string]any{})
			}
		}
		if u.Hostname() == "amp-api.music.apple.com" {
			for _, rt := range apple {
				if rt.path.MatchString(u.Path) {
					return answer(200, rt.answer(u))
				}
			}
		}
		return answer(404, "{}")
	})
	ts.setSettings(map[string]string{"wrapper": "https://wrapper.example"})
	// Signed in, and Lossless chosen before.
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "us")
	ts.storage.set(qualityKey, "lossless")
	clk := ts.clk

	type prep struct {
		id    string
		ahead bool
	}
	prepared := func() []prep {
		var out []prep
		for _, p := range rec.prepared() {
			out = append(out, prep{p.id, p.ahead})
		}
		return out
	}
	stream := func(id string) {
		t.Helper()
		if _, err := ts.Stream(id, "lossless"); err != nil {
			t.Fatalf("stream %s: %v", id, err)
		}
	}
	// play plays songs as the app does: the audio of the one starting, its
	// lyrics, and the next one's audio ahead.
	play := func(list []string, from, n int, each float64) {
		for i := from; i < from+n; i++ {
			if i == from {
				stream(list[i])
			}
			ts.Lyrics(list[i])
			if i+1 < len(list) {
				stream(list[i+1])
			}
			clk.settle(t)
			// The app asks other things while it plays.
			clk.minutes(t, each/2)
			ts.IsLiked([]string{list[i]})
			clk.minutes(t, each/2)
		}
	}
	// playsOn opens nothing more, plays the song at, and checks the twelve
	// after are worked out, then nothing more.
	playsOn := func(name string, list []string, at int) {
		t.Helper()
		clk.minutes(t, 11) // nothing played for a while, and the minute's limit is clear
		mu.Lock()
		before := map[string]bool{}
		for id := range workedOut {
			before[id] = true
		}
		mu.Unlock()
		rec.clear()
		stream(list[at])
		clk.settle(t)
		var want []prep
		if !before[list[at]] {
			want = append(want, prep{list[at], false})
		}
		for _, id := range list[at+1 : min(at+13, len(list))] {
			if !before[id] {
				want = append(want, prep{id, true})
			}
		}
		if got := prepared(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: worked out %v, want %v", name, got, want)
		}
		rec.clear()
		play(list, at+1, 8, 3.5)
		if got := rec.toWrapper(); len(got) != 0 {
			t.Errorf("%s: asked the wrapper while the songs worked out play: %v", name, got)
		}
	}

	// Home: its songs are noted, not worked out, and play on.
	clk.minutes(t, 1)
	rec.clear()
	ts.Home()
	clk.settle(t)
	if got := prepared(); len(got) != 0 {
		t.Errorf("home, which the app loads again by itself, works out %v", got)
	}
	playsOn("home", idRange(1000, 30), 2)

	// Home's Recently Played: ten songs here, so the twelve after the first are nine.
	ts.Home()
	playsOn("recently played", idRange(1100, 10), 0)

	mu.Lock()
	recentFrom = 1150
	mu.Unlock()
	ts.Recent()
	playsOn("recent", idRange(1150, 10), 0)

	ts.Browse("charts")
	playsOn("charts", idRange(1000, 30), 15)

	ts.Album("800")
	playsOn("an album", idRange(1200, 20), 1)

	ts.Artist("900")
	playsOn("an artist's top songs", idRange(1300, 20), 0)

	ts.Playlist("pl.1")
	playsOn("a playlist", idRange(1400, 20), 3)

	ts.Liked(0)
	playsOn("liked", idRange(1600, 20), 0)

	// A song on a page the listener opened stays on it when home comes again.
	ts.Album("801")
	ts.Home() // Recently Played, here 1100 to 1109, isn't 1500's
	clk.minutes(t, 11)
	rec.clear()
	stream("1505")
	clk.settle(t)
	var ids []string
	for _, p := range prepared() {
		ids = append(ids, p.id)
	}
	if !reflect.DeepEqual(ids, idRange(1505, 13)) {
		t.Errorf("a song on a page opened: %v", ids)
	}

	// Fetched in the background, nothing goes to the wrapper.
	clk.minutes(t, 11)
	rec.clear()
	ts.liked(0, true)
	ts.libraryPlaylist("p.fav", true)
	clk.settle(t)
	if got := rec.toWrapper(); len(got) != 0 {
		t.Errorf("in the background: %v", got)
	}

	// Songs the app plays from its cache: their lyrics say they play, and the wrapper sleeps.
	clk.minutes(t, 11)
	ts.asked(true)() // browsing, which wakes it
	clk.settle(t)
	wakes := func() int {
		n := 0
		for _, a := range rec.toWrapper() {
			if strings.HasSuffix(a.url, "/wake") {
				n++
			}
		}
		return n
	}
	if n := wakes(); n != 1 {
		t.Errorf("woken %d times", n)
	}
	ts.Lyrics("1200") // a song starts, from the app's cache
	rec.clear()
	for i := range 10 {
		clk.minutes(t, 4)
		ts.Lyrics(strconv.Itoa(1201 + i))
		ts.IsLiked([]string{strconv.Itoa(1201 + i)})
		ts.Home()
	}
	clk.settle(t)
	if got := rec.toWrapper(); len(got) != 0 {
		t.Errorf("from the cache: %v", got)
	}

	// A station: each ten songs are worked out as they come, and the wrapper sleeps between.
	clk.minutes(t, 11)
	rec.clear()
	station, err := ts.StationNext("ra.1")
	if err != nil {
		t.Fatal(err)
	}
	var queue []string
	for _, tr := range station {
		queue = append(queue, tr.ID)
	}
	clk.settle(t)
	// All ten, the first as it plays.
	stream(queue[0])
	clk.settle(t)
	got := map[string]bool{}
	for _, p := range prepared() {
		got[p.id] = true
	}
	want := map[string]bool{}
	for _, id := range idRange(3000, 10) {
		want[id] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the station's first ten: %v", got)
	}
	// Three batches, played as the app plays them: it asks for the next ten
	// as the last but one starts, and plays on into them.
	for at := 0; at < 28; at++ {
		if at == len(queue)-2 {
			more, err := ts.StationNext("ra.1")
			if err != nil {
				t.Fatal(err)
			}
			for _, tr := range more {
				queue = append(queue, tr.ID)
			}
		}
		ts.Lyrics(queue[at])
		if at+1 < len(queue) {
			stream(queue[at+1])
		}
		clk.settle(t)
		clk.minutes(t, 1.75)
		ts.IsLiked([]string{queue[at]})
		clk.minutes(t, 1.75)
	}
	var aheadIDs []string
	for _, p := range prepared() {
		if p.ahead && !slices.Contains(aheadIDs, p.id) {
			aheadIDs = append(aheadIDs, p.id)
		}
	}
	slices.Sort(aheadIDs)
	if !reflect.DeepEqual(aheadIDs, idRange(3000, 30)) {
		t.Errorf("worked out ahead: %v", aheadIDs)
	}
	// What the wrapper was asked, by when: three bursts, half an hour apart.
	var times []time.Time
	for _, a := range rec.toWrapper() {
		times = append(times, a.at)
	}
	var long []time.Duration
	for i := 1; i < len(times); i++ {
		if g := times[i].Sub(times[i-1]); g >= 5*time.Minute {
			long = append(long, g)
		}
	}
	if len(long) != 2 {
		t.Errorf("long gaps: %v", long)
	}
	for _, g := range long {
		if g < 25*time.Minute {
			t.Errorf("a gap of %v", g)
		}
	}
	// Woken once, as the listener picked the station, and not again.
	if n := wakes(); n != 1 || !strings.HasSuffix(rec.toWrapper()[0].url, "/s/wake") {
		t.Errorf("woken %d times, first %v", n, rec.toWrapper()[0])
	}
}
