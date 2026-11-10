package player

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"mygo-music/internal/music"
)

// served answers each track as a song of its own address on a server, as a
// plugin does, counting what its source and the server are asked.
type served struct {
	nothing
	url string

	mu      sync.Mutex
	asked   map[string]int // streams, by track
	fetched map[string]int // requests, by path
}

func (s *served) Stream(id, _ string) (*music.Stream, error) {
	s.mu.Lock()
	s.asked[id]++
	s.mu.Unlock()
	return &music.Stream{URL: s.url + "/" + id, Type: "m4a"}, nil
}

func (s *served) count(of map[string]int, key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return of[key]
}

func servedQueue(t *testing.T, ids ...string) (*Player, chan func(), *served, []music.Track) {
	t.Helper()
	b, err := os.ReadFile("testdata/seek-aac-frag.m4a")
	if err != nil {
		t.Fatal(err)
	}
	src := &served{asked: map[string]int{}, fetched: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		src.mu.Lock()
		src.fetched[r.URL.Path]++
		src.mu.Unlock()
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	src.url = srv.URL
	ch := make(chan func(), 64)
	p := New(func(fn func()) { ch <- fn })
	p.SetVolume(0)
	p.Source = func(string) music.Source { return src }
	t.Cleanup(p.stop)
	var q []music.Track
	for _, id := range ids {
		q = append(q, music.Track{ID: id, Source: "s"})
	}
	return p, ch, src, q
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); !cond(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
	}
}

// The next song is asked for once the current one plays, and comes in
// meanwhile: it then plays from that, its source asked for it once and its
// address fetched once, as a plugin's may answer only one request.
func TestNextSongComesInAhead(t *testing.T) {
	p, ch, src, q := servedQueue(t, "a", "b")
	p.PlayQueue(q, 0, music.Origin{})
	settle(t, ch, p)
	eventually(t, "the next song didn't come in ahead", func() bool { return src.count(src.fetched, "/b") == 1 })
	p.Next()
	settle(t, ch, p)
	if p.Current().ID != "b" || p.Err() != "" || !p.Playing() {
		t.Fatalf("on %s, playing %v, err %q", p.Current().ID, p.Playing(), p.Err())
	}
	if asked, fetched := src.count(src.asked, "b"), src.count(src.fetched, "/b"); asked != 1 || fetched != 1 {
		t.Errorf("b asked for %d times, fetched %d", asked, fetched)
	}
}

// What was asked for ahead goes once it isn't next: the last song has nothing
// after it, and the first again only with the queue repeating.
func TestAheadFollowsWhatsNext(t *testing.T) {
	p, ch, src, q := servedQueue(t, "a", "b", "c")
	p.PlayQueue(q, 0, music.Origin{})
	settle(t, ch, p)
	p.Jump(2)
	settle(t, ch, p)
	if p.ahead != nil {
		t.Errorf("still has %s ahead, after the last song", p.ahead.id)
	}
	p.CycleRepeat() // to RepeatAll
	p.Jump(1)
	settle(t, ch, p)
	p.Jump(2)
	settle(t, ch, p)
	if p.ahead == nil || p.ahead.id != "a" {
		t.Errorf("repeating, has %+v ahead", p.ahead)
	}
	// Once as it was picked, and once ahead of its turn after b, which played.
	if src.count(src.asked, "c") != 2 {
		t.Errorf("c was asked for %d times", src.count(src.asked, "c"))
	}
}
