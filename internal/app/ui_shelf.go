package app

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// Shelves are how Apple Music lays out its pages: sections in the order its
// service gives them, each in its style (music.Shelf…), of items of any
// kind. A page puts them in rows of its list, built only as they scroll into
// view, as a library's songs can be thousands:
//
//	rows := a.shelfLayout(c, page.Sections, false)
//	ui.List(c.Key(a.route), &a.trackList, len(rows), func(i int) {
//		a.shelfRow(c, page.Sections, rows[i])
//	})
//
// A section without a style is laid out as before shelves, by its kinds.

// shelfGap is the room between the items of a shelf, across and down.
const shelfGap = 20

// sectionGap is the room above a section's title, below the one before.
const sectionGap = 30

// shelfMetrics are how wide the items of each style are at the page's width:
// as many as fit across it, at least so wide, filling it.
type shelfMetrics struct {
	inner float32 // the width of the page's content
}

func (a *app) shelfMetrics(c *ui.Context) shelfMetrics {
	w, _ := c.Size()
	return shelfMetrics{inner: max(w-sidebarWidth-2*pagePad, 200)}
}

// fit returns how many items of at least min fit across, and how wide each is.
func (m shelfMetrics) fit(min float32) (int, float32) {
	n := max(1, int((m.inner+shelfGap)/(min+shelfGap)))
	return n, (m.inner - shelfGap*float32(n-1)) / float32(n)
}

// The cover that heads a section made from one thing, and the room around it.
const (
	seedCover  = 58
	seedRadius = 6
	seedGap    = 12
)

// The least widths of the items of each style.
const (
	// Five to a row in a window 1600 wide, as Apple Music's.
	portraitMin = 230
	squareMin   = 150
	circleMin   = 128
	brickMin    = 240
	// A brick in rows that wrap, as Search's categories.
	tileMin   = 190
	heroMin   = 360
	tracksMin = 260
)

// shelfLayout lays sections out in rows. With wrap, bricks wrap in rows as
// Search shows its categories, rather than scroll sideways.
func (a *app) shelfLayout(c *ui.Context, secs []music.Section, wrap bool) []browseRow {
	m := a.shelfMetrics(c)
	squares, _ := m.fit(squareMin)
	circles, _ := m.fit(circleMin)
	tiles, _ := m.fit(tileMin)
	legacy := a.columns(c, 156)
	var rows []browseRow
	for s := range secs {
		sec := &secs[s]
		titled := sec.Title != "" || sec.MorePage != ""
		if titled {
			rows = append(rows, browseRow{kind: rowTitle, section: s})
		}
		// The first row of a section without a title keeps it off the one
		// before, as a title would.
		at := len(rows)
		cards := func(kind rowKind, n, cols int) {
			for from := 0; from < n; from += cols {
				rows = append(rows, browseRow{kind: kind, section: s, from: from, to: min(from+cols, n)})
			}
		}
		switch sec.Style {
		case "":
			if !titled && s > 0 {
				// Untitled, as before shelves: nothing keeps it off.
				at = -1
			}
			for i := range sec.Tracks {
				rows = append(rows, browseRow{kind: rowTrack, section: s, from: i, to: i})
			}
			cards(rowAlbums, len(sec.Albums), legacy)
			cards(rowArtists, len(sec.Artists), legacy)
			cards(rowPlaylists, len(sec.Playlists), legacy)
			cards(rowStations, len(sec.Stations), legacy)
			cards(rowLinks, len(sec.Links), legacy)
			if sec.Empty != "" && len(sec.Tracks)+len(sec.Albums)+len(sec.Artists)+len(sec.Playlists)+len(sec.Stations)+len(sec.Links) == 0 {
				rows = append(rows, browseRow{kind: rowEmpty, section: s})
			}
		case music.ShelfText:
			if sec.Subtitle != "" {
				rows = append(rows, browseRow{kind: rowText, section: s})
			}
		case music.ShelfGrid:
			cols := squares
			if allOf(sec.Items, music.ItemArtist) {
				cols = circles
			}
			cards(rowGrid, len(sec.Items), cols)
		case music.ShelfList:
			tracks := a.sectionTracks(secs, s)
			for i := range tracks.tracks {
				rows = append(rows, browseRow{kind: rowSong, section: s, from: i, to: i})
			}
		case music.ShelfLinks:
			cards(rowLink, len(sec.Items), m.linkColumns())
		case music.ShelfFeatured:
			if len(sec.Items) > 0 {
				rows = append(rows, browseRow{kind: rowFeatured, section: s})
			}
		case music.ShelfBrick:
			if wrap {
				cards(rowTiles, len(sec.Items), tiles)
				break
			}
			fallthrough
		default:
			if len(sec.Items) > 0 {
				rows = append(rows, browseRow{kind: rowShelf, section: s})
			}
		}
		if sec.Empty != "" && sec.Style != "" && len(sec.Items) == 0 {
			rows = append(rows, browseRow{kind: rowEmpty, section: s})
		}
		if !titled && s > 0 && at >= 0 && at < len(rows) {
			rows[at].spaced = true
		}
	}
	return rows
}

// allOf reports whether items are all of kind.
func allOf(items []music.Item, kind string) bool {
	for i := range items {
		if items[i].Kind != kind {
			return false
		}
	}
	return len(items) > 0
}

