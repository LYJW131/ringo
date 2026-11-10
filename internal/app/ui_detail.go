package app

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// What the pages of an album and a playlist share, as Apple Music lays them
// out: a header of the cover beside the title, who made it, what the editors
// say of it and the buttons that play it; the songs in rows; and after
// them, the sections of the page, as shelves. A page whose picture gives a
// color is tinted with it, fading down to the page's own.

// detailCover is the size of the cover heading an album's or a playlist's
// page.
const detailCover = 232

// tintHeight is how far down the window a page's tint fades out.
const tintHeight = 720

// detailList lists the n rows of a page, which row builds, under its bars,
// tinted from the top of the window with tint if it has one.
func (a *app) detailList(c *ui.Context, n int, tint music.Colors, row func(i int)) {
	top, bottom := a.insets(c)
	// Rows out of view are not built, their marks with them: a list
	// scrolled away from its start or end has content under the bars.
	a.underTop, a.underBottom = true, true
	ui.Box(c.Key("detail")).Grow(1).Children(func() {
		// Under the list, positioned before it: what is positioned later
		// paints over it.
		if color, ok := pageTint(tint.BG, a.pal); ok {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(tintHeight).IgnoreInput(true).
				LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
					{Color: color, At: 0},
					{Color: color.Mix(a.pal.bg, 0.5), At: 0.55},
					{Color: a.pal.bg, At: 1},
				}})
		}
		list := ui.List(c.Key(a.route), &a.trackList, n, func(i int) {
			ui.Column(c).Children(func() {
				if i == 0 {
					a.startMark(c)
				}
				row(i)
				if i == n-1 {
					a.endMark(c)
				}
			})
		}).Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
		a.pageBars(c, list, n)
	})
}

// pageTint is the color a page is tinted with from the color hex of its
// picture: as dark as keeps the light text of the dark appearance easy to
// read on it, or as light as keeps the dark text of the light one.
func pageTint(hex string, p palette) (ui.Color, bool) {
	c := hexColor(hex, ui.Transparent)
	if c == ui.Transparent {
		return c, false
	}
	toward, limit := ui.RGB(255, 255, 255), 0.72
	if p.dark {
		toward, limit = ui.RGB(0, 0, 0), 0.045
	}
	// Mixed in steps, as luminance is far from linear in the mix.
	for range 20 {
		l := luminance(c)
		if p.dark && l <= limit || !p.dark && l >= limit {
			break
		}
		c = c.Mix(toward, 0.12)
	}
	return c, true
}

// luminance is the relative luminance of c, as WCAG reckons contrast.
func luminance(c ui.Color) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// detailHeader heads an album's or a playlist's page: its cover, moving
// where it has a video, and beside it what lines builds, its title and who made it and the rest, then the
// editors' words, notes, and the buttons that play it, with what extra adds
// at their end. extra is told whether the row is short of room for Add to
// Library in words, which then goes to its icon alone, as Apple Music's does.
func (a *app) detailHeader(c *ui.Context, cover, video string, lines func(), notes string, tracks []music.Track, extra func(compact bool)) {
	p := a.pal
	ui.Row(c).Gap(30).AlignItems(ui.End).Margin(0, 12, 28, 12).Children(func() {
		box := ui.Box(c).Size(detailCover, detailCover).Radius(8).Shrink(0).Shadow(0, 14, 34, -10, p.cardShade).Border(0.5, p.hairline)
		box.Children(func() {
			a.coverFill(c, cover, 600, 8)
			a.motionOver(c, box, video, detailCover, detailCover, motionHead, 8)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			lines()
			if notes = strings.TrimSpace(notes); notes != "" {
				a.notes(c, notes)
			}
			// What the row needs with every button in words: Play, Shuffle,
			// the spacer, Add to Library and the dots, and the gaps between.
			room := a.shelfMetrics(c).inner - detailCover - 30
			need := float32(4*pillGap + roundSize)
			if len(tracks) > 0 {
				need += pillWidth(c, T("Play"), true) + pillWidth(c, T("Shuffle"), true)
			}
			compact := need+pillWidth(c, T("Add to Library"), true) > room
			// Short of room even so, as where the system's font is a wide
			// one, Shuffle goes to its icon too: the row never runs past the
			// page's edge, which would hide its menu.
			tight := compact && need+roundSize > room
			ui.Row(c).Gap(pillGap).AlignItems(ui.Center).Margin(18, 0, 0, 0).Children(func() {
				if len(tracks) > 0 {
					a.playButtons(c, tracks, tight)
				}
				ui.Spacer(c)
				if extra != nil {
					extra(compact)
				}
			})
		})
	})
}

// pillGap is the room between the buttons of a page's header.
const pillGap = 10

// roundSize is the size of the round buttons of a page's header, as its
// dots.
const roundSize = 36

