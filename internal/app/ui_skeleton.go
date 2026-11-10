package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// Skeletons stand in for what is loading: gray blocks in its shape, which
// it takes the place of without moving anything, and over them a lighter
// band sweeping, across all the blocks of a view in step.

// shimmer is how the blocks of a skeleton look.
type shimmer struct {
	base, sheen ui.Color
	span        float32 // the width the band crosses: the window's
	still       bool    // the desktop asks for less motion: no band
}

const (
	shimmerPeriod = 1600 * time.Millisecond
	shimmerSweep  = 1100 * time.Millisecond // of the period, the band crossing; then it rests
	shimmerBand   = 260                     // DIPs
)

// pageShimmer is the shimmer of the pages, on their background.
func (a *app) pageShimmer(c *ui.Context) shimmer {
	// The page is loading: what takes its place arrives later (footEdge).
	a.skelOf = a.route
	return a.rowShimmer(c)
}

// rowShimmer is pageShimmer for blocks in the rows of a page that shows,
// standing in for those to come: the page isn't loading, only its end is.
func (a *app) rowShimmer(c *ui.Context) shimmer {
	s := shimmer{base: ui.RGBA(0, 0, 0, 0.06), sheen: ui.RGBA(0, 0, 0, 0.022)}
	if a.pal.dark {
		s.base, s.sheen = white.Alpha(0.07), white.Alpha(0.13)
	}
	return s.in(c)
}

// npShimmer is the shimmer over the now playing view's backdrop.
func npShimmer(c *ui.Context) shimmer {
	return shimmer{base: white.Alpha(0.1), sheen: white.Alpha(0.2)}.in(c)
}

func (s shimmer) in(c *ui.Context) shimmer {
	s.span, _ = c.Size()
	s.still = c.Preferences().ReduceMotion
	return s
}

// paint fills r, its corners rounded by radius, as a block of the skeleton.
func (s shimmer) paint(p *ui.Painter, r ui.Rect, radius float32) {
	if s.still || r.W <= 0 {
		p.Fill(r, s.base, radius)
		return
	}
	at := time.Duration(p.Now().UnixNano() % int64(shimmerPeriod))
	if at >= shimmerSweep {
		p.Fill(r, s.base, radius)
		p.After(shimmerPeriod - at)
		return
	}
	p.AnimationFrameRate(60)
	t := float32(at) / float32(shimmerSweep)
	x := -shimmerBand/2 + t*(s.span+shimmerBand)
	if x+shimmerBand/2 < r.X || x-shimmerBand/2 > r.X+r.W {
		p.Fill(r, s.base, radius)
		return
	}
	// The band's place along the block, which it may lie partly beyond.
	along := func(x float32) float32 { return (x - r.X) / r.W }
	p.FillGradient(r, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
		{Color: s.base, At: along(x - shimmerBand/2)},
		{Color: s.sheen, At: along(x)},
		{Color: s.base, At: along(x + shimmerBand/2)},
	}}, radius)
}

// bone is a block of the skeleton, as big as it is laid out.
func (s shimmer) bone(c *ui.Context, radius float32) ui.Element {
	return ui.Box(c).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) { s.paint(p, r, radius) })
}

// text is a line of text at size, as tall as the line and as wide as it
// is laid out, its block the height of the letters.
func (s shimmer) text(c *ui.Context, size, lineHeight float32) ui.Element {
	return ui.Box(c).Shrink(0).Height(size * lineHeight).Draw(func(p *ui.Painter, r ui.Rect) {
		h := size * 0.78
		s.paint(p, ui.Rect{X: r.X, Y: r.Y + (r.H-h)/2, W: r.W, H: h}, h*0.32)
	})
}

// boneWidths vary the lines of a skeleton, as the lengths of what they
// stand for do, as percentages.
var boneWidths = [...]float32{62, 44, 78, 53, 70, 38, 66, 49, 82, 57, 41, 73}

func boneWidth(i int) float32 { return boneWidths[i%len(boneWidths)] }

// Skeletons of the pages, shaped as homePage, trackRows and the cards they
// show.

// homeSkeleton is Home's shelves while they load, as browseRows starts them.
// A listener's Home leads with cards three to four, Apple's picks.
func (a *app) homeSkeleton(c *ui.Context, s shimmer) {
	a.shelfSkeletonWith(c, s, a.account != nil)
}

// shelfSkeleton is a page of shelves while it loads: titles over rows of
// squares, as many as fit across.
func (a *app) shelfSkeleton(c *ui.Context, s shimmer) { a.shelfSkeletonWith(c, s, false) }