// shelfRow shows a row that shelfLayout laid out of secs.
func (a *app) shelfRow(c *ui.Context, secs []music.Section, r browseRow) {
	sec := &secs[r.section]
	top := float32(0)
	if r.spaced {
		top = sectionGap
	}
	m := a.shelfMetrics(c)
	switch r.kind {
	case rowTitle:
		a.shelfTitle(c, sec, r.section == 0)
	case rowShelf:
		a.shelf(c, secs, r.section, m, top)
	case rowGrid, rowTiles:
		a.shelfGrid(c, secs, r, m, top)
	case rowSong:
		tracks := a.sectionTracks(secs, r.section)
		ui.Box(c).Margin(top, 0, 0, 0).Children(func() { a.trackRow(c, tracks.tracks, r.from, true, "") })
	case rowLink:
		cols := m.linkColumns()
		ui.Row(c).Gap(shelfGap).Margin(0, 12).Children(func() {
			for i := r.from; i < r.from+cols; i++ {
				if i < r.to {
					a.linkRow(c.Key(i), secs, r.section, i).Grow(1).Basis(0).MinWidth(0)
				} else {
					// The last row's links keep the columns of those above.
					ui.Box(c.Key(i)).Grow(1).Basis(0)
				}
			}
		})
	case rowFeatured:
		a.featured(c, secs, r.section, top)
	case rowText:
		a.shelfText(c, sec, top)
	case rowTrack:
		a.trackRow(c, sec.Tracks, r.from, true, "")
	case rowEmpty:
		ui.Column(c).Margin(0, 12, cardRowGap, 12).Children(func() { a.empty(c, sec.Empty) })
	default:
		a.cardRow(c, sec, r, a.columns(c, 156))
	}
}

// shelfTitle is a section's title, which opens the page with all of it, if
// there is one, as its arrow says.
func (a *app) shelfTitle(c *ui.Context, sec *music.Section, first bool) {
	p := a.pal
	margin := float32(sectionGap)
	if first {
		margin = 0
	}
	text, seed := sec.Title, sec.Seed
	if seed != nil {
		text = seed.Title
	}
	lines := func() {
		// A section made from one thing is headed by it: its cover, and
		// its name under what Apple says of the section.
		if seed != nil && sec.Kicker != "" {
			ui.Text(c, sec.Kicker).FontSize(13).TextColor(p.muted).SingleLine()
		}
		title := func(color ui.Color) {
			ui.Text(c, text).FontSize(18).FontWeight(700).LetterSpacing(-0.3).TextColor(color).SingleLine().Shrink(1).MinWidth(0)
		}
		if sec.MorePage == "" {
			ui.Row(c).Children(func() { title(p.text) })
		} else {
			b := ui.ButtonBase(c.Key("more")).AlignSelf(ui.Start).Gap(3).AlignItems(ui.Center).FocusRing(false).Label(sec.Title)
			color := p.text
			if b.Hovered() {
				color = p.muted
			}
			b.Children(func() {
				title(color)
				ui.Icon(c, iconForward).Size(15, 15).TextColor(p.muted).Margin(1, 0, 0, 0)
			})
			if b.Clicked() {
				a.open(route{page: pageBrowse, id: sec.MorePage})
			}
		}
		if sec.Subtitle != "" && sec.Style != music.ShelfText {
			ui.Text(c, sec.Subtitle).FontSize(13).TextColor(p.muted).SingleLine()
		}
	}
	if seed == nil {
		ui.Column(c).Margin(margin, 12, 10, 12).Gap(2).Children(lines)
		return
	}
	ui.Row(c).Margin(margin, 12, 12, 12).Gap(seedGap).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(seedCover, seedCover).Radius(seedRadius).Shrink(0).Border(0.5, p.hairline).Children(func() {
			a.itemPicture(c, seed, false, 160, seedRadius)
		})
		ui.Column(c).Gap(2).Shrink(1).MinWidth(0).Children(lines)
	})
}

// shelfState is where the shelf of section s scrolls to.
func (a *app) shelfState(s int) *shelfScroll {
	if a.shelves == nil {
		a.shelves = map[int]*shelfScroll{}
	}
	st := a.shelves[s]
	if st == nil {
		st = &shelfScroll{focused: -1}
		a.shelves[s] = st
	}
	return st
}

