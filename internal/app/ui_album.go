package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// albumData is what the app has of an album.
type albumData struct {
	album   *music.AlbumPage
	loading bool
	err     string
}

func (a *app) loadAlbum(lib *library, id string) {
	d := lib.albums[id]
	if d == nil {
		d = &albumData{}
		lib.albums[id] = d
	}
	if d.album != nil || d.loading {
		return
	}
	d.loading, d.err = true, ""
	go func() {
		al, err := answered(lib.src.Album(id))
		a.post(func() {
			d.loading = false
			if err != nil {
				d.err = a.problem(lib, err, T("Couldn't load this album"))
				return
			}
			d.album = al
		})
	}()
}

// The rows of an album's page, after its header.
const (
	albumDisc     = iota // "Disc 2", over its songs, of an album of more than one
	albumTrack           // a song
	albumFoot            // when it came out, how long it is, and its copyright
	albumComplete        // Show Complete Album, under a library album with some of its songs
	albumShelf           // a row of its sections
)

type albumRow struct {
	kind int
	i    int // the song, the disc, or the row of the sections
}

// albumRows lays an album's page out after its header: its songs, under
// their discs' numbers if there are several, its foot, Show Complete Album
// if the library has only some of it, and its sections.
func albumRows(tracks []music.Track, complete bool, shelves int) []albumRow {
	discs := false
	for i := range tracks {
		if tracks[i].Disc > 1 {
			discs = true
		}
	}
	var rows []albumRow
	disc := 0
	for i := range tracks {
		if d := max(tracks[i].Disc, 1); discs && d != disc {
			disc = d
			rows = append(rows, albumRow{albumDisc, d})
		}
		rows = append(rows, albumRow{albumTrack, i})
	}
	rows = append(rows, albumRow{albumFoot, 0})
	if complete {
		rows = append(rows, albumRow{albumComplete, 0})
	}
	for i := range shelves {
		rows = append(rows, albumRow{albumShelf, i})
	}
	return rows
}

func (a *app) albumPage(c *ui.Context) {
	id := a.route.id
	lib := a.library
	d := a.albums[id]
	if d == nil {
		a.loadAlbum(lib, id)
		d = a.albums[id]
	}
	if d.album == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadAlbum(lib, id) })
			return
		}
		a.albumSkeleton(c, a.pageShimmer(c))
		return
	}
	page := d.album
	shelves := a.shelfRows(c, page.Sections)
	rows := albumRows(page.Tracks, page.Complete != "", len(shelves))
	various := variousArtists(page)
	a.detailList(c, len(rows)+1, page.Album.Colors, func(i int) {
		if i == 0 {
			a.albumHeader(c, lib, page)
			return
		}
		r := rows[i-1]
		switch r.kind {
		case albumDisc:
			ui.Text(c, Tf("Disc %d", r.i)).FontSize(13).FontWeight(700).TextColor(a.pal.muted).Margin(18, 12, 6, 12)
		case albumTrack:
			a.albumTrackRow(c, page.Tracks, r.i, various)
		case albumFoot:
			a.albumFoot(c, page)
		case albumComplete:
			a.albumComplete(c, page.Complete)
		case albumShelf:
			a.sectionsRow(c, page.Sections, shelves, r.i)
		}
	})
}

// variousArtists reports whether an album's songs are by others than it is,
// whose rows then say by whom.
func variousArtists(page *music.AlbumPage) bool {
	by := artistNames(page.Album.Artists)
	for i := range page.Tracks {
		if names := page.Tracks[i].ArtistNames(); names != "" && names != by {
			return true
		}
	}
	return false
}

