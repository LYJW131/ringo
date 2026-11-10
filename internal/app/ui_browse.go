package app

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// Pages a source makes up itself: what its Home shows, and any other, opened
// from a link, a section's "see all" or an entry it adds to the sidebar. A page
// goes in rows of a list, built only as they scroll into view: a library's
// albums can be thousands.

// browseData is what the app has of a page, kept as it loads anew.
type browseData struct {
	page    *music.Browse
	at      time.Time // when page came
	loading bool
	err     string
	// next is how the page's next part, its Next, is coming along.
	next paged
	// paged says the page has its next parts joined to it, which loading
	// it anew would drop: it is, only as asked again (loadBrowse).
	paged bool
}

// browseID is the page of the source's that r shows, if it shows one.
func browseID(r route) string {
	switch r.page {
	case pageBrowse:
		return r.id
	case pageSearch:
		if r.query == "" {
			return music.PageSearchLanding
		}
		return music.PageSearch + r.query
	}
	return ""
}

// loadBrowse loads a page, unless it has, or again. One loaded longer than
// listsFresh ago is asked for anew, as it may change as the app plays, such
// as songs played lately, and shows as it was while it is: but not one the
// listener paged through, which would go back to its first part as they
// come back to it.
func (a *app) loadBrowse(lib *library, page string, again bool) {
	if lib == nil || page == "" {
		return
	}
	d := lib.browses[page]
	if d == nil {
		d = &browseData{}
		lib.browses[page] = d
	}
	if d.loading || d.page != nil && !again && (d.paged || time.Since(d.at) <= listsFresh) {
		return
	}
	d.loading, d.err = true, ""
	go func() {
		b, err := answered(lib.src.Browse(page))
		a.post(func() {
			d.loading = false
			if err != nil {
				d.err = a.problem(lib, err, T("Couldn't load this page"))
				return
			}
			d.page, d.at, d.next, d.paged = b, time.Now(), paged{loaded: true, more: b.Next != ""}, false
			a.keepPages(lib)
		})
	}()
}

// loadNext loads the part of a page that follows what it has, its Next: its
// sections go on from the last one's, items after items.
func (a *app) loadNext(lib *library, d *browseData) {
	if d.page == nil || d.page.Next == "" || d.next.loading {
		return
	}
	was := d.page
	d.next.loading, d.next.err = true, ""
	go func() {
		b, err := answered(lib.src.Browse(was.Next))
		a.post(func() {
			d.next.loading = false
			// Loaded anew meanwhile, from its start.
			if d.page != was {
				return
			}
			if err != nil {
				d.next.err = a.problem(lib, err, T("Couldn't load this page"))
				return
			}
			d.page, d.paged = joinPages(was, b), true
			d.next.more = d.page.Next != ""
		})
	}()
}

// joinPages is page and its next part, as one: the next one's first section
// goes on from page's last. Neither is changed, as page may be showing.
func joinPages(page, next *music.Browse) *music.Browse {
	joined := *page
	joined.Next = next.Next
	if next.Total > 0 {
		joined.Total = next.Total
	}
	joined.Sections = slices.Clone(page.Sections)
	more := next.Sections
	if n := len(joined.Sections); n > 0 && len(more) > 0 {
		last, add := joined.Sections[n-1], more[0]
		last.Items = slices.Concat(last.Items, add.Items)
		last.Tracks = slices.Concat(last.Tracks, add.Tracks)
		last.Albums = slices.Concat(last.Albums, add.Albums)
		last.Artists = slices.Concat(last.Artists, add.Artists)
		last.Playlists = slices.Concat(last.Playlists, add.Playlists)
		last.Stations = slices.Concat(last.Stations, add.Stations)
		last.Links = slices.Concat(last.Links, add.Links)
		joined.Sections[n-1] = last
		more = more[1:]
	}
	joined.Sections = append(joined.Sections, more...)
	return &joined
}

