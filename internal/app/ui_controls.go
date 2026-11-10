package app

import (
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
	"ringo/internal/player"
)

// The controls over a cover's colors, in Now Playing, are of two kinds.
// What plays, the buttons before and after it and the speaker are bare
// glyphs in white, lit by a soft disc under the pointer. What is beside
// them, adding the song, its menu, shuffling and repeating, the station
// and how the song sounds, is on glass: Liquid Glass, as the buttons of
// Apple Music's own player are, which lets the cover's colors through,
// bent and lit along its rim.

// The tints of Now Playing's glass: under the pointer, pressed, and lit,
// as a button that is on. At rest it has none.
var (
	glassHover = white.Alpha(0.1)
	glassPress = white.Alpha(0.2)
	glassLit   = white.Alpha(0.92)
)

// glassFlat is how much white glass is where the desktop asks for less
// transparency, under its tint.
const glassFlat = 0.12

// pressSpring is how a bare button of Now Playing gives as it is pressed
// and comes back as it is let go, a little past where it was.
var pressSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond, Bounce: 0.32}}

// buttonLook is how a place draws its icon buttons: the button of an icon,
// and how far it reaches past the icon at each side.
type buttonLook struct {
	pad  float32
	icon func(c *ui.Context, icon *ui.SVG, size float32, color ui.Color, label string) ui.Element
}

// glassPad is how far a disc of glass reaches past its icon.
const glassPad = 7

var (
	// plainLook is a page's, and the player bar's: iconButton.
	plainLook = buttonLook{8, iconButton}
	// glassLook is Now Playing's: glassButton.
	glassLook = buttonLook{glassPad, glassButton}
)

// glassButton is an icon button on a disc of glass, which lights under the
// pointer and grows a little as it is pressed.
func glassButton(c *ui.Context, icon *ui.SVG, size float32, color ui.Color, label string) ui.Element {
	b := glassDisc(c, size+2*glassPad, false, label).TextColor(color)
	b.Children(func() { ui.Icon(c, icon).Size(size, size) })
	return b
}

// glassToggle is a glassButton that is on or off: on, its disc is lit,
// and its icon is cut out of it in ink, the dark of what is under it.
func glassToggle(c *ui.Context, icon *ui.SVG, size float32, on bool, color, ink ui.Color, label string) ui.Element {
	b := glassDisc(c, size+2*glassPad, on, label)
	if on {
		color = ink
	}
	b.TextColor(b.AnimateColor("ink", color, ui.Animation{Duration: hoverFade}))
	b.Children(func() { ui.Icon(c, icon).Size(size, size) })
	return b
}

// glassDisc is the disc of a glass button, across wide.
func glassDisc(c *ui.Context, across float32, lit bool, label string) ui.Element {
	b := ui.ButtonBase(c).Size(across, across).Shrink(0).Center().Radius(999).Label(label)
	tint := ui.Transparent
	switch {
	case lit:
		tint = glassLit
	case b.Pressed():
		tint = glassPress
	case b.Hovered():
		tint = glassHover
	}
	return glassOn(c, b, tint, true)
}

// glassOn makes e a pane of glass, tinted with tint, which it goes to
// over a moment: clear glass, which lightens what is under it whatever
// the theme, as Now Playing is dark in both. Pressed, a pane that is a
// button grows a little, as AppKit's do. Where the desktop asks for less
// transparency, it is a flat tint.
func glassOn(c *ui.Context, e ui.Element, tint ui.Color, button bool) ui.Element {
	tint = e.AnimateColor("glass", tint, ui.Animation{Duration: hoverFade})
	if c.Preferences().ReduceTransparency {
		// The tints are white, as the flat glass is: one over the other.
		return e.Background(white.Alpha(1 - (1-glassFlat)*(1-float32(tint.A)/255)))
	}
	return e.Material(glass.Glass{Style: glass.Clear, Tint: tint, Interactive: button})
}

