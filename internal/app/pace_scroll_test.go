package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"ringo/internal/music"
)

// songsOf makes the library's songs n of them, a hundred to a part as Apple
// gives them, of which the source asks for parts at a time.
func (p *pace) songsOf(n, parts int) {
	fake(p.a).browse = func(page string) (*music.Browse, error) {
		p.answer("browse")
		from := 0
		if _, at, ok := strings.Cut(page, "|"); ok {
			fmt.Sscan(at, &from)
		}
		to := min(from+100*parts, n)
		b := &music.Browse{Total: n, Sections: []music.Section{{Style: music.ShelfList, Items: paceSongs(from, to-from)}}}
		if to < n {
			b.Next = fmt.Sprint(music.PageLibrarySongs, "|", to)
		}
		return b, nil
	}
}

// scrolling scrolls the page down at speed DIPs a second until its end, as
// far as it is loaded, shows with nothing more to come. It says how long that
// takes in the test's time, and in how many frames the end of what was
// loaded, or what stands in for what isn't, was in view.
func (p *pace) scrolling(speed float32) (time.Duration, int) {
	p.t.Helper()
	start, waits := p.clock(), 0
	d := p.a.browses[music.PageLibrarySongs]
	for p.clock()-start < 10*time.Minute {
		p.tt.Scroll(700, 400, 0, speed*0.048)
		p.step(48 * time.Millisecond)
		_, last := p.a.trackList.Visible()
		if last > len(d.page.Sections[0].Items) {
			waits++
		}
		if d.page.Next == "" && p.a.trackList.AtEnd() {
			return p.clock() - start, waits
		}
	}
	p.t.Fatal("never came to the end")
	return 0, 0
}

// Scrolling through a thousand songs in a hard fling never comes to the end of
// what's loaded, so that the time it takes is the scrolling's alone.
func TestLibraryScrollWaitsForNothing(t *testing.T) {
	const speed = 6000
	p := newPace(t, 400*time.Millisecond)
	p.songsOf(1000, 3)
	p.a.startLibrary()
	p.idle(2 * time.Second)
	p.a.open(route{page: pageBrowse, id: music.PageLibrarySongs})
	p.idle(time.Second)
	before := p.requests
	took, waits := p.scrolling(speed)
	scrolling := time.Duration(1000 * 54 / float64(speed) * float64(time.Second))
	t.Logf("%d px/s: %.1f s to the end (%.1f s of scrolling), %d frames with the end in view, %d requests, at most %d at once",
		speed, took.Seconds(), scrolling.Seconds(), waits, p.requests-before, p.mostFlying)
	if waits > 0 {
		t.Errorf("the end of what was loaded was in view for %d frames", waits)
	}
	if took > scrolling+time.Second {
		t.Errorf("it took %.1f s to scroll to the end, for %.1f s of scrolling", took.Seconds(), scrolling.Seconds())
	}
	if p.requests-before > 6 {
		t.Errorf("%d requests for the rest of a thousand songs", p.requests-before)
	}
}

// likedOf makes the favorite songs n of them, of which the source gives a
// hundred for the first part and parts at once for those after it, and, if
// said, says how many there are in all.
func (p *pace) likedOf(n, parts int, said bool) {
	fake(p.a).liked = func(offset int) (*music.Page, error) {
		p.answer("liked")
		got := 1
		if offset > 0 {
			got = parts
		}
		to := min(offset+100*got, n)
		page := &music.Page{More: to < n}
		if said {
			page.Total = n
		}
		for i := offset; i < to; i++ {
			page.Tracks = append(page.Tracks, song(fmt.Sprint("s", i), fmt.Sprint("Song ", i), "Someone"))
		}
		return page, nil
	}
}

// scrollingLiked is scrolling for the favorite songs: it counts the frames
// in which something past the songs loaded, blocks for those to come or the
// foot, was in view, and those in which the foot said "Loading…".
func (p *pace) scrollingLiked(speed float32) (waits, loading int) {
	p.t.Helper()
	start := p.clock()
	for p.clock()-start < 10*time.Minute {
		p.tt.Scroll(700, 400, 0, speed*0.048)
		p.step(48 * time.Millisecond)
		_, last := p.a.trackList.Visible()
		// Past the end there's only the foot, with nothing in it.
		if last > len(p.a.likedList) && p.a.likedPaged.more {
			waits++
		}
		if _, ok := p.tt.Find("Loading…"); ok {
			loading++
		}
		if !p.a.likedPaged.more && !p.a.likedPaged.loading && p.a.trackList.AtEnd() {
			return waits, loading
		}
	}
	p.t.Fatal("never came to the end")
	return 0, 0
}

// Scrolling through a thousand favorite songs in a hard fling shows nothing
// past the songs loaded where the source says how many there are, as it is
// for the library's songs: the next parts are asked for before the end is in
// view, and blocks of a row's height stand in for those to come, with no
// foot saying they are loading. Where it doesn't say, the foot does, as it
// did; one part to a request and no total is how it was before, which a
// slower source and a gentler fling than the library's, as the last two are,
// show the difference of.
func TestLikedScrollWaitsForNothing(t *testing.T) {
	for _, c := range []struct {
		parts   int
		said    bool
		latency time.Duration
		speed   float32
	}{
		{3, true, 400 * time.Millisecond, 6000},
		{1, false, time.Second, 4000},
		{3, true, time.Second, 4000},
	} {
		p := newPace(t, c.latency)
		p.likedOf(1000, c.parts, c.said)
		p.a.startLibrary()
		p.idle(2 * time.Second)
		p.a.open(route{page: pageLiked})
		p.idle(time.Second)
		waits, loading := p.scrollingLiked(c.speed)
		t.Logf("%d parts to a request, total said %v, %d ms, %.0f px/s: %d frames with something past the songs in view, %d with the foot loading, %d requests in all",
			c.parts, c.said, c.latency.Milliseconds(), c.speed, waits, loading, p.asked["liked"])
		if got := len(p.a.likedList); got != 1000 {
			t.Errorf("%d songs listed at the end, want 1000", got)
		}
		if c.said && loading > 0 {
			t.Errorf("the foot said loading for %d frames, where the blocks stand in", loading)
		}
		if c.said && c.latency < time.Second && waits > 0 {
			t.Errorf("something past the songs loaded was in view for %d frames", waits)
		}
		if c.said && p.asked["liked"] > 5 {
			t.Errorf("%d requests for a thousand favorite songs, want the first part and three of three parts", p.asked["liked"])
		}
	}
}
