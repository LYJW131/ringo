package app

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"ringo/internal/lyrics"
	"ringo/internal/music"
)

// Vinyl sets the stage's lyrics around a record (ui_stage_deck.go): its
// lines are spokes of a wheel about the record's center, the one sung
// level, the others turned away above and below it, dimmer and out of
// focus the farther they are, and the wheel turns a line on as each is
// due. A word is lit in a chip of its own color as it is sung, springing
// up as it lands, and a small square hops from word to word along the
// line, turning a quarter as it goes.
//
// What more a line gets is chosen once a song, from its lyrics alone
// (planVinyl): most lines only turn in; one after a breath or a long
// pause pulses the wheel, a few of the strongest split their colors for
// an instant, and a chorus coming in does all of it, with a glow around
// the record. A word held a second or more rises and glows a letter at a
// time, and a long one swells the wheel while it lasts.
//
// Like the stage's other styles, everything follows from where playback
// is: seeking shows the frame playing would.

const (
	vinylWheel = 362.5                  // how far from the record's center a line starts
	vinylStep  = 12.7                   // degrees from a line to the next
	vinylLead  = 170 * time.Millisecond // how long before its first word a line turns in
	vinylType  = 25                     // the size of its words
)

// vinylHues are the colors of the words' chips, each word its own by what
// it says.
var vinylHues = []ui.Color{
	ui.RGB(124, 110, 252), ui.RGB(76, 225, 96), ui.RGB(252, 101, 118),
	ui.RGB(246, 197, 83), ui.RGB(253, 144, 204), ui.RGB(176, 124, 251),
}

// vinylToken is a word on the wheel, or a rest after a line's last: when
// it is sung, and until when it stays lit; where it sits in its line.
type vinylToken struct {
	text    string
	on, off time.Duration
	held    time.Duration // how long it is sung, where the source says
	fade    float32       // seconds its chip takes to go
	hue     int
	space   bool // a space follows it
	rest    bool
	last    bool // it ends its line of the lyrics
	x, w    float32
	// A word held long is emphasized: by how much it rises and glows, over
	// how many seconds, and where each of its characters starts.
	rise, glow, over float32
	chars            []float32
}

// vinylLine is a spoke of the wheel: a line of the lyrics, or a part of
// one too long for a spoke.
type vinylLine struct {
	start, end time.Duration // when its first word is sung, and its last is over
	next       time.Duration // when the spoke after it starts
	of         int           // the line of the lyrics it is of
	cont       bool          // it goes on with a line the spoke before began
	key        string        // what it says, for telling a line sung again
	chorus     bool          // its line is sung more than once
	toks       []vinylToken
}

// vinylHit is an accent on a line: the wheel's pulse, the record's glow
// and the split of the line's colors, each from 0 to 1, and whether the
// wheel draws in a little just before it.
type vinylHit struct {
	at                time.Duration
	tier              float32 // 3 a chorus coming in, 2 and 1 lesser, 0.5 one to fill a long quiet
	strength          float32
	pulse, glow, tear float32
	squeeze           bool
}

// vinylSwell is the wheel swelling over a word held long.
type vinylSwell struct {
	at, over time.Duration
	amp      float32
}

// vinylSet is a song's lyrics set on the wheel, for a stage of a size.
type vinylSet struct {
	first       *lyrics.Line // the lines it is of, by their first
	n           int
	size, avail float32
	lines       []vinylLine
	hits        []vinylHit
	swells      []vinylSwell
}

// vinylKey is what a text says, whatever its case, spaces and punctuation.
func vinylKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

func vinylHue(text string) int {
	h := uint32(2166136261)
	for _, r := range vinylKey(text) {
		h = (h ^ uint32(r)) * 16777619
	}
	return int(h % uint32(len(vinylHues)))
}

func wide(s string) bool {
	for _, r := range s {
		return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
	}
	return false
}

// grow is how AMLL has a held word's emphasis grow with its length: hardly
// at all under a second or so, then ever more slowly.
func grow(x float64) float64 {
	if x > 1 {
		return math.Sqrt(x)
	}
	return x * x * x
}

