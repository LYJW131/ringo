package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/lyrics"
)

// The lyrics follow the song as a column with weight: as a line gives way
// to the next, the offset goes to the new line at once, and each line is
// held back by what it has still to go, on a spring of its own, setting off
// a moment after the line above it. So the line sung leads, and the lines
// under it follow one by one, the room between them opening and closing as
// they go.

const (
	// lyricFocusY is how far down the pane the line sung sits.
	lyricFocusY = 0.3
	// lyricBreath is the room the line sung has, above and below, more
	// than the others have between them.
	lyricBreath = 10
	// lyricStagger is how long after the line above it a line sets off,
	// for as many lines as lyricStaggerMost: past those, they go together.
	lyricStagger     = 45 * time.Millisecond
	lyricStaggerMost = 8
	// lyricOmega is the springs' ω: a line or two away, a line comes within
	// half a pixel in about 700 ms.
	lyricOmega = 10.5
	// lyricBounce is the springs' ζ over a line or two, a little short of
	// critical, so that a line arrives a pixel past its place and settles
	// back; lyricFar is the distance from which they are critical, and
	// don't overshoot: a seek across the song would by a line's height.
	lyricBounce = 0.82
	lyricNear   = 140
	lyricFar    = 520
	// lyricBrowse is how long the lyrics stay where the listener scrolled
	// them, sharp to read, before they follow the song again.
	lyricBrowse = 3 * time.Second

	// breakMin is how long a break in the singing shows dots: one long
	// enough for them to come, breathe a few times and go.
	breakMin = 7 * time.Second
	// breakLead is how long after a line ends its break begins: the line
	// stays lit a moment, as its last note rings.
	breakLead = 600 * time.Millisecond
	// dotsHeight is the room a break's dots take between two lines.
	dotsHeight = 40
)

// lyricFocus is what the lyrics are on: the line sung, or the break after
// it, where its dots are.
type lyricFocus struct {
	line int  // the line sung, or the one the break follows; -1 before the first
	rest bool // in a break
}

// focusAt returns what the lyrics are on at pos, and how long until that
// changes, 0 for never.
func focusAt(lines []lyrics.Line, pos time.Duration) (lyricFocus, time.Duration) {
	cur := lyrics.Index(lines, pos)
	if cur+1 >= len(lines) {
		return lyricFocus{line: cur}, 0
	}
	from, to, ok := breakAfter(lines, cur)
	switch {
	case ok && pos >= from:
		return lyricFocus{line: cur, rest: true}, to - pos
	case ok:
		return lyricFocus{line: cur}, from - pos
	}
	return lyricFocus{line: cur}, lines[cur+1].At - pos
}

// breakAfter returns when the break after line i, or before the first for
// -1, begins and ends, and whether there is one.
func breakAfter(lines []lyrics.Line, i int) (from, to time.Duration, ok bool) {
	if i+1 >= len(lines) {
		return 0, 0, false
	}
	to = lines[i+1].At
	if i >= 0 {
		from = lines[i].End + breakLead
	}
	return from, to, to-from >= breakMin
}

// anchor is the line the focus is on, or the one after its break: the line
// that leads the others.
func (f lyricFocus) anchor() int {
	if f.rest {
		return f.line + 1
	}
	return f.line
}

// before reports whether f comes before g in the song.
func (f lyricFocus) before(g lyricFocus) bool {
	return f.line < g.line || f.line == g.line && !f.rest && g.rest
}

// away is how many lines line i is from the focus, 0 for the line sung: the
// lines on either side of a break are both one away.
func (f lyricFocus) away(i int) int {
	switch {
	case i > f.line:
		return i - f.line
	case f.rest:
		return f.line - i + 1
	}
	return f.line - i
}

// rest is how far below its place in the column line i rests with the
// focus on f: the line sung has room above and below it, and a break's
// dots theirs.
func (f lyricFocus) restOf(i int) float32 {
	switch {
	case i < f.anchor():
		return 0
	case f.rest:
		return 2*lyricBreath + dotsHeight
	case i == f.line:
		return lyricBreath
	}
	return 2 * lyricBreath
}

// rowSpring holds a line back from where it rests, by x DIPs t after start:
// e^(−ζωt)·(a·cos ω't + b·sin ω't), ω' = ω√(1−ζ²).
type rowSpring struct {
	on    bool
	a, b  float64
	zeta  float64
	start time.Time
	// A move it has yet to set off on, at at: until then it goes on as it
	// was, hold further from where it will rest.
	wait bool
	hold float64
	at   time.Time
}

// raw returns the spring's x at now, and how fast it changes, in DIPs a
// second.
func (r *rowSpring) raw(now time.Time) (x, v float64) {
	t := max(now.Sub(r.start).Seconds(), 0)
	damp := r.zeta * lyricOmega
	w := lyricOmega * math.Sqrt(1-r.zeta*r.zeta)
	e := math.Exp(-damp * t)
	sin, cos := math.Sincos(w * t)
	return e * (r.a*cos + r.b*sin), e * ((r.b*w-damp*r.a)*cos - (r.a*w+damp*r.b)*sin)
}