func (a *app) albumHeader(c *ui.Context, lib *library, page *music.AlbumPage) {
	p := a.pal
	al := &page.Album
	lines := func() {
		a.detailTitle(c, al.Name, al.Explicit)
		ui.RichText(c).FontSize(22).TextColor(p.accent).MaxLines(1).Children(func() {
			for i, ar := range al.Artists {
				if i > 0 {
					ui.Text(c, ", ")
				}
				if ar.ID != "" {
					a.link(c.Key(i), ar.Name, p.accent, p.accent.Mix(p.text, 0.3), route{page: pageArtist, id: ar.ID})
				} else {
					ui.Text(c, ar.Name)
				}
			}
		})
		// Short of room, the badges go under the genre and year rather than
		// be cut short.
		ui.Row(c).Wrap().Gap(8).GapY(5).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
			var meta []string
			if al.Genre != "" {
				meta = append(meta, al.Genre)
			}
			if y := year(al.Released); y != "" {
				meta = append(meta, fmt.Sprintf(TC("%s", "year"), y))
			}
			if len(meta) > 0 {
				ui.Text(c, strings.Join(meta, " · ")).FontSize(11.5).FontWeight(600).TextColor(p.muted).SingleLine().Shrink(1).MinWidth(0)
			}
			a.traitBadges(c, al.Traits, p.muted)
		})
	}
	a.detailHeader(c, al.ImageURL, al.VideoURL, lines, al.Notes, page.Tracks, func(compact bool) {
		a.libraryButton(c, lib, music.ItemAlbum, al.ID, al.Saved, al.Favorite, compact)
		a.moreButton(c, a.albumItems(lib, *al))
	})
}

// albumTrackRow is song i of an album's page: its number, which turns to a
// play button under the pointer and dancing bars as it plays, a star if it
// is among the album's most played, its title, its artists if the album is
// of various, its length and its menu. A click plays the album from it.
func (a *app) albumTrackRow(c *ui.Context, tracks []music.Track, i int, various bool) {
	p := a.pal
	tr := &tracks[i]
	playing := sameTrack(a.pl.Current(), tr)
	row := ui.ButtonBase(c.Key(tr.ID)).Height(44).Margin(0, 12).PaddingX(8).Gap(10).Radius(6).AlignItems(ui.Center).
		FocusRing(false).Label(tr.Name).ContextMenu(a.songItems(tr, ""))
	hovered := row.Hovered()
	bg := ui.Transparent
	if hovered {
		bg = p.hover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
	if tr.Unavailable {
		row.Opacity(0.42)
	}
	a.wantLike(tr)
	row.Children(func() {
		// The line between rows, as Music draws them, short of the ends.
		ui.Box(c).Absolute().Top(0).Left(8).Right(8).Height(0.5).Background(p.hairline)
		ui.Box(c).Width(12).Center().Children(func() {
			if tr.Popular {
				ui.Icon(c, iconStarFill).Size(9, 9).TextColor(p.faint).Label(T("Popular Track"))
			}
		})
		ui.Box(c).Width(24).Center().Children(func() {
			n := tr.Number
			if n == 0 {
				n = i + 1
			}
			switch {
			case playing:
				a.nowPlayingBars(c)
			case hovered && !tr.Unavailable:
				ui.Icon(c, iconPlay).Size(12, 12).TextColor(p.text)
			default:
				ui.Textf(c, "%d", n).FontSize(13).FontFeatures("tnum").TextColor(p.muted)
			}
		})
		ui.Row(c).Grow(3).Basis(0).MinWidth(0).Gap(7).AlignItems(ui.Center).Children(func() {
			if tr.Video {
				ui.Icon(c, iconVideo).Size(15, 15).TextColor(p.muted).Shrink(0).Label(T("Music Video"))
			}
			name := p.text
			if playing {
				name = p.accent
			}
			ui.Text(c, tr.Name).FontSize(13.5).TextColor(name).SingleLine().Shrink(1).MinWidth(0)
			if tr.Explicit {
				a.explicitMark(c)
			}
		})
		if various {
			a.artistLinks(c, tr, p.muted, p.text).Grow(2).Basis(0).MinWidth(0).FontSize(13).SingleLine()
		}
		ui.Box(c).Size(30, 30).Center().Children(func() {
			if a.isLiked(tr) || hovered {
				a.likeButton(c.Key("like"), tr, 14, p.muted)
			}
		})
		ui.Text(c, clock(tr.Length())).FontSize(13).FontFeatures("tnum").TextColor(p.muted).Width(44).TextAlign(ui.End)
		a.songMenu(c.Key("menu"), tr, 15, p.muted).Margin(0, -6, 0, 0)
	})
	if row.Clicked() {
		a.playRow(tracks, i)
	}
}

// albumFoot ends an album's songs: when it came out, how many there are
// and how long they play, and its copyright.
func (a *app) albumFoot(c *ui.Context, page *music.AlbumPage) {
	p := a.pal
	al := &page.Album
	ui.Column(c).Margin(0, 20, 0, 20).Gap(3).Children(func() {
		// The line under the last song, as long as those between the songs
		// and right under it: 24 lower, it read as the head of what follows
		// rather than the end of the list.
		ui.Box(c).Height(0.5).Margin(0, 0, 18, 0).Background(p.hairline)
		if al.Released != "" {
			ui.Text(c, released(al.Released)).FontSize(12.5).TextColor(p.muted)
		}
		if len(page.Tracks) > 0 {
			ui.Text(c, listLength(page.Tracks)).FontSize(12.5).TextColor(p.muted)
		}
		if al.Copyright != "" {
			ui.Text(c, al.Copyright).FontSize(12.5).TextColor(p.muted)
		}
		if al.Label != "" {
			a.albumLabel(c, al)
		}
	})
}

// albumLabel is who released an album, under its copyright as in Music:
// a heading, and the label's name, which opens its page if it has one.
func (a *app) albumLabel(c *ui.Context, al *music.Album) {
	p := a.pal
	ui.Text(c, T("Record Label")).FontSize(11.5).FontWeight(600).TextColor(p.muted).Margin(14, 0, 0, 0)
	if al.LabelPage == "" {
		ui.Text(c, al.Label).FontSize(12.5).TextColor(p.text)
		return
	}
	b := ui.ButtonBase(c.Key("label")).AlignSelf(ui.Start).Gap(2).AlignItems(ui.Center).FocusRing(false).Label(al.Label)
	color := p.accent
	if b.Hovered() {
		color = p.accent.Mix(p.text, 0.3)
	}
	b.Children(func() {
		ui.Text(c, al.Label).FontSize(12.5).TextColor(color)
		ui.Icon(c, iconForward).Size(11, 11).TextColor(color)
	})
	if b.Clicked() {
		a.open(route{page: pageBrowse, id: al.LabelPage})
	}
}

// albumComplete is the bar under an album of the listener's library that
// has only some of its songs, which opens the whole album, as Music's does.
func (a *app) albumComplete(c *ui.Context, id string) {
	p := a.pal
	label := T("Show Complete Album")
	bar := ui.ButtonBase(c.Key("complete")).Height(44).Margin(22, 12, 0, 12).PaddingX(16).Radius(10).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(label)
	bg := p.hover
	if bar.Pressed() {
		bg = p.press
	} else if bar.Hovered() {
		bg = p.hover.Mix(p.press, 0.5)
	}
	bar.Background(bar.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond})).Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(p.accent)
	})
	if bar.Clicked() {
		a.open(route{page: pageAlbum, id: id})
	}
}