// setVinyl sets lines on the wheel, their words size high and avail wide
// at most a spoke, measure giving a text's width at a size.
func setVinyl(lines []lyrics.Line, size, avail float32, measure func(text string, size float32) float32) *vinylSet {
	set := &vinylSet{n: len(lines), size: size, avail: avail}
	if len(lines) == 0 {
		return set
	}
	set.first = &lines[0]
	const never = time.Duration(math.MaxInt64 / 2)

	counts := map[string]int{}
	for i := range lines {
		counts[vinylKey(lines[i].Text)]++
	}
	for i := range lines {
		l := &lines[i]
		next := never
		if i+1 < len(lines) {
			next = lines[i+1].At
		}
		end := min(max(l.End, l.At), next)
		// Its words: the syllables of a word spaced into words are one.
		var toks []vinylToken
		joins := false
		for _, w := range l.Words {
			text := strings.TrimSpace(w.Text)
			if text == "" {
				continue
			}
			space := strings.TrimRightFunc(w.Text, unicode.IsSpace) != w.Text
			if joins && !wide(text) {
				t := &toks[len(toks)-1]
				t.text += text
				t.space = space
			} else {
				toks = append(toks, vinylToken{text: text, on: w.Start, space: space})
			}
			joins = !space && !wide(text)
		}
		if len(toks) == 0 {
			toks = []vinylToken{{text: l.Text, on: l.At, space: true}}
		}
		for k := range toks {
			t := &toks[k]
			t.hue = vinylHue(t.text)
			t.w = measure(t.text, size)
			until := end
			if k+1 < len(toks) {
				until = toks[k+1].on
			}
			if l.Sung {
				t.held = max(0, min(until, next)-t.on)
			}
		}
		toks[len(toks)-1].last = true
		for k := range toks {
			t := &toks[k]
			secs := t.held.Seconds()
			letters := len([]rune(vinylKey(t.text)))
			if secs < 1 || !wide(t.text) && (letters < 2 || letters > 7) {
				continue
			}
			by, glow, over := 1.0, 1.0, 1.0
			if t.last {
				by, glow, over = 1.6, 1.5, 1.2
			}
			t.rise = float32(min(1.2, grow(secs/2)*0.6*by))
			t.glow = float32(min(0.8, grow(secs/3)*0.5*glow))
			t.over = float32(secs * over)
			at := float32(0)
			for _, r := range t.text {
				t.chars = append(t.chars, at)
				at += measure(string(r), size)
			}
		}
		// A pause after it is counted out in rests, as a score has them.
		if gap := next - end; next != never && gap >= 700*time.Millisecond {
			n := min(2, int((gap-250*time.Millisecond)/(450*time.Millisecond)))
			for k := range n {
				toks = append(toks, vinylToken{text: "_", on: end + 120*time.Millisecond + time.Duration(k)*450*time.Millisecond,
					hue: toks[len(toks)-1].hue, rest: true, space: true, w: measure("_", size)})
			}
		}

		// Onto as many spokes as its words need.
		var parts [][]vinylToken
		at := float32(0)
		for _, t := range toks {
			gap := size * 0.6
			if n := len(parts); n > 0 && len(parts[n-1]) > 0 && !parts[n-1][len(parts[n-1])-1].space {
				gap = size * 0.36
			}
			if len(parts) == 0 || at+gap+t.w > avail && !t.rest {
				parts = append(parts, nil)
				at, gap = 0, 0
			}
			t.x = at + gap
			at = t.x + t.w
			parts[len(parts)-1] = append(parts[len(parts)-1], t)
		}
		for k, part := range parts {
			line := vinylLine{start: part[0].on, end: end, next: next, of: i, cont: k > 0, toks: part}
			if k == 0 {
				line.start, line.key, line.chorus = l.At, vinylKey(l.Text), counts[vinylKey(l.Text)] > 1
			}
			if k+1 < len(parts) {
				line.end, line.next = parts[k+1][0].on, parts[k+1][0].on
			}
			set.lines = append(set.lines, line)
		}
	}

	// A word stays lit until the next is sung; a line's last, until the
	// wheel turns it away, or half a second after the line is over.
	var all []*vinylToken
	for i := range set.lines {
		l := &set.lines[i]
		for k := range l.toks {
			all = append(all, &l.toks[k])
		}
		t := &l.toks[len(l.toks)-1]
		t.off = min(l.next-vinylLead, max(l.end, t.on)+500*time.Millisecond)
		t.fade = 0.15
	}
	for i, t := range all {
		if t.fade == 0 {
			t.off, t.fade = all[i+1].on, 0.28
		} else if i+1 < len(all) {
			t.off = min(t.off, all[i+1].on)
		}
	}
	set.hits, set.swells = planVinyl(set.lines)
	return set
}

