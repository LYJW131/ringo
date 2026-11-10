package app

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// artistLinks shows a track's artists as its credits read, each a link to
// their page in color, brightening to hover under the pointer, and then,
// with album, a link to its album. An artist credited by name alone is
// plain text.
func (a *app) artistLinks(c *ui.Context, tr *music.Track, color, hover ui.Color, album ...bool) ui.Element {
	return ui.RichText(c).TextColor(color).Children(func() {
		for i, ar := range tr.Artists {
			if i > 0 {
				ui.Text(c, ", ")
			}
			if ar.ID != "" {
				a.link(c.Key(i), ar.Name, color, hover, route{page: pageArtist, id: ar.ID})
			} else {
				ui.Text(c, ar.Name)
			}
		}
		if len(album) > 0 && album[0] && tr.Album.Name != "" {
			if len(tr.Artists) > 0 {
				ui.Text(c, " — ")
			}
			if tr.Album.ID != "" {
				a.link(c.Key("album"), tr.Album.Name, color, hover, route{page: pageAlbum, id: tr.Album.ID})
			} else {
				ui.Text(c, tr.Album.Name)
			}
		}
	})
}

// link is text in a RichText that opens a page.
func (a *app) link(c *ui.Context, text string, color, hover ui.Color, r route) {
	link := ui.Link(c, text, "").TextColor(color)
	if link.Hovered() {
		link.TextColor(hover)
	}
	// Once built: a song's row is in a list, which a new page resets.
	link.OnClick(func() { a.openPage(r) })
}

// openPage opens a page in the main window, from wherever a link to it
// shows: over Now Playing, which closes, or the notch.
func (a *app) openPage(r route) {
	a.open(r)
	a.showMain()
	a.redraw()
}

// openArtist opens an artist's page in the main window.
func (a *app) openArtist(id string) { a.openPage(route{page: pageArtist, id: id}) }

// artistData is what the app has of an artist: who they are, with their songs
// and albums, and their page's sections.
type artistData struct {
	page    *music.ArtistPage
	loading bool
	err     string
	tab     int // what a page without sections lists: artistSongs or artistAlbums
	// secs are the page's sections, their biography not among them.
	secs []music.Section
}

// What an artist's page without sections lists.
const (
	artistSongs = iota
	artistAlbums
)

func (a *app) loadArtist(id string) {
	lib := a.library
	d := lib.artists[id]
	if d == nil {
		d = &artistData{}
		lib.artists[id] = d
	}
	if d.page != nil || d.loading {
		return
	}
	d.loading, d.err = true, ""
	go func() {
		ar, err := answered(lib.src.Artist(id))
		a.post(func() {
			d.loading = false
			if err != nil {
				d.err = a.problem(err, T("Couldn't load this artist"))
				return
			}
			d.page, d.secs = ar, artistSections(ar)
		})
	}()
}

// artistSections are the sections of an artist's page, as the source gave
// them: their biography is behind the button by their name, as in Music on
// the Mac, not at the page's foot.
func artistSections(page *music.ArtistPage) []music.Section {
	return slices.DeleteFunc(slices.Clone(page.Sections), func(s music.Section) bool { return s.Style == music.ShelfText })
}

// artistPage shows an artist as Apple Music does: their picture and name
// over the sections of their page. A page without sections, as of an
// artist of the library, lists their songs or their albums.
func (a *app) artistPage(c *ui.Context) {
	id := a.route.id
	d := a.artists[id]
	if d == nil {
		a.loadArtist(id)
		d = a.artists[id]
	}
	if d.page == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadArtist(id) })
			return
		}
		// Most artists' pages are of shelves.
		s := a.pageShimmer(c)
		a.scrollPage(c, func() {
			a.artistHeaderSkeleton(c, s)
			a.shelfSkeleton(c, s)
		})
		return
	}
	if len(d.secs) == 0 {
		a.artistLists(c, d)
		return
	}
	secs := d.secs
	rows := a.artistRows(c, secs)
	// Tinted from their photo where it heads the page; their picture
	// moving fades into the page as it is.
	tint := d.page.Artist.Colors
	if d.page.Artist.WideURL != "" {
		tint = music.Colors{}
	}
	a.detailList(c, len(rows)+1, tint, func(i int) {
		if i == 0 {
			a.artistHeader(c, d)
			return
		}
		r := rows[i-1]
		switch {
		case r.pair:
			a.latestAndTop(c, secs)
		default:
			a.shelfRow(c, secs, r.browseRow)
		}
	})
}