// shelf is a section's items in a row that scrolls sideways, out to the
// edges of the page: with a touchpad, or a screen at a time with the arrows
// that show as the pointer is over it.
func (a *app) shelf(c *ui.Context, secs []music.Section, s int, m shelfMetrics, top float32) {
	sec := &secs[s]
	st := a.shelfState(s)
	var w, artTop, artH float32
	switch sec.Style {
	case music.ShelfHero:
		_, w = m.fit(heroMin)
		artTop, artH = heroHead(c, sec), w*9/16
	case music.ShelfPortrait:
		_, w = m.fit(portraitMin)
		artH = w * 4 / 3
	case music.ShelfLive:
		var n int
		n, w = m.fit(heroMin)
		if n == 1 {
			// One to a page would be half the window, and hide that there
			// are more: as narrow as bricks, a corner of the next showing.
			n, _ = m.fit(brickMin)
			w = (m.inner - shelfGap*float32(n)) / (float32(n) + 0.25)
		}
		artH = w * 9 / 16
	case music.ShelfBrick:
		_, w = m.fit(brickMin)
		artH = w * 9 / 16
	case music.ShelfCircle:
		_, w = m.fit(circleMin)
		artH = w
	case music.ShelfTracks:
		_, w = m.fit(tracksMin)
		artH = tracksRows * trackCellHeight
	default:
		_, w = m.fit(squareMin)
		artH = w
	}
	// Its items in a row as wide as they all are, shifted by how far it is
	// scrolled (shelfScroll) and clipped to the shelf; the tracks in columns.
	units := len(sec.Items)
	if sec.Style == music.ShelfTracks {
		units = (units + tracksRows - 1) / tracksRows
	}
	step := w + shelfGap
	st.MaxX = max(0, float32(units)*step-shelfGap-m.inner)
	box := ui.Box(c.Key("shelf")).Margin(top, 12-pagePad, 0, 12-pagePad)
	hovered := box.Hovered()
	box.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputScroll {
			a.overShelf = st
		}
		return false
	})
	// The focus brings its item into view only as the keys move it: a press
	// on one half out of view must not slide it from under the pointer.
	box.ObserveInput(func(ev ui.InputEvent) {
		switch ev.Kind {
		case ui.InputKeyDown:
			st.keyed = true
		case ui.InputPointerDown:
			st.keyed = false
		}
	})
	box.Children(func() {
		ui.Box(c.Key("scroll")).Clip().Label(sec.Title).Children(func() {
			row := ui.Row(c.Key("row")).Width(2*pagePad+float32(units)*step-shelfGap).Shrink(0).Padding(0, pagePad).Gap(shelfGap).AlignItems(ui.Start)
			focus := -1
			row.Children(func() {
				switch sec.Style {
				case music.ShelfTracks:
					for col := range units {
						column := ui.Column(c.Key(col)).Width(w).Shrink(0)
						column.Children(func() {
							for i := col * tracksRows; i < min((col+1)*tracksRows, len(sec.Items)); i++ {
								a.trackCell(c.Key(i), secs, s, i)
							}
						})
						if column.FocusWithin() {
							focus = col
						}
					}
				default:
					for i := range sec.Items {
						item := ui.Box(c.Key(i)).Shrink(0)
						item.Children(func() { a.shelfItem(c, secs, s, i, w) })
						if item.FocusWithin() {
							focus = i
						}
					}
				}
			})
			if focus != st.focused {
				st.focused = focus
				if focus >= 0 && st.keyed {
					st.reveal(float32(focus)*step, float32(focus)*step+w, m.inner)
				}
			}
			row.Translate(-st.place(row), 0)
		})
		// A screen at a time, as far as there is more that way.
		page := m.inner + shelfGap
		y := artTop + artH/2 - 16
		if hovered && st.X > 0.5 {
			if a.shelfArrow(c.Key("prev"), iconBack, T("Previous Page")).Absolute().Left(10).Top(y).Clicked() {
				st.glideTo(st.X - page)
			}
		}
		if hovered && st.X < st.MaxX-0.5 {
			if a.shelfArrow(c.Key("next"), iconForward, T("Next Page")).Absolute().Right(10).Top(y).Clicked() {
				st.glideTo(st.X + page)
			}
		}
	})
}

// shelfArrow is a button over a shelf's edge that scrolls it a screen.
func (a *app) shelfArrow(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c).Size(32, 32).Radius(999).Center().Label(label).FocusRing(false).TextColor(p.text).
		Shadow(0, 2, 10, 0, ui.RGBA(0, 0, 0, 0.25))
	fill := p.bg.Mix(p.text, 0.08)
	if b.Hovered() {
		fill = p.bg.Mix(p.text, 0.16)
	}
	b.Background(fill)
	b.Children(func() { ui.Icon(c, icon).Size(16, 16) })
	return b
}

// shelfItem is an item of a shelf in its style, w wide.
func (a *app) shelfItem(c *ui.Context, secs []music.Section, s, i int, w float32) {
	switch secs[s].Style {
	case music.ShelfHero:
		a.heroCard(c, secs, s, i, w)
	case music.ShelfPortrait:
		a.portraitCard(c, secs, s, i, w)
	case music.ShelfLive:
		a.liveCard(c, secs, s, i, w)
	case music.ShelfBrick:
		a.brickCard(c, secs, s, i, w)
	case music.ShelfCircle:
		a.circleCard(c, secs, s, i, w)
	default:
		if secs[s].Items[i].Kind == music.ItemArtist {
			a.circleCard(c, secs, s, i, w)
			return
		}
		a.squareCard(c, secs, s, i, w)
	}
}

// shelfGrid is a row of a section's items in rows that wrap: cards in the
// columns they keep whatever the number in the last row.
func (a *app) shelfGrid(c *ui.Context, secs []music.Section, r browseRow, m shelfMetrics, top float32) {
	sec := &secs[r.section]
	min := float32(squareMin)
	switch {
	case r.kind == rowTiles:
		min = tileMin
	case allOf(sec.Items, music.ItemArtist):
		min = circleMin
	}
	_, w := m.fit(min)
	ui.Row(c).Gap(shelfGap).AlignItems(ui.Start).Margin(top, 12, 24, 12).Children(func() {
		for i := r.from; i < r.to; i++ {
			if r.kind == rowTiles {
				a.brickCard(c.Key(i), secs, r.section, i, w)
			} else {
				a.shelfItem(c.Key(i), secs, r.section, i, w)
			}
		}
	})
}

// Opening and playing what an item is.

// openExternal opens an address in the listener's browser; tests replace it.
var openExternal = func(url string) { go mygo.Shell.OpenExternal(url) }