// shelfSkeletonWith is shelfSkeleton, its first row cards three to four,
// as portraitCard's, with no words under them, as they have none.
func (a *app) shelfSkeletonWith(c *ui.Context, s shimmer, portrait bool) {
	m := a.shelfMetrics(c)
	n, w := m.fit(squareMin)
	pn, pw := m.fit(portraitMin)
	ui.Column(c.Key("shelf-skeleton")).PaddingX(12).Children(func() {
		for row := range 2 {
			ui.Column(c.Key(row)).Margin(float32(row)*sectionGap, 0, 0, 0).Gap(12).Children(func() {
				s.text(c, 18, 1.3).Width(140)
				ui.Row(c).Gap(shelfGap).Children(func() {
					if portrait && row == 0 {
						for i := range pn {
							s.bone(c.Key(i), portraitRadius).Size(pw, pw*4/3)
						}
						return
					}
					for i := range n {
						ui.Column(c.Key(i)).Width(w).Gap(7).Children(func() {
							s.bone(c, artRadius).Size(w, w)
							s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i + row))
						})
					}
				})
			})
		}
	})
}

// librarySkeleton is a page of the listener's library, id, while it loads:
// its songs in rows, or its cards in a grid, without a title over them, as
// the page has none.
func (a *app) librarySkeleton(c *ui.Context, s shimmer, id string) {
	if base, _ := music.ParseLibraryPage(id); base == music.PageLibrarySongs {
		_, h := c.Size()
		for i := range max(8, int(h/54)) {
			a.trackRowSkeleton(c.Key(i), s, i, true)
		}
		return
	}
	artists := id == music.PageLibraryArtist
	ui.Column(c.Key("library-skeleton")).Children(func() {
		for row := range 2 {
			a.cardsSkeletonRow(c.Key(row), s, artists, row)
		}
	})
}

// cardsSkeletonRow is row number row of a library's cards while they load, or
// of those yet to come.
func (a *app) cardsSkeletonRow(c *ui.Context, s shimmer, artists bool, row int) {
	// As shelfGrid lays out the cards, squareCard and circleCard their
	// pictures and cardLines their words: an artist's name alone, under
	// their round picture.
	min, gap, align := float32(squareMin), float32(7), ui.Start
	if artists {
		min, gap, align = circleMin, 8, ui.Center
	}
	n, w := a.shelfMetrics(c).fit(min)
	round := float32(artRadius)
	if artists {
		round = w / 2
	}
	ui.Row(c).Gap(shelfGap).AlignItems(ui.Start).Margin(0, 12, 24, 12).Children(func() {
		for i := range n {
			ui.Column(c.Key(i)).Width(w).Gap(gap).AlignItems(align).Children(func() {
				s.bone(c, round).Size(w, w)
				ui.Column(c).Gap(1).Width(w).AlignItems(align).Children(func() {
					s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i + row))
					if !artists {
						s.text(c, 12, 1.3).WidthPercent(boneWidth(i+row+4) * 0.7)
					}
				})
			})
		}
	})
}

// trackSkeleton is trackRows while its tracks load: rows of blocks under
// the header, which may hold blocks of its own.
func (a *app) trackSkeleton(c *ui.Context, s shimmer, covers bool, header func(c *ui.Context)) {
	_, h := c.Size()
	a.rowsSkeleton(c, header, func() {
		for i := range max(8, int(h/54)) {
			a.trackRowSkeleton(c.Key(i), s, i, covers)
		}
	})
}

// rowsSkeleton is a page of rows while they load, rows under header.
func (a *app) rowsSkeleton(c *ui.Context, header func(c *ui.Context), rows func()) {
	top, bottom := a.insets(c)
	a.underTop, a.underBottom = false, true
	ui.Column(c.Key("skeleton")).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).Children(func() {
		a.startMark(c)
		header(c)
		rows()
	})
}

// trackRowSkeleton is trackRow's row i while it loads.
func (a *app) trackRowSkeleton(c *ui.Context, s shimmer, i int, covers bool) {
	w, _ := c.Size()
	wide := w-sidebarWidth > 820
	ui.Row(c).Height(54).Shrink(0).PaddingX(12).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(26).Center().Children(func() { s.text(c, 13, 1.3).Width(12) })
		if covers {
			s.bone(c, 6).Size(38, 38)
		}
		ui.Column(c).Grow(3).Basis(0).MinWidth(0).Gap(3).Children(func() {
			s.text(c, 14, 1.3).WidthPercent(boneWidth(i) * 0.8)
			s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i+5) * 0.55)
		})
		if wide {
			ui.Box(c).Grow(2).Basis(0).MinWidth(0).Children(func() {
				s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i+3) * 0.8)
			})
		}
		ui.Box(c).Size(32, 32)
		ui.Row(c).Width(44).Justify(ui.End).Children(func() { s.text(c, 12.5, 1.3).Width(30) })
	})
}