// seed sets the spring off at at, x from rest and moving at v.
func (r *rowSpring) seed(at time.Time, x, v float64) {
	z := lyricBounce + (0.999-lyricBounce)*float64(smoothstep(lyricNear, lyricFar, float32(math.Abs(x))))
	r.on, r.zeta, r.start = true, z, at
	r.a, r.b = x, (v+z*lyricOmega*x)/(lyricOmega*math.Sqrt(1-z*z))
}

// value returns how far from rest the line shows at now, and how fast that
// changes; at rest, the spring is let go.
func (r *rowSpring) value(now time.Time) (x, v float64) {
	if !r.on {
		return 0, 0
	}
	if r.wait && !now.Before(r.at) {
		// It sets off from where it had got to when its time came.
		x, v = r.raw(r.at)
		r.seed(r.at, x+r.hold, v)
		r.wait, r.hold = false, 0
	}
	x, v = r.raw(now)
	if r.wait {
		return x + r.hold, v
	}
	if math.Abs(x) < 0.3 && math.Abs(v) < 12 {
		*r = rowSpring{}
		return 0, 0
	}
	return x, v
}

// shift moves where the line rests d DIPs down, at now: it stays where it
// shows, and sets off for there after delay.
func (r *rowSpring) shift(now time.Time, d float64, delay time.Duration) {
	if d == 0 && !r.on {
		return
	}
	x, v := r.value(now)
	switch {
	case r.wait:
		// Not yet off on the last move: it goes all the way in one.
		r.hold -= d
	case delay <= 0:
		r.seed(now, x-d, v)
	default:
		if !r.on {
			*r = rowSpring{on: true, start: now}
		}
		r.wait, r.hold, r.at = true, -d, now.Add(delay)
	}
}

// lyricFlow moves the lines to the focus: the pane's offset, set at once,
// and a spring for each line.
type lyricFlow struct {
	rows []rowSpring
	// focus is what the lines rest for, once seen.
	focus lyricFocus
	seen  bool
	// next is the focus to go to with the offset that brings it into
	// place, set in the next frame: the layout of the frame that chose
	// them takes the offset too, its lines built for the last.
	next struct {
		on    bool
		y     float32
		focus lyricFocus
	}
	set    float32 // the offset it set
	height float32 // how tall the pane was as the focus was brought into place
	moving bool
	// jumped says this frame set the offset: the lines' places are still
	// the last frame's, before it.
	jumped bool
	// The listener scrolled: until browse, the lines stay put. barUntil is
	// the scroll bar's time that said so.
	browse   time.Time
	barUntil time.Time
}

// to has the lines go to focus, the pane scrolled to y, from the next
// frame.
func (f *lyricFlow) to(c *ui.Context, focus lyricFocus, y float32) {
	f.next.on, f.next.y, f.next.focus = true, y, focus
	c.AnimationFrame()
}

// step takes the flow into this frame, for n lines: it sets the offset it
// was sent to, and asks for the next frame while a line is on its way. The
// listener scrolling takes the lines from where they show.
func (f *lyricFlow) step(c *ui.Context, s *ui.ScrollState, n int) {
	now := c.Now()
	f.jumped = false
	if len(f.rows) != n {
		f.rows = make([]rowSpring, n)
	}
	if f.next.on {
		f.next.on = false
		// The first time, and with less motion, the lines are there at once.
		still := !f.seen || c.Preferences().ReduceMotion
		by, from, to := f.next.y-s.Y, f.focus, f.next.focus
		for i := range f.rows {
			if still {
				f.rows[i] = rowSpring{}
				continue
			}
			wave := i - to.anchor()
			if to.before(from) {
				wave = -wave // going back, the wave runs up the column
			}
			delay := time.Duration(min(max(wave, 0), lyricStaggerMost)) * lyricStagger
			f.rows[i].shift(now, float64(to.restOf(i)-from.restOf(i)-by), delay)
		}
		s.Y, f.set, f.focus, f.seen = f.next.y, f.next.y, to, true
		f.jumped, f.moving = true, !still
	}
	if !f.moving {
		return
	}
	if s.Y != f.set {
		// The listener took over: the offset takes what the leading line
		// had still to go, which stops where it shows, and the others
		// settle around it.
		lead := min(max(f.focus.anchor(), 0), n-1)
		x0, v0 := f.rows[lead].value(now)
		for i := range f.rows {
			if x, v := f.rows[i].value(now); f.rows[i].on {
				f.rows[i] = rowSpring{}
				f.rows[i].seed(now, x-x0, v-v0)
			}
		}
		s.Y = max(0, min(s.Y-float32(x0), s.MaxY))
		f.set = s.Y
	}
	f.moving = false
	for i := range f.rows {
		if f.rows[i].value(now); f.rows[i].on {
			f.moving = true
		}
	}
	if f.moving {
		c.AnimationFrame()
	}
}

// offset is how far below its place in the column line i shows at now.
func (f *lyricFlow) offset(i int, now time.Time) float32 {
	x, _ := f.rows[i].value(now)
	return f.focus.restOf(i) + float32(x)
}

// browsing reports whether the listener scrolled the lyrics a moment ago,
// and for how long they stay put yet. until is till when their scroll bar
// shows, which the listener scrolling them sets.
func (f *lyricFlow) browsing(now, until time.Time) (bool, time.Duration) {
	if until != f.barUntil {
		f.barUntil, f.browse = until, until.Add(lyricBrowse-barLinger)
	}
	left := f.browse.Sub(now)
	return left > 0, left
}