// near says a and b say nearly the same: a chorus whose words change a
// little is the chorus still.
func near(a, b string) bool {
	if a == b {
		return true
	}
	x, y := []rune(a), []rune(b)
	if len(x) < 4 || len(y) < 4 {
		return false
	}
	prev := make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(x); i++ {
		cur := make([]int, len(y)+1)
		cur[0] = i
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return float64(prev[len(y)]) < 0.25*float64(max(len(x), len(y)))
}

func quantile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[min(len(s)-1, int(float64(len(s))*q))]
}

// planVinyl chooses the accents of a song's lines, and where its held
// words swell the wheel. The lyrics say when: a line's first word. How
// much is by the line: one a chorus comes in with gets everything, twice
// at most in a minute and a half; of the rest, the strongest quarter, by
// the breath before them, their held words and their length, pulse the
// wheel and may split their colors, the next few only pulse it lightly,
// and the others do nothing, as most lines should. A chorus's line sung
// again does what it did the first time, and a quiet of more than 24
// seconds gets one light accent in its middle.
func planVinyl(lines []vinylLine) ([]vinylHit, []vinylSwell) {
	type cand struct {
		l                *vinylLine
		at               time.Duration
		gap              float64
		entrance, chorus bool
		group            int
		strength         float64
		tier             float32
	}
	var heads []*vinylLine
	for i := range lines {
		if !lines[i].cont {
			heads = append(heads, &lines[i])
		}
	}
	// The words of a line of the lyrics, over all its spokes.
	words := func(h *vinylLine) (n int, longest time.Duration) {
		for i := range lines {
			if lines[i].of != h.of {
				continue
			}
			for _, t := range lines[i].toks {
				if !t.rest {
					n++
					longest = max(longest, t.held)
				}
			}
		}
		return
	}
	group := make([]int, len(heads))
	sung := map[int]int{}
	for i, h := range heads {
		group[i] = i
		for j := range i {
			if near(heads[j].key, h.key) {
				group[i] = group[j]
				break
			}
		}
		sung[group[i]]++
	}
	cands := make([]*cand, len(heads))
	strengths := make([]float64, len(heads))
	for i, h := range heads {
		c := &cand{l: h, at: h.toks[0].on, gap: 9, group: group[i], chorus: sung[group[i]] > 1}
		if i > 0 {
			c.gap = (h.start - min(heads[i-1].lineEnd(lines), h.start)).Seconds()
		}
		run := 0
		for j := i - 1; j >= 0 && sung[group[j]] < 2; j-- {
			run++
		}
		c.entrance = c.chorus && (i == 0 || sung[group[i-1]] < 2 && run >= 2)
		n, longest := words(h)
		c.strength = 0.6*min(1, c.gap/2.5) + 0.25*min(1, longest.Seconds()/2) + 0.15*min(1, float64(n)/12)
		cands[i], strengths[i] = c, c.strength
	}
	q75, q60 := quantile(strengths, 0.75), quantile(strengths, 0.6)
	first := map[int]float32{}
	lastHero := -time.Hour
	for _, c := range cands {
		switch {
		case c.entrance && c.at-lastHero >= 45*time.Second:
			c.tier, lastHero = 3, c.at
		case c.strength >= q75:
			c.tier = 2
		case c.strength >= q60:
			c.tier = 1
		}
		if was, ok := first[c.group]; !ok {
			first[c.group] = c.tier
		} else if c.chorus && c.tier < 3 {
			c.tier = max(c.tier, min(2, was))
		}
	}
	// The greater accents no closer than 3.2 seconds to each other, the
	// stronger kept.
	byRank := append([]*cand(nil), cands...)
	sort.SliceStable(byRank, func(i, j int) bool {
		if byRank[i].tier != byRank[j].tier {
			return byRank[i].tier > byRank[j].tier
		}
		return byRank[i].strength > byRank[j].strength
	})
	var kept []*cand
	for _, c := range byRank {
		if c.tier == 0 {
			continue
		}
		crowded := false
		for _, k := range kept {
			apart := 450 * time.Millisecond
			if k.tier >= 2 && c.tier >= 2 {
				apart = 3200 * time.Millisecond
			}
			if d := k.at - c.at; d < apart && d > -apart {
				crowded = true
			}
		}
		if !crowded {
			kept = append(kept, c)
		}
	}
	byTime := func() { sort.Slice(kept, func(i, j int) bool { return kept[i].at < kept[j].at }) }
	byTime()
	for range 20 {
		if len(heads) == 0 {
			break
		}
		// The longest quiet, from before the first line to after the last.
		from, to := heads[0].start-time.Second, heads[len(heads)-1].lineEnd(lines)+time.Second
		edges := []time.Duration{from}
		for _, k := range kept {
			edges = append(edges, k.at)
		}
		edges = append(edges, to)
		at, quiet := 0, time.Duration(0)
		for i := 0; i+1 < len(edges); i++ {
			if d := edges[i+1] - edges[i]; d > quiet {
				at, quiet = i, d
			}
		}
		if quiet <= 24*time.Second {
			break
		}
		var best *cand
		for _, c := range cands {
			if c.tier == 0 && c.at >= edges[at]+5*time.Second && c.at <= edges[at+1]-5*time.Second && (best == nil || c.strength > best.strength) {
				best = c
			}
		}
		if best == nil {
			break
		}
		best.tier = 0.5
		kept = append(kept, best)
		byTime()
	}

	var heroes int
	for _, k := range kept {
		if k.tier == 3 {
			heroes++
		}
	}
	hits := make([]vinylHit, 0, len(kept))
	hero, lastTear := 0, -time.Hour
	for _, k := range kept {
		gain := float32(0.75) // a verse's are lesser than a chorus's
		if k.chorus {
			gain = 1
		}
		h := vinylHit{at: k.at, tier: k.tier, strength: float32(k.strength)}
		switch k.tier {
		case 3:
			// The first and the last chorus in full, those between less.
			amp := float32(0.7)
			if hero == 0 || hero == heroes-1 {
				amp = 1
			}
			hero++
			h.pulse, h.glow, h.tear, lastTear = amp, amp, amp, k.at
			// Drawn in a little first, unless a word just sprang up there.
			h.squeeze = true
			for i := range lines {
				for _, t := range lines[i].toks {
					if t.on < k.at && t.on > k.at-200*time.Millisecond {
						h.squeeze = false
					}
				}
			}
		case 2:
			h.pulse = 0.8 * gain
			if k.at-lastTear >= 5*time.Second {
				h.tear, lastTear = 0.5, k.at
			}
		case 1:
			h.pulse = 0.45 * gain
		default:
			h.pulse = 0.25 * gain
		}
		hits = append(hits, h)
	}

	var swells []vinylSwell
	for i := range lines {
		for _, t := range lines[i].toks {
			if t.rest || t.held < time.Second || t.held < 1500*time.Millisecond && !t.last {
				continue
			}
			swells = append(swells, vinylSwell{at: t.on, over: min(t.held, 2400*time.Millisecond),
				amp: float32(min(1, max(0.3, grow(t.held.Seconds()/2)*0.9)))})
		}
	}
	return hits, swells
}

