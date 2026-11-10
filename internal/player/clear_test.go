package player

import (
	"strings"
	"testing"

	"ringo/internal/music"
)

// Clearing what's up next leaves the song playing as it plays, with what
// played before it and nothing after: the next song, asked for ahead, is
// let go, and going to the next plays none of those cleared.
func TestClearNext(t *testing.T) {
	p, ch, src, q := servedQueue(t, "a", "b", "c", "d")
	p.PlayQueue(q, 0, music.Origin{})
	settle(t, ch, p)
	p.Next()
	settle(t, ch, p)
	eventually(t, "the next song wasn't asked for ahead", func() bool { return src.count(src.asked, "c") == 1 })
	gen, out := p.gen, p.out
	p.ClearNext()
	if got := strings.Join(ids(p), " "); got != "a b" {
		t.Fatalf("the queue plays %s", got)
	}
	if p.gen != gen || p.out != out || p.Current().ID != "b" || !p.Playing() {
		t.Errorf("loaded again: gen %d to %d, same audio %v, on %s", gen, p.gen, p.out == out, p.Current().ID)
	}
	if next, ok := p.upNext(); ok || p.ahead != nil {
		t.Errorf("up next is %+v, ahead %+v", next, p.ahead)
	}
	p.Next()
	settle(t, ch, p)
	if id := p.Current().ID; id == "c" || id == "d" {
		t.Errorf("Next plays %s, which was cleared", id)
	}
	// With nothing after the song playing, there's nothing to clear.
	p.Jump(1)
	settle(t, ch, p)
	p.ClearNext()
	if got := strings.Join(ids(p), " "); got != "a b" {
		t.Errorf("cleared again, the queue plays %s", got)
	}
}

// Shuffled, what's cleared is what was to play after the song playing, and
// those kept are in the order they were given once shuffle goes off.
func TestClearNextShuffled(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]music.Track{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}})
	p.shuffle = true
	p.order, p.at, p.track = []int{2, 0, 3, 1}, 1, &p.queue[0]
	p.ClearNext()
	order, at := p.PlayOrder()
	if got := strings.Join(ids(p), " "); got != "3 1" || p.Queue()[order[at]].ID != "1" {
		t.Fatalf("shuffled, the queue plays %s from %d", got, at)
	}
	p.ToggleShuffle()
	order, at = p.PlayOrder()
	if got := strings.Join(ids(p), " "); got != "1 3" || p.Queue()[order[at]].ID != "1" {
		t.Errorf("unshuffled, the queue plays %s from %d", got, at)
	}
}

// Cleared, the queue ends with the song playing, and what goes on from
// there is asked anew: AutoPlay for songs like it, whose answer to what it
// was asked before the clearing is dropped, and a radio for more.
func TestClearNextAsksForMore(t *testing.T) {
	p := &Player{at: -1}
	var answers []func([]music.Track)
	p.More = func(got func([]music.Track)) bool {
		answers = append(answers, got)
		return true
	}
	p.Cue([]music.Track{{ID: "1"}, {ID: "2"}})
	p.refill()
	p.ClearNext()
	if len(answers) != 2 {
		t.Fatalf("AutoPlay asked %d times", len(answers))
	}
	answers[0]([]music.Track{{ID: "old"}})
	answers[1]([]music.Track{{ID: "new"}})
	if got := strings.Join(ids(p), " "); got != "1 new" {
		t.Errorf("the queue plays %s", got)
	}

	p = &Player{at: -1}
	fed := 0
	p.Cue([]music.Track{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}})
	p.AttachRadio("station", func(got func([]music.Track)) { fed++ })
	p.ClearNext()
	if fed != 1 || p.Radio() != "station" || strings.Join(ids(p), " ") != "1" {
		t.Errorf("the radio was asked %d times, is %q, and the queue plays %v", fed, p.Radio(), ids(p))
	}
}
