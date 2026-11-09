package amhelper

import (
	"context"
	"sync"
)

// The wrapper answers one request at a time: wrapper-lite's /m3u8 and /key
// share a lock. So amhelper sends it one at a time too, and picks the next
// for the listener: a song they play goes before the songs worked out ahead
// of them, as amdl ranks a key needed now before one fetched ahead. A song
// worked out ahead that they then play is theirs from then on, whatever asked
// for it first. A request already sent isn't called back: the song they play
// waits for at most that one.
type queue struct {
	mu      sync.Mutex
	busy    bool
	waiting []*waiter
	played  *lru[struct{}]
}

type waiter struct {
	song  string
	ahead bool
	ready chan struct{}
}

func newQueue() *queue { return &queue{played: newLRU[struct{}](200)} }

type aheadKey struct{}

// ahead marks ctx as working a song out ahead of its playing.
func ahead(ctx context.Context) context.Context { return context.WithValue(ctx, aheadKey{}, true) }

func isAhead(ctx context.Context) bool { v, _ := ctx.Value(aheadKey{}).(bool); return v }

// play says the listener plays song: what it waits for goes first.
func (q *queue) play(song string) {
	q.mu.Lock()
	q.played.put(song, struct{}{})
	q.mu.Unlock()
}

// first reports whether w goes before the ones worked out ahead. The key
// every song opens with ("0") is everyone's.
func (q *queue) first(w *waiter) bool {
	if !w.ahead || w.song == "0" {
		return true
	}
	_, ok := q.played.get(w.song)
	return ok
}

// acquire waits for the wrapper to be free for song, and returns what
// frees it again.
func (q *queue) acquire(ctx context.Context, song string) (func(), error) {
	q.mu.Lock()
	if !q.busy {
		q.busy = true
		q.mu.Unlock()
		return q.release, nil
	}
	w := &waiter{song: song, ahead: isAhead(ctx), ready: make(chan struct{})}
	q.waiting = append(q.waiting, w)
	q.mu.Unlock()
	select {
	case <-w.ready:
		return q.release, nil
	case <-ctx.Done():
		q.mu.Lock()
		for i, x := range q.waiting {
			if x == w {
				q.waiting = append(q.waiting[:i], q.waiting[i+1:]...)
				q.mu.Unlock()
				return nil, ctx.Err()
			}
		}
		q.mu.Unlock()
		// Handed the wrapper as ctx ended: hand it on.
		q.release()
		return nil, ctx.Err()
	}
}

func (q *queue) release() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.waiting) == 0 {
		q.busy = false
		return
	}
	next := 0
	for i, w := range q.waiting {
		if q.first(w) {
			next = i
			break
		}
	}
	w := q.waiting[next]
	q.waiting = append(q.waiting[:next], q.waiting[next+1:]...)
	close(w.ready)
}