// lineEnd is when the line of the lyrics h begins is over, on whichever
// spoke it ends.
func (h *vinylLine) lineEnd(lines []vinylLine) time.Duration {
	end := h.end
	for i := range lines {
		if lines[i].of == h.of {
			end = max(end, lines[i].end)
		}
	}
	return end
}

// wheelAt is the spoke that is the line at pos, -1 before the first, and
// where the wheel is: the spoke it has level, between two while it turns.
// It eases to each spoke as that one's turn comes, from wherever the one
// before left it.
func (s *vinylSet) wheelAt(pos time.Duration) (active int, at float32) {
	active = sort.Search(len(s.lines), func(i int) bool { return s.lines[i].start-vinylLead > pos }) - 1
	if active <= 0 {
		return active, 0
	}
	from := max(1, active-4)
	v := float64(from - 1)
	for k := from; k <= active; k++ {
		until := pos
		if k < active {
			until = s.lines[k+1].start - vinylLead
		}
		v = float64(k) - (float64(k)-v)*math.Exp(-10*(until-(s.lines[k].start-vinylLead)).Seconds())
	}
	return active, float32(v)
}

// vinylMotion is what the accents make of the wheel at a moment.
type vinylMotion struct {
	zoom        float32 // how much larger the wheel is, or smaller
	tear, boost float32 // how far the line's colors split, in the deck's DIPs, and how much its unsung words lift
	deck        vinylAccent
}