func (a *app) browsePage(c *ui.Context) {
	a.browseView(c, a.route.id)
}

// browseView shows the page id of the source's, as it loads.
func (a *app) browseView(c *ui.Context, id string) {
	lib := a.library
	d := lib.browses[id]
	if d == nil || d.page == nil {
		failed := d != nil && !d.loading
		a.scrollPage(c, func() {
			// Under the page's title, which says what didn't load.
			ui.Box(c).PaddingX(12).Children(func() { a.browseHeader(c, &music.Browse{}, id) })
			switch {
			case failed:
				a.failure(ui.Column(c).Padding(60, 12, 0, 12), c, d.err, func() { a.loadBrowse(lib, id, true) })
			case strings.HasPrefix(id, "library:"):
				a.librarySkeleton(c, a.pageShimmer(c), id)
			default:
				a.shelfSkeleton(c, a.pageShimmer(c))
			}
		})
		return
	}
	a.browseRows(c, d.page, id, func(c *ui.Context) { a.browseHeader(c, d.page, id) }, d)
}

// pageTitles are what the app calls the pages it opens of the source's
// itself, for those the source leaves untitled.
var pageTitles = map[string]string{
	music.PageNew:           N("New"),
	music.PageRadio:         N("Radio"),
	music.PageRecentlyAdded: N("Recently Added"),
	music.PageLibraryArtist: N("Artists"),
	music.PageLibraryAlbums: N("Albums"),
	music.PageLibrarySongs:  N("Songs"),
	music.PageMadeForYou:    N("Made for You"),
	music.PageAllPlaylists:  N("All Playlists"),
}

// browseHeader heads a page the source made up: its title, and what it says
// of it, a line to each line. With a picture, it's of someone, and headed as
// an artist's page is.
func (a *app) browseHeader(c *ui.Context, page *music.Browse, id string) {
	p := a.pal
	if page.Title == "" && pageTitles[id] != "" {
		titled := *page
		titled.Title = T(pageTitles[id])
		page = &titled
	}
	if page.WideURL != "" {
		a.wideHeader(c, page)
		return
	}
	var lines []string
	for line := range strings.SplitSeq(page.Description, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	about := func() {
		for i, line := range lines {
			t := ui.Text(c.Key(i), line).FontSize(13).TextColor(p.muted).MaxLines(2).LineHeight(1.45)
			if i == 0 {
				t.Margin(2, 0, 0, 0)
			}
		}
	}
	if page.ImageURL == "" {
		if len(lines) == 0 {
			if page.Title != "" {
				a.heading(c, page.Title, page.Subtitle)
			}
			return
		}
		ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
			if page.Subtitle != "" {
				ui.Text(c, page.Subtitle).FontSize(12).FontWeight(600).TextColor(p.muted).LetterSpacing(0.4)
			}
			ui.Text(c, page.Title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
			about()
		})
		return
	}
	// As far below as an artist's sections are, as its first title keeps
	// none of its own.
	ui.Row(c).Gap(28).AlignItems(ui.End).Margin(0, 0, 34, 0).Children(func() {
		ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFillWith(c, page.ImageURL, 600, artistPhotoSize/2, iconUser)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			if page.Subtitle != "" {
				ui.Text(c, page.Subtitle).FontSize(12).FontWeight(600).TextColor(p.accent)
			}
			ui.Text(c, page.Title).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			about()
		})
	})
}