// artistRow is a row of an artist's page: one of its sections', or the
// latest release beside the top songs.
type artistRow struct {
	browseRow
	pair bool
}

// pairWidth is the least width of a page that has the latest release
// beside the top songs, as Apple Music does; narrower, they go one over
// the other.
const pairWidth = 780

// artistRows lays the sections of an artist's page out in rows.
func (a *app) artistRows(c *ui.Context, secs []music.Section) []artistRow {
	pair := len(secs) >= 2 && secs[0].Style == music.ShelfFeatured && len(secs[0].Items) > 0 &&
		secs[1].Style == music.ShelfTracks && a.shelfMetrics(c).inner >= pairWidth
	var rows []artistRow
	if pair {
		rows = append(rows, artistRow{pair: true})
	}
	for _, r := range a.shelfLayout(c, secs, false) {
		if !pair || r.section >= 2 {
			rows = append(rows, artistRow{browseRow: r})
		}
	}
	return rows
}

// latestAndTop is an artist's latest release beside their top songs, the
// release as small as leaves the songs two columns, as Apple Music has them.
func (a *app) latestAndTop(c *ui.Context, secs []music.Section) {
	m := a.shelfMetrics(c)
	left := max(270, m.inner*0.3)
	const gap = 40
	ui.Row(c).AlignItems(ui.Start).Gap(gap).Children(func() {
		ui.Column(c).Width(left).Shrink(0).Children(func() {
			a.shelfTitle(c, &secs[0], true)
			a.latestCard(c, secs)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			a.shelfTitle(c, &secs[1], true)
			a.shelf(c, secs, 1, shelfMetrics{inner: m.inner - left - gap}, 0)
		})
	})
}

// latestCard is an artist's latest release: its cover, and beside it when
// it came out, its title and what it holds.
func (a *app) latestCard(c *ui.Context, secs []music.Section) {
	p := a.pal
	it := &secs[0].Items[0]
	const size = 118
	b := a.itemButton(c, secs, 0, 0).Row().Gap(14).AlignItems(ui.Center).Margin(0, 12)
	hovered := b.Hovered()
	b.Children(func() {
		ui.Box(c).Size(size, size).Radius(artRadius).Shrink(0).Border(0.5, p.hairline).Children(func() {
			a.itemPicture(c, it, false, 400, artRadius)
			a.hoverShade(c, hovered, artRadius)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			if it.Badge != "" {
				ui.Text(c, it.Badge).FontSize(10.5).FontWeight(600).TextColor(p.muted).SingleLine()
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, it.Title).FontSize(13.5).FontWeight(500).MaxLines(2).LineHeight(1.3).Shrink(1).MinWidth(0)
				if it.Explicit {
					a.explicitMark(c)
				}
			})
			if it.Subtitle != "" {
				ui.Text(c, it.Subtitle).FontSize(12.5).TextColor(p.muted).SingleLine()
			}
			// The plus that adds it to the library, as Music on the Mac has
			// it here, then its star; neither until the library says which.
			if it.Kind == music.ItemAlbum && !a.library.signedOut() {
				if in, known := a.albumInLibrary(it.ID); known {
					fav := it.Album != nil && it.Album.Favorite
					ui.Box(c).Margin(8, 0, 0, 0).Children(func() {
						a.libraryButton(c, music.ItemAlbum, it.ID, in, fav, true)
					})
				}
			}
		})
	})
}

// The head of an artist's page, as Music on the Mac has it. Where Apple
// made their picture moving, it plays across the page from its top edge,
// sixteen to nine, fading into the page, their name and the buttons in the
// middle over its foot. Others have their round photo over their name, on
// the page tinted from the photo (detailList), though Apple has banners of
// many of them.
const (
	// artistHeadMax is how much of the window's height the moving head
	// takes at most, so a wide window still shows the page under it.
	artistHeadMax = 0.75
	// artistPlaySize and artistButtonSize are the round buttons under their
	// name: the one that plays, and those beside it.
	artistPlaySize   = 72
	artistButtonSize = 46
)

