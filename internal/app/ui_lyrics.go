package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/lyrics"
	"ringo/internal/music"
)

// The lyrics beside the cover follow the song as Apple Music's do: the line
// sung stands whole and sharp at its full size, and the others fall away
// from it into a blur, fainter and softer the farther they are, so that
// the eye rests on the one line. A line gives way to the next as a wave
// down the column (lyricflow.go), light crosses the line sung word by word
// (lyrictype.go), the vocals behind a line follow it in smaller type, a
// duet's other singer keeps to the right, and a break in the song shows
// three dots that breathe until the next line. Scrolled by the listener,
// the lines stay put and sharp, to read, and follow again a moment after.

// linePadX is the room on either side of a line's text, inside the line.
const linePadX = 12

func (a *app) lyricsPane(c *ui.Context, tr *music.Track, h float32) {
	pl := a.pl
	lines, loaded := pl.Lyrics()
	if len(lines) == 0 {
		col := ui.Column(c).Grow(1)
		if loaded {
			col.Center()
		}
		col.Children(func() {
			if !loaded {
				lyricsSkeleton(c, npShimmer(c))
				return
			}
			ui.Text(c, T("No lyrics. Enjoy the music.")).FontSize(24).FontWeight(700).TextColor(white.Alpha(0.4))
		})
		return
	}
	a.lyricsOf(tr.ID)
	if lyrics.Still(lines) {
		a.stillLines(c, lines, linesStyle{size: 24, weight: 700, lineHeight: 1.35}, 96)
		return
	}
	pos := pl.Position()
	focus, next := focusAt(lines, pos)
	// Redraw as the next line begins, or the break before it.
	if pl.Playing() && next > 0 {
		c.After(next + 20*time.Millisecond)
	}
	// The lines follow the song by themselves, which isn't the listener
	// scrolling them: only where they are before the flow moves them is.
	a.bars.note(c, "lyrics", barPlace{of: a.lyricTrack, y: a.lyricScroll.Y, last: -1})
	a.lyricFlow.step(c, &a.lyricScroll, len(lines))
	a.bars.settle("lyrics", barPlace{of: a.lyricTrack, y: a.lyricScroll.Y, last: -1})

	scroll := ui.Scroll(c.Key("lyrics")).Grow(1).TrackScroll(&a.lyricScroll).Padding(0, 8, h*0.5, 0)
	a.bars.show(c, scroll, "lyrics")
	a.timedLines(c, scroll, lines, pos, focus, linesStyle{size: 30, weight: 800, lineHeight: 1.25})
}

// lyricsOf has the lyrics' place and type be those of track: another
// song's start at its top, set anew.
func (a *app) lyricsOf(track string) {
	if a.lyricTrack != track {
		a.lyricTrack = track
		a.lyricScroll, a.lyricFlow, a.lyricType = ui.ScrollState{}, lyricFlow{}, lyricType{}
	}
}

// stillLines shows lyrics that have no times, as Apple Music has of some
// songs: all of them to read, scrolled by the listener from the top, none
// lit and none to seek to. Followed as timed lines are, they stood at their
// last line from the song's first second, every line having begun at 0.
// foot is the room left under the last line.
func (a *app) stillLines(c *ui.Context, lines []lyrics.Line, st linesStyle, foot float32) {
	lang := lyrics.Lang(lines)
	scroll := ui.Scroll(c.Key("still-lyrics")).Grow(1).TrackScroll(&a.lyricScroll).Padding(0, 8, foot, 0)
	a.bars.quiet(c, scroll, "still-lyrics", barPlace{of: a.lyricTrack, y: a.lyricScroll.Y, last: -1})
	scroll.Children(func() {
		for i, l := range lines {
			ui.Column(c.Key(i)).AlignItems(ui.Start).Padding(7, 12).Gap(4).Children(func() {
				ui.Text(c, l.Text).Lang(lang).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.3).TextColor(white.Alpha(0.86))
				if l.Translation != "" {
					ui.Text(c, l.Translation).FontSize(st.size * 0.62).FontWeight(600).TextColor(white.Alpha(0.6))
				}
			})
		}
	})
}

// linesStyle is how timedLines sets its lines: the size, weight and
// leading of their type.
type linesStyle struct {
	size       float32
	weight     int
	lineHeight float32
}