func easeInOut(t float32) float32 {
	t = clamp01(t)
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - (2-2*t)*(2-2*t)/2
}

// motionAt is the wheel's motion at pos.
func (s *vinylSet) motionAt(pos time.Duration) vinylMotion {
	var m vinylMotion
	var pulse, glow, swell, squeeze float32
	for _, h := range s.hits {
		dt := float32((pos - h.at).Seconds())
		switch {
		case dt < -0.08 || dt > 3:
			continue
		case dt < 0:
			if h.squeeze {
				squeeze = 0.006 * easeInOut(1+dt/0.08)
			}
			continue
		}
		pulse = max(pulse, h.pulse*float32(math.Exp(float64(-dt/0.15))))
		glow = max(glow, h.glow*float32(math.Exp(float64(-dt/0.35))))
		if h.tear > 0 && dt < 0.3 {
			m.tear = 4.25 * h.tear * float32(math.Exp(float64(-dt/0.05)))
			m.boost = min(1, h.tear) * float32(math.Exp(float64(-dt/0.06)))
		}
	}
	for _, w := range s.swells {
		u, over := float32((pos - w.at).Seconds()), float32(w.over.Seconds())
		if u < 0 || u > over+0.45 {
			continue
		}
		env := float32(1)
		switch {
		case u < over*0.4:
			env = easeInOut(u / (over * 0.4))
		case u > over:
			env = 1 - easeInOut((u-over)/0.45)
		}
		swell = max(swell, env*w.amp)
	}
	// A pulse and a swell do not add up: the greater shows.
	m.zoom = max(0.0118*pulse, 0.0075*swell) - squeeze
	m.deck = vinylAccent{pulse: pulse, glow: max(float32(math.Pow(float64(glow), 1.4)), 0.28*swell)}
	return m
}

// vinylSquare is the square over a line at a moment: where its middle is
// along the line and how far it has hopped up, in the line's own sizes;
// how it is turned and squashed, its color and how much of it shows.
type vinylSquare struct {
	x, up, turn, sx, sy, alpha float32
	color                      ui.Color
}

// squareAt is the square over l at pos, unit DIPs to one of the deck's:
// waiting over the first word, a little flat; then on each word as it
// lands, and off to the next in the last three tenths of a second before
// that one is sung, in an arc the higher the farther it goes, a quarter
// turn on the way; and after the last, tipping over as it goes out.
func (l *vinylLine) squareAt(pos time.Duration, unit float32) vinylSquare {
	toks := l.toks
	mid := func(i int) float32 { return toks[i].x + toks[i].w/2 }
	hue := func(i int) ui.Color { return vinylHues[toks[i].hue] }
	if pos < toks[0].on {
		return vinylSquare{x: mid(0), up: -2 * unit, sx: 12.5 / 12, sy: 10.0 / 12, alpha: 1, color: hue(0)}
	}
	j := 0
	for j+1 < len(toks) && toks[j+1].on <= pos {
		j++
	}
	if j+1 < len(toks) {
		leaves := max(toks[j].on, toks[j+1].on-300*time.Millisecond)
		if pos >= leaves {
			p := clamp01(float32((pos - leaves).Seconds() / max(0.01, (toks[j+1].on-leaves).Seconds())))
			far := abs32(mid(j+1)-mid(j)) / unit
			e := easeInOut(p)
			return vinylSquare{
				x: lerp(mid(j), mid(j+1), e), up: min(25, 12+0.18*far) * unit * 4 * p * (1 - p), turn: 90 * e,
				sx: 1, sy: 1, alpha: 1, color: hue(j).Mix(hue(j+1), clamp01((p-0.15)/0.7)),
			}
		}
	}
	since := float32((pos - toks[j].on).Seconds())
	sq := vinylSquare{x: mid(j), up: 3.5 * unit * float32(math.Exp(float64(-since/0.15))), sx: 1, sy: 1, alpha: 1, color: hue(j)}
	if since < 0.034 {
		sq.sx, sq.sy = 12.5/12, 11.5/12 // landing
	}
	if j+1 == len(toks) && pos > toks[j].off {
		u := clamp01(float32((pos - toks[j].off).Seconds()) / 0.12)
		sq.alpha, sq.turn = 1-u, 17*u
	}
	return sq
}

// How a spoke shows by how many spokes it is from the level one: fainter,
// and more out of focus, in the deck's DIPs.
var (
	vinylFaint = []float32{1, 0.75, 0.53, 0.29, 0.1, 0}
	vinylSoft  = []float32{0, 1.75, 3.25, 5, 6.5, 8}
)

