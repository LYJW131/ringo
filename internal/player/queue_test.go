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