// lyricsSoft reports whether the lyrics blur: the lines away from the one
// sung, and the halo of what is lit. Each is a layer, which a GPU blurs and
// places for next to nothing, and which a renderer on the CPU draws pixel
// by pixel: there, a line giving way to the next took several times as
// long a frame, so the lines only dim. The line sung tells which it is as
// it paints, so the first frames of a window are without.
func (a *app) lyricsSoft() bool { return a.lyricGPU || a.lyricSoft }

// lineDepth is how a line n lines from the one sung shows: how much of it,
// and how blurred, in DIPs. The line sung is whole and sharp, and the rest
// fall away from it.
func lineDepth(n int) (alpha, blur float32) {
	if n <= 0 {
		return 1, 0
	}
	return max(0.22, 0.47-0.07*float32(n)), min(0.85*float32(n), 3.4)
}

// timedLines fills scroll with lines that follow the track playing at
// pos: the line of focus lit, and brought to a third of the way down once
// the focus changes, each line going there in its own time (lyricFlow); a
// click on one seeks to it.
func (a *app) timedLines(c *ui.Context, scroll ui.Element, lines []lyrics.Line, pos time.Duration, focus lyricFocus, st linesStyle) {
	now := c.Now()
	flow, ty := &a.lyricFlow, &a.lyricType
	// The lines are set in type as wide as the pane last was.
	box := scroll.Bounds()
	width := ty.width
	if box.W > 0 {
		width = box.W - 8 - 2*linePadX
	} else {
		c.AnimationFrame() // not laid out yet: the next frame sets them
	}
	if ty.set(lines, width, st) {
		// Set anew, as in a window resized, they are laid out in this frame:
		// the next brings the focus into place again, without a wave.
		flow.seen, flow.jumped = false, true
	}
	browsing, left := flow.browsing(now, a.bars.bar("lyrics").until)
	if browsing {
		c.After(left)
	}
	var lead ui.Element
	scroll.Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			for i := range lines {
				row := a.lyricLine(c, lines, i, pos, focus, browsing)
				if i == focus.anchor() {
					lead = row
				}
			}
		})
	})
	// Bring the focus a third of the way down, once it changed; but not
	// from under the listener reading elsewhere.
	if browsing || flow.seen && flow.focus == focus {
		return
	}
	if flow.jumped {
		// The lines' places are of the frame before the offset moved.
		c.AnimationFrame()
		return
	}
	target := float32(0)
	if lead.Valid() {
		// Where the lines are going, without where they still are.
		lb := lead.LayoutBounds()
		if lb.H <= 0 || box.H <= 0 {
			return // not laid out yet: the next frame scrolls
		}
		target = lb.Y - box.Y + a.lyricScroll.Y + lyricBreath - box.H*lyricFocusY
	}
	flow.to(c, focus, min(max(0, target), a.lyricScroll.MaxY))
}

// lyricLine is line i of lines, as it shows at pos with the lyrics on
// focus: where the flow has it, as deep as it is from the line sung, and
// the dots of a break over it; browsing shows the lines sharp, for the
// listener reading them.
func (a *app) lyricLine(c *ui.Context, lines []lyrics.Line, i int, pos time.Duration, focus lyricFocus, browsing bool) ui.Element {
	l, ty := &lines[i], &a.lyricType
	side, originX := ui.Start, float32(0)
	if l.Other {
		side, originX = ui.End, 1
	}
	row := ui.Column(c.Key(i))
	if y := a.lyricFlow.offset(i, c.Now()); y != 0 {
		row.Translate(0, y)
	}
	row.Children(func() {
		if focus.rest && i == focus.line+1 {
			from, to, _ := breakAfter(lines, focus.line)
			a.lyricDots(c, from, to)
		}
		line := ui.ButtonBase(c.Key("line")).Column().AlignItems(side).Padding(10, linePadX).Radius(12).FocusRing(false).Label(l.Text)
		on := !focus.rest && i == focus.line
		alpha, blur := lineDepth(focus.away(i))
		if !a.lyricsSoft() {
			blur = 0
		}
		if browsing && !on {
			alpha, blur = 0.5, 0
		}
		if line.Hovered() {
			line.Background(white.Alpha(0.06))
			if !on {
				alpha, blur = max(alpha, 0.72), 0
			}
		}
		scale := float32(0.94)
		if on {
			scale = 1
		}
		line.TransformOrigin(originX, 0.5).AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.1}})
		line.Children(func() {
			// The line fades as a pose, in frames that only paint.
			text := ui.Column(c.Key("text")).AlignItems(side).Gap(4)
			text.AnimatePose(ui.Pose{Fade: 1 - alpha}, ui.Animation{Duration: 380 * time.Millisecond})
			// Its blur goes with the lines' own way to their places, which
			// builds the view anyway.
			if b := text.AnimateTo("blur", blur, ui.Animation{Duration: 420 * time.Millisecond, Ease: ui.EaseInOut, NonMotion: true}); b > 0.05 {
				text.Blur(b)
			}
			text.Children(func() {
				a.sungText(c.Key("words"), ty.lines[i], on)
				if behind := ty.behind[i]; behind != nil {
					a.sungText(c.Key("behind"), behind, on).Opacity(0.72)
				}
				if l.Translation != "" {
					ui.Text(c, l.Translation).FontSize(17).FontWeight(600).TextColor(white.Alpha(0.7)).TextAlign(side).Margin(2, 0, 0, 0)
				}
			})
		})
		if line.Clicked() {
			// The listener chose where the song goes: the lines follow it
			// there at once.
			a.lyricFlow.browse = time.Time{}
			a.pl.Seek(l.At)
		}
	})
	return row
}

