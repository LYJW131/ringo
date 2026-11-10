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

// listener is a source that keeps what it's told of what plays.
type listener struct {
	nothing
	url string

	mu  sync.Mutex
	got []music.PlayEvent
}

func (l *listener) Stream(id, _ string) (*music.Stream, error) {
	return &music.Stream{URL: l.url, Type: "m4a"}, nil
}

func (l *listener) Played(ev music.PlayEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.got = append(l.got, ev)
}

// take returns what it was told since it was last asked.
func (l *listener) take() []music.PlayEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.got
	l.got = nil
	return out
}

func listenedQueue(t *testing.T, ids ...string) (*Player, chan func(), *listener, []music.Track) {
	t.Helper()
	b, err := os.ReadFile("testdata/seek-aac-frag.m4a")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	t.Cleanup(srv.Close)
	src := &listener{url: srv.URL}
	ch := make(chan func(), 64)
	p := New(func(fn func()) { ch <- fn })
	p.SetVolume(0)
	p.Source = func(string) music.Source { return src }
	t.Cleanup(p.stop)
	var q []music.Track
	for _, id := range ids {
		q = append(q, music.Track{ID: id, Source: "s", Duration: 5000})
	}
	return p, ch, src, q
}

// run runs what the player posted, for a pause to land.
func run(t *testing.T, ch chan func()) {
	t.Helper()
	select {
	case fn := <-ch:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("nothing posted")
	}
}

// show is what an event says, for a test to compare: its action, with why
// it stopped, and the song.
func show(ev music.PlayEvent) [3]any {
	switch ev.Action {
	case music.Stopped:
		return [3]any{ev.Action, ev.Why, ev.Track.ID}
	}
	return [3]any{ev.Action, nil, ev.Track.ID}
}

func told(t *testing.T, src *listener, want ...[3]any) []music.PlayEvent {
	t.Helper()
	got := src.take()
	if len(got) != len(want) {
		t.Fatalf("told %d things, want %d: %v", len(got), len(want), got)
	}
	for i := range got {
		if show(got[i]) != want[i] {
			t.Errorf("told %v, want %v", show(got[i]), want[i])
		}
	}
	return got
}

// The source is told, as they happen and where in the song they are, a
// song starting, being paused, played on, jumped within and skipped, and
// the queue changing for another, as Apple wants to hear of them.
func TestSourceToldWhatHappens(t *testing.T) {
	p, ch, src, q := listenedQueue(t, "a", "b", "c")
	album := music.Origin{Kind: music.FromAlbum, ID: "1"}
	p.Autoplay = func() bool { return true }
	p.PlayQueue(q, 0, album)
	settle(t, ch, p)
	got := told(t, src, [3]any{music.Started, nil, "a"})
	if got[0].Position != 0 || got[0].Track.Origin != album || !got[0].Mode.Autoplay || got[0].Mode.Shuffle {
		t.Errorf("the start: %+v", got[0])
	}

	// A jump says where from and to.
	p.Seek(time.Second)
	got = told(t, src, [3]any{music.Sought, nil, "a"})
	if got[0].Position != time.Second || got[0].From >= time.Second {
		t.Errorf("the seek: from %v to %v", got[0].From, got[0].Position)
	}
	for len(ch) > 0 {
		run(t, ch)
	}
	time.Sleep(2 * seekGap)
	for len(ch) > 0 {
		run(t, ch)
	}

	// A pause says where it was paused, not by the clock, and going on says
	// where from. A seek while paused is not a play.
	p.TogglePlay()
	run(t, ch)
	got = told(t, src, [3]any{music.Stopped, music.StopPaused, "a"})
	paused := got[0].Position
	if paused < time.Second || paused > 2*time.Second {
		t.Errorf("paused at %v after jumping to 1s", paused)
	}
	p.Seek(500 * time.Millisecond)
	told(t, src)
	p.TogglePlay()
	got = told(t, src, [3]any{music.Resumed, nil, "a"})
	if got[0].Position < 500*time.Millisecond || got[0].Position > time.Second {
		t.Errorf("played on from %v after a seek to 0.5s while paused", got[0].Position)
	}
	// Pausing again, and quitting while paused, tell nothing twice.
	p.TogglePlay()
	run(t, ch)
	told(t, src, [3]any{music.Stopped, music.StopPaused, "a"})
	p.Exit()
	told(t, src)
	p.TogglePlay()
	told(t, src, [3]any{music.Resumed, nil, "a"})

	// Skipping on, and back, say they did.
	p.Next()
	settle(t, ch, p)
	got = told(t, src, [3]any{music.Stopped, music.StopNext, "a"}, [3]any{music.Started, nil, "b"})
	if got[1].Track.Origin != album {
		t.Errorf("the next song is played from %+v", got[1].Track.Origin)
	}
	p.Prev()
	settle(t, ch, p)
	told(t, src, [3]any{music.Stopped, music.StopPrev, "b"}, [3]any{music.Started, nil, "a"})
	p.Jump(2)
	settle(t, ch, p)
	told(t, src, [3]any{music.Stopped, music.StopChanged, "a"}, [3]any{music.Started, nil, "c"})

	// Another queue, of another playlist, ends what plays for it.
	list := music.Origin{Kind: music.FromPlaylist, ID: "pl.2", Version: "v"}
	p.PlayQueue(q[:2], 1, list)
	settle(t, ch, p)
	got = told(t, src, [3]any{music.Stopped, music.StopChanged, "c"}, [3]any{music.Started, nil, "b"})
	if got[1].Track.Origin != list {
		t.Errorf("the playlist's song is played from %+v", got[1].Track.Origin)
	}
	// What is queued next keeps where it's from.
	p.PlayLast([]music.Track{{ID: "z", Source: "s"}}, album)
	if o := p.Queue()[len(p.Queue())-1].Origin; o != album {
		t.Errorf("a song queued last is from %+v", o)
	}

	// The last song played out ends as it ends, not once something else plays.
	p.PlayQueue(q[:1], 0, album)
	settle(t, ch, p)
	told(t, src, [3]any{music.Stopped, music.StopChanged, "b"}, [3]any{music.Started, nil, "a"})
	p.ended()
	told(t, src, [3]any{music.Stopped, music.StopEnded, "a"})
	p.Exit()
	told(t, src)

	// Quitting while a song plays ends it, once.
	p.PlayQueue(q, 0, album)
	settle(t, ch, p)
	told(t, src, [3]any{music.Started, nil, "a"})
	p.Exit()
	told(t, src, [3]any{music.Stopped, music.StopQuit, "a"})
	p.Exit()
	told(t, src)
}

// A song that played out on repeat ends, and starts again, and the song
// that didn't start, as one that can't play, is not ended.
func TestSourceToldOfRepeatAndFailures(t *testing.T) {
	p, ch, src, q := listenedQueue(t, "a", "b")
	p.CycleRepeat()
	p.CycleRepeat() // one
	p.PlayQueue(q, 0, music.Origin{})
	settle(t, ch, p)
	told(t, src, [3]any{music.Started, nil, "a"})
	p.ended()
	got := told(t, src, [3]any{music.Stopped, music.StopEnded, "a"}, [3]any{music.Started, nil, "a"})
	if got[1].Position != 0 || got[1].Mode.Repeat != music.RepeatOne {
		t.Errorf("repeated from %v in mode %+v", got[1].Position, got[1].Mode)
	}

	// Switched off mid-song: what plays is told over, once, and no more of it.
	p.StopTelling()
	told(t, src, [3]any{music.Stopped, music.StopOther, "a"})
	p.StopTelling()
	p.Exit()
	told(t, src)
}