// wideHeader heads a page with a picture across it, its title over it in
// the picture's colors.
func (a *app) wideHeader(c *ui.Context, page *music.Browse) {
	p := a.pal
	w := a.shelfMetrics(c).inner
	h := min(w*0.36, 320)
	ui.Box(c).Height(h).Radius(artRadius+3).Margin(0, 0, 26, 0).Background(hexColor(page.Colors.BG, p.hover)).Children(func() {
		bmp := bitmap(a.art.get(sized(music.Wide(page.WideURL, wideArt, wideArt*9/16), wideArt), false))
		box := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(artRadius + 3)
		in := artIn(box, bmp)
		box.Children(func() { artImage(c, bmp, artRadius+3, in) })
		ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(h * 0.55).Radius(artRadius + 3).
			LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
				{Color: ui.RGBA(0, 0, 0, 0), At: 0},
				{Color: ui.RGBA(0, 0, 0, 0.45), At: 1},
			}})
		ui.Column(c).Absolute().Left(24).Right(24).Bottom(20).Gap(4).Children(func() {
			if page.Subtitle != "" {
				ui.Text(c, page.Subtitle).FontSize(12).FontWeight(600).TextColor(hexColor(page.Colors.Text2, white.Alpha(0.8))).SingleLine()
			}
			ui.Text(c, page.Title).FontSize(30).FontWeight(800).LetterSpacing(-0.6).TextColor(hexColor(page.Colors.Text1, white)).MaxLines(2)
		})
	})
}

// A browseRow is a row of a page: a section's title, a shelf of its items,
// a row of them where they wrap or are a line each, or, of a section without
// a style, its tracks one to a row or its cards so many to a row.
type browseRow struct {
	kind     rowKind
	section  int
	from, to int
	// spaced keeps the first row of a section without a title off the
	// section before.
	spaced bool
}

type rowKind int

const (
	rowTitle rowKind = iota
	rowTrack
	rowAlbums
	rowArtists
	rowPlaylists
	rowStations
	rowLinks
	rowEmpty
	// The rows of shelves.
	rowShelf
	rowGrid
	rowTiles
	rowSong
	rowLink
	rowFeatured
	rowText
)

// Going on through a page that comes in parts. The next part is asked for
// well before the listener scrolls to the end of what's loaded, so that the
// end is seldom in view: once the page opens, for a library's lists, which
// the source gives a few parts of at once, up to eagerItems; then as the last
// row in view comes within aheadScreens of the end of what's loaded.
const (
	eagerItems   = 400
	aheadScreens = 3
	// pendingItems is how many items stand in, as blocks in the shape of
	// the rows to come, past what's loaded: no more than the next ask brings,
	// so that the page doesn't show a length it can't back.
	pendingItems = 300
)

// nearEnd reports whether the list showed in the last frame is within
// aheadScreens of the end of its rows loaded, or didn't show one yet.
func (a *app) nearEnd(rows int) bool {
	first, last := a.trackList.Visible()
	if last < first {
		return true
	}
	return rows-1-last <= aheadScreens*(last-first+1)
}

// loadedItems is how many items a page's sections hold.
func loadedItems(page *music.Browse) int {
	n := 0
	for i := range page.Sections {
		n += len(page.Sections[i].Items)
	}
	return n
}

// wantsMore reports whether a page of rows loaded rows long should ask for its
// next part now.
func (a *app) wantsMore(page *music.Browse, rows int) bool {
	return page.Total > 0 && loadedItems(page) < eagerItems || a.nearEnd(rows)
}

// pendingRows is how many rows stand in for the items of a page that are yet
// to come: as many as it says it has in all, but pendingItems at most. A
// page that doesn't say has none, and a foot instead.
func (a *app) pendingRows(c *ui.Context, page *music.Browse) int {
	if page.Total <= 0 || len(page.Sections) == 0 {
		return 0
	}
	last := &page.Sections[len(page.Sections)-1]
	per := 1
	switch last.Style {
	case music.ShelfList:
	case music.ShelfGrid:
		m := a.shelfMetrics(c)
		per, _ = m.fit(squareMin)
		if allOf(last.Items, music.ItemArtist) {
			per, _ = m.fit(circleMin)
		}
	default:
		return 0
	}
	left := min(page.Total-loadedItems(page), pendingItems)
	if left <= 0 {
		return 0
	}
	return (left + per - 1) / per
}

