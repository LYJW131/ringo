package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Scroll bars show while what they're of is scrolled, and a moment after,
// then fade, as macOS's do. MyGo shows a container's while the pointer is
// over it, which here is nearly always: the sidebar's bar stood there for as
// long as the pointer was on the sidebar, scrolled or not. So each container
// is told to hide its bars (Scrollbars) or to keep them (KeepScrollbars) by
// whether the listener has just scrolled it, which the app sees as its place
// changing between two frames.

const (
	// barLinger is how long a bar stays after the last scroll.
	barLinger = 900 * time.Millisecond
	// barFade is how long it takes to go.
	barFade = 250 * time.Millisecond
	// barEdge is how near the edge its bar is on the pointer keeps one
	// that shows, to be pressed and dragged.
	barEdge = 16
)

// barPlace is where a container is scrolled to, as far as the app can tell.
// A Scroll says its offset (x, y). A List doesn't: it says the rows it shows
// (first, last, none while last is below first), and the page's start mark,
// in its first row, says where that is while it's built (mark, marked).
type barPlace struct {
	// of is what the container shows. Another's place isn't this one
	// scrolled: a page opened, rows that came in, a window resized.
	of          any
	x, y        float32
	first, last int
	mark        float32
	marked      bool
}

// scrolled reports whether now is from scrolled away, both of the same
// thing. A place the app can't tell (no rows shown, no mark) says nothing.
func (from barPlace) scrolled(now barPlace) bool {
	if from.of != now.of {
		return false
	}
	far := func(a, b float32) bool { return a-b > 1 || b-a > 1 }
	if far(from.x, now.x) || far(from.y, now.y) {
		return true
	}
	if from.marked && now.marked && far(from.mark, now.mark) {
		return true
	}
	return from.first <= from.last && now.first <= now.last && (from.first != now.first || from.last != now.last)
}

// barCalm is how many frames after a container shows something else its
// place says nothing: the app puts it where it was, or where the song
// playing is, and that lands a frame after it's asked for.
const barCalm = 2

// bar is one container's: where it was in the last frame, and till when
// its bars show.
type bar struct {
	place barPlace
	seen  bool
	calm  int
	until time.Time
}

// barSet are the bars of a window. They fade together, by the theme's
// color for scroll bars, which is the window's: alpha is how much of it the
// frame being built shows, and next what its containers ask of the next.
type barSet struct {
	each        map[string]*bar
	alpha, next float32
}

// frame starts a frame of the window: its theme's scroll bars as faint as
// the containers of the last asked.
func (s *barSet) frame(t *ui.Theme) {
	s.alpha, s.next = s.next, 0
	t.Scrollbar = t.Scrollbar.Alpha(s.alpha)
}

func (s *barSet) bar(name string) *bar {
	if s.each == nil {
		s.each = map[string]*bar{}
	}
	b := s.each[name]
	if b == nil {
		b = &bar{}
		s.each[name] = b
	}
	return b
}

// note takes where the container name is, and has its bars show if the
// listener scrolled it there since the last frame.
func (s *barSet) note(c *ui.Context, name string, place barPlace) {
	b := s.bar(name)
	switch {
	case b.place.of != place.of:
		b.calm = barCalm
	case b.calm > 0:
		b.calm--
	case b.seen && b.place.scrolled(place):
		b.until = c.Now().Add(barLinger)
	}
	b.place, b.seen = place, true
}

// settle takes where the app itself put the container, as lyrics that
// follow a song are, which is not the listener scrolling.
func (s *barSet) settle(name string, place barPlace) {
	b := s.bar(name)
	b.place, b.seen = place, true
}

// show has el, the container name, show its bars or not, and asks for the
// frames in which they go.
func (s *barSet) show(c *ui.Context, el ui.Element, name string) {
	b := s.bar(name)
	now := c.Now()
	left := b.until.Sub(now) + barFade
	if left <= 0 {
		el.Scrollbars(false)
		return
	}
	// The pointer on a bar keeps it, and so a thumb can be dragged, and held.
	if x, _, over := el.PointerPosition(); over && x >= el.Bounds().W-barEdge {
		b.until = now.Add(barLinger)
		left = barLinger + barFade
	}
	el.KeepScrollbars(true)
	k := float32(1)
	if left < barFade {
		k = float32(left) / float32(barFade)
		c.AnimationFrame()
	} else {
		c.After(left - barFade)
	}
	s.next = max(s.next, k)
	if s.alpha != k {
		// The theme is this frame's: the next has what was asked.
		c.AnimationFrame()
	}
}

// quiet is note and show, for a container the app doesn't scroll itself.
func (s *barSet) quiet(c *ui.Context, el ui.Element, name string, place barPlace) {
	s.note(c, name, place)
	s.show(c, el, name)
}

// pageOf is what a page's container shows: the page, how many rows of it
// there are, and the window's size, which changes the rows a list shows.
type pageOf struct {
	r    route
	rows int
	w, h float32
}

// pageBars are the bars of the page's container: a Scroll that keeps its
// place in pageScroll (rows below 0), or a List of rows in trackList.
func (a *app) pageBars(c *ui.Context, el ui.Element, rows int) {
	w, h := c.Size()
	place := barPlace{of: pageOf{a.route, rows, w, h}, x: a.pageScroll.X, y: a.pageScroll.Y, last: -1}
	if rows >= 0 {
		place.first, place.last = a.trackList.Visible()
		// The mark is built with the first row, after this in a frame:
		// what's here is the last frame's, as the rows shown are.
		place.mark, place.marked = a.markY, a.markBuilt
		a.markBuilt = false
	}
	a.bars.quiet(c, el, "page", place)
}
