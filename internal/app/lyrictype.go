package app

import (
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"ringo/internal/lyrics"
)

// The lines of the lyrics are set in type once, and painted from it: a Text
// lights a word by its color alone, all of it at once. Painted, light
// crosses a word from its left to its right with a soft edge, the word
// lifts as the light reaches it, a long note swells, and what is lit has a
// halo. The line sung paints itself again as the song goes, in frames that
// only paint: the view isn't built to move the light.

const (
	// lyricUnsung is how bright the words of the line sung are before the
	// light reaches them.
	lyricUnsung = 0.5
	// lyricLift is how far a word lifts as it is sung, and lyricSwell how
	// much further, and larger, a long note goes, as shares of the type's
	// size.
	lyricLift  = 0.06
	lyricSwell = 0.07
	// lyricLong is how long a word is held to count as a long note.
	lyricLong = 900 * time.Millisecond
	// The halo of a word as it is sung, of a long note, and of what has
	// been sung; haloFade is how long it takes to fall from the one to
	// the other.
	haloSung = 0.55
	haloLong = 0.9
	haloRest = 0.2
	haloFade = 0.7
)

// lyricSweep fills text with light as far as Edge, in DIPs from the left of
// its box, over an edge Soft wide, and with Dim of it beyond.
var lyricSweep = shader.MustCompile(`package main

var Edge float
var Soft float
var Dim float

func Fragment(dst vec4, pos vec2) vec4 {
	a := mix(1, Dim, smoothstep(Edge-Soft, Edge, pos.x))
	return vec4(a, a, a, a)
}
`)

var (
	sweepEdge   = lyricSweep.Uniform("Edge")
	sweepSoft   = lyricSweep.Uniform("Soft")
	sweepDim    = lyricSweep.Uniform("Dim")
	sweepValues = lyricSweep.NewValues()
)

// lyricType is the lyrics of a song set in type, for a pane of a width.
type lyricType struct {
	of    *lyrics.Line // the first of the lines it is of
	width float32
	st    linesStyle
	lang  string
	duet  bool
	lines []*lyricText
	// behind are the backing vocals of the lines that have them.
	behind []*lyricText
	// extra is what shows under the lines (lyricsExtras), and said how the
	// lines that have it are said, set only while that is what shows.
	extra string
	said  []*lyricText
}

// set sets lines in type for a pane width wide, with extra under them,
// unless they are, and reports whether it did: the lines are then other
// sizes than they were laid out at.
func (ty *lyricType) set(lines []lyrics.Line, width float32, st linesStyle, extra string) bool {
	if len(lines) == len(ty.lines) && ty.of == &lines[0] && ty.width == width && ty.st == st && ty.extra == extra {
		return false
	}
	*ty = lyricType{of: &lines[0], width: width, st: st, lang: lyrics.Lang(lines), extra: extra}
	for i := range lines {
		ty.duet = ty.duet || lines[i].Other
	}
	// A duet's lines stop short of the far side, to show whose they are.
	w := width
	if ty.duet {
		w = float32(math.Floor(float64(width) * 0.86))
	}
	main := ui.Font{Size: st.size, Weight: st.weight, Lang: ty.lang, LineHeight: st.lineHeight}
	small := ui.Font{Size: float32(math.Round(float64(st.size) * 0.6)), Weight: 700, Lang: ty.lang, LineHeight: 1.3}
	// How a line is said is set as Music sets it: half the line's size,
	// bold and as white as the line, close under it, as a part of it. In
	// the app's own font: Latin letters, whatever the lyrics are written in.
	under := ui.Font{Size: float32(math.Round(float64(st.size) * 0.5)), Weight: 700, LineHeight: 1.25}
	ty.lines, ty.behind, ty.said = make([]*lyricText, len(lines)), make([]*lyricText, len(lines)), make([]*lyricText, len(lines))
	for i := range lines {
		l := &lines[i]
		align := ui.Start
		if l.Other {
			align = ui.End
		}
		ty.lines[i] = setLyric(l.Text, l.Words, l.Sung, main, -0.4, w, align)
		if len(l.Behind) > 0 {
			ty.behind[i] = setLyric("", l.Behind, true, small, -0.2, w, align)
		}
		if extra == extraPronunciation && l.Pronunciation != "" {
			// Its long notes don't swell: the line's own do, over it.
			ty.said[i] = setLyric(l.Pronunciation, l.Pronounced, false, under, 0, w, align)
		}
	}
	return true
}