// pendingRow is row i of those that stand in for what is yet to come, as
// high as the row of the real thing, so that nothing moves as it takes their
// place.
func (a *app) pendingRow(c *ui.Context, page *music.Browse, i int) {
	last := &page.Sections[len(page.Sections)-1]
	s := a.rowShimmer(c)
	if last.Style == music.ShelfList {
		a.trackRowSkeleton(c, s, i, true)
		return
	}
	a.cardsSkeletonRow(c, s, allOf(last.Items, music.ItemArtist), i)
}

// browseRows shows a page under a header, which scrolls with it, and asks
// for its next part ahead of the listener's scrolling, if d says it has one.
// Where the page says how many items it has, blocks stand in for those to
// come; else its foot says it is loading.
func (a *app) browseRows(c *ui.Context, page *music.Browse, pageID string, header func(c *ui.Context), d *browseData) {
	layout := a.shelfLayout(c, page.Sections, pageID == music.PageSearchLanding)
	top, bottom := a.insets(c)
	a.underTop, a.underBottom = true, true
	pending := 0
	if d != nil && d.next.more && d.next.err == "" {
		pending = a.pendingRows(c, page)
	}
	foot := d != nil && (d.next.more || d.next.err != "") && pending == 0
	rows := len(layout) + 1 + pending
	if foot {
		rows++
	}
	lib := a.library
	ui.List(c.Key(a.route), &a.trackList, rows, func(i int) {
		if i == 0 {
			ui.Column(c).PaddingX(12).Children(func() {
				a.startMark(c)
				header(c)
				if page.Next != "" || len(layout) > 0 {
					return
				}
				if strings.HasPrefix(pageID, music.PageSearch) {
					// A search that found nothing says so, as Apple Music's.
					ui.Column(c).Gap(6).Padding(60, 0, 0, 0).AlignItems(ui.Center).Children(func() {
						ui.Text(c, T("No Results")).FontSize(20).FontWeight(700).LetterSpacing(-0.3)
						ui.Text(c, T("Try a new search.")).FontSize(13).TextColor(a.pal.muted)
					})
					return
				}
				a.empty(c, T("No items"))
			})
			return
		}
		if i == rows-1 {
			defer a.endMark(c)
		}
		switch {
		case i > len(layout)+pending:
			a.pageFoot(c, &d.next, func() { a.loadNext(lib, d) })
		case i > len(layout):
			a.pendingRow(c, page, i-len(layout)-1)
		default:
			a.shelfRow(c, page.Sections, layout[i-1])
		}
	}).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
	if d != nil && d.next.more && !d.next.loading && d.next.err == "" && a.wantsMore(page, len(layout)+1) {
		a.loadNext(lib, d)
	}
}

// cardRowGap is the room under a row of cards.
const cardRowGap = 28

// cardRow is a row of a section's cards, which keep their columns whatever
// the number in the last row.
func (a *app) cardRow(c *ui.Context, sec *music.Section, r browseRow, cols int) {
	ui.Grid(c).Columns(cols).GapX(20).Margin(0, 12, cardRowGap, 12).Children(func() {
		for i := r.from; i < r.to; i++ {
			switch r.kind {
			case rowAlbums:
				a.albumCard(c.Key(sec.Albums[i].ID), &sec.Albums[i])
			case rowArtists:
				a.artistCard(c.Key(sec.Artists[i].ID), &sec.Artists[i])
			case rowPlaylists:
				a.playlistCard(c.Key(sec.Playlists[i].ID), &sec.Playlists[i])
			case rowStations:
				a.stationCard(c.Key(sec.Stations[i].ID), &sec.Stations[i])
			case rowLinks:
				l := &sec.Links[i]
				if a.coverCard(c.Key(l.Page), l.Title, l.ImageURL, l.Subtitle, nil) {
					a.open(route{page: pageBrowse, id: l.Page})
				}
			}
		}
	})
}