func between(table []float32, d float32) float32 {
	i := min(int(d), len(table)-2)
	return lerp(table[i], table[i+1], min(1, d-float32(i)))
}

func vinylSpan(text string, size float32, lang string, c ui.Color) ui.Span {
	return ui.Span{Text: text, Size: size, Weight: 500, Font: "monospace", Lang: lang, Color: c}
}

// vinylStage shows lines on the wheel, in box, the stage, around the deck:
// with none, as while a song's lyrics are on their way, the deck alone.
func (a *app) vinylStage(c *ui.Context, box ui.Element, tr *music.Track, lines []lyrics.Line, accent ui.Color) {
	pl := a.pl
	w, h := box.LayoutBounds().W, box.LayoutBounds().H
	if w == 0 || h == 0 {
		// The stage's first frame: its size is known in the next.
		w, h = c.Size()
		h -= 160
		c.AnimationFrame()
	}
	unit := min(h/vinylBase, w/620)
	size := float32(math.Round(float64(vinylType*unit)*2) / 2)
	lang, locale := lyrics.Lang(lines), c.Locale()
	avail := w - (vinylWheel+34)*unit
	set := a.vinyl
	if set == nil || set.n != len(lines) || len(lines) > 0 && set.first != &lines[0] || set.size != size || set.avail != avail {
		set = setVinyl(lines, size, avail, func(text string, size float32) float32 {
			tw, _ := c.MeasureText(0, vinylSpan(text, size, lang, white))
			return tw
		})
		a.vinyl = set
	}

	pos, playing := a.vinylPos(c.Now()), pl.Playing()
	still := c.Preferences().ReduceMotion
	active, at := set.wheelAt(pos)
	if playing {
		switch {
		case still:
			// Nothing paints between builds: often enough for the words.
			c.After(50 * time.Millisecond)
		case active >= 1 && pos-(set.lines[active].start-vinylLead) < 700*time.Millisecond:
			c.AnimationFrame() // the wheel is turning
		case active+1 < len(set.lines):
			c.After(min(250*time.Millisecond, max(0, set.lines[active+1].start-vinylLead-pos)))
		}
	}

	dur := pl.Duration()
	press := pressingFor(accent, tr)
	// The deck is an element of its own, painted in every frame the record
	// turns in, which paint nothing else but the line sung.
	deck := func() {
		ui.Box(c.Key("deck")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			pos := a.vinylPos(p.Now())
			a.paintDeck(p, r, unit, tr, press, pos, dur, pl.Playing(), set.motionAt(pos).deck)
			if pl.Playing() && !still {
				p.AnimationFrameRate(60)
			}
		})
	}
	// The spokes fade out toward the stage's top, where the tabs are, and
	// its foot.
	black := ui.RGB(0, 0, 0)
	box.Children(func() {
		deck()
		spokes := ui.Box(c.Key("spokes")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().
			MaskLinearGradient(ui.LinearGradient{Angle: 180, Stops: stops(0.0, ui.Transparent, 0.04, ui.Transparent, 0.2, black, 0.9, black, 1.0, ui.Transparent)})
		spokes.Children(func() { a.vinylSpokes(c, set, lines, w, h, unit, size, at, active, still, lang, locale) })
	})
}

// vinylSpokes shows the spokes of set near where the wheel is, at, each
// turned to its place around the record.
func (a *app) vinylSpokes(c *ui.Context, set *vinylSet, lines []lyrics.Line, w, h, unit, size, at float32, active int, still bool, lang, locale string) {
	high := 110 * unit
	for i := max(0, int(at)-5); i < min(len(set.lines), int(at)+7); i++ {
		slot := float32(i) - at
		d := abs32(slot)
		if d > 4.6 {
			continue
		}
		l := &set.lines[i]
		under := ""
		if i == active {
			under = a.under(&lines[l.of])
		}
		e := ui.Box(c.Key(i)).Absolute().Left(0).Top(h/2-high/2).Width(w).Height(high).
			TransformOrigin(0, 0.5).Rotate(slot * vinylStep).Opacity(between(vinylFaint, d)).PassThrough()
		if soft := between(vinylSoft, d) * unit; soft > 0.3 {
			e.Blur(soft)
		}
		e.Draw(func(p *ui.Painter, r ui.Rect) {
			a.paintVinylLine(p, r, set, l, vinylLineLook{unit: unit, size: size, away: d, level: i == active, shown: i <= active,
				still: still, lang: lang, under: under, locale: locale})
		})
	}
}

