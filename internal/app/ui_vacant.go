package app

import (
	"github.com/egoist/mygo/ui"

	"ringo/brand"
)

// What a page shows in place of what it lists, where it has nothing or
// couldn't load it: a glyph over a few words, in the middle of the room the
// page has, as Music's pages say so. One look for all of them, where each
// used to say it in its own corner.

// thinIcon is strokeIcon for a glyph shown several times as large as an
// icon: its stroke thinner, to weigh no more than the words under it.
func thinIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

var (
	glyphNote   = thinIcon(`<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>`)
	glyphSearch = thinIcon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)
	glyphStar   = thinIcon(`<path d="m12 3 2.7 5.8 6.3.8-4.6 4.3 1.2 6.2L12 17l-5.6 3.1 1.2-6.2L3 9.6l6.3-.8z"/>`)
	glyphAlbums = thinIcon(`<rect x="3" y="6" width="15" height="15" rx="2.5"/><path d="M7 3h11.5A2.5 2.5 0 0 1 21 5.5V17"/><circle cx="10.5" cy="13.5" r="3"/>`)
	glyphAlert  = thinIcon(`<circle cx="12" cy="12" r="9.5"/><path d="M12 7.5v5.2"/><path d="M12 16.4v.1"/>`)

	// markRingo is the app's mark, in its own colors.
	markRingo = ui.MustParseSVG(brand.Mark)
)

// vacantGlyph is the size of the glyph over what vacant says.
const vacantGlyph = 44

// vacant says a page has nothing to show: icon, then title and hint under
// it, each where set, and what actions builds after them, buttons that may
// put it right. It returns the column, for its caller to place.
func (a *app) vacant(c *ui.Context, icon *ui.SVG, title, hint string, actions func()) ui.Element {
	p := a.pal
	col := ui.Column(c.Key("vacant")).AlignItems(ui.Center).Gap(6)
	col.Children(func() {
		switch icon {
		case nil:
		case markRingo:
			// In its own colors, not the text's as an icon is.
			ui.Image(c, icon).Size(72, 72).Margin(0, 0, 14, 0)
		default:
			ui.Icon(c, icon).Size(vacantGlyph, vacantGlyph).TextColor(p.faint).Margin(0, 0, 8, 0)
		}
		if title != "" {
			ui.Text(c, title).FontSize(20).FontWeight(700).LetterSpacing(-0.3).TextAlign(ui.Center).MaxWidth(520)
		}
		if hint != "" {
			ui.Text(c, hint).FontSize(13).LineHeight(1.4).TextColor(p.muted).TextAlign(ui.Center).MaxWidth(440)
		}
		if actions != nil {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(14, 0, 0, 0).Children(actions)
		}
	})
	return col
}

// vacantUnder is the room to leave over what vacant shows under a page's
// header about header high, for it to sit in the middle of what the header
// leaves of the window, a little above it, where the eye takes the middle to
// be. From the window's size alone, so that it holds still as the page
// scrolls.
func (a *app) vacantUnder(c *ui.Context, header float32) float32 {
	_, h := c.Size()
	top, bottom := a.insets(c)
	// About what vacant takes with a glyph and two lines.
	const block = 110
	return max(36, (h-top-pageGap-bottom-header-block)*0.42)
}

// empty says a list under a page's header, about header high, has nothing
// in it.
func (a *app) empty(c *ui.Context, icon *ui.SVG, msg string, header float32) {
	a.vacant(c, icon, "", msg, nil).Padding(a.vacantUnder(c, header), 0, 40, 0).AlignSelf(ui.Center)
}

// quietPill is a pill button that isn't the page's main one, as Shuffle
// beside Play.
func (a *app) quietPill(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	p := a.pal
	return pillButton(c, icon, label, p.hover.Over(p.bg), p.text)
}