// albumGrid shows albums as cards, each opening its page.
func (a *app) albumGrid(c *ui.Context, albums []music.Album) {
	ui.Grid(c).Columns(a.columns(c, 156)).GapX(20).GapY(28).Children(func() {
		for i := range albums {
			a.albumCard(c.Key(albums[i].ID), &albums[i])
		}
	})
}

// artistNames joins the names of an album's artists.
func artistNames(artists []music.Artist) string {
	names := make([]string, len(artists))
	for i, ar := range artists {
		names[i] = ar.Name
	}
	return strings.Join(names, ", ")
}

// albumCard is an album's cover, lifting under the pointer, over its name
// and the year it came out.
func (a *app) albumCard(c *ui.Context, al *music.Album) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(al.Name).
		ContextMenu(a.albumItems(a.library, *al))
	hovered := card.Hovered()
	card.Children(func() {
		art := ui.Box(c).AspectRatio(1).Radius(10)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.AnimatePose(lift, ui.Snappy)
		art.Children(func() { a.coverFill(c, al.ImageURL, 400, 10) })
		ui.Text(c, al.Name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		var meta []string
		if a.route.page != pageArtist && len(al.Artists) > 0 {
			meta = append(meta, artistNames(al.Artists))
		}
		if len(al.Released) >= 4 {
			meta = append(meta, al.Released[:4])
		}
		ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
	})
	if card.Clicked() {
		a.open(route{page: pageAlbum, id: al.ID})
	}
}