// lyricText is a line of lyrics set in type: its text laid out, and where
// each of its words is.
type lyricText struct {
	layout *ui.TextLayout
	w, h   float32
	size   float32
	words  []wordBox
	// whole says the line lights all at once: one read from right to
	// left, whose words the light would cross backwards.
	whole bool
	// until is when the last of the line's words has been sung, and its
	// halo has faded.
	until time.Duration
}

// wordBox is a word of a line set in type.
type wordBox struct {
	// row is the row of the line the word is on, y and h the row's top and
	// height, and left and right the word's reach along it: from its left
	// edge to the next word's, the row's ends for its first and last.
	row         int
	y, h        float32
	left, right float32
	start, end  time.Duration
	long        bool
}

// setLyric sets text, or with none the words joined, in font, wrapped at
// width, and finds its words in it. sung says the words' times are their
// own, for the long notes among them to be told.
func setLyric(text string, words []lyrics.Word, sung bool, font ui.Font, spacing, width float32, align ui.Align) *lyricText {
	at, ok := wordRanges(text, words)
	if !ok || text == "" {
		// The words aren't the text's, as far as can be told: theirs it is.
		var b strings.Builder
		for _, w := range words {
			b.WriteString(w.Text)
		}
		text = strings.TrimSpace(b.String())
		at, _ = wordRanges(text, words)
	}
	layout := ui.ShapeRichTextWith([]ui.Span{{Text: text, LetterSpacing: spacing}}, font, ui.TextShape{Width: width, Align: align})
	_, h := layout.Size()
	t := &lyricText{layout: layout, w: width, h: h, size: font.Size}
	rows := layout.Lines()
	for _, r := range rows {
		t.whole = t.whole || r.RTL
	}
	if at == nil || t.whole {
		return t
	}
	for k, w := range words {
		if at[k][0] == at[k][1] {
			continue
		}
		rects := layout.SelectionRects(ui.TextInputRange{Start: at[k][0], End: at[k][1]})
		var total float32
		for _, r := range rects {
			total += r.W
		}
		// A word broken across rows is sung along each in turn.
		from := w.Start
		for _, r := range rects {
			d := w.End - w.Start
			if total > 0 {
				d = time.Duration(float32(d) * r.W / total)
			}
			row := 0
			for i, lr := range rows {
				if r.Y+r.H/2 >= lr.Rect.Y {
					row = i
				}
			}
			t.words = append(t.words, wordBox{
				row: row, y: rows[row].Rect.Y, h: rows[row].Rect.H, left: r.X, right: r.X + r.W, start: from, end: from + d,
				long: sung && len(rects) == 1 && d >= lyricLong && utf8.RuneCountInString(strings.TrimSpace(w.Text)) <= 8,
			})
			from += d
		}
		t.until = max(t.until, w.End)
	}
	t.until += time.Duration(haloFade * float64(time.Second))
	// A word reaches to the next on its row, over the space between them,
	// and the first and the last of a row past its ends, where a glyph
	// may lean out of its box.
	for k := range t.words {
		w := &t.words[k]
		lr := rows[w.row].Rect
		if k == 0 || t.words[k-1].row != w.row {
			w.left = min(w.left, lr.X) - 6
		}
		if k+1 == len(t.words) || t.words[k+1].row != w.row {
			w.right = max(w.right, lr.X+lr.W) + 6
		} else {
			w.right = t.words[k+1].left
		}
	}
	return t
}

// wordRanges finds each of words in text, one after another, as offsets in
// UTF-16 units, and reports whether they are all there.
func wordRanges(text string, words []lyrics.Word) ([][2]int, bool) {
	out := make([][2]int, len(words))
	at, units := 0, 0
	for k, w := range words {
		for at < len(text) {
			r, n := utf8.DecodeRuneInString(text[at:])
			if !unicode.IsSpace(r) {
				break
			}
			at, units = at+n, units+utf16.RuneLen(r)
		}
		word := strings.TrimSpace(w.Text)
		if !strings.HasPrefix(text[at:], word) {
			return nil, false
		}
		n := 0
		for _, r := range word {
			n += utf16.RuneLen(r)
		}
		out[k] = [2]int{units, units + n}
		at, units = at+len(word), units+n
	}
	return out, true
}

// wordLook is how a word of the line sung shows at a moment.
type wordLook struct {
	lit   float32 // how much of it the light has crossed
	rise  float32 // how far it has lifted
	swell float32 // how far a long note has swelled
	halo  float32 // how bright its halo is
}

