package app

import (
	"math"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

var white = ui.RGB(255, 255, 255)

// npSpring is how Now Playing comes and goes, sliding or growing out of the
// player bar's cover and shrinking back into it, and how it springs back
// from a pull too short to close it.
var npSpring = ui.SpringOptions{Duration: 340 * time.Millisecond}

// npFade is how Now Playing comes and goes while the desktop asks for less
// motion: it fades, in place.
var npFade = ui.Animation{Duration: 200 * time.Millisecond, NonMotion: true}

// npState tracks Now Playing opening and closing, and reports whether it is
// open and whether it has come to rest, open or closed, as it last showed.
// Call it once a build, before the window's content.
func (a *app) npState(c *ui.Context) (open, settled bool) {
	open = a.nowPlaying && a.pl.Current() != nil
	if open != a.npOpen {
		a.npOpen = open
		// The window buttons light over its dark backdrop, and as the
		// theme over the pages (Windows).
		if a.win != nil {
			buttons := mygo.ThemeSystem
			if open {
				buttons = mygo.ThemeDark
			}
			a.win.SetWindowButtonsTheme(buttons)
		}
	}
	return open, a.npAtRest(open)
}

// npAtRest reports whether Now Playing showed at rest: all open, with
// nothing pulling on it, or all gone.
func (a *app) npAtRest(open bool) bool {
	if open {
		return a.npT >= 1 && a.npShift == 0 && !a.npHeld
	}
	return a.npT <= 0
}

// nowPlayingLayer holds Now Playing over the whole window. It slides up
// from below the window as it opens, as it is, and back down out of it as
// it closes; with less motion, it only fades. Growing out of the cover, an
// experiment the listener turns on, it grows out of the cover in the
// player bar as it opens, a card that takes the window, the cover flying
// out of the bar to its place, and shrinks back into it as it closes.
// Pulled down, it follows the pointer, growing out of the cover smaller and
// rounder the farther it goes, and let go far or fast enough it falls the
// rest of the way; otherwise it springs back. The layer stays on top all
// the way, rather than leaving as an exit transition would, under the rest
// of the window: that would show it going over the sidebar's material
// while the page above it hid it at once.
func (a *app) nowPlayingLayer(c *ui.Context, open, settled bool) {
	w, h := c.Size()
	layer := ui.Box(c.Key("now-playing-layer")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
	// Unless it grows out of the cover, an experiment, it slides up, or
	// with less motion only fades.
	reduce := c.Preferences().ReduceMotion
	a.npStill = reduce || !a.settings.GrowFromCover
	slide := a.npStill && !reduce
	spring := npSpring
	_, dy, held := layer.Dragged()
	held = held && open
	if held {
		a.npPull += dy
	}
	if _, vy, ok := layer.DragEnded(); ok && open {
		if a.npPull > h*0.2 || a.npPull > 24 && vy > 700 {
			// Let go far enough, or thrown: it falls the rest of the way
			// as fast as it was thrown.
			a.nowPlaying, open = false, false
			spring.Velocity = -max(vy, 0) / h
		}
	}
	if !held {
		a.npPull = 0
	}
	a.npHeld = held

	target := float32(0)
	if open {
		target = 1
	}
	var t, shift float32
	if reduce {
		t = layer.AnimateTo("open", target, npFade)
	} else {
		t = layer.AnimateTo("open", target, ui.Animation{Spring: spring})
	}
	if held {
		// Pulled up, it gives less and less.
		shift = layer.Jump("pull", ui.Rubberband(a.npPull, 0, h, h))
	} else {
		shift = layer.AnimateTo("pull", 0, ui.Animation{Spring: npSpring})
	}
	fresh, was := a.npT <= 0 && t > 0, a.npT
	if reduce {
		// Fading, its first frame is the one its fade starts in, still clear:
		// laid out there, it shows from the next, not a frame later.
		fresh = was <= 0 && t <= 0 && open
	}
	a.npT, a.npShift = t, shift
	if a.npAtRest(open) != settled {
		// Another build, for the page under it to come or go, and the
		// bar's cover with it.
		c.AnimationFrame()
	}
	// Back in the bar, it shows a frame more, its cover over the bar's:
	// the bar, built before it, hid its own cover in this frame, and
	// shows it from the next.
	if t <= 0 && !open && was <= 0 || a.pl.Current() == nil {
		// Closed: nothing to paint or click.
		layer.Invisible()
		a.queueAt, a.wallState.track = -1, ""
		return
	}
	switch {
	case fresh:
		// Its first frame lays it out, for what flies to its place to know
		// where that is; it shows from the next.
		layer.Opacity(0)
		c.AnimationFrame()
	case slide:
		// It follows a pull down, as it does growing out of the cover.
		if k := 1 - clamp01(t); k > 0 || shift != 0 {
			layer.Translate(0, h*k+shift)
		}
	case a.npStill:
		if t < 1 {
			layer.Opacity(max(t, 0))
		}
	default:
		a.npCard(layer, t, shift, w, h)
	}
	if !open {
		// On its way out, the window under it takes the clicks.
		layer.PassThrough()
	}
	layer.Children(func() { a.nowPlayingView(c) })
}

// npCard clips the layer to the card Now Playing is, t of the way open,
// from the player bar's cover to the window, and moves it shift down, a
// little smaller and rounder for it.
func (a *app) npCard(layer ui.Element, t, shift, w, h float32) {
	k := 1 - clamp01(t) // how near the bar's cover it is
	pulled := clamp01(shift / h)
	if k == 0 && shift == 0 {
		return
	}
	from := a.npFromRect(w, h)
	radius := npFromRadius*k + 28*clamp01(shift/120)
	layer.ClipInset(from.Y*k, (w-from.X-from.W)*k, (h-from.Y-from.H)*k, from.X*k, radius)
	if shift != 0 {
		s := 1 - 0.12*pulled
		layer.TransformOrigin(0.5, 0).Scale(s, s).Translate(0, shift)
	}
}

// npFromRect is the player bar's cover, in a window w by h, that Now
// Playing grows out of: where the bar last showed it, or where the bar
// would, before it has.
func (a *app) npFromRect(w, h float32) ui.Rect {
	if a.npFrom.W > 0 {
		return a.npFrom
	}
	return ui.Rect{X: sidebarWidth + 21, Y: h - (playerBarHeight+48)/2, W: 48, H: 48}
}

// npMorph moves e, a cover of Now Playing, out of the player bar's cover
// to its place as Now Playing opens, and back into it as it closes: the
// song in the bar grows into the view.
func (a *app) npMorph(c *ui.Context, e ui.Element) ui.Element {
	if a.npT >= 1 || a.npStill {
		return e
	}
	b := e.LayoutBounds()
	if b.W <= 0 {
		// Not laid out yet: the next frame places it.
		c.AnimationFrame()
		return e.Opacity(0)
	}
	w, h := c.Size()
	from := a.npFromRect(w, h)
	k := 1 - a.npT
	s := 1 + (from.W/b.W-1)*k
	return e.TransformOrigin(0, 0).Scale(s, s).Translate((from.X-b.X)*k, (from.Y-b.Y)*k)
}

// npBreathe scales e, a cover that breathes, to scale: there as a pose
// moves it, while Now Playing is open, and at once on its way to or from
// the bar, coming to its full size as it gets there, the size of the
// bar's cover it takes the place of.
func (a *app) npBreathe(e ui.Element, scale float32) {
	if a.npT < 1 && !a.npStill {
		s := lerp(1, scale, clamp01(a.npT))
		e.Scale(s, s)
		return
	}
	e.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.25}})
}