// openItem does what a click on item i of section s does: opens its page,
// or plays it, a song with the songs of its section from it on.
func (a *app) openItem(secs []music.Section, s, i int) {
	it := &secs[s].Items[i]
	switch it.Kind {
	case music.ItemAlbum:
		a.open(route{page: pageAlbum, id: itemID(it, it.Album != nil, func() string { return it.Album.ID })})
	case music.ItemPlaylist:
		a.open(route{page: pagePlaylist, id: itemID(it, it.Playlist != nil, func() string { return it.Playlist.ID })})
	case music.ItemArtist:
		switch {
		case it.Page != "":
			// An artist of the library opens as what the library has of
			// them: their library ID isn't one the catalog's page takes.
			a.open(route{page: pageBrowse, id: it.Page})
		default:
			a.open(route{page: pageArtist, id: itemID(it, it.Artist != nil, func() string { return it.Artist.ID })})
		}
	case music.ItemStation:
		a.playStation(itemStation(it))
	case music.ItemSong, music.ItemVideo:
		// A video plays as its song does, its sound alone.
		t := a.sectionTracks(secs, s)
		if at := t.at[i]; at >= 0 && !t.tracks[at].Unavailable {
			a.pl.PlayTrack(t.tracks, at, a.pageOrigin())
		}
	case music.ItemLink:
		switch {
		case it.Page != "":
			a.open(route{page: pageBrowse, id: it.Page})
		case it.URL != "":
			openExternal(it.URL)
		}
	}
}

// itemID is the ID of what it holds, if it has, or else its own.
func itemID(it *music.Item, has bool, id func() string) string {
	if has && id() != "" {
		return id()
	}
	return it.ID
}

// itemStation is the station an item is.
func itemStation(it *music.Item) *music.Station {
	if it.Station != nil {
		return it.Station
	}
	return &music.Station{ID: it.ID, Name: it.Title, ImageURL: it.ImageURL}
}

// playItem plays an item from its card without opening it, for what its
// card has a play button for: an album, a playlist or a station.
func (a *app) playItem(it *music.Item) func() {
	switch it.Kind {
	case music.ItemAlbum:
		id := itemID(it, it.Album != nil, func() string { return it.Album.ID })
		return func() { a.playAlbum(id) }
	case music.ItemPlaylist:
		id := itemID(it, it.Playlist != nil, func() string { return it.Playlist.ID })
		return func() { a.loadPlaylist(id, true) }
	case music.ItemStation:
		return func() { a.playStation(itemStation(it)) }
	}
	return nil
}

// playAlbum plays an album from its first song, loading it first if it
// hasn't.
func (a *app) playAlbum(id string) {
	lib := a.library
	if d := lib.albums[id]; d != nil && d.album != nil {
		a.pl.PlayQueue(d.album.Tracks, firstPlayable(d.album.Tracks), albumOrigin(id))
		return
	}
	go func() {
		al, err := answered(lib.src.Album(id))
		a.post(func() {
			if err != nil {
				a.actionFailed(err, T("Couldn't load this album"))
				return
			}
			if d := lib.albums[id]; d == nil || d.album == nil {
				lib.albums[id] = &albumData{album: al}
			}
			a.pl.PlayQueue(al.Tracks, firstPlayable(al.Tracks), albumOrigin(id))
		})
	}()
}

// itemMenu is the menu of an item, as far as it has one: a song's, an
// album's, a playlist's or an artist's.
func (a *app) itemMenu(it *music.Item) func(m *ui.Menu) {
	switch it.Kind {
	case music.ItemSong, music.ItemVideo:
		if it.Track != nil {
			return a.songItems(it.Track, "")
		}
	case music.ItemAlbum:
		al := music.Album{ID: it.ID, Name: it.Title, URL: it.URL}
		if it.Album != nil {
			al = *it.Album
			al.ID = itemID(it, true, func() string { return it.Album.ID })
		}
		if al.Name == "" {
			al.Name = it.Title
		}
		if al.URL == "" {
			al.URL = it.URL
		}
		return a.albumItems(al)
	case music.ItemPlaylist:
		pl := music.Playlist{ID: it.ID, Name: it.Title, URL: it.URL}
		if it.Playlist != nil {
			pl = *it.Playlist
			pl.ID = itemID(it, true, func() string { return it.Playlist.ID })
		}
		if pl.Name == "" {
			pl.Name = it.Title
		}
		if pl.URL == "" {
			pl.URL = it.URL
		}
		return a.playlistItemsOf(pl)
	case music.ItemArtist:
		ar := music.Artist{ID: it.ID, Name: it.Title, URL: it.URL}
		if it.Artist != nil {
			ar = *it.Artist
			ar.ID = itemID(it, true, func() string { return it.Artist.ID })
		}
		if ar.URL == "" {
			ar.URL = it.URL
		}
		return a.artistItems(ar)
	}
	return nil
}

// shelfTracks are the songs of a section's items, which play together, and
// where each item's is among them, -1 for an item that is no song.
type shelfTracks struct {
	first  *music.Item // the items they are of, as they were
	n      int
	tracks []music.Track
	at     []int
}