// artistHeader heads an artist's page: their picture moving across it with
// their name over its foot, or their round photo over their name; and under
// the name, the buttons that show their biography, play their top songs and
// make them a favorite.
func (a *app) artistHeader(c *ui.Context, d *artistData) {
	p := a.pal
	ar := &d.page.Artist
	if ar.WideURL == "" {
		ui.Column(c).AlignItems(ui.Center).Margin(32, 12, 28, 12).Children(func() {
			ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).
				ContextMenu(a.artistItems(*ar)).Children(func() {
				a.coverFillWith(c, ar.ImageURL, 600, artistPhotoSize/2, iconUser)
			})
			ui.Text(c, ar.Name).FontSize(40).FontWeight(800).LetterSpacing(-0.8).MaxLines(2).TextAlign(ui.Center).Margin(26, 0, 0, 0)
			ui.Row(c).Gap(24).AlignItems(ui.Center).Margin(20, 0, 0, 0).Children(func() { a.artistButtons(c, d, false) })
		})
		return
	}
	// Across the page from edge to edge, from under the bar at its top.
	top, _ := a.insets(c)
	w, wh := c.Size()
	w -= sidebarWidth
	h := min(w*9/16, wh*artistHeadMax)
	head := ui.Box(c).Height(h).Margin(-(top + pageGap), 12-pagePad, 6, 12-pagePad).Background(hexColor(ar.Colors.BG, p.hover)).
		ContextMenu(a.artistItems(*ar))
	head.Children(func() {
		// The video's first frame until it plays: cut as it is.
		bmp := bitmap(a.art.get(sized(music.Wide(ar.WideURL, wideArt, wideArt*9/16), wideArt), false))
		box := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0)
		in := artIn(box, bmp)
		box.Children(func() {
			if bmp != nil && in > 0 {
				ui.Image(c, bmp).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).ObjectPosition(0.5, 0.4).Opacity(in)
			}
		})
		a.motionOver(c, head, ar.VideoURL, w, h, motionArtist, 0, 0.5, 0.4)
		// Darker toward its foot, where their name is, and then the page.
		ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(h * 0.6).
			LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
				{Color: ui.RGBA(0, 0, 0, 0), At: 0},
				{Color: ui.RGBA(0, 0, 0, 0.35), At: 0.55},
				{Color: p.bg, At: 1},
			}})
		ui.Column(c).Absolute().Left(pagePad).Right(pagePad).Bottom(54).AlignItems(ui.Center).Children(func() {
			ui.Text(c, ar.Name).FontSize(40).FontWeight(800).LetterSpacing(-0.8).TextColor(white).SingleLine().
				TextShadow(0, 1, 12, ui.RGBA(0, 0, 0, 0.35))
			ui.Row(c).Gap(24).AlignItems(ui.Center).Margin(18, 0, 0, 0).Children(func() { a.artistButtons(c, d, true) })
		})
	})
}

// artistButtons show an artist's biography, play their top songs and make
// them a favorite, round and in the middle: the one that plays large and white, the others in a
// shade. Over their picture moving, they're in its colors whatever the
// appearance.
func (a *app) artistButtons(c *ui.Context, d *artistData, over bool) {
	p := a.pal
	ar := &d.page.Artist
	fill, ink := p.text.Alpha(0.08), p.text
	playFill, playInk := p.text, p.bg
	if over || p.dark {
		fill, ink, playFill, playInk = ui.RGBA(255, 255, 255, 0.14), white, white, ui.RGB(0, 0, 0)
	}
	// Their biography, born or formed, and genre, over the window; greyed
	// out for an artist Apple wrote nothing of.
	about := a.artistRound(c.Key("about"), T("About"), fill)
	about.Tooltip(T("About")).Children(func() { ui.Icon(c, iconInfo).Size(20, 20).TextColor(ink) })
	if strings.TrimSpace(ar.Bio) == "" {
		about.Disabled(true).Opacity(0.4)
	} else if about.Clicked() {
		a.bioOpen = true
	}
	if songs := d.page.Tracks; len(songs) > 0 {
		b := ui.ButtonBase(c.Key("play")).Size(artistPlaySize, artistPlaySize).Radius(999).Center().Background(playFill).TextColor(playInk).
			Label(T("Play")).FocusRing(false).Shadow(0, 6, 18, -4, ui.RGBA(0, 0, 0, 0.3))
		if b.Hovered() {
			b.Background(playFill.Mix(playInk, 0.08))
		}
		b.Children(func() { ui.Icon(c, iconPlay).Size(30, 30).Margin(0, 0, 0, 4) })
		if b.Clicked() {
			if a.pl.Shuffle() {
				a.pl.ToggleShuffle()
			}
			a.pl.PlayQueue(songs, firstPlayable(songs), artistOrigin(ar.ID))
		}
	}
	on := a.library.isFavorite(music.ItemArtist, ar.ID, ar.Favorite)
	icon, color, label := iconStar, ink, T("Favorite")
	if on {
		icon, color, label = iconStarFill, p.accent, T("Undo Favorite")
	}
	b := a.artistRound(c.Key("favorite"), label, fill)
	b.Tooltip(label).Children(func() { ui.Icon(c, icon).Size(20, 20).TextColor(color) })
	if b.Clicked() {
		a.setFavorite(music.ItemArtist, ar.ID, !on)
	}
}