// npRadius is the corner radius, r at rest, of a cover size across that
// npMorph moves: its corners, shrinking with it, stay as round on screen
// as they are on its way to the bar's cover's, and are that round as it
// gets there, rather than going sharp and rounding at once as the bar's
// cover takes its place.
func (a *app) npRadius(c *ui.Context, r, size float32) float32 {
	if a.npT >= 1 || a.npStill {
		return r
	}
	w, h := c.Size()
	k := 1 - clamp01(a.npT)
	s := 1 + (a.npFromRect(w, h).W/size-1)*k // as npMorph scales it
	return min(lerp(r, npFromRadius, k)/s, size/2)
}

// npFromRadius is the corner radius of the player bar's cover.
const npFromRadius = 8

// npRise fades e in and lifts it into place as Now Playing opens, once the
// cover is on its way: the nth of the things that do, one after another,
// going again the other way as it closes.
func (a *app) npRise(e ui.Element, n int) ui.Element {
	if a.npT >= 1 || a.npStill {
		return e
	}
	from := 0.3 + 0.07*float32(n)
	k := smoothstep(from, from+0.4, a.npT)
	return e.Opacity(k).Translate(0, 24*(1-k))
}

// nowPlayingView fills the window with what plays: its cover over a slow
// drift of its own colors, the controls, and lyrics that follow the song.
func (a *app) nowPlayingView(c *ui.Context) {
	pl := a.pl
	tr := pl.Current()
	w, h := c.Size()
	art := a.art.get(sized(tr.Album.ImageURL, 600), true)
	// The colors stay the last song's while its cover loads, rather than
	// going gray between the two.
	shown := art
	if art == nil && tr.Album.ImageURL != "" {
		shown = a.npArt
	}
	a.npArt = shown
	tint := ui.RGB(120, 120, 130)
	if shown != nil && !shown.failed {
		tint = shown.tint
	}
	accent := tint.Mix(white, 0.45)

	// Near the bar, the card is only its cover, the rest gone clear.
	bg := ui.Hex("#0b0b0d")
	if t := a.npT; t < 1 && !a.npStill {
		bg = bg.Alpha(smoothstep(0, 0.35, t))
	}
	root := ui.Box(c.Key("now-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(bg).TextColor(white)
	// The pointer is over the view whenever it is in the window.
	pointerIn := root.Hovered()
	root.Children(func() {
		// Its colors bloom out of the bar's cover as it opens.
		bloom := ui.Box(c.Key("bloom")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough()
		if t := a.npT; t < 1 && !a.npStill {
			from := a.npFromRect(w, h)
			s := 1 + 0.3*(1-clamp01(t))
			bloom.Opacity(smoothstep(0.05, 0.55, t)).
				TransformOrigin((from.X+from.W/2)/w, (from.Y+from.H/2)/h).Scale(s, s)
		}
		bloom.Children(func() { a.backdrop(c, shown, tint) })
		// Over the backdrop, which is positioned too: positioned elements
		// paint over those in the flow.
		// The bodies of the panes in a layer of their own, over the
		// backdrop: one leaving goes under its siblings, the backdrop too
		// were it one.
		ui.Box(c.Key("np-bodies")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() {
			body := ui.Column(c.Key(npBodyKind(a.npPane))).Absolute().Top(0).Left(0).Right(0).Bottom(0)
			a.paneFade(c, body)
			body.Children(func() { a.nowPlayingBody(c, tr, art, accent, w, h, pointerIn) })
		})
		a.npChrome(c, w, pointerIn)
	})
}

// npBodyKind is the body of Now Playing that shows pane: the stage's, the
// wall's, or the cover's beside the lyrics or Up Next.
func npBodyKind(pane int) string {
	switch pane {
	case paneStage:
		return "stage-body"
	case paneWall:
		return "wall-body"
	}
	return "cover-body"
}

// paneFade crosses e, a pane of Now Playing or the body that holds it, over
// to the one taking its place as another tab is chosen, once Now Playing is
// open; its way in is its own. The one leaving fades slowly at first as the
// other comes quickly, so that the backdrop hardly shows between them. With
// less motion, they change at once.
func (a *app) paneFade(c *ui.Context, e ui.Element) {
	if a.npT < 1 || c.Preferences().ReduceMotion {
		return
	}
	e.Transition(ui.ElementTransition{Duration: 180 * time.Millisecond, Enter: &ui.Motion{Opacity: 0}, Exit: &ui.Motion{Opacity: 0}, ExitEase: ui.EaseInOut})
}

// npChrome is what every pane of Now Playing has in the same place, over
// whichever shows, and which stays as they cross: the top strip, which
// drags the window, with the chevron that closes the view, the tabs, and
// what the lyrics show under their lines. They show only while the pointer
// is in the window.
func (a *app) npChrome(c *ui.Context, w float32, pointerIn bool) {
	a.npRise(ui.Row(c.Key("top-strip")).Absolute().Top(0).Left(0).Right(0).Height(topBar(c)).Justify(ui.Center).AlignItems(ui.Center).DragWindow(), 0).Children(func() {
		reveal(ui.Box(c.Key("close-reveal")), pointerIn).Children(func() {
			if bareIconButton(c.Key("close"), iconDown, 22, white.Alpha(0.7), T("Close")).Clicked() {
				a.nowPlaying = false
			}
		})
	})
	a.npRise(reveal(npTabsRow(c, w), pointerIn), 0).Children(func() {
		a.paneTabs(c)
		if a.npPane == paneStage {
			a.stageStyleButton(c)
		}
	})
	// Level with the tabs, at the other side: over the lyrics, and where
	// the stage and the wall end, which show them too.
	if a.npPane != paneQueue {
		a.npRise(reveal(ui.Row(c.Key("lyrics-tools")).Absolute().Top(topBar(c)).Right(npSideX(w)).Height(npTabsHeight).AlignItems(ui.Center), pointerIn), 0).Children(func() {
			a.lyricsExtraButton(c)
		})
	}
}

func (a *app) nowPlayingBody(c *ui.Context, tr *music.Track, art *artwork, accent ui.Color, w, h float32, pointerIn bool) {
	pl := a.pl
	if a.npPane != paneQueue {
		a.queueAt = -1
	}
	if a.npPane != paneWall {
		a.wallState.track = ""
	}
	{
		// Room for the top strip, which is over the body (npChrome).
		ui.Box(c).Height(topBar(c))
		if a.npPane == paneStage {
			// The stage takes the whole view: the lyrics are the show, the
			// controls a slim bar at the foot.
			a.stage(c, tr, accent)
			a.stageBar(c, tr, art, accent)
			return
		}
		if a.npPane == paneWall {
			// The wall takes the whole view too, under the song playing.
			a.wallPane(c, tr, art, accent, w, h, pointerIn)
			return
		}

		side := float32(math.Min(float64(w)*0.36, 440))
		// The cover leaves room above it for the tabs, starting as far
		// under them as the wall's song does.
		coverRoom := h - 436
		coverSize := min(side, coverRoom)
		// In a window taller than the cover grows, the column is shorter
		// than the window by what the cover falls short of its room, and,
		// centred, sits half of that lower.
		coverSunk := (coverRoom - coverSize) / 2
		// The column is as wide as the cover, for the cover, what plays
		// and the tabs over it to start at one edge, but wide enough for
		// the controls.
		side = max(coverSize, 280)
		padX := npSideX(w)
		ui.Row(c).Grow(1).Padding(0, padX, 44, padX).Gap(72).AlignItems(ui.Stretch).Children(func() {
			var coverTop float32
			ui.Column(c).Width(side).Justify(ui.Center).Gap(0).Children(func() {
				coverTop = ui.Box(c.Key("cover-top")).Height(0).LayoutBounds().Y
				// The cover flies out of the bar's as Now Playing opens. Kept
				// smaller than the column by a short window, it centers on
				// what's under it.
				coverAlign := ui.Start
				if coverSize < side {
					coverAlign = ui.Center
				}
				a.npMorph(c, ui.Box(c.Key("cover-morph")).Size(coverSize, coverSize).AlignSelf(coverAlign)).Children(func() {
					a.npCover(c, tr, art, coverSize)
				})

				// Every row of the column starts at the cover's left and ends
				// at its right. The station first, over what plays, and beside
				// that the buttons that keep it, level with its two lines.
				a.npRise(ui.Column(c).Margin(32, 0, 14, 0), 1).Children(func() {
					if r := a.station(); r != "" {
						a.radioChip(c, r)
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						names := ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Margin(0, 6, 0, 0)
						names.Children(func() {
							fadingTitle(c, tr.Name, 23, 700, names.LayoutBounds().W)
							a.artistLinks(c, tr, white.Alpha(0.62), white, true).FontSize(16).FontWeight(500).SingleLine()
						})
						a.likeButtonAs(c.Key("like"), tr, 16, white.Alpha(0.92), glassLook)
						a.songMenuAs(c.Key("more"), tr, 16, white.Alpha(0.92), glassLook)
					})
				})
				// Taller than it looks, for the pointer, over the rows beside it.
				a.npRise(a.scrubber(c.Key("np-scrub"), &a.npScrub, glassBar(6, 10), 24, accent.Mix(white, 0.35)).Margin(-3, 0), 2)
				// As tall with the quality's badge as without it: the badge goes
				// while the next song loads, and the column would move with it.
				a.npRise(ui.Row(c).Height(18).Margin(4, 0, 0, 0).AlignItems(ui.Center), 2).Children(func() {
					at := a.npScrub.at(pl)
					scrubTime(c.Key("elapsed"), clock(at), a.npScrub.active).FontSize(11.5)
					ui.Spacer(c)
					glassBadge(c, pl.Stream())
					ui.Spacer(c)
					status := "-" + clock(pl.Duration()-at)
					if pl.Loading() {
						status = T("Buffering…")
					}
					scrubTime(c.Key("left"), status, a.npScrub.active).FontSize(11.5)
				})
				a.npRise(ui.Box(c).Margin(16, 0, 11, 0), 3).Children(func() {
					a.transport(c, transportLook{color: white, on: glassInk(accent), size: 28, glass: true, spread: true})
				})
				a.npRise(ui.Box(c), 4).Children(func() {
					a.volumeSlider(c, glassBar(4, 7).dimmed(), white.Alpha(0.6))
				})
			})

			pane := ui.Column(c.Key("pane")).Grow(1).Basis(0).MinWidth(0)
			a.npRise(pane, 2)
			pane.Children(func() {
				// The pane starts level with the cover beside it, but below the
				// tabs, which start level with the pane's top: they come and go
				// with the pointer, so they sit out of the flow, never over it.
				// In a tall window it starts where the cover would, as tall
				// as its room: level with the cover sunk in the middle, it
				// left the top of the window empty, cutting off the lines
				// before the one sung.
				b := pane.LayoutBounds()
				if b.H > 0 && coverTop > 0 {
					if top := max(coverTop-coverSunk-b.Y, npTabsHeight+8); top != a.npPaneTop {
						a.npPaneTop = top
						c.AnimationFrame()
					}
				} else {
					c.AnimationFrame() // not laid out yet: the next frame places it
				}
				ui.Box(c).Height(a.npPaneTop)
				// The lyrics and Up Next cross beside the cover, which stays.
				inner := ui.Column(c.Key(a.npPane)).Grow(1).Basis(0).MinHeight(0)
				a.paneFade(c, inner)
				inner.Children(func() {
					switch a.npPane {
					case paneLyrics:
						a.lyricsPane(c, tr, h)
					default:
						a.queuePane(c)
					}
				})
			})
		})
	}
}

// npMaxWidth is how wide the cover and the pane beside it go, at most.
const npMaxWidth = 1280

// npSideX is how far in from the sides of a view w wide the cover and the
// pane beside it are: in a wide window they keep together in the middle,
// rather than the pane taking all the width the cover leaves.
func npSideX(w float32) float32 {
	return max(72, (w-npMaxWidth)/2)
}

// npTabsHeight is how tall the row of Now Playing's tabs is, for what is
// under it to start below it.
const npTabsHeight = 28

// npTabsRow is the row of Now Playing's tabs, in the same place for every
// pane, so that they stay put going from one to another: at the top, level
// with the cover's left. They start from the left, so that only the last,
// the comments, moves as its count changes.
func npTabsRow(c *ui.Context, w float32) ui.Element {
	return ui.Row(c.Key("tabs")).Absolute().Top(topBar(c)).Left(npSideX(w)).Height(npTabsHeight).Gap(6).AlignItems(ui.Center)
}

// reveal fades e in while the pointer is in the window, and out when it
// leaves.
func reveal(e ui.Element, shown bool) ui.Element {
	pose := ui.Pose{Fade: 1}
	if shown {
		pose = ui.Pose{}
	}
	return e.AnimatePose(pose, ui.Animation{Duration: 200 * time.Millisecond})
}

// lyricsPaneChosen is the pane of lyrics the listener chose last: what
// the app starts on, before the player is made.
func (a *app) lyricsPaneChosen() int {
	if a.settings.LyricsStage {
		return paneStage
	}
	return paneLyrics
}

// The panes of Now Playing.
const (
	paneLyrics = iota
	paneQueue
	paneStage
	paneWall
)

// npTab is a tab of Now Playing: its pane, and what it says.
type npTab struct {
	pane  int
	label string
}

// npTabs returns the tabs of Now Playing, in order, for what plays: ⌘1
// chooses the first, ⌘2 the second, and on.
func (a *app) npTabs() []npTab {
	return []npTab{{paneLyrics, T("Lyrics")}, {paneStage, T("Stage")}, {paneQueue, T("Up Next")}, {paneWall, T("Wall")}}
}

func (a *app) paneTabs(c *ui.Context) {
	for _, t := range a.npTabs() {
		a.paneTab(c, t.pane, t.label)
	}
}

// choosePane shows pane i in Now Playing, remembering lyrics or the stage,
// whichever was chosen, for the next song.
func (a *app) choosePane(i int) {
	a.npPane = i
	if (i == paneLyrics || i == paneStage) && a.settings.LyricsStage != (i == paneStage) {
		a.settings.LyricsStage = i == paneStage
		a.settings.save()
	}
}

// radioChip says which radio plays, over the title, on a slip of glass: a
// click on it, or on its cross, leaves the radio, playing on the song.
func (a *app) radioChip(c *ui.Context, name string) {
	exit := Tf("Leave %s", radioLabel(name))
	chip := ui.ButtonBase(c.Key("radio-chip")).Height(22).Gap(5).Padding(0, 7, 0, 8).Margin(0, 0, 8, 0).Radius(999).
		AlignItems(ui.Center).AlignSelf(ui.Start).FocusRing(false).Label(exit).Tooltip(exit)
	tint, fg, cross := ui.Transparent, white.Alpha(0.78), float32(0.55)
	switch {
	case chip.Pressed():
		tint, fg, cross = glassPress, white, 1
	case chip.Hovered():
		tint, fg, cross = glassHover, white, 1
	}
	glassOn(c, chip, tint, true).TextColor(chip.AnimateColor("fg", fg, ui.Animation{Duration: hoverFade}))
	chip.Children(func() {
		ui.Icon(c, iconRadio).Size(12, 12)
		ui.Text(c, radioLabel(name)).FontSize(11.5).FontWeight(600).LetterSpacing(0.2).SingleLine()
		ui.Icon(c, iconX).Size(10, 10).Opacity(cross)
	})
	if chip.Clicked() {
		a.exitRadio()
	}
}

// qualityBadge marks a lossless stream, as it plays, with the badge of an
// album's page (traitBadge): Hi-Res Lossless past 48 kHz, as Apple Music
// has it, and Lossless up to that, or where the rate isn't known, whichever
// level it was asked at. A song's Dolby Atmos version, played as either of
// its stereo versions, has Dolby Atmos's. Its tooltip says what the stream
// is. The other AAC has none.
func qualityBadge(c *ui.Context, st *music.Stream, color ui.Color) {
	if label := qualityLabel(st); label != "" {
		traitBadge(c, label, color).Shrink(0).Tooltip(streamSpec(st))
	}
}

// qualityLabel is what the badge of a stream says, or "" for one without a
// badge: Dolby Atmos, lossless, or Hi-Res above 48 kHz.
func qualityLabel(st *music.Stream) string {
	switch {
	case st == nil:
		return ""
	case music.IsAtmos(st.Quality):
		return traitAtmos
	case !music.IsLossless(st.Quality):
		return ""
	case st.SampleRate > 48000:
		return traitHiRes
	}
	return traitLosless
}

// streamSpec says what a stream with a badge is, as 24-bit/96 kHz ALAC, or
// a Dolby Atmos version's as binaural rendering, AAC, 256 kbps; "" where
// the source doesn't know.
func streamSpec(st *music.Stream) string {
	if music.IsAtmos(st.Quality) {
		how := T("Binaural rendering")
		if st.Quality == music.AtmosQualities[music.AtmosSpeakers] {
			how = T("Stereo downmix")
		}
		if st.BitRate <= 0 {
			return how + " · AAC"
		}
		return Tf("%s · AAC · %d kbps", how, st.BitRate/1000)
	}
	if st.SampleRate <= 0 || st.BitDepth <= 0 {
		return ""
	}
	khz := strconv.FormatFloat(float64(st.SampleRate)/1000, 'f', -1, 64)
	return Tf("%d-bit/%s kHz ALAC", st.BitDepth, khz)
}

// stageBar is the slim bar along the foot of the stage: what plays, where
// it is, and the controls.
func (a *app) stageBar(c *ui.Context, tr *music.Track, art *artwork, accent ui.Color) {
	pl := a.pl
	ui.Column(c).Padding(0, 48, 28, 48).Gap(10).Children(func() {
		a.npRise(a.scrubber(c.Key("stage-scrub"), &a.npScrub, glassBar(5, 8), 16, accent.Mix(white, 0.35)), 2)
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
				if bmp := bitmap(art); bmp != nil {
					cov := a.npMorph(c, ui.Box(c.Key(tr.ID)).Size(44, 44))
					in := artIn(cov, bmp)
					cov.Children(func() {
						r := a.npRadius(c, 8, 44)
						artImage(c, bmp, r, in)
						a.motionOver(c, cov, songMoving(tr), 44, 44, motionThumb, r)
					})
				}
				// All but the cover, which flies to the bar's, fade as the
				// rest of Now Playing does.
				a.npRise(ui.Row(c).Shrink(1).MinWidth(0).Gap(12).AlignItems(ui.Center), 3).Children(func() {
					ui.Column(c).Shrink(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, tr.Name).FontSize(14).FontWeight(700).SingleLine()
						a.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12).SingleLine()
					})
					a.likeButtonAs(c.Key("like"), tr, 14, white.Alpha(0.92), glassLook)
					a.songMenuAs(c.Key("more"), tr, 14, white.Alpha(0.92), glassLook).Margin(0, 0, 0, -4)
				})
			})
			a.npRise(ui.Row(c), 3).Children(func() {
				a.transport(c, transportLook{color: white, on: glassInk(accent), size: 22, glass: true})
			})
			a.npRise(ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End), 4).Children(func() {
				scrubTime(c.Key("times"), clock(a.npScrub.at(pl))+" / "+clock(pl.Duration()), a.npScrub.active).FontSize(11.5)
			})
		})
	})
}