// sectionTracks returns the songs of section s, made once for its items as
// they are: a library's songs page through thousands.
func (a *app) sectionTracks(secs []music.Section, s int) *shelfTracks {
	items := secs[s].Items
	var first *music.Item
	if len(items) > 0 {
		first = &items[0]
	}
	if a.shelfSongs == nil {
		a.shelfSongs = map[int]*shelfTracks{}
	}
	if t := a.shelfSongs[s]; t != nil && t.first == first && t.n == len(items) {
		return t
	}
	t := &shelfTracks{first: first, n: len(items), at: make([]int, len(items))}
	for i := range items {
		t.at[i] = -1
		if tr := items[i].Track; tr != nil && (items[i].Kind == music.ItemSong || items[i].Kind == music.ItemVideo) {
			t.at[i] = len(t.tracks)
			t.tracks = append(t.tracks, *tr)
		}
	}
	a.shelfSongs[s] = t
	return t
}

// The cards.

// itemColor is the color of an item's picture, which shows while it loads.
func itemColor(it *music.Item, fallback ui.Color) ui.Color {
	return hexColor(it.Colors.BG, fallback)
}

// hexColor is a color as Apple Music gives it, rrggbb, or fallback.
func hexColor(s string, fallback ui.Color) ui.Color {
	if c, ok := hexColorOK(s); ok {
		return c
	}
	return fallback
}

// hexColorOK is the color s gives, and whether it gives one.
func hexColorOK(s string) (ui.Color, bool) {
	v, err := strconv.ParseUint(s, 16, 32)
	if len(s) != 6 || err != nil {
		return ui.Transparent, false
	}
	return ui.RGB(uint8(v>>16), uint8(v>>8), uint8(v)), true
}

// wideArt is the size wide pictures are asked for at, the widest shown.
const wideArt = 1200

// itemPicture fills its parent with an item's picture, wide if wide and it
// has one, else its square one, corners rounded by radius.
func (a *app) itemPicture(c *ui.Context, it *music.Item, wide bool, px int, radius float32) {
	url := it.ImageURL
	if wide && it.WideURL != "" {
		url, px = music.Wide(it.WideURL, wideArt, wideArt*9/16), wideArt
	}
	a.pictureAt(c, it, url, px, radius)
}

// pictureAt is itemPicture of the picture at url, which it asks for as px
// wide.
func (a *app) pictureAt(c *ui.Context, it *music.Item, url string, px int, radius float32) {
	bmp := bitmap(a.art.get(sized(url, px), false))
	box := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius)
	in := artIn(box, bmp)
	// The picture's own color while it comes; without one, the blank and
	// what kind of thing goes there.
	_, colored := hexColorOK(it.Colors.BG)
	if in < 1 {
		box.Background(itemColor(it, a.pal.blank))
	}
	box.Children(func() {
		if in < 1 && !colored {
			icon := iconNote
			if it.Kind == music.ItemArtist {
				icon = iconUser
			}
			a.blankIcon(c, icon, 1-in)
		}
		artImage(c, bmp, radius, in)
	})
}

// itemButton is the button of an item's card: it opens or plays the item as
// it is clicked, and has its menu.
func (a *app) itemButton(c *ui.Context, secs []music.Section, s, i int) ui.Element {
	it := &secs[s].Items[i]
	label := it.Title
	if label == "" && it.Track != nil {
		label = it.Track.Name
	}
	b := ui.ButtonBase(c).Column().AlignItems(ui.Stretch).Justify(ui.Start).FocusRing(false).Label(label)
	if menu := a.itemMenu(it); menu != nil {
		b.ContextMenu(menu)
	}
	if b.Clicked() {
		a.openItem(secs, s, i)
	}
	return b
}

// artRadius rounds the corners of a shelf's pictures, as Apple Music's.
const artRadius = 7

// hoverShade darkens a picture under the pointer, as Apple Music's do. It
// fades as the card's play button comes and goes, in step with it.
func (a *app) hoverShade(c *ui.Context, hovered bool, radius float32) {
	shade := ui.Box(c.Key("shade")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius).IgnoreInput(true)
	target := ui.Transparent
	if hovered {
		target = ui.RGBA(0, 0, 0, 0.18)
	}
	shade.Background(shade.AnimateColor("shade", target, ui.Snappy))
}

// explicitMark is the E an explicit song or album has after its title.
func (a *app) explicitMark(c *ui.Context) {
	p := a.pal
	ui.Box(c).Size(12, 12).Radius(2).Center().Shrink(0).Background(p.muted).Label(T("Explicit")).Children(func() {
		ui.Text(c, "E").FontSize(8.5).FontWeight(700).TextColor(p.bg)
	})
}

// cardLines are the title and line under a card's picture.
func (a *app) cardLines(c *ui.Context, it *music.Item, center bool) {
	p := a.pal
	title := ui.Row(c).Gap(5).AlignItems(ui.Start)
	if center {
		title.Justify(ui.Center)
	}
	title.Children(func() {
		t := ui.Text(c, it.Title).FontSize(12.5).FontWeight(500).MaxLines(2).LineHeight(1.3).Shrink(1).MinWidth(0)
		if center {
			t.TextAlign(ui.Center).MaxLines(1)
		}
		if it.Explicit {
			ui.Box(c).Margin(2, 0, 0, 0).Children(func() { a.explicitMark(c) })
		}
	})
	if it.Subtitle != "" {
		sub := ui.Text(c, it.Subtitle).FontSize(12).TextColor(p.muted).SingleLine()
		if center {
			sub.TextAlign(ui.Center)
		}
	}
}