// playlistSkeleton is a playlist's page while it loads: its header, the
// head of its table and its songs, each where playlistColumns and
// playlistRow put theirs, so that nothing moves as the songs come.
func (a *app) playlistSkeleton(c *ui.Context, s shimmer) {
	_, h := c.Size()
	wide := a.playlistWide(c)
	a.rowsSkeleton(c, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) }, func() {
		// The head's hairline, without its words.
		ui.Row(c.Key("head")).Height(30).Shrink(0).Margin(0, 12).BorderWidth(0, 0, 0.5, 0).BorderColor(a.pal.hairline)
		for i := range max(8, int(h/52)) {
			ui.Row(c.Key(i)).Height(52).Shrink(0).Margin(0, 12).PaddingX(8).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Grow(3).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
					s.bone(c, 4).Size(40, 40)
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						s.text(c, 13.5, 1.3).WidthPercent(boneWidth(i) * 0.8)
						if !wide {
							s.text(c, 12.5, 1.3).WidthPercent(boneWidth(i+5) * 0.55)
						}
					})
				})
				if wide {
					for k := range 2 {
						ui.Box(c.Key(k)).Grow(2).Basis(0).MinWidth(0).Children(func() {
							s.text(c, 13, 1.3).WidthPercent(boneWidth(i+3+4*k) * 0.7)
						})
					}
				}
				ui.Box(c).Size(30, 30)
				ui.Row(c).Width(44).Justify(ui.End).Children(func() { s.text(c, 13, 1.3).Width(30) })
				ui.Box(c).Width(24)
			})
		}
	})
}

// albumSkeleton is an album's page while it loads: its header, and its songs
// in albumTrackRow's rows of a line each. Most albums are of one disc, whose
// songs have no title over them.
func (a *app) albumSkeleton(c *ui.Context, s shimmer) {
	_, h := c.Size()
	a.rowsSkeleton(c, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) }, func() {
		for i := range max(10, int(h/44)) {
			ui.Row(c.Key(i)).Height(44).Shrink(0).Margin(0, 12).PaddingX(8).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Width(12)
				ui.Box(c).Width(24).Center().Children(func() { s.text(c, 13, 1.3).Width(10) })
				ui.Box(c).Grow(3).Basis(0).MinWidth(0).Children(func() {
					s.text(c, 13.5, 1.3).WidthPercent(boneWidth(i) * 0.6)
				})
				ui.Box(c).Size(30, 30)
				ui.Row(c).Width(44).Justify(ui.End).Children(func() { s.text(c, 13, 1.3).Width(30) })
				ui.Box(c).Width(24)
			})
		}
	})
}

// playlistHeaderSkeleton is playlistHeader while the playlist loads.
func (a *app) playlistHeaderSkeleton(c *ui.Context, s shimmer) {
	ui.Row(c).Gap(30).AlignItems(ui.End).Margin(0, 12, 28, 12).Children(func() {
		s.bone(c, 8).Size(detailCover, detailCover)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			s.text(c, 12, 1.3).Width(36)
			s.text(c, 30, 1.2).WidthPercent(48)
			s.text(c, 13, 1.3).Width(180)
			ui.Column(c).Margin(2, 0, 0, 0).Children(func() {
				s.text(c, 12.5, 1.45).WidthPercent(78)
				s.text(c, 12.5, 1.45).WidthPercent(52)
			})
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				s.bone(c, 18).Size(92, 36)
				s.bone(c, 18).Size(120, 36)
			})
		})
	})
}

// Skeletons of the now playing view.

// lyricsSkeleton is lyricsPane's lines while they load.
func lyricsSkeleton(c *ui.Context, s shimmer) {
	ui.Column(c).Gap(6).Padding(0, 8, 0, 0).Children(func() {
		for i := range 7 {
			ui.Box(c.Key(i)).Padding(10, 12).Children(func() {
				s.text(c, 30, 1.25).WidthPercent(boneWidth(i+1) * 0.9)
			})
		}
	})
}

// stageSkeleton is stageLine's words while the lyrics load.
func stageSkeleton(c *ui.Context, s shimmer, f, w float32) {
	ui.Row(c).Wrap().Justify(ui.Center).AlignItems(ui.Center).Padding(0, w*0.1).Gap(f * 0.35).Children(func() {
		for i, n := range []float32{2.2, 1.3, 3.1, 1.8} {
			s.bone(c.Key(i), f*0.22).Size(f*n, f*0.85)
		}
	})
}