func (a *app) paneTab(c *ui.Context, i int, label string) {
	b := ui.ButtonBase(c.Key(i)).Padding(6, 14).Radius(999).FocusRing(false).Label(label)
	bg, fg := ui.Transparent, white.Alpha(0.55)
	if a.npPane == i {
		bg, fg = white.Alpha(0.14), white
	} else if b.Hovered() {
		bg = white.Alpha(0.06)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() { ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(fg) })
	if b.Clicked() {
		a.choosePane(i)
	}
}

// queueItems is songItems for a row of the queue, with what takes it out of
// the queue first if it is still to play, as Apple Music's Up Next has.
func (a *app) queueItems(tr *music.Track, i int, toPlay bool) func(m *ui.Menu) {
	rest := a.songItems(tr, "")
	if !toPlay {
		return rest
	}
	return func(m *ui.Menu) {
		// The queue may have changed while the menu was open: only the
		// song the menu was opened on goes.
		if q := a.pl.Queue(); m.Item(T("Remove from Up Next")).Chosen() && i < len(q) && q[i].ID == tr.ID {
			a.pl.Remove(i)
		}
		m.Separator()
		rest(m)
	}
}

// queuePane lists the whole queue in play order, the track playing marked
// among the others: a click plays one there, and the list stays as it is.
// AutoPlay ends it, on glass.
func (a *app) queuePane(c *ui.Context) {
	pl := a.pl
	order, at := pl.PlayOrder()
	// Both as they are now: a song removed as the rows are built (Remove)
	// leaves these as they were, and the rows too.
	tracks := pl.Queue()
	// Opened, it shows the track playing at its top; after, it follows it
	// as little as keeps it in view.
	if at >= 0 && at != a.queueAt {
		if a.queueAt < 0 {
			a.queueList.ScrollTo(at, ui.Start)
		} else {
			a.queueList.ScrollIntoView(at)
		}
		a.queueAt = at
	}
	// A row dragged to a place among those still to play moves there; one
	// dragged to or from any other, or past the end, goes back where it was,
	// as the list shows nothing of: the player, which says no, is the rule.
	a.queueList.Reorder = func(rows []int, to int) {
		if len(rows) != 1 || rows[0] >= len(order) {
			return
		}
		from, to := rows[0], min(to, len(order))
		if to > from {
			to--
		}
		pl.Move(order[from], to)
	}
	a.queueList.Key = func(row int) any {
		if row >= len(order) {
			return "autoplay"
		}
		return order[row]
	}
	queue := ui.List(c.Key("queue"), &a.queueList, len(order)+1, func(r int) {
		if r == len(order) {
			a.queueFoot(c)
			return
		}
		i := order[r]
		tr := &tracks[i]
		playing := r == at
		row := ui.ButtonBase(c.Key(i)).Height(58).PaddingX(10).Gap(14).Radius(12).AlignItems(ui.Center).FocusRing(false).Label(tr.Name).
			ContextMenu(a.queueItems(tr, i, r > at))
		switch {
		case playing:
			row.Background(white.Alpha(0.12))
		case row.Hovered():
			row.Background(white.Alpha(0.08))
		}
		if tr.Unavailable {
			row.Opacity(0.4)
		}
		row.Children(func() {
			a.cover(c, tr.Album.ImageURL, songMoving(tr), 96, 42, 7)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, tr.Name).FontSize(14).FontWeight(600).SingleLine()
				a.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12.5).SingleLine()
			})
			ui.Text(c, clock(tr.Length())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.4))
		})
		if row.Clicked() && !tr.Unavailable && !playing {
			pl.Jump(i)
		}
	}).Grow(1).Padding(8, 0, 40, 0).Gap(2)
	// Following the track playing isn't the listener scrolling: what it
	// shows then is another thing (queueAt).
	w, h := c.Size()
	place := barPlace{of: [4]float32{float32(len(order)), float32(a.queueAt), w, h}}
	place.first, place.last = a.queueList.Visible()
	a.bars.quiet(c, queue, "queue", place)
}