// squareCard is a square picture over the item's title and a line under it:
// a play button shows on it under the pointer, for what plays.
func (a *app) squareCard(c *ui.Context, secs []music.Section, s, i int, w float32) {
	it := &secs[s].Items[i]
	b := a.itemButton(c, secs, s, i).Width(w).Shrink(0).Gap(7)
	hovered := b.Hovered()
	b.Children(func() {
		ui.Box(c).Size(w, w).Radius(artRadius).Border(0.5, a.pal.hairline).Children(func() {
			a.itemPicture(c, it, false, 400, artRadius)
			a.hoverShade(c, hovered, artRadius)
			if play := a.playItem(it); play != nil {
				a.coverPlayButton(c, hovered, play)
			}
		})
		ui.Column(c).Gap(1).Children(func() { a.cardLines(c, it, false) })
	})
}

// circleCard is an artist's round picture over their name.
func (a *app) circleCard(c *ui.Context, secs []music.Section, s, i int, w float32) {
	it := &secs[s].Items[i]
	b := a.itemButton(c, secs, s, i).Width(w).Shrink(0).Gap(8)
	hovered := b.Hovered()
	b.Children(func() {
		ui.Box(c).Size(w, w).Radius(w / 2).Children(func() {
			a.itemPicture(c, it, false, 400, w/2)
			a.hoverShade(c, hovered, w/2)
		})
		ui.Column(c).Gap(1).Children(func() { a.cardLines(c, it, true) })
	})
}

// brickCard is a picture wider than tall: a video's, its title under it, or
// a category's, its title over it.
func (a *app) brickCard(c *ui.Context, secs []music.Section, s, i int, w float32) {
	it := &secs[s].Items[i]
	over := it.Kind == music.ItemLink
	h := w * 9 / 16
	b := a.itemButton(c, secs, s, i).Width(w).Shrink(0).Gap(7)
	hovered := b.Hovered()
	b.Children(func() {
		ui.Box(c).Size(w, h).Radius(artRadius).Children(func() {
			a.itemPicture(c, it, true, 600, artRadius)
			if over {
				ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(h * 0.5).Radius(artRadius).
					LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
						{Color: ui.RGBA(0, 0, 0, 0), At: 0},
						{Color: ui.RGBA(0, 0, 0, 0.45), At: 1},
					}})
				ui.Text(c, it.Title).Absolute().Left(12).Right(12).Bottom(10).FontSize(14).FontWeight(600).
					TextColor(white).SingleLine()
			}
			a.hoverShade(c, hovered, artRadius)
		})
		if !over {
			ui.Column(c).Gap(1).Children(func() { a.cardLines(c, it, false) })
		}
	})
}

// heroClear is how far what a hero says keeps from its picture.
const heroClear = 12

// heroHead is the height of what the heroes of a section say over their
// pictures: that of the one that says most, for the pictures to stand in a
// row, and heroClear under it. Its lines are measured, as fonts differ in
// how tall theirs are: a head as high as three lines of Latin text ran the
// last line of a Chinese one down to the picture on Linux.
func heroHead(c *ui.Context, sec *music.Section) float32 {
	line := func(text string, size float32, weight int) float32 {
		if text == "" {
			return 0
		}
		_, h := c.MeasureText(0, ui.Span{Text: text, Size: size, Weight: weight})
		return h
	}
	var most float32
	for i := range sec.Items {
		it := &sec.Items[i]
		h := line(it.Title, 15, 0) + line(it.Subtitle, 15, 0)
		if it.Badge != "" {
			h += line(it.Badge, 10.5, 600) + heroBadgeGap
		}
		most = max(most, h)
	}
	return most + heroClear
}

// heroBadgeGap is the gap under a hero's badge.
const heroBadgeGap = 3

// heroCard is a picture that leads a page: over it, a badge, its title and a
// line, and on it, what the editors say of it.
func (a *app) heroCard(c *ui.Context, secs []music.Section, s, i int, w float32) {
	p := a.pal
	it := &secs[s].Items[i]
	h := w * 9 / 16
	b := a.itemButton(c, secs, s, i).Width(w).Shrink(0)
	hovered := b.Hovered()
	b.Children(func() {
		ui.Column(c).Height(heroHead(c, &secs[s])).Justify(ui.End).Padding(0, 0, heroClear, 0).Children(func() {
			if it.Badge != "" {
				ui.Text(c, it.Badge).FontSize(10.5).FontWeight(600).TextColor(p.muted).SingleLine().Margin(0, 0, heroBadgeGap, 0)
			}
			ui.Text(c, it.Title).FontSize(15).SingleLine()
			if it.Subtitle != "" {
				ui.Text(c, it.Subtitle).FontSize(15).TextColor(p.muted).SingleLine()
			}
		})
		ui.Box(c).Size(w, h).Radius(artRadius+1).Border(0.5, p.hairline).Children(func() {
			a.itemPicture(c, it, true, 600, artRadius+1)
			if it.Note != "" {
				ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(h * 0.4).Radius(artRadius + 1).
					LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
						{Color: ui.RGBA(0, 0, 0, 0), At: 0},
						{Color: ui.RGBA(0, 0, 0, 0.5), At: 1},
					}})
				ui.Text(c, it.Note).Absolute().Left(14).Right(14).Bottom(12).FontSize(12).TextColor(white).MaxLines(2).LineHeight(1.35)
			}
			a.hoverShade(c, hovered, artRadius+1)
			if play := a.playItem(it); play != nil {
				a.coverPlayButton(c, hovered, play)
			}
		})
	})
}

// portraitRadius rounds the corners of a portrait card, as Apple Music's.
const portraitRadius = 8

