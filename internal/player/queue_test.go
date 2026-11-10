package player

import (
	"strings"
	"testing"

	"ringo/internal/music"
)

func tracksOf(ids ...string) []music.Track {
	var q []music.Track
	for _, id := range ids {
		q = append(q, music.Track{ID: id})
	}
	return q
}

// Removing a song still to play takes it out of the queue and the order, the
// song playing playing on; the one that has played, or plays, can't go.
func TestRemove(t *testing.T) {
	p := &Player{at: -1}
	p.Cue(tracksOf("1", "2", "3", "4"))
	p.at, p.track = 1, &p.queue[1]
	if p.Remove(0) || p.Remove(1) {
		t.Fatal("removed a song that has played, or plays")
	}
	if !p.Remove(2) {
		t.Fatal("didn't remove a song to play")
	}
	if got := strings.Join(ids(p), " "); got != "1 2 4" {
		t.Errorf("the queue plays %s", got)
	}
	if len(p.Queue()) != 3 || p.Current().ID != "2" {
		t.Errorf("queue %v, playing %v", p.Queue(), p.Current())
	}
	if _, at := p.PlayOrder(); at != 1 {
		t.Errorf("at %d", at)
	}
}

// Shuffled, a song is removed from where it is in the play order, and the
// order of the rest holds.
func TestRemoveShuffled(t *testing.T) {
	p := &Player{at: -1}
	p.Cue(tracksOf("1", "2", "3", "4", "5", "6"))
	p.ToggleShuffle()
	before := ids(p)
	gone := before[2]
	var i int
	for k, tr := range p.Queue() {
		if tr.ID == gone {
			i = k
		}
	}
	if !p.Remove(i) {
		t.Fatal("didn't remove")
	}
	want := append(append([]string{}, before[:2]...), before[3:]...)
	if got := ids(p); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the queue plays %v, want %v", got, want)
	}
	if p.Current().ID != before[0] {
		t.Errorf("playing %s", p.Current().ID)
	}
}

// The song asked for ahead, removed, isn't played after the one playing: the
// one after it is asked for in its place, and plays.
func TestRemoveNextAhead(t *testing.T) {
	p, ch, src, q := servedQueue(t, "a", "b", "c")
	p.PlayQueue(q, 0, music.Origin{})
	settle(t, ch, p)
	eventually(t, "b didn't come in ahead", func() bool { return src.count(src.fetched, "/b") == 1 })
	if p.ahead == nil || p.ahead.id != "b" {
		t.Fatalf("has %+v ahead", p.ahead)
	}
	if !p.Remove(1) {
		t.Fatal("didn't remove b")
	}
	if p.ahead == nil || p.ahead.id != "c" {
		t.Fatalf("after removing b, has %+v ahead", p.ahead)
	}
	p.Next()
	settle(t, ch, p)
	if p.Current().ID != "c" || p.Err() != "" || !p.Playing() {
		t.Fatalf("on %s, playing %v, err %q", p.Current().ID, p.Playing(), p.Err())
	}
	if src.count(src.asked, "b") > 1 || src.count(src.asked, "c") != 1 {
		t.Errorf("b asked for %d times, c %d", src.count(src.asked, "b"), src.count(src.asked, "c"))
	}
}

// Removing the last song leaves nothing next: what was asked for ahead goes,
// and at the end, the queue is over rather than playing the removed song.
func TestRemoveLastDropsAhead(t *testing.T) {
	p, ch, src, q := servedQueue(t, "a", "b")
	p.PlayQueue(q, 0, music.Origin{})
	settle(t, ch, p)
	eventually(t, "b didn't come in ahead", func() bool { return src.count(src.fetched, "/b") == 1 })
	if !p.Remove(1) {
		t.Fatal("didn't remove b")
	}
	if p.ahead != nil {
		t.Errorf("still has %s ahead", p.ahead.id)
	}
	if len(p.Queue()) != 1 {
		t.Errorf("queue %v", p.Queue())
	}
}

