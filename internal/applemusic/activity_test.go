package applemusic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sync"
	"testing"
	"time"

	"ringo/internal/music"
)

// An activity service that keeps what it was sent, in the order it came.
type activityLog struct {
	mu     sync.Mutex
	posts  []activityPost
	asked  []string // songs looked up in the catalog
	gate   chan struct{}
	length int64
}

type activityPost struct {
	header http.Header
	body   struct {
		ClientID  string           `json:"client_id"`
		EventType string           `json:"event_type"`
		Data      []map[string]any `json:"data"`
	}
}

var songLookup = regexp.MustCompile(`^/v1/catalog/jp/songs/(\d+)$`)

func (l *activityLog) serve(t *testing.T) func(*http.Request) *http.Response {
	return func(r *http.Request) *http.Response {
		// A song the app never listed: its length comes from the catalog.
		if m := songLookup.FindStringSubmatch(r.URL.Path); m != nil {
			l.mu.Lock()
			l.asked = append(l.asked, m[1])
			l.mu.Unlock()
			return answer(200, map[string]any{"data": []any{map[string]any{"id": m[1], "type": "songs", "attributes": map[string]any{"durationInMillis": l.length}}}})
		}
		if r.URL.String() != activityURL {
			return answer(404, "")
		}
		if r.Method != "POST" {
			t.Errorf("method %s", r.Method)
		}
		if l.gate != nil {
			<-l.gate
		}
		var p activityPost
		p.header = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &p.body)
		l.mu.Lock()
		l.posts = append(l.posts, p)
		l.mu.Unlock()
		return answer(200, "{}")
	}
}

// take is what was sent since it was last asked, once the source has sent all.
func (l *activityLog) take(t *testing.T, ts *testSource) []activityPost {
	t.Helper()
	ts.clk.settle(t)
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.posts
	l.posts = nil
	return out
}

// sent is what an event says, for a test to compare.
type sent struct {
	Type     int
	Reason   int
	Hint     int
	Start    int64
	End      int64
	Song     string
	Duration int64
	Cont     int
	IDs      map[string]any
	Feature  string
}

// sentOf is the events of posts, each of which has to hold one.
func sentOf(t *testing.T, posts []activityPost) []sent {
	t.Helper()
	num := func(v any) int64 { f, _ := v.(float64); return int64(f) }
	var out []sent
	for _, p := range posts {
		if p.body.ClientID != "JSCLIENT" || p.body.EventType != "JSPLAY" || len(p.body.Data) != 1 {
			t.Fatalf("a post of %+v", p.body)
		}
		e := p.body.Data[0]
		ids, _ := e["container-ids"].(map[string]any)
		song, _ := e["ids"].(map[string]any)["subscription-adam-id"].(string)
		feature, _ := e["feature-name"].(string)
		out = append(out, sent{
			Type: int(num(e["event-type"])), Reason: int(num(e["end-reason-type"])), Hint: int(num(e["event-reason-hint-type"])),
			Start: num(e["start-position-in-milliseconds"]), End: num(e["end-position-in-milliseconds"]),
			Song: song, Duration: num(e["media-duration-in-milliseconds"]),
			Cont: int(num(e["container-type"])), IDs: ids, Feature: feature,
		})
	}
	return out
}

func activitySource(t *testing.T) (*testSource, *activityLog) {
	t.Helper()
	l := &activityLog{length: 300000}
	ts := newTestSource(t, l.serve(t))
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "jp")
	return ts, l
}

const sec = int64(time.Second / time.Millisecond)