// portraitInk is the color of the words over a portrait card's foot, in
// the picture's own for them if it gives one that can be read on ground,
// else white or black by how dark ground is.
func portraitInk(it *music.Item, ground ui.Color) ui.Color {
	read := func(ink ui.Color) float64 {
		a, b := luminance(ink)+0.05, luminance(ground)+0.05
		return max(a, b) / min(a, b)
	}
	if ink, ok := hexColorOK(it.Colors.Text1); ok && read(ink) >= 4.5 {
		return ink
	}
	if dark := ui.RGB(18, 18, 20); read(dark) > read(white) {
		return dark
	}
	return white
}

// portraitCard is a card of Home's picks, three to four. Its picture is a
// tall one that fills it, which has its own title, moving where it has a
// video, the words and their scrim over it still; or, without one, the
// cover over the card's own ground, the cover's color, that it fades into.
// The words stand on its foot: why it is there, then who's on it, or else
// its title and a line.
func (a *app) portraitCard(c *ui.Context, secs []music.Section, s, i int, w float32) {
	it := &secs[s].Items[i]
	h := w * 4 / 3
	b := a.itemButton(c, secs, s, i).Width(w).Shrink(0)
	hovered := b.Hovered()
	tall := it.TallURL != ""
	says := it.Badge != "" || it.Artists != "" || !tall
	b.Children(func() {
		ground := itemColor(it, a.pal.blank)
		ink := white
		box := ui.Box(c).Size(w, h).Radius(portraitRadius).Border(0.5, a.pal.hairline)
		if !tall {
			ink = portraitInk(it, ground)
		}
		box.Children(func() {
			if !tall {
				// The ground is only the cover's lower part, that it
				// doesn't ring the cover's rounded corners.
				ui.Box(c.Key("ground")).Absolute().Left(0).Right(0).Bottom(0).Top(w * 0.5).Radius(portraitRadius).Background(ground)
			}
			if tall {
				a.pictureAt(c, it, music.Wide(it.TallURL, 900, 1200), 900, portraitRadius)
				a.motionOver(c, box, it.TallVideoURL, w, h, motionCard, portraitRadius)
				if says {
					// A scrim for the words, as the picture may be light
					// at its foot.
					ui.Box(c.Key("scrim")).Absolute().Left(0).Right(0).Bottom(0).Height(h * 0.45).Radius(portraitRadius).
						LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
							{Color: ui.RGBA(0, 0, 0, 0), At: 0},
							{Color: ui.RGBA(0, 0, 0, 0.6), At: 1},
						}})
				}
			} else {
				ui.Box(c.Key("cover")).Absolute().Top(0).Left(0).Right(0).Height(w).Children(func() {
					a.itemPicture(c, it, false, 600, portraitRadius)
				})
				// The cover's foot melts into the ground it stands on, the
				// last of it past the cover's own edge.
				ui.Box(c.Key("melt")).Absolute().Left(0).Right(0).Top(w * 0.6).Height(w*0.4 + 1).
					LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
						{Color: ground.Alpha(0), At: 0},
						{Color: ground, At: 1},
					}})
			}
			if says {
				ui.Column(c.Key("words")).Absolute().Left(14).Right(14).Bottom(14).Gap(2).Children(func() {
					if it.Badge != "" {
						ui.Text(c, it.Badge).FontSize(11.5).FontWeight(600).TextColor(ink.Alpha(0.7)).SingleLine()
					}
					switch {
					case it.Artists != "":
						ui.Text(c, it.Artists).FontSize(12.5).FontWeight(500).TextColor(ink).MaxLines(3).LineHeight(1.3)
					case !tall:
						ui.Text(c, it.Title).FontSize(14).FontWeight(700).TextColor(ink).MaxLines(2).LineHeight(1.25)
						if it.Subtitle != "" {
							ui.Text(c, it.Subtitle).FontSize(12).TextColor(ink.Alpha(0.7)).SingleLine()
						}
					}
				})
			}
			a.hoverShade(c, hovered, portraitRadius)
			if play := a.playItem(it); play != nil {
				a.coverPlayButton(c, hovered, play)
			}
		})
	})
}

// liveCard is a station on the air: its wide picture, under it the station,
// and what's on and when.
func (a *app) liveCard(c *ui.Context, secs []music.Section, s, i int, w float32) {
	p := a.pal
	it := &secs[s].Items[i]
	h := w * 9 / 16
	b := a.itemButton(c, secs, s, i).Width(w).Shrink(0).Gap(8)
	hovered := b.Hovered()
	b.Children(func() {
		ui.Box(c).Size(w, h).Radius(artRadius + 1).Children(func() {
			a.itemPicture(c, it, true, 600, artRadius+1)
			a.hoverShade(c, hovered, artRadius+1)
			if play := a.playItem(it); play != nil {
				a.coverPlayButton(c, hovered, play)
			}
		})
		ui.Column(c).Gap(1).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if it.Badge != "" {
					ui.Text(c, it.Badge).FontSize(10.5).FontWeight(700).TextColor(p.accent).SingleLine()
				}
				ui.Text(c, it.Title).FontSize(13).FontWeight(500).SingleLine().Shrink(1).MinWidth(0)
			})
			if it.Subtitle != "" {
				ui.Text(c, it.Subtitle).FontSize(12).TextColor(p.muted).MaxLines(2).LineHeight(1.3)
			}
			if it.Note != "" {
				ui.Text(c, it.Note).FontSize(12).TextColor(p.muted).SingleLine()
			}
		})
	})
}