// vinylLineLook is how a spoke is painted: the sizes of its stage, how far
// from level it is, whether it is the line sung and whether its turn has
// come; and what shows under it.
type vinylLineLook struct {
	unit, size, away    float32
	level, shown, still bool
	lang, locale        string
	under               string
}

// paintVinylLine paints the spoke l in r, whose left edge's middle is the
// record's center, as it is where playback is now.
func (a *app) paintVinylLine(p *ui.Painter, r ui.Rect, set *vinylSet, l *vinylLine, look vinylLineLook) {
	pos := a.vinylPos(p.Now())
	unit, size := look.unit, look.size
	// The line sung moves with the song: it is painted in every frame the
	// record is. The others change only as the wheel turns, when the view
	// is built anew, and the accents' motion is the sung line's alone.
	var m vinylMotion
	if look.level {
		m = set.motionAt(pos)
		if a.pl.Playing() && !look.still {
			p.AnimationFrameRate(60)
		}
	}
	cx, cy := r.X, r.Y+r.H/2
	paint := func() {
		on := clamp01((0.6 - look.away) / 0.2)
		x := cx + vinylWheel*unit
		_, th := p.MeasureText(0, vinylSpan("Ag", size, look.lang, white))
		top := cy - th/2

		if look.level && m.tear > 0.15 {
			// Its colors apart for an instant, as a picture torn.
			for _, t := range l.toks {
				p.RichText(x+t.x-m.tear*unit, top, 0, vinylSpan(t.text, size, look.lang, ui.RGB(0, 235, 255).Alpha(0.42)))
				p.RichText(x+t.x+m.tear*unit, top, 0, vinylSpan(t.text, size, look.lang, ui.RGB(255, 30, 40).Alpha(0.48)))
			}
		}
		for i := range l.toks {
			t := &l.toks[i]
			hue := vinylHues[t.hue]
			since := float32((pos - t.on).Seconds())
			after := float32((pos - t.off).Seconds())
			// Waiting, dim; sung, in its color on its chip, sprung up; then
			// its chip gone and its color nearly white.
			color, chip, pop := white.Alpha(0.19), float32(0), float32(0)
			if look.level {
				color = white.Alpha(0.19 + 0.26*m.boost)
			}
			if since >= 0 {
				color, chip, pop = hue, 1, float32(math.Exp(float64(-since/0.1)))
				if after >= 0 {
					chip = max(0, 1-after/t.fade)
					color = hue.Mix(ui.RGB(255, 250, 247).Mix(hue, 0.06), min(1, after/0.36))
				}
			}
			box := ui.Rect{X: x + t.x - 0.19*size, Y: cy - 0.62*size, W: t.w + 0.38*size, H: 1.2 * size}
			word := func() {
				if chip > 0.004 {
					p.FillGradient(box, ui.LinearGradient{Angle: 180, Stops: stops(
						0.0, hue.Alpha(0.4*chip), 0.07, hue.Alpha(0.32*chip), 0.5, hue.Alpha(0.27*chip), 1.0, hue.Alpha(0.24*chip))}, 0.2*size)
				}
				if t.chars != nil && since > -0.45 && since < t.over*1.45+0.1 {
					paintHeld(p, t, x+t.x, top, since, size, look.lang, color, hue)
					return
				}
				p.RichText(x+t.x, top, 0, vinylSpan(t.text, size, look.lang, color))
			}
			if pop > 0.004 {
				p.Transform(ui.Translation(0, -7*unit*pop).Then(ui.Scaling(1+0.135*pop, 1+0.135*pop)), box.X+box.W/2, box.Y+box.H*0.55, word)
			} else {
				word()
			}
			if a.settings.StageSparks && !t.rest {
				paintSparks(p, box, since, float64(t.on/time.Millisecond), t.last, hue)
			}
		}

		if sq := l.squareAt(pos, unit); look.shown && sq.alpha > 0.002 {
			side := 12 * unit
			sx, sy := x+sq.x, cy-0.6*size-4*unit-side/2-sq.up
			b := ui.Rect{X: sx - side/2, Y: sy - side/2, W: side, H: side}
			p.Transform(ui.Rotation(sq.turn).Then(ui.Scaling(sq.sx, sq.sy)), sx, sy, func() {
				p.Shadow(b, 1.5*unit, 0, 0, 6*unit, 0, sq.color.Alpha(0.5*sq.alpha))
				p.Fill(b, sq.color.Mix(ui.RGB(0, 0, 0), 0.3).Alpha(sq.alpha), 1.5*unit)
				p.Stroke(b, sq.color.Alpha(sq.alpha), 1.5*unit, 2.5*unit)
			})
		}
		if look.under != "" {
			p.RichText(x, cy+0.6*size+10*unit, set.avail, ui.Span{Text: look.under, Size: 13 * unit, Weight: 500, Lang: look.locale, Color: white.Alpha(0.55 * on)})
		}
	}
	if m.zoom > 0.00004 || m.zoom < -0.00004 {
		p.Transform(ui.Scaling(1+m.zoom, 1+m.zoom), cx, cy, paint)
	} else {
		paint()
	}
}

