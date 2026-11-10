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
	// secs are the page's sections, with their biography as one where the
	// source gave it apart (Artist.Bio).
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

// artistSections are the sections of an artist's page: as the source gave
// them, and their biography after them if none of them is it.
func artistSections(page *music.ArtistPage) []music.Section {
	secs := page.Sections
	if len(secs) == 0 || strings.TrimSpace(page.Artist.Bio) == "" ||
		slices.ContainsFunc(secs, func(s music.Section) bool { return s.Style == music.ShelfText }) {
		return secs
	}
	return append(slices.Clip(secs), music.Section{Title: T("About"), Style: music.ShelfText, Subtitle: page.Artist.Bio})
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
	a.detailList(c, len(rows)+1, d.page.Artist.Colors, func(i int) {
		if i == 0 {
			a.artistHeader(c, d)
			return
		}
		r := rows[i-1]
		switch {
		case r.pair:
			a.latestAndTop(c, secs)
		case r.kind == rowText:
			a.artistBio(c, &d.page.Artist, &secs[r.section])
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
		})
	})
}

// wideHeaderHeight is the height of the picture heading an artist's page,
// at most.
const wideHeaderHeight = 400

// artistHeader heads an artist's page: their wide picture across it,
// moving where they have a video, their name over it, or their round photo
// over their name; and the buttons that
// play their top songs and make them a favorite.
func (a *app) artistHeader(c *ui.Context, d *artistData) {
	p := a.pal
	ar := &d.page.Artist
	if ar.WideURL == "" {
		ui.Column(c).AlignItems(ui.Center).Gap(14).Margin(8, 12, 30, 12).Children(func() {
			ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).
				ContextMenu(a.artistItems(*ar)).Children(func() {
				a.coverFillWith(c, ar.ImageURL, 600, artistPhotoSize/2, iconUser)
			})
			ui.Text(c, ar.Name).FontSize(34).FontWeight(800).LetterSpacing(-0.8).MaxLines(2).TextAlign(ui.Center)
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() { a.artistButtons(c, d, p.muted) })
		})
		return
	}
	// Across the page from edge to edge, from under the bar at its top.
	top, _ := a.insets(c)
	w, _ := c.Size()
	w -= sidebarWidth
	h := min(w*0.42, wideHeaderHeight)
	ink := hexColor(ar.Colors.Text1, white)
	head := ui.Box(c).Height(h).Margin(-(top + pageGap), 12-pagePad, 30, 12-pagePad).Background(hexColor(ar.Colors.BG, p.hover)).
		ContextMenu(a.artistItems(*ar))
	head.Children(func() {
		// As wide as the head is at its tallest, which Apple crops around
		// the artist; wider, the head cuts it from nearer its top, where
		// their face is.
		bmp := bitmap(a.art.get(sized(music.Wide(ar.WideURL, wideArt, wideArt*21/50), wideArt), false))
		box := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0)
		in := artIn(box, bmp)
		box.Children(func() {
			if bmp != nil && in > 0 {
				ui.Image(c, bmp).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).ObjectPosition(0.5, 0.3).Opacity(in)
			}
		})
		a.motionOver(c, head, ar.VideoURL, w, h, motionArtist, 0, 0.5, 0.3)
		ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(h * 0.5).
			LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
				{Color: ui.RGBA(0, 0, 0, 0), At: 0},
				{Color: ui.RGBA(0, 0, 0, 0.45), At: 1},
			}})
		ui.Row(c).Absolute().Left(pagePad).Right(pagePad).Bottom(26).Gap(16).AlignItems(ui.Center).Children(func() {
			a.artistButtons(c, d, ink.Alpha(0.85))
			ui.Text(c, ar.Name).FontSize(40).FontWeight(800).LetterSpacing(-1).TextColor(ink).SingleLine().Shrink(1).MinWidth(0)
		})
	})
}

// artistButtons play an artist's top songs and make them a favorite.
func (a *app) artistButtons(c *ui.Context, d *artistData, color ui.Color) {
	p := a.pal
	ar := &d.page.Artist
	if songs := d.page.Tracks; len(songs) > 0 {
		b := ui.ButtonBase(c.Key("play")).Size(44, 44).Radius(999).Center().Background(p.accent).TextColor(p.onAccent).
			Label(T("Play")).FocusRing(false).Shadow(0, 4, 14, -2, ui.RGBA(0, 0, 0, 0.3))
		b.Children(func() { ui.Icon(c, iconPlay).Size(18, 18).Margin(0, 0, 0, 2) })
		if b.Clicked() {
			if a.pl.Shuffle() {
				a.pl.ToggleShuffle()
			}
			a.pl.PlayQueue(songs, firstPlayable(songs), artistOrigin(ar.ID))
		}
	}
	a.favoriteButton(c.Key("favorite"), music.ItemArtist, ar.ID, ar.Favorite, 20, color)
}

// bioLines is how many lines of an artist's biography show, until More
// shows it all.
const bioLines = 4

// artistBio is an artist's biography, its first lines until More shows it
// all, over when they were born or formed, where they're from and their
// genre.
func (a *app) artistBio(c *ui.Context, ar *music.Artist, sec *music.Section) {
	p := a.pal
	open := a.notesOpen == a.route
	text := strings.TrimSpace(sec.Subtitle)
	width := min(760, a.shelfMetrics(c).inner)
	const size = 13.5
	_, h := c.MeasureText(width, ui.Span{Text: text, Size: size})
	_, line := c.MeasureText(0, ui.Span{Text: "M", Size: size})
	more := line > 0 && h/line > bioLines+0.5
	ui.Column(c.Key("bio")).Margin(0, 12, 0, 12).Gap(10).MaxWidth(760).Children(func() {
		if open || !more {
			i := 0
			for para := range strings.SplitSeq(text, "\n") {
				if para = strings.TrimSpace(para); para != "" {
					ui.Text(c.Key(i), para).FontSize(size).LineHeight(1.5).TextColor(p.muted)
					i++
				}
			}
		} else {
			ui.Text(c, strings.Join(strings.Fields(text), " ")).FontSize(size).LineHeight(1.5).TextColor(p.muted).MaxLines(bioLines)
		}
		if more {
			label := T("More")
			if open {
				label = T("Less")
			}
			b := ui.ButtonBase(c.Key("more")).AlignSelf(ui.Start).FocusRing(false).Label(label)
			color := p.text
			if b.Hovered() {
				color = p.accent
			}
			b.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(700).TextColor(color) })
			if b.Clicked() {
				if open {
					a.notesOpen = route{}
				} else {
					a.notesOpen = a.route
				}
			}
		}
		born := T("Born")
		if ar.Group {
			born = T("Formed")
		}
		facts := []struct{ label, value string }{
			{born, ar.Born},
			{T("From"), ar.Origin},
			{T("Genre"), ar.Genre},
		}
		ui.Row(c).Gap(40).Margin(6, 0, 0, 0).Children(func() {
			for i, f := range facts {
				if f.value == "" {
					continue
				}
				ui.Column(c.Key(i)).Gap(2).Children(func() {
					ui.Text(c, f.label).FontSize(11).FontWeight(600).TextColor(p.faint)
					ui.Text(c, f.value).FontSize(13).TextColor(p.text)
				})
			}
		})
	})
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