func at(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

// What the web player says of a listener who plays two songs of an album,
// pauses and plays on, jumps, skips to the next, plays a playlist instead
// and lets its last song play out: each of these an event of its own, sent
// as it happens, with where in the song it is and where it's played from.
func TestActivity(t *testing.T) {
	ts, log := activitySource(t)
	album := music.Origin{Kind: music.FromAlbum, ID: "1440763349"}
	list := music.Origin{Kind: music.FromPlaylist, ID: "pl.fc2f59c513644e81b8c40ffd3e339134", Version: "8d98"}
	one := music.Track{ID: "1440763351", Duration: 328208, Origin: album}
	two := music.Track{ID: "1440763354", Duration: 278311, Origin: album}
	pick := music.Track{ID: "1886240225", Duration: 230901, Origin: list}
	say := func(a music.PlayAction, t music.Track, pos time.Duration) {
		ts.Played(music.PlayEvent{Action: a, Track: t, Position: pos})
	}
	stop := func(t music.Track, pos time.Duration, why music.StopReason) {
		ts.Played(music.PlayEvent{Action: music.Stopped, Track: t, Position: pos, Why: why})
	}

	say(music.Started, one, 0)
	stop(one, at(40400), music.StopPaused)
	say(music.Resumed, one, at(40400))
	ts.Played(music.PlayEvent{Action: music.Sought, Track: one, From: at(48700), Position: at(308300)})
	stop(one, at(328208), music.StopEnded)
	say(music.Started, two, 0)
	stop(two, at(17900), music.StopNext)
	say(music.Started, one, 0)
	stop(one, at(5100), music.StopChanged)
	say(music.Started, pick, 0)
	stop(pick, at(230901), music.StopEnded)
	posts := log.take(t, ts)

	albumIDs := map[string]any{"album-adam-id": "1440763349"}
	listIDs := map[string]any{"global-playlist-id": list.ID, "playlist-version-hash": "8d98"}
	const albumF, listF = "new / album_detail", "new / playlist_detail"
	want := []sent{
		{Type: 1, Hint: 1, Song: one.ID, Duration: 328208, Cont: 3, IDs: albumIDs, Feature: albumF},
		// Paused: where it was paused, in whole seconds.
		{Type: 0, Reason: 3, Start: 0, End: 40 * sec, Song: one.ID, Duration: 328208, Cont: 3, IDs: albumIDs, Feature: albumF},
		// Played on from there, which is the same album.
		{Type: 1, Hint: 0, Start: 40 * sec, Song: one.ID, Duration: 328208, Cont: 3, IDs: albumIDs, Feature: albumF},
		// A jump is two ends, of the stretch before it and the jump.
		{Type: 0, Reason: 12, Start: 40 * sec, End: 48 * sec, Song: one.ID, Duration: 328208, Cont: 3, IDs: albumIDs, Feature: albumF},
		{Type: 0, Reason: 13, Start: 48 * sec, End: 308 * sec, Song: one.ID, Duration: 328208, Cont: 3, IDs: albumIDs, Feature: albumF},
		// Played out: to its length, from where the jump left it.
		{Type: 0, Reason: 7, Start: 308 * sec, End: 328208, Song: one.ID, Duration: 328208, Cont: 3, IDs: albumIDs, Feature: albumF},
		{Type: 1, Hint: 0, Song: two.ID, Duration: 278311, Cont: 3, IDs: albumIDs, Feature: albumF},
		{Type: 0, Reason: 2, End: 17 * sec, Song: two.ID, Duration: 278311, Cont: 3, IDs: albumIDs, Feature: albumF},
		{Type: 1, Hint: 0, Song: one.ID, Duration: 328208, Cont: 3, IDs: albumIDs, Feature: albumF},
		// Chosen from another playlist: it ends for it, and the next says its container changed.
		{Type: 0, Reason: 5, End: 5 * sec, Song: one.ID, Duration: 328208, Cont: 3, IDs: albumIDs, Feature: albumF},
		{Type: 1, Hint: 1, Song: pick.ID, Duration: 230901, Cont: 2, IDs: listIDs, Feature: listF},
		{Type: 0, Reason: 7, End: 230901, Song: pick.ID, Duration: 230901, Cont: 2, IDs: listIDs, Feature: listF},
	}
	got := sentOf(t, posts)
	if len(got) != len(want) {
		t.Fatalf("%d events, want %d:\n%+v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("event %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
	// As the web player asks: with its tokens, and ids of their own.
	h := posts[0].header
	if h.Get("Authorization") != "Bearer "+ts.storage.getString(devTokenKey) || h.Get("media-user-token") != "token" {
		t.Error("tokens")
	}
	seen := map[any]bool{}
	for _, p := range posts {
		id, _ := p.body.Data[0]["persistent-id"].(string)
		if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(id) || seen[id] {
			t.Errorf("persistent-id %q", id)
		}
		seen[id] = true
	}
}

// A song of the listener's own, or one without a catalog ID, is not
// reported; nor is anything with the setting off or the listener signed out.
// What was playing as the setting went off had its end sent first by the
// player, and plays that go on after it start anew.
func TestActivityReportsOnlyWhatItShould(t *testing.T) {
	ts, log := activitySource(t)
	song := music.Track{ID: "1783644617", Duration: 256613}
	other := music.Track{ID: "1880587625", Duration: 1000}
	upload := music.Track{ID: "i.upload"}
	say := func(a music.PlayAction, t music.Track, pos time.Duration) {
		ts.Played(music.PlayEvent{Action: a, Track: t, Position: pos})
	}
	stop := func(t music.Track, pos time.Duration) {
		ts.Played(music.PlayEvent{Action: music.Stopped, Track: t, Position: pos, Why: music.StopNext})
	}
	count := func() int { return len(log.take(t, ts)) }

	say(music.Started, upload, 0)
	stop(upload, at(5000))
	if n := count(); n != 0 {
		t.Errorf("%d posts for a song with no catalog ID", n)
	}

	ts.setSettings(map[string]string{"listeningHistory": " OFF "})
	say(music.Started, song, 0)
	stop(song, at(5000))
	if n := count(); n != 0 {
		t.Errorf("%d posts while off", n)
	}
	// Turned on in the middle of a song, its end isn't sent, as its start
	// wasn't; the play that goes on from its pause starts anew.
	ts.setSettings(map[string]string{"listeningHistory": ""})
	stop(song, at(6000))
	say(music.Resumed, song, at(6000))
	got := sentOf(t, log.take(t, ts))
	if len(got) != 1 || got[0].Type != 1 || got[0].Start != 6*sec {
		t.Errorf("after it was turned on: %+v", got)
	}
	stop(song, at(9000))
	log.take(t, ts)

	ts.storage.remove(userTokenKey)
	say(music.Started, other, 0)
	if n := count(); n != 0 {
		t.Errorf("%d posts signed out", n)
	}
}

// A song with no length from the page is asked of the catalog once, for what
// is sent of it.
func TestActivityLengthOfASongNotListed(t *testing.T) {
	ts, log := activitySource(t)
	song := music.Track{ID: "1783644617"}
	ts.Played(music.PlayEvent{Action: music.Started, Track: song})
	ts.Played(music.PlayEvent{Action: music.Stopped, Track: song, Position: at(3000), Why: music.StopPaused})
	got := sentOf(t, log.take(t, ts))
	if len(got) != 2 || got[0].Duration != 300000 || got[1].Duration != 300000 {
		t.Errorf("sent %+v", got)
	}
	if fmt.Sprint(log.asked) != "[1783644617]" {
		t.Errorf("looked up %v", log.asked)
	}
}

// What the player says as the app quits is sent before the app is, up to a
// wait, and a wait that runs out doesn't hold the app.
func TestActivityFlush(t *testing.T) {
	ts, log := activitySource(t)
	log.gate = make(chan struct{})
	song := music.Track{ID: "1783644617", Duration: 256613}
	ts.Played(music.PlayEvent{Action: music.Started, Track: song})
	ts.Played(music.PlayEvent{Action: music.Stopped, Track: song, Position: at(3000), Why: music.StopQuit})

	start := time.Now()
	ts.Flush(50 * time.Millisecond)
	if time.Since(start) < 40*time.Millisecond {
		t.Error("didn't wait for what was told")
	}
	log.mu.Lock()
	n := len(log.posts)
	log.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d sent while the service was held", n)
	}

	done := make(chan struct{})
	go func() {
		ts.Flush(10 * time.Second)
		close(done)
	}()
	close(log.gate)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting after everything was sent")
	}
	got := sentOf(t, log.take(t, ts))
	if len(got) != 2 || got[1].Reason != 17 {
		t.Errorf("sent %+v", got)
	}
}

// The web player's version comes from its own script, as its developer
// token does, and until it's read is the one it had on 2026-11-10.
func TestActivityBuildVersion(t *testing.T) {
	script := `const a=1;`
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		switch r.URL.Path {
		case "", "/":
			return answer(200, `<script src="/assets/index~1.js">`)
		case "/assets/index~1.js":
			return answer(200, script)
		}
		return answer(404, "")
	})
	if got := ts.webBuild(); got != "2644.9.0-external" {
		t.Errorf("before it's read: %s", got)
	}
	script = `const Do="2650.1.0-external",qa="` + devJWT(ts.clk.Now().AddDate(1, 0, 0)) + `";`
	if _, err := ts.developerToken(true); err != nil {
		t.Fatal(err)
	}
	if got := ts.webBuild(); got != "2650.1.0-external" {
		t.Errorf("after it's read: %s", got)
	}
}