// artistRound is a round button beside the one that plays an artist's top
// songs.
func (a *app) artistRound(c *ui.Context, label string, fill ui.Color) ui.Element {
	b := ui.ButtonBase(c).Size(artistButtonSize, artistButtonSize).Shrink(0).Radius(999).Center().Label(label).FocusRing(false)
	if b.Hovered() {
		fill = fill.Mix(ui.RGB(255, 255, 255), 0.12)
	}
	return b.Background(fill)
}

// artistLists is the page of an artist without sections: who they are over
// their songs or their albums.
func (a *app) artistLists(c *ui.Context, d *artistData) {
	header := func(c *ui.Context) { a.artistListsHeader(c, d) }
	tab := d.tab
	switch {
	case len(d.page.Tracks) == 0:
		tab = artistAlbums
	case len(d.page.Albums) == 0:
		tab = artistSongs
	}
	if tab == artistAlbums {
		a.scrollPage(c, func() {
			header(c)
			a.albumGrid(c, d.page.Albums)
			if len(d.page.Albums) == 0 {
				a.empty(c, T("No albums"))
			}
		})
		return
	}
	a.trackRows(c, d.page.Tracks, true, header)
}

// scrollPage scrolls a page of cards and rows, inset as one of tracks is:
// a tab of a page that lists tracks puts its header in the same place.
func (a *app) scrollPage(c *ui.Context, content func()) {
	top, bottom := a.insets(c)
	scroll := ui.Scroll(c.Key(a.route)).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
	a.pageBars(c, scroll, -1)
	scroll.Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		content()
	})
}

// empty says a list has nothing in it.
func (a *app) empty(c *ui.Context, msg string) {
	ui.Text(c, msg).FontSize(13).TextColor(a.pal.muted).Padding(40, 0).AlignSelf(ui.Center)
}

// artistPhotoSize is the width of an artist's round photo on their page.
const artistPhotoSize = 180

// artistListsHeader heads the page of an artist without sections: their
// round photo and name over the buttons, and the switch between their songs
// and albums.
func (a *app) artistListsHeader(c *ui.Context, d *artistData) {
	p := a.pal
	songs := d.page.Tracks
	ar := &d.page.Artist
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFillWith(c, ar.ImageURL, 600, artistPhotoSize/2, iconUser)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, ar.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			var meta []string
			if len(songs) > 0 {
				meta = append(meta, songCount(len(songs)))
			}
			if n := len(d.page.Albums); n == 1 {
				meta = append(meta, T("1 album"))
			} else if n > 1 {
				meta = append(meta, Tf("%d albums", n))
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(p.muted)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(10, 0, 0, 0).Children(func() {
				if len(songs) > 0 {
					a.playButtons(c, songs)
				}
				a.favoriteButton(c.Key("favorite"), music.ItemArtist, ar.ID, ar.Favorite, 18, p.muted)
			})
		})
	})
	title := T("Songs")
	if d.tab == artistAlbums || len(songs) == 0 {
		title = T("Albums")
	}
	ui.Row(c).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
		ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3)
		ui.Spacer(c)
		if len(songs) > 0 && len(d.page.Albums) > 0 {
			segmented(c.Key("tab"), &d.tab, T("Songs"), T("Albums"))
		}
	})
}