// A radio is asked for more once removing leaves its queue near its end, as
// it is when the queue is played down to it.
func TestRemoveRefillsRadio(t *testing.T) {
	p := &Player{at: -1}
	p.Cue(tracksOf("1", "2", "3", "4"))
	asked := 0
	p.AttachRadio("station", func(got func([]music.Track)) { asked++ })
	asked = 0
	p.Remove(3)
	p.Remove(2)
	if asked != 1 {
		t.Errorf("the radio was asked %d times", asked)
	}
}

// Moving a song still to play puts it at the place asked in the order, the
// song playing playing on, and not before it or from before it.
func TestMove(t *testing.T) {
	p := &Player{at: -1}
	p.Cue(tracksOf("1", "2", "3", "4", "5"))
	p.at, p.track = 1, &p.queue[1]
	if !p.Move(4, 2) {
		t.Fatal("didn't move")
	}
	if got := strings.Join(ids(p), " "); got != "1 2 5 3 4" {
		t.Errorf("the queue plays %s", got)
	}
	// Not shuffled, the queue is as it plays.
	if q := p.Queue(); q[2].ID != "5" || p.Current().ID != "2" {
		t.Errorf("queue %v, playing %v", q, p.Current())
	}
	if _, at := p.PlayOrder(); at != 1 {
		t.Errorf("at %d", at)
	}
	for _, c := range []struct{ i, at int }{{0, 3}, {1, 3}, {3, 1}, {3, 0}, {3, 5}, {4, 4}} {
		if p.Move(c.i, c.at) {
			t.Errorf("moved %d to %d", c.i, c.at)
		}
	}
	if got := strings.Join(ids(p), " "); got != "1 2 5 3 4" {
		t.Errorf("after those, the queue plays %s", got)
	}
}

// Shuffled, a move changes the order only: it plays as moved, and the queue
// is as it was given.
func TestMoveShuffled(t *testing.T) {
	p := &Player{at: -1}
	p.Cue(tracksOf("1", "2", "3", "4", "5", "6"))
	p.ToggleShuffle()
	before := ids(p)
	last := before[5]
	var i int
	for k, tr := range p.Queue() {
		if tr.ID == last {
			i = k
		}
	}
	if !p.Move(i, 1) {
		t.Fatal("didn't move")
	}
	want := []string{before[0], last, before[1], before[2], before[3], before[4]}
	if got := ids(p); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the queue plays %v, want %v", got, want)
	}
	if p.Current().ID != before[0] {
		t.Errorf("playing %s", p.Current().ID)
	}
	if p.Queue()[0].ID != "1" || p.Queue()[5].ID != "6" {
		t.Errorf("the queue as given changed: %v", p.Queue())
	}
}

// A song moved to be next is the one asked for ahead and played after the
// song playing; the one that was next waits after it.
func TestMoveToNextAhead(t *testing.T) {
	p, ch, src, q := servedQueue(t, "a", "b", "c")
	p.PlayQueue(q, 0, music.Origin{})
	settle(t, ch, p)
	eventually(t, "b didn't come in ahead", func() bool { return src.count(src.fetched, "/b") == 1 })
	if !p.Move(2, 1) {
		t.Fatal("didn't move c")
	}
	if p.ahead == nil || p.ahead.id != "c" {
		t.Fatalf("after moving c, has %+v ahead", p.ahead)
	}
	p.Next()
	settle(t, ch, p)
	if p.Current().ID != "c" || p.Err() != "" || !p.Playing() {
		t.Fatalf("on %s, playing %v, err %q", p.Current().ID, p.Playing(), p.Err())
	}
	if src.count(src.asked, "c") != 1 {
		t.Errorf("c asked for %d times", src.count(src.asked, "c"))
	}
	p.Next()
	settle(t, ch, p)
	if p.Current().ID != "b" {
		t.Errorf("then %s", p.Current().ID)
	}
}
