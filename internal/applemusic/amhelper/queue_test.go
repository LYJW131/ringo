package amhelper

import (
	"context"
	"testing"
	"time"
)

// waitIn queues a request for song and reports, by name, when it gets the wrapper.
func waitIn(t *testing.T, q *queue, ctx context.Context, song, name string, got chan<- string) {
	t.Helper()
	go func() {
		release, err := q.acquire(ctx, song)
		if err != nil {
			got <- "cancelled " + name
			return
		}
		got <- name
		release()
	}()
	// In order: each in the queue before the next.
	for {
		q.mu.Lock()
		n := len(q.waiting)
		queued := n > 0 && q.waiting[n-1].song == song
		q.mu.Unlock()
		if queued {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func order(t *testing.T, got <-chan string, n int) []string {
	t.Helper()
	var out []string
	for range n {
		select {
		case s := <-got:
			out = append(out, s)
		case <-time.After(2 * time.Second):
			t.Fatalf("stuck after %v", out)
		}
	}
	return out
}

func TestQueuePlayedSongsGoFirst(t *testing.T) {
	q := newQueue()
	release, _ := q.acquire(context.Background(), "busy")
	got := make(chan string, 8)
	bg := context.Background()
	waitIn(t, q, ahead(bg), "1", "ahead 1", got)
	waitIn(t, q, ahead(bg), "2", "ahead 2", got)
	waitIn(t, q, bg, "3", "played 3", got)
	waitIn(t, q, ahead(bg), "0", "opening key", got)
	q.play("2") // the listener plays a song being worked out ahead
	release()
	// The played ones, and the opening key, in the order they came; then the rest.
	want := []string{"ahead 2", "played 3", "opening key", "ahead 1"}
	if g := order(t, got, 4); !equal(g, want) {
		t.Fatalf("served %v, want %v", g, want)
	}
}

func TestQueueSkipsWhoGaveUp(t *testing.T) {
	q := newQueue()
	release, _ := q.acquire(context.Background(), "busy")
	got := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())
	waitIn(t, q, ctx, "1", "gave up", got)
	waitIn(t, q, context.Background(), "2", "waited", got)
	cancel()
	if g := order(t, got, 1); g[0] != "cancelled gave up" {
		t.Fatalf("got %v", g)
	}
	release()
	if g := order(t, got, 1); g[0] != "waited" {
		t.Fatalf("got %v", g)
	}
	if again, err := q.acquire(context.Background(), "3"); err != nil {
		t.Fatal(err)
	} else {
		again()
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