// glyphButton is a bare icon button over a cover's colors: a disc of
// light shows under the pointer, pad past the icon at each side, and it
// gives as it is pressed.
func glyphButton(c *ui.Context, icon *ui.SVG, size, pad float32, color ui.Color, label string) ui.Element {
	b := glyphDisc(c, size, pad, color, label)
	b.Children(func() { ui.Icon(c, icon).Size(size, size) })
	return b
}

// glyphDisc is the button of glyphButton, without its icon.
func glyphDisc(c *ui.Context, size, pad float32, color ui.Color, label string) ui.Element {
	b := ui.ButtonBase(c).Size(size+2*pad, size+2*pad).Shrink(0).Center().Radius(999).Label(label).TextColor(color)
	bg, scale := ui.Transparent, float32(1)
	switch {
	case b.Pressed():
		bg, scale = white.Alpha(0.16), 0.88
	case b.Hovered():
		bg = white.Alpha(0.09)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: hoverFade}))
	b.AnimatePose(ui.Pose{Scale: scale}, pressSpring)
	return b
}

// transportLook is how a row of the controls looks: its icons' color, that
// of shuffle and repeat off and on, and the size the icons are sized from.
// On glass, over a cover's colors, shuffle and repeat are glass buttons,
// lit while they are on, their icons in on; spread, the row takes its
// place's width, shuffle at its start and repeat at its end.
type transportLook struct {
	color, dim, on ui.Color
	size           float32
	glass, spread  bool
}

// transport is the row of controls: shuffle, previous, play, next, repeat.
func (a *app) transport(c *ui.Context, l transportLook) {
	pl := a.pl
	row := ui.Row(c).AlignItems(ui.Center).Justify(ui.Center)
	switch {
	case l.spread:
		row.Justify(ui.SpaceBetween).AlignSelf(ui.Stretch)
	case l.glass:
		row.Gap(l.size * 0.55)
	default:
		row.Gap(l.size * 0.5)
	}
	toggle := func(c *ui.Context, icon *ui.SVG, on bool, label string) ui.Element {
		if l.glass {
			return glassToggle(c, icon, l.size*0.57, on, l.color.Alpha(0.85), l.on, label)
		}
		color := l.dim
		if on {
			color = l.on
		}
		return iconButton(c, icon, l.size*0.62, color, label)
	}
	step := func(c *ui.Context, icon *ui.SVG, label string) ui.Element {
		if l.glass {
			return glyphButton(c, icon, l.size*1.05, l.size*0.4, l.color, label)
		}
		return iconButton(c, icon, l.size*0.85, l.color, label)
	}
	row.Children(func() {
		if toggle(c.Key("shuffle"), iconShuffle, pl.Shuffle(), T("Shuffle")).Clicked() {
			pl.ToggleShuffle()
		}
		if step(c.Key("prev"), iconPrev, T("Previous")).Clicked() {
			pl.Prev()
		}
		var play ui.Element
		if l.glass {
			play = glyphDisc(c.Key("play"), l.size*1.3, l.size*0.42, l.color, T("Play/Pause"))
			play.Children(func() { playPause(c, play, pl.Playing(), l.size*1.3) })
		} else {
			play = playButton(c.Key("play"), pl.Playing(), l.size*1.25, l.color)
		}
		if play.Clicked() {
			pl.TogglePlay()
		}
		if step(c.Key("next"), iconNext, T("Next")).Clicked() {
			pl.Next()
		}
		ricon := iconRepeat
		if pl.Repeat() == player.RepeatOne {
			ricon = iconRepeat1
		}
		if toggle(c.Key("repeat"), ricon, pl.Repeat() != player.RepeatOff, T("Repeat")).Clicked() {
			pl.CycleRepeat()
		}
	})
}

// barLook is how a bar that fills looks, the progress or the volume: its
// track and what fills it, and how thick it is at rest and under the
// pointer or dragged, where what fills it brightens to bright.
type barLook struct {
	track, fill, bright ui.Color
	thin, thick         float32
}