// sungText shows t, a line set in type: as it is sung while on, the light
// as far across it as the song has got, over the halo of what is lit, and
// else as it rests.
func (a *app) sungText(c *ui.Context, t *lyricText, on bool) ui.Element {
	box := ui.Box(c).Size(t.w, t.h)
	if !on {
		return box.Draw(func(p *ui.Painter, r ui.Rect) { p.TextLayout(t.layout, r.X, r.Y, white) })
	}
	still := c.Preferences().ReduceMotion
	box.Children(func() {
		if a.lyricsSoft() {
			ui.Box(c.Key("halo")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Blur(t.size * 0.22).
				Draw(func(p *ui.Painter, r ui.Rect) { t.paintHalo(p, r, a.pl.Position(), still) })
		}
		ui.Box(c.Key("lit")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().
			Draw(func(p *ui.Painter, r ui.Rect) {
				a.lyricGPU = p.Accelerated()
				pos := a.pl.Position()
				t.paint(p, r, pos, still)
				// The light moves on in frames that only paint, until the
				// last word is sung and its halo has faded.
				if a.pl.Playing() && pos < t.until {
					p.AnimationFrameRate(60)
				}
			})
	})
	return box
}

// lyricDots shows the three dots of a break in the song, from from to to,
// over the line that follows it: they come in, fill one after another as
// the break goes by, breathing, and swell and go just before the line.
func (a *app) lyricDots(c *ui.Context, from, to time.Duration) {
	const (
		dot, gap = 11, 8
		leave    = 0.7 // how many seconds before the line they go
	)
	still := c.Preferences().ReduceMotion
	ui.Box(c.Key("dots")).Absolute().Left(linePadX+2).Top(-(dotsHeight + lyricBreath)).Size(3*dot+2*gap, dotsHeight).PassThrough().
		Draw(func(p *ui.Painter, r ui.Rect) {
			pos := a.pl.Position()
			t, left := float32((pos - from).Seconds()), float32((to - pos).Seconds())
			if t < 0 || left <= 0 {
				return
			}
			shown := smoothstep(0, 0.45, t)
			alpha, scale := shown, 0.7+0.3*shown
			if left < leave {
				u := 1 - left/leave
				alpha *= 1 - smoothstep(0.5, 1, u)
				scale *= 1 + 0.3*smoothstep(0, 0.5, u) - 0.7*smoothstep(0.5, 1, u)
			} else if !still {
				scale *= 1 + 0.09*float32(math.Sin(float64(t)*2*math.Pi/2.8))
			}
			if still {
				scale = 1
			}
			filled := clamp01(t/max(0.1, float32((to-from).Seconds())-leave)) * 3
			p.Transform(ui.Scaling(scale, scale), r.X+r.W/2, r.Y+r.H/2, func() {
				for i := range 3 {
					lit := clamp01(filled - float32(i))
					at := ui.Rect{X: r.X + float32(i)*(dot+gap), Y: r.Y + (r.H-dot)/2, W: dot, H: dot}
					p.Fill(at, white.Alpha((0.3+0.7*lit)*alpha), dot/2)
				}
			})
			if a.pl.Playing() {
				p.AnimationFrameRate(30)
			}
		})
}