// Where a song is played from, as Apple has it said: by the kind and the IDs
// MusicKit makes of each, which a library's differ from the catalog's.
func TestActivityContainers(t *testing.T) {
	for _, c := range []struct {
		name string
		from music.Origin
		typ  int
		ids  map[string]any
	}{
		{"album", music.Origin{Kind: music.FromAlbum, ID: "1538265733"}, 3, map[string]any{"album-adam-id": "1538265733"}},
		{"library album", music.Origin{Kind: music.FromAlbum, ID: "l.AbCd"}, 3, map[string]any{"cloud-album-id": "l.AbCd"}},
		{"playlist", music.Origin{Kind: music.FromPlaylist, ID: "pl.1", Version: "v"}, 2, map[string]any{"global-playlist-id": "pl.1", "playlist-version-hash": "v"}},
		{"library playlist", music.Origin{Kind: music.FromPlaylist, ID: "p.2", CatalogID: "pl.3"}, 2, map[string]any{"global-playlist-id": "pl.3", "universal-library-id": "p.2"}},
		{"a library playlist of its own", music.Origin{Kind: music.FromPlaylist, ID: "p.2"}, 2, map[string]any{"universal-library-id": "p.2"}},
		{"station", music.Origin{Kind: music.FromStation, ID: "ra.1234"}, 1, map[string]any{"station-id": "ra.1234"}},
		{"personal station", music.Origin{Kind: music.FromStation, ID: "st.99"}, 1, map[string]any{"station-id": "st.99", "station-personalized-id": 99}},
		{"artist", music.Origin{Kind: music.FromArtist, ID: "12"}, 4, nil},
		{"nothing", music.Origin{}, 0, nil},
	} {
		typ, ids, _ := containerOf(c.from)
		if len(ids) == 0 {
			ids = nil
		}
		if typ != c.typ || !reflect.DeepEqual(ids, c.ids) {
			t.Errorf("%s: type %d, ids %v; want %d, %v", c.name, typ, ids, c.typ, c.ids)
		}
	}
}