// paintHeld paints a word held long, at (x, top), since seconds after it
// was first sung, as AMLL emphasizes one: its characters set off one after
// another through the first two fifths of it, each rising, growing and
// glowing to its fullest halfway, spreading a little from the word's
// middle; and all of it floating up a little, from just before.
func paintHeld(p *ui.Painter, t *vinylToken, x, top, since, size float32, lang string, color, hue ui.Color) {
	n := float32(len(t.chars))
	i := 0
	for _, r := range t.text {
		at := t.chars[i]
		delay := t.over / 2.5 / n * float32(i)
		u := (since - delay) / t.over
		env := float32(0)
		if u > 0 && u < 1 {
			env = 1 - (2*u-1)*(2*u-1)
		}
		float := float32(math.Sin(math.Pi * float64(clamp01((since-delay+0.4)/(t.over*1.4)))))
		spread := float32(0)
		if n > 1 {
			spread = (float32(i) - (n-1)/2) / ((n - 1) / 2)
		}
		dx, dy := 0.03*t.rise*env*spread*size, -(0.025*t.rise*env+0.05*float)*size
		k := 1 + 0.1*t.rise*env
		ch := string(r)
		cw, chh := p.MeasureText(0, vinylSpan(ch, size, lang, color))
		p.Transform(ui.Translation(dx, dy).Then(ui.Scaling(k, k)), x+at+cw/2, top+chh*0.7, func() {
			if halo := min(0.3, 0.3*t.glow*env) * size; halo > 0.3 {
				// A glow without a blur: the character again, around itself.
				for a := range 8 {
					rad := float64(a) * math.Pi / 4
					p.RichText(x+at+halo*0.6*float32(math.Cos(rad)), top+halo*0.6*float32(math.Sin(rad)), 0, vinylSpan(ch, size, lang, hue.Alpha(0.09*env)))
				}
			}
			p.RichText(x+at, top, 0, vinylSpan(ch, size, lang, color))
		})
		i++
	}
}

// vinylClock is where playback is, for what moves with it every frame. The
// player's own position comes in steps as its audio is handed over, a
// frame's worth now and half of one the next, and a record turned by it
// stutters: the clock goes on at the pace of the frames, and is drawn
// toward the player's position, to which it jumps on a seek or a stall.
type vinylClock struct {
	pos, raw  time.Duration
	at, moved time.Time // when it was last read, and when raw last changed
}

func (k *vinylClock) read(now time.Time, pos time.Duration, playing bool) time.Duration {
	dt := now.Sub(k.at)
	if dt == 0 && !k.at.IsZero() && playing {
		return k.pos // the same frame
	}
	if pos != k.raw {
		k.raw, k.moved = pos, now
	}
	want := k.pos + dt
	off := pos - want
	if k.at.IsZero() || !playing || dt < 0 || dt > 300*time.Millisecond || now.Sub(k.moved) > 100*time.Millisecond ||
		off > 150*time.Millisecond || off < -150*time.Millisecond {
		k.pos, k.at = pos, now
		return pos
	}
	k.pos, k.at = want+time.Duration(float64(off)*min(1, dt.Seconds()/0.4)), now
	return k.pos
}

// vinylPos is where playback is at now, a frame's time, for Vinyl.
func (a *app) vinylPos(now time.Time) time.Duration {
	return a.deck.clock.read(now, a.pl.Position(), a.pl.Playing())
}
