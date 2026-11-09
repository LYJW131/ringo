package applemusic

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"testing"
	"time"

	"mygo-music/internal/music"
)

// Plays reported to Apple's activity service as the web player reports
// them, against a stand-in that keeps what it was sent.
func TestActivity(t *testing.T) {
	type sent struct {
		header http.Header
		body   struct {
			ClientID  string           `json:"client_id"`
			EventType string           `json:"event_type"`
			Data      []map[string]any `json:"data"`
		}
	}
	var (
		mu      sync.Mutex
		posts   []sent
		lookups []string
	)
	song := regexp.MustCompile(`^/v1/catalog/jp/songs/(\d+)$`)
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		// A song the app never listed: its length comes from the catalog.
		if m := song.FindStringSubmatch(r.URL.Path); m != nil {
			mu.Lock()
			lookups = append(lookups, m[1])
			mu.Unlock()
			return answer(200, map[string]any{"data": []any{map[string]any{"id": m[1], "type": "songs", "attributes": map[string]any{"durationInMillis": 300000}}}})
		}
		if r.URL.String() != activityURL {
			return answer(404, "")
		}
		if r.Method != "POST" {
			t.Errorf("method %s", r.Method)
		}
		var p sent
		p.header = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &p.body)
		mu.Lock()
		posts = append(posts, p)
		mu.Unlock()
		return answer(200, "{}")
	})
	ts.storage.set(userTokenKey, "token")
	ts.storage.set(storefrontKey, "jp")
	play := func(id string) { ts.Played(music.Track{ID: id}) }
	take := func() []sent {
		ts.clk.settle(t)
		mu.Lock()
		defer mu.Unlock()
		out := posts
		posts = nil
		return out
	}
	num := func(v any) float64 { f, _ := v.(float64); return f }
	adam := func(e map[string]any) string {
		ids, _ := e["ids"].(map[string]any)
		s, _ := ids["subscription-adam-id"].(string)
		return s
	}

	// A song's start is sent as it plays, and its end with the next one's start.
	ts.noteDuration("1783644617", 256613)
	play("1783644617")
	got := take()
	if len(got) != 1 {
		t.Fatalf("%d posts", len(got))
	}
	req := got[0]
	if req.header.Get("Authorization") != "Bearer "+ts.storage.getString(devTokenKey) || req.header.Get("media-user-token") != "token" {
		t.Error("tokens")
	}
	if req.body.ClientID != "JSCLIENT" || req.body.EventType != "JSPLAY" || len(req.body.Data) != 1 {
		t.Fatalf("body %+v", req.body)
	}
	start := req.body.Data[0]
	if num(start["event-type"]) != 1 || adam(start) != "1783644617" || num(start["media-duration-in-milliseconds"]) != 256613 || start["store-front"] != "jp" {
		t.Errorf("start %v", start)
	}
	// Skipped a moment later: the end says so, and the next song starts.
	ts.clk.advance(t, time.Second)
	play("1880587625")
	got = take()
	if len(got) != 1 || len(got[0].body.Data) != 2 {
		t.Fatalf("posts %+v", got)
	}
	end, next := got[0].body.Data[0], got[0].body.Data[1]
	if num(end["event-type"]) != 0 || adam(end) != "1783644617" || num(end["end-reason-type"]) != 2 || num(end["end-position-in-milliseconds"]) >= 256613 {
		t.Errorf("end %v", end)
	}
	if num(next["event-type"]) != 1 || adam(next) != "1880587625" || num(next["media-duration-in-milliseconds"]) != 300000 {
		t.Errorf("next %v", next)
	}
	if !slices.Equal(lookups, []string{"1880587625"}) {
		t.Errorf("looked up %v", lookups)
	}
	if end["persistent-id"] == next["persistent-id"] {
		t.Error("the same persistent-id")
	}

	// A song played out ends naturally, at its length.
	ts.noteDuration("1", 1)
	play("1")
	take()
	ts.clk.advance(t, 5*time.Millisecond)
	play("i.upload")
	got = take()
	if len(got) != 1 || len(got[0].body.Data) != 1 {
		t.Fatalf("a song outside the catalog is reported: %+v", got)
	}
	if e := got[0].body.Data[0]; num(e["end-reason-type"]) != 7 || num(e["end-position-in-milliseconds"]) != 1 {
		t.Errorf("end %v", e)
	}
	play("i.upload")
	if n := len(take()); n != 0 {
		t.Errorf("%d posts for nothing", n)
	}

	// Off in the setting, or signed out, sends nothing.
	play("1783644617")
	take()
	ts.setSettings(map[string]string{"listeningHistory": " OFF "})
	play("1880587625")
	if n := len(take()); n != 0 {
		t.Errorf("%d posts while off", n)
	}
	ts.setSettings(map[string]string{"listeningHistory": ""})
	play("1783644617")
	if got := take(); len(got) != 1 || len(got[0].body.Data) != 1 {
		t.Errorf("something left over from before it was off: %+v", got)
	}
	ts.storage.remove(userTokenKey)
	play("1880587625")
	if n := len(take()); n != 0 {
		t.Errorf("%d posts signed out", n)
	}
	ts.storage.set(userTokenKey, "token")

	// Plays close together each end the one just before them.
	play("1783644617")
	take()
	var wg sync.WaitGroup
	for _, id := range []string{"1001", "1783644617"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			play(id)
		}()
	}
	wg.Wait()
	play("1880587625")
	var ends []string
	for _, p := range take() {
		for _, e := range p.body.Data {
			if num(e["event-type"]) == 0 {
				ends = append(ends, adam(e))
			}
		}
	}
	slices.Sort(ends)
	if !slices.Equal(ends, []string{"1001", "1783644617", "1783644617"}) {
		t.Errorf("ends %v", ends)
	}
}