// sungLook is how a word shows once sung, and its halo faded.
var sungLook = wordLook{lit: 1, rise: 1, halo: haloRest}

// look is how the word shows at pos.
func (w *wordBox) look(pos time.Duration) wordLook {
	t := float32((pos - w.start).Seconds())
	if t <= 0 {
		return wordLook{}
	}
	d := float32(max(w.end-w.start, 80*time.Millisecond).Seconds())
	after := t - d
	if after >= haloFade {
		return sungLook
	}
	k := wordLook{lit: clamp01(t / d), rise: easeOutCubic(t / min(max(d, 0.2), 0.45))}
	peak := float32(haloSung)
	if w.long {
		peak = haloLong
		// A long note swells through its length, and settles after it.
		k.swell = float32(math.Sin(math.Pi * float64(clamp01(t/(d+0.3)))))
	}
	glow := clamp01(t / 0.12)
	if after > 0 {
		glow = 1 - easeOutCubic(after/haloFade)
	}
	k.halo = haloRest + (peak-haloRest)*glow
	return k
}

// runs has each run of the line's words that look alike at pos seen: a word
// on its way by itself, and those sung, or yet to be, a row of them at a
// time. reach is the run's, in the line's own space.
func (t *lyricText) runs(pos time.Duration, seen func(reach ui.Rect, k wordLook)) {
	for i := 0; i < len(t.words); {
		w := &t.words[i]
		k := w.look(pos)
		j := i + 1
		if k == sungLook || k == (wordLook{}) {
			for j < len(t.words) && t.words[j].row == w.row && t.words[j].look(pos) == k {
				j++
			}
		}
		seen(ui.Rect{X: w.left, Y: w.y, W: t.words[j-1].right - w.left, H: w.h}, k)
		i = j
	}
}

// lifted paints fn as a word that looks k does, lifted, and swelled about
// the middle of reach, a little below it, where its feet are; still does
// neither.
func (t *lyricText) lifted(p *ui.Painter, reach ui.Rect, k wordLook, still bool, fn func()) {
	if still || k.rise == 0 && k.swell == 0 {
		fn()
		return
	}
	s := 1 + lyricSwell*k.swell
	move := ui.Scaling(s, s).Then(ui.Translation(0, -t.size*(lyricLift*k.rise+lyricSwell*k.swell)))
	p.Transform(move, reach.X+reach.W/2, reach.Y+reach.H*0.62, fn)
}

// paint paints the line as it is sung at pos, at r: the light as far as it
// has got. still keeps the words where they are, for less motion.
func (t *lyricText) paint(p *ui.Painter, r ui.Rect, pos time.Duration, still bool) {
	if t.whole || len(t.words) == 0 {
		p.TextLayout(t.layout, r.X, r.Y, white)
		return
	}
	t.runs(pos, func(reach ui.Rect, k wordLook) {
		reach.X, reach.Y = reach.X+r.X, reach.Y+r.Y
		t.lifted(p, reach, k, still, func() {
			p.Clip(reach, 0, func() {
				switch {
				case k.lit <= 0:
					p.TextLayout(t.layout, r.X, r.Y, white.Alpha(lyricUnsung))
				case k.lit >= 1:
					p.TextLayout(t.layout, r.X, r.Y, white)
				default:
					// The edge is all before the word as it begins, and
					// all past it as it ends.
					soft := min(t.size*0.6, reach.W)
					sweepValues.Float(sweepEdge, k.lit*(reach.W+soft)).Float(sweepSoft, soft).Float(sweepDim, lyricUnsung)
					lyricSweep.Fill(p, reach, shader.Options{Values: sweepValues}, func() {
						p.TextLayout(t.layout, r.X, r.Y, white)
					})
				}
			})
		})
	})
}

// paintHalo paints what of the line the light has crossed at pos, at r, as
// bright as its halo is: blurred, it is the halo.
func (t *lyricText) paintHalo(p *ui.Painter, r ui.Rect, pos time.Duration, still bool) {
	if t.whole {
		return
	}
	t.runs(pos, func(reach ui.Rect, k wordLook) {
		if k.lit <= 0 {
			return
		}
		reach.X, reach.Y = reach.X+r.X, reach.Y+r.Y
		lit := reach
		lit.W *= k.lit
		t.lifted(p, reach, k, still, func() {
			p.Clip(lit, 0, func() { p.TextLayout(t.layout, r.X, r.Y, white.Alpha(k.halo)) })
		})
	})
}