// What the web player sends, field for field and of the same kind, as
// recorded in testdata/web-play-events.json: a field more or less, or one of
// another type, is what tells Apple it isn't the web player.
func TestActivityFieldsAreTheWebPlayers(t *testing.T) {
	raw, err := os.ReadFile("testdata/web-play-events.json")
	if err != nil {
		t.Fatal(err)
	}
	var web struct {
		Events map[string]map[string]any
	}
	if err := json.Unmarshal(raw, &web); err != nil {
		t.Fatal(err)
	}
	ts, log := activitySource(t)
	album := music.Origin{Kind: music.FromAlbum, ID: "1440763349"}
	list := music.Origin{Kind: music.FromPlaylist, ID: "pl.fc2f59c513644e81b8c40ffd3e339134", Version: "8d98"}
	for _, c := range []struct {
		kind string
		from music.Origin
	}{{"album", album}, {"playlist", list}} {
		song := music.Track{ID: "1440763351", Duration: 328208, Origin: c.from}
		ts.Played(music.PlayEvent{Action: music.Started, Track: song, Mode: music.Mode{Shuffle: true, Repeat: music.RepeatAll}})
		ts.Played(music.PlayEvent{Action: music.Stopped, Track: song, Position: at(40000), Why: music.StopPaused})
		posts := log.take(t, ts)
		for i, what := range []string{"start", "end"} {
			ours := posts[i].body.Data[0]
			theirs := web.Events[c.kind+" "+what]
			if theirs == nil {
				t.Fatalf("no web %s %s", c.kind, what)
			}
			if diff := shapeDiff("", ours, theirs); diff != "" {
				t.Errorf("%s %s: %s", c.kind, what, diff)
			}
		}
	}
}

// shapeDiff says how a differs from b in the names of fields and the kind
// of their values, objects looked into, or "" for none.
func shapeDiff(path string, a, b map[string]any) string {
	for k, bv := range b {
		av, ok := a[k]
		if !ok {
			return path + k + " is missing"
		}
		if reflect.TypeOf(av) != reflect.TypeOf(bv) {
			return fmt.Sprintf("%s%s is a %T, not a %T", path, k, av, bv)
		}
		if bm, ok := bv.(map[string]any); ok {
			if d := shapeDiff(path+k+".", av.(map[string]any), bm); d != "" {
				return d
			}
		}
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return path + k + " is not the web player's"
		}
	}
	return ""
}