// detailTitle is the title of an album or a playlist, with the E of one
// that's explicit.
func (a *app) detailTitle(c *ui.Context, title string, explicit bool) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(24).FontWeight(700).LetterSpacing(-0.4).MaxLines(2).LineHeight(1.2).Shrink(1).MinWidth(0)
		if explicit {
			a.explicitMark(c)
		}
	})
}

// notesLines is how many lines of what the editors say a header shows,
// until More shows it all.
const notesLines = 3

// notesWidth is how wide what the editors say goes, at most.
const notesWidth = 600

// notes is what the editors say of an album or a playlist, as many lines
// as notesLines, with More after them where there's more to it, which
// shows it all.
func (a *app) notes(c *ui.Context, text string) {
	p := a.pal
	text = strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " \n ")), " ")
	text = strings.ReplaceAll(text, " \n ", "\n")
	open := a.notesOpen == a.route
	const size, leading = 12.5, 1.45
	width := min(notesWidth, a.shelfMetrics(c).inner-detailCover-30)
	_, h := c.MeasureText(width, ui.Span{Text: text, Size: size})
	_, line := c.MeasureText(0, ui.Span{Text: "M", Size: size})
	more := line > 0 && h/line > notesLines+0.5
	ui.Column(c.Key("notes")).MaxWidth(notesWidth).Margin(16, 0, 0, 0).Gap(2).Children(func() {
		t := ui.Text(c, text).FontSize(size).LineHeight(leading).TextColor(p.muted)
		if !open {
			t.MaxLines(notesLines)
		}
		if !more {
			return
		}
		label := T("More")
		if open {
			label = T("Less")
		}
		b := ui.ButtonBase(c.Key("more")).AlignSelf(ui.End).FocusRing(false).Label(label)
		color := p.text
		if b.Hovered() {
			color = p.accent
		}
		b.Children(func() { ui.Text(c, label).FontSize(11.5).FontWeight(700).TextColor(color) })
		if b.Clicked() {
			if open {
				a.notesOpen = route{}
			} else {
				a.notesOpen = a.route
			}
		}
	})
}

// libraryButton adds what kind and id name to the listener's library; once
// it's there, it is the star that makes it a favorite instead, as Apple
// Music has it, where a thing is added before it's a favorite. Deleting it
// from the library is for its menu. compact, the button to add is its icon
// alone, its words its label and tooltip. was and favorite are what the
// source said of it.
func (a *app) libraryButton(c *ui.Context, kind, id string, was, favorite, compact bool) {
	p := a.pal
	if a.library.inLibrary(kind, id, was) {
		on := a.library.isFavorite(kind, id, favorite)
		icon, color, label := iconStar, p.text, T("Favorite")
		if on {
			icon, color, label = iconStarFill, p.accent, T("Undo Favorite")
		}
		b := a.roundHeaderButton(c.Key("favorite"), label)
		b.Tooltip(label).Children(func() { ui.Icon(c, icon).Size(15, 15).TextColor(color) })
		// It comes where the button that added it was, up from a little smaller.
		scale := b.AnimateTo("pop", 1, ui.Animation{Spring: ui.SpringOptions{Duration: 450 * time.Millisecond, Bounce: 0.4}})
		b.Scale(scale, scale)
		if a.justKept == itemKey(kind, id) {
			a.justKept = ""
			b.Jump("pop", 0.6)
		}
		if b.Clicked() {
			b.Jump("pop", 0.8)
			a.setFavorite(kind, id, !on)
		}
		return
	}
	label := T("Add to Library")
	var b ui.Element
	if compact {
		b = a.roundHeaderButton(c.Key("library"), label)
		b.Tooltip(label).Children(func() { ui.Icon(c, iconPlus).Size(14, 14) })
	} else {
		b = pillButton(c.Key("library"), iconPlus, label, p.hover.Over(p.bg), p.text)
	}
	if b.Clicked() {
		a.justKept = itemKey(kind, id)
		a.setInLibrary(kind, id, true)
	}
}

// moreButton is the dots that open a page's menu, menu.
func (a *app) moreButton(c *ui.Context, menu func(m *ui.Menu)) {
	if menu == nil {
		return
	}
	b := a.roundHeaderButton(c.Key("more"), T("More"))
	b.Children(func() { ui.Icon(c, iconMore).Size(16, 16) })
	b.Menu(menu)
}

// roundHeaderButton is a round button of a page's header, labeled label,
// filled as its pills are.
func (a *app) roundHeaderButton(c *ui.Context, label string) ui.Element {
	b := ui.ButtonBase(c).Size(roundSize, roundSize).Shrink(0).Radius(999).Center().Label(label).FocusRing(false).TextColor(a.pal.text)
	fill := a.pal.hover.Over(a.pal.bg)
	if b.Hovered() {
		fill = fill.Mix(ui.RGB(255, 255, 255), 0.12)
	}
	b.Background(fill)
	return b
}