// grown returns how thick a bar of the look is on s, a slider hot or not,
// and how far along it is from thin to thick. It is quick out of the
// blocks: a spring starts still, and the bar grew only a few frames after
// the pointer came.
func (l barLook) grown(s ui.Element, hot bool) (thick, k float32) {
	to := float32(0)
	if hot {
		to = 1
	}
	k = s.AnimateTo("grown", to, ui.Animation{Duration: 160 * time.Millisecond, Ease: ui.EaseOut})
	return lerp(l.thin, l.thick, k), k
}

// draw fills the bar along the middle of r, thick, to frac of the way, k
// of the way to bright: a tube part full, its ends round and what fills it
// cut straight where it ends.
func (l barLook) draw(p *ui.Painter, r ui.Rect, thick, frac, k float32) {
	bar := ui.Rect{X: r.X, Y: r.Y + (r.H-thick)/2, W: r.W, H: thick}
	p.Fill(bar, l.track, thick/2)
	p.Clip(bar, thick/2, func() {
		p.Fill(ui.Rect{X: bar.X, Y: bar.Y, W: bar.W * frac, H: thick}, l.fill.Mix(l.bright, k), 0)
	})
}

// glassBar is the look of a bar over a cover's colors: white, a little
// let through until the pointer is on it.
func glassBar(thin, thick float32) barLook {
	return barLook{track: white.Alpha(0.2), fill: white.Alpha(0.86), bright: white, thin: thin, thick: thick}
}

// dimmed is the look a shade quieter at rest, for the bar that isn't the
// song's: the volume under the progress.
func (l barLook) dimmed() barLook {
	l.track, l.fill = white.Alpha(0.16), white.Alpha(0.58)
	return l
}

// glassInk is the color of an icon cut out of a lit disc of glass, over
// the colors of a cover whose accent is accent: their dark.
func glassInk(accent ui.Color) ui.Color {
	return accent.Mix(ui.RGB(0, 0, 0), 0.78)
}

// glassBadge marks how a stream sounds, as qualityBadge does, on a slip
// of glass.
func glassBadge(c *ui.Context, st *music.Stream) {
	if label := qualityLabel(st); label != "" {
		badge := ui.Text(c.Key(label), T(label)).FontSize(9.5).FontWeight(700).LetterSpacing(0.3).TextColor(white.Alpha(0.82)).SingleLine().
			Padding(2.5, 7).Radius(999).Shrink(0).Tooltip(streamSpec(st))
		glassOn(c, badge, ui.Transparent, false)
	}
}

// scrubTime is a time beside a progress bar, which brightens while the
// bar is dragged: it says where to, then.
func scrubTime(c *ui.Context, text string, dragged bool) ui.Element {
	color := white.Alpha(0.55)
	if dragged {
		color = white
	}
	t := ui.Text(c, text).FontWeight(500).FontFeatures("tnum").SingleLine()
	return t.TextColor(t.AnimateColor("lit", color, ui.Animation{Duration: hoverFade}))
}

// fadingTitle is a name on one line, size tall and of weight, in a place
// room wide: one too long for it fades out at its end, rather than ending
// in dots, and says the rest under the pointer.
func fadingTitle(c *ui.Context, name string, size float32, weight int, room float32) ui.Element {
	const spacing = -0.015 // of its size, between letters
	t := ui.Text(c, name).FontSize(size).FontWeight(weight).LetterSpacing(spacing * size)
	if room <= 0 {
		// Not laid out yet: the next frame knows its room.
		c.AnimationFrame()
		return t.SingleLine()
	}
	w, _ := c.MeasureText(0, ui.Span{Text: name, Size: size, Weight: weight})
	if w += spacing * size * float32(utf8.RuneCountInString(name)); w <= room {
		return t.SingleLine()
	}
	// Whole on its line, past its place, where the mask takes it away.
	black := ui.RGB(0, 0, 0)
	return t.NoWrap().MinWidth(0).Tooltip(name).MaskLinearGradient(ui.LinearGradient{Angle: 90,
		Stops: []ui.GradientStop{{Color: black, At: 0}, {Color: black, At: 1 - 2*size/room}, {Color: ui.Transparent, At: 1}}})
}
