package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mygo-music/internal/music"
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
	p.a.startLibrary(p.a.library)
	p.idle(2 * time.Second)
	p.a.open(route{page: pageBrowse, src: "fake", id: music.PageLibrarySongs})
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