// The badges of how something sounds at its best (music.Traits), in our own
// words: Apple Music's words for them are not among its texts.
var (
	traitAtmos   = N("Dolby Atmos")
	traitHiRes   = N("Hi-Res Lossless")
	traitLosless = N("Lossless")
	traitMaster  = N("Apple Digital Master")
)

// traitLabels are the badges of traits, the best first.
func traitLabels(tr music.Traits) []string {
	var out []string
	if tr.Atmos {
		out = append(out, traitAtmos)
	}
	switch {
	case tr.HiRes:
		out = append(out, traitHiRes)
	case tr.Lossless:
		out = append(out, traitLosless)
	}
	if tr.DigitalMaster {
		out = append(out, traitMaster)
	}
	return out
}

// traitBadge is a small outlined label of how something sounds.
func traitBadge(c *ui.Context, label string, color ui.Color) ui.Element {
	return ui.Text(c.Key(label), T(label)).FontSize(9.5).FontWeight(700).LetterSpacing(0.3).TextColor(color).SingleLine().
		Padding(1, 5).Radius(3).Border(1, color.Alpha(0.55))
}

// traitBadgeWidth is how wide traitBadge draws label.
func traitBadgeWidth(c *ui.Context, label string) float32 {
	text := T(label)
	w, _ := c.MeasureText(0, ui.Span{Text: text, Size: 9.5, Weight: 700})
	return w + 0.3*float32(utf8.RuneCountInString(text)) + 2*5 + 2
}

// traitBadges are the badges of traits, in a row.
func (a *app) traitBadges(c *ui.Context, tr music.Traits, color ui.Color) {
	labels := traitLabels(tr)
	if len(labels) == 0 {
		return
	}
	ui.Row(c.Key("traits")).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		for _, l := range labels {
			traitBadge(c, l, color)
		}
	})
}

// released is the day something came out, as the listener's language says
// a date, from as the source has it, such as 2024-03-01: a year alone stays
// one, and what isn't a date is as it was.
func released(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format(T("January 2, 2006"))
	}
	return s
}

// year is the year of a date as the source has it, or "".
func year(s string) string {
	if len(s) >= 4 {
		if _, err := time.Parse("2006", s[:4]); err == nil {
			return s[:4]
		}
	}
	return ""
}

// listLength says how many songs tracks are and how long they play, as the
// foot of an album's page does.
func listLength(tracks []music.Track) string {
	var d time.Duration
	for i := range tracks {
		d += tracks[i].Length()
	}
	m := int(d.Round(time.Minute) / time.Minute)
	length := Tf("%d min", m)
	if m >= 60 {
		length = Tf("%d hr %d min", m/60, m%60)
	}
	return fmt.Sprintf(TC("%s, %s", "length"), songCount(len(tracks)), length)
}

// songCount says how many songs there are, as Apple Music does.
func songCount(n int) string {
	if n == 1 {
		return T("1 Song")
	}
	return Tf("%d Songs", n)
}

// weekdays are the days of the week, from Sunday, as an update is told.
var weekdays = [...]string{N("Sunday"), N("Monday"), N("Tuesday"), N("Wednesday"), N("Thursday"), N("Friday"), N("Saturday")}

// updated says when a playlist last changed, from modified as the source
// has it, such as 2026-03-01T08:00:00Z, as now is: today, yesterday, the day
// of the week within one, last week, or else the day. Unreadable, it says
// nothing.
func updated(modified string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, modified)
	if err != nil {
		return ""
	}
	t = t.In(now.Location())
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()) }
	days := int(day(now).Sub(day(t)).Hours()/24 + 0.5)
	switch {
	case days <= 0:
		return T("Updated Today")
	case days == 1:
		return T("Updated Yesterday")
	case days < 7:
		return Tf("Updated %s", T(weekdays[t.Weekday()]))
	case days < 14:
		return T("Updated Last Week")
	}
	return Tf("Updated %s", t.Format(T("January 2, 2006")))
}

// shelfRows are the rows of a page's sections after its songs, laid out as
// shelves, which rows from from on of a page's list show (sectionsRow).
func (a *app) shelfRows(c *ui.Context, secs []music.Section) []browseRow {
	if len(secs) == 0 {
		return nil
	}
	return a.shelfLayout(c, secs, false)
}

// sectionsRow shows row r of the sections of a page after its songs, the
// first well below them.
func (a *app) sectionsRow(c *ui.Context, secs []music.Section, rows []browseRow, i int) {
	r := rows[i]
	if i == 0 {
		// The first title keeps no room above it of its own (shelfTitle).
		ui.Box(c).Height(sectionGap + 10)
		if r.kind != rowTitle {
			r.spaced = false
		}
	}
	a.shelfRow(c, secs, r)
}