// The rows of songs.

// tracksRows is how many songs a column of a tracks shelf has.
const tracksRows = 4

// trackCellHeight is the height of a song in a tracks shelf.
const trackCellHeight = 52

// trackCell is a song of a tracks shelf: its cover, title and artists, and
// its menu at the end. A click plays its section from it.
func (a *app) trackCell(c *ui.Context, secs []music.Section, s, i int) {
	p := a.pal
	it := &secs[s].Items[i]
	t := a.sectionTracks(secs, s)
	var tr *music.Track
	if at := t.at[i]; at >= 0 {
		tr = &t.tracks[at]
	}
	b := a.itemButton(c, secs, s, i).Row().Height(trackCellHeight).Gap(10).AlignItems(ui.Center).PaddingX(0)
	hovered := b.Hovered()
	playing := tr != nil && sameTrack(a.pl.Current(), tr)
	b.Children(func() {
		ui.Box(c).Size(40, 40).Radius(4).Shrink(0).Children(func() {
			a.itemPicture(c, it, false, 96, 4)
			if hovered && tr != nil {
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(4).Center().Background(ui.RGBA(0, 0, 0, 0.35)).Children(func() {
					ui.Icon(c, iconPlay).Size(14, 14).TextColor(white)
				})
			}
		})
		// The text and the menu share the line over them, as Music's rows.
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Height(trackCellHeight).AlignItems(ui.Center).
			BorderWidth(0.5, 0, 0, 0).BorderColor(p.hairline).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					color := p.text
					if playing {
						color = p.accent
					}
					ui.Text(c, it.Title).FontSize(13).TextColor(color).SingleLine().Shrink(1).MinWidth(0)
					if it.Explicit {
						a.explicitMark(c)
					}
				})
				if it.Subtitle != "" {
					ui.Text(c, it.Subtitle).FontSize(12).TextColor(p.muted).SingleLine()
				}
			})
			if tr != nil {
				a.songMenu(c.Key("menu"), tr, 15, p.muted)
			}
		})
	})
}

// linkColumns is how many columns a section of links takes: a line each
// would stretch a wide window's links across it.
func (m shelfMetrics) linkColumns() int {
	switch {
	case m.inner < 700:
		return 1
	case m.inner < 1100:
		return 2
	}
	return 3
}

// linkRow is a link of a section of links, a line each in its column, with
// an arrow to where it goes.
func (a *app) linkRow(c *ui.Context, secs []music.Section, s, i int) ui.Element {
	p := a.pal
	it := &secs[s].Items[i]
	b := a.itemButton(c, secs, s, i).Row().Height(44).AlignItems(ui.Center).Gap(10).
		BorderWidth(0, 0, 0.5, 0).BorderColor(p.hairline)
	color := p.text
	if b.Hovered() {
		color = p.muted
	}
	b.Children(func() {
		ui.Text(c, it.Title).FontSize(14).TextColor(color).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if it.Subtitle != "" {
			ui.Text(c, it.Subtitle).FontSize(12.5).TextColor(p.muted).SingleLine()
		}
		icon := iconForward
		if it.Page == "" && it.URL != "" {
			icon = iconExternal
		}
		ui.Icon(c, icon).Size(14, 14).TextColor(p.muted)
	})
	return b
}

// featured is one item shown large: its picture, and beside it what it is,
// its title and what the editors say of it, as an artist's latest release.
func (a *app) featured(c *ui.Context, secs []music.Section, s int, top float32) {
	p := a.pal
	it := &secs[s].Items[0]
	b := a.itemButton(c, secs, s, 0).Row().Gap(18).AlignItems(ui.Center).Margin(top, 12, 0, 12)
	hovered := b.Hovered()
	b.Children(func() {
		ui.Box(c).Size(180, 180).Radius(artRadius).Shrink(0).Border(0.5, p.hairline).Children(func() {
			a.itemPicture(c, it, false, 400, artRadius)
			a.hoverShade(c, hovered, artRadius)
			if play := a.playItem(it); play != nil {
				a.coverPlayButton(c, hovered, play)
			}
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			if it.Badge != "" {
				ui.Text(c, it.Badge).FontSize(11).FontWeight(600).TextColor(p.muted).SingleLine()
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, it.Title).FontSize(17).FontWeight(600).MaxLines(2).LineHeight(1.25).Shrink(1).MinWidth(0)
				if it.Explicit {
					a.explicitMark(c)
				}
			})
			if it.Subtitle != "" {
				ui.Text(c, it.Subtitle).FontSize(13).TextColor(p.muted).SingleLine()
			}
			if it.Note != "" {
				ui.Text(c, it.Note).FontSize(12.5).TextColor(p.muted).MaxLines(3).LineHeight(1.4).Margin(4, 0, 0, 0)
			}
		})
	})
}

// shelfText is a section's words, its Subtitle, as paragraphs.
func (a *app) shelfText(c *ui.Context, sec *music.Section, top float32) {
	p := a.pal
	ui.Column(c).Margin(top, 12, 0, 12).Gap(10).MaxWidth(760).Children(func() {
		i := 0
		for para := range strings.SplitSeq(sec.Subtitle, "\n") {
			if para = strings.TrimSpace(para); para != "" {
				ui.Text(c.Key(i), para).FontSize(13.5).LineHeight(1.5).TextColor(p.muted)
				i++
			}
		}
	})
}