func (a *app) artistHeaderSkeleton(c *ui.Context, s shimmer) {
	ui.Column(c).AlignItems(ui.Center).Gap(14).Margin(8, 12, 30, 12).Children(func() {
		s.bone(c, artistPhotoSize/2).Size(artistPhotoSize, artistPhotoSize)
		s.text(c, 34, 1.2).Width(220)
		s.bone(c, 22).Size(44, 44)
	})
}

// The panel of an artist's biography, at most as Music on the Mac has it.
const (
	bioWidth  = 704
	bioHeight = 802
	// bioArt is how many pixels wide their photo is fetched for it.
	bioArt = 1200
)

// artistBioDialog is an artist's biography over the window, as the button
// by their name opens it in Music on the Mac: their photo across its top,
// fading into its color, their name over its foot; when they were born or
// formed, where they're from and their genre, those Apple has; and what
// Apple wrote of them, all of it, scrolling in the panel.
func (a *app) artistBioDialog(c *ui.Context) {
	var ar *music.Artist
	if d := a.artists[a.route.id]; a.route.page == pageArtist && d != nil && d.page != nil {
		ar = &d.page.Artist
	}
	if ar == nil || strings.TrimSpace(ar.Bio) == "" {
		a.bioOpen = false
	}
	p := a.pal
	ui.DialogBase(c.Key("artist-bio"), &a.bioOpen, func(backdrop, panel ui.Element) {
		if ar == nil {
			return
		}
		w, h := c.Size()
		pw, ph := min(bioWidth, w-80), min(bioHeight, h-80)
		ground := hexColor(ar.Colors.BG, p.bg)
		ink, muted := hexColor(ar.Colors.Text1, p.text), hexColor(ar.Colors.Text2, p.muted)
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Size(pw, ph).Radius(18).Clip().Background(ground).Border(1, p.hairline).
			Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.45)).Label(T("About"))
		dialogIn(c, backdrop, panel)
		ui.Scroll(c.Key("scroll")).Grow(1).Children(func() {
			ui.Box(c).Size(pw, pw).Shrink(0).Children(func() {
				a.coverFillWith(c, ar.ImageURL, bioArt, 0, iconUser)
				ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(pw * 0.45).
					LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
						{Color: ground.Alpha(0), At: 0},
						{Color: ground, At: 1},
					}})
				ui.Text(c, ar.Name).Absolute().Left(26).Right(26).Bottom(8).FontSize(28).FontWeight(800).
					LetterSpacing(-0.5).TextColor(ink).MaxLines(2)
			})
			ui.Column(c).Padding(14, 26, 30, 26).Gap(14).Children(func() {
				born := T("Born")
				if ar.Group {
					born = T("Formed")
				}
				for i, f := range []struct{ label, value string }{
					{born, ar.Born},
					{T("From"), ar.Origin},
					{T("Genre"), ar.Genre},
				} {
					if f.value == "" {
						continue
					}
					ui.Column(c.Key(i)).Gap(3).Children(func() {
						ui.Text(c, f.label).FontSize(11).FontWeight(600).TextColor(muted)
						ui.Text(c, f.value).FontSize(13.5).FontWeight(600).TextColor(ink)
					})
				}
				ui.Text(c, T("About")).FontSize(17).FontWeight(700).TextColor(ink).Margin(12, 0, 0, 0)
				i := 0
				for para := range strings.SplitSeq(strings.TrimSpace(ar.Bio), "\n") {
					if para = strings.TrimSpace(para); para != "" {
						ui.Text(c.Key(100+i), para).FontSize(13.5).LineHeight(1.55).TextColor(ink.Alpha(0.9))
						i++
					}
				}
			})
		})
		closer := ui.ButtonBase(c.Key("close")).Absolute().Top(16).Left(16).Size(30, 30).Radius(999).Center().
			Label(T("Close")).FocusRing(false).Background(ui.RGBA(0, 0, 0, 0.3))
		if closer.Hovered() {
			closer.Background(ui.RGBA(0, 0, 0, 0.45))
		}
		closer.Children(func() { ui.Icon(c, iconX).Size(15, 15).TextColor(white) })
		if closer.Clicked() {
			a.bioOpen = false
		}
	})
}
