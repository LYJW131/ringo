package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// A shelf scrolls across by itself, not as a scroll container of MyGo's: a
// container around the page's own scrolling takes a touchpad's scroll
// whenever it has the least across in it, which two fingers going down a
// page always have, and the page then stutters as the shelves sway. So the
// page decides, once a gesture has begun, which way it goes (touchGesture),
// and a shelf that is not the way takes none of it.

// The ways a touchpad gesture is told to go: how long a pause ends one, how
// far it must go to be told, and how much further across than down it must
// have gone to be across.
const (
	gestureRest   = 100 * time.Millisecond
	gestureLock   = 3
	gestureAcross = 2
)

type gestureWay uint8

const (
	wayUnknown gestureWay = iota
	wayAcross
	wayDown
)

// touchGesture is the way the touchpad's scrolling goes, one for the whole
// window: a gesture that moves on to another shelf goes the same way.
type touchGesture struct {
	last     time.Time
	momentum bool
	way      gestureWay
	// dx and dy are how far it has gone while its way is not yet known.
	dx, dy float32
}

// feed takes a scroll of a touchpad, and returns the way its gesture goes,
// and how far across to scroll for it: the gesture's first few points all
// at once when they decide it goes across.
func (g *touchGesture) feed(ev ui.InputEvent) (gestureWay, float32) {
	// The fingers coming down again on momentum begin another gesture, as
	// a pause does, but not the pause between the momentum's own events.
	rested := ev.Time.Sub(g.last) > gestureRest && !(ev.Momentum && g.way != wayUnknown)
	if g.last.IsZero() || rested || g.momentum && !ev.Momentum {
		*g = touchGesture{}
	}
	g.last, g.momentum = ev.Time, ev.Momentum
	switch g.way {
	case wayAcross:
		return wayAcross, ev.DX
	case wayDown:
		return wayDown, 0
	}
	g.dx += ev.DX
	g.dy += ev.DY
	if math.Hypot(float64(g.dx), float64(g.dy)) < gestureLock {
		return wayUnknown, 0
	}
	if abs(g.dx) > gestureAcross*abs(g.dy) {
		g.way = wayAcross
		return wayAcross, g.dx
	}
	g.way = wayDown
	return wayDown, 0
}

func abs(x float32) float32 { return max(x, -x) }

// wheelAcross returns how far a mouse wheel's notch scrolls a shelf across:
// with Shift held, as the wheel does everywhere, or if the wheel goes across
// itself. An ordinary notch goes down the page.
func wheelAcross(ev ui.InputEvent) (float32, bool) {
	switch {
	case ev.Mods&ui.Shift != 0 && ev.DX == 0:
		return ev.DY, ev.DY != 0
	case ev.DX != 0 && abs(ev.DX) > abs(ev.DY):
		return ev.DX, true
	}
	return 0, false
}

// shelfScroll is how far a shelf is scrolled across.
type shelfScroll struct {
	// X is how far its items are scrolled, as it shows them, and MaxX as far
	// as they go.
	X, MaxX float32
	// to is where it is going: at once as the fingers go, or gliding (smooth)
	// as an arrow or a wheel's notch takes it.
	to     float32
	smooth bool
	// focused is the item that had the focus, and keyed that a key, not the
	// pointer, last put it there.
	focused int
	keyed   bool
}

func (s *shelfScroll) clamp(x float32) float32 { return max(0, min(x, s.MaxX)) }

// scrollBy scrolls dx across at once, from where it shows.
func (s *shelfScroll) scrollBy(dx float32) {
	if s.smooth {
		s.to, s.smooth = s.X, false
	}
	s.to = s.clamp(s.to + dx)
}

// glideTo scrolls to x, gliding.
func (s *shelfScroll) glideTo(x float32) { s.to, s.smooth = s.clamp(x), true }

// place returns where the shelf shows its items this frame, which e, the
// row of them, animates to.
func (s *shelfScroll) place(e ui.Element) float32 {
	s.to = s.clamp(s.to)
	if !s.smooth {
		s.X = e.Jump("x", s.to)
		return s.X
	}
	s.X = e.AnimateTo("x", s.to, ui.Smooth)
	// The spring rests a hair short; half a point is where the eye stops
	// telling.
	if abs(s.X-s.to) < 0.5 {
		s.X, s.smooth = s.to, false
	}
	return s.X
}

// reveal glides the shelf as little as shows the item whose place is from
// from to to along its items, as far as its width, inner, shows of them.
func (s *shelfScroll) reveal(from, to, inner float32) {
	switch {
	case from < s.to:
		s.glideTo(from)
	case to > s.to+inner:
		s.glideTo(to - inner)
	}
}

// scrollInput is the page's first look at its scrolling, before it scrolls
// anything: it has the shelf under the pointer, if there is one, take what
// goes across, and leaves what goes down to the page.
func (a *app) scrollInput(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll {
		return false
	}
	st := a.overShelf
	a.overShelf = nil
	if !ev.Precise {
		dx, ok := wheelAcross(ev)
		if !ok || st == nil {
			return false
		}
		st.glideTo(st.to + dx)
		return true
	}
	way, dx := a.touch.feed(ev)
	switch {
	case way == wayAcross && st != nil:
		st.scrollBy(dx)
		return true
	case way == wayAcross:
		// Across where nothing scrolls that way: not the page's either.
		return true
	}
	return false
}
