package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/i18n"
	"ringo/internal/music"
	"ringo/internal/player"
)

const pagePad = 40

// columns returns how many cards of at least min DIPs fit across the page.
func (a *app) columns(c *ui.Context, min float32) int {
	w, _ := c.Size()
	inner := w - sidebarWidth - 2*pagePad
	return max(2, int((inner+20)/(min+20)))
}

func (a *app) heading(c *ui.Context, title, sub string) {
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		if sub != "" {
			ui.Text(c, sub).FontSize(12).FontWeight(600).TextColor(a.pal.muted).LetterSpacing(0.4)
		}
		ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
	})
}

func (a *app) section(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(34, 0, 14, 0)
}

// homePage is what the source makes up itself, if it does; else what it played
// lately and its playlists; else, with nothing yet, a way to get started.
func (a *app) homePage(c *ui.Context) {
	lib := a.library
	if lib.home != nil && len(lib.home.Sections) > 0 {
		a.browseRows(c, lib.home, "", func(c *ui.Context) { a.heading(c, T("Home"), "") }, nil)
		return
	}
	loading := lib.homeLoading || !lib.started || lib.accountUnknown()
	// None of this shows until Home is known to have nothing: the playlists,
	// loaded first, would show here, then make way.
	nothing := len(lib.recent) == 0 && len(lib.mine) == 0 && len(lib.saved) == 0 || loading
	if nothing && !loading {
		if lib.homeErr != "" {
			a.centered(c, lib.homeErr, func() { a.loadHome() })
			return
		}
		a.emptyHome(c)
		return
	}
	top, bottom := a.insets(c)
	scroll := ui.Scroll(c.Key(a.route)).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad-12, bottom+48, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
	a.pageBars(c, scroll, -1)
	scroll.Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		ui.Box(c).PaddingX(12).Children(func() { a.heading(c, T("Home"), "") })
		if nothing {
			a.homeSkeleton(c, a.pageShimmer(c))
			return
		}
		if len(lib.recent) > 0 {
			ui.Box(c).PaddingX(12).Children(func() { a.section(c, T("Recently Played")).Margin(0, 0, 8, 0) })
			recent := lib.recent[:min(len(lib.recent), 8)]
			for i := range recent {
				a.trackRow(c, recent, i, true, "")
			}
		}
		lists := append(append([]music.Playlist(nil), lib.mine...), lib.saved...)
		if len(lists) > 0 {
			ui.Column(c).PaddingX(12).Children(func() {
				a.section(c, T("Playlists"))
				ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
					for i := range lists {
						a.playlistCard(c.Key(lists[i].ID), &lists[i])
					}
				})
			})
		}
	})
}

// emptyHome invites the listener to do what gets the source going: under
// the app's mark, to sign in, which is all Home is without them.
func (a *app) emptyHome(c *ui.Context) {
	p := a.pal
	lib := a.library
	top, bottom := a.insets(c)
	ui.Column(c.Key("empty-home")).Grow(1).Padding(top, 40, bottom, 40).Center().Children(func() {
		if !lib.signedOut() {
			a.vacant(c, glyphNote, T("Nothing here yet"), T("Search for something to play."), nil)
			return
		}
		a.vacant(c, markRingo, Tf("Sign in to %s", lib.name()),
			T("Your library and playlists show here once you sign in."), func() {
				if pillButton(c.Key("sign-in"), iconUser, T("Sign In"), p.accent, p.onAccent).Clicked() {
					a.promptSignIn()
				}
			})
	})
}

// playlistCard is a square cover with the playlist's name below: it lifts
// under the pointer, where a play button shows on it.
func (a *app) playlistCard(c *ui.Context, pl *music.Playlist) {
	id := pl.ID
	if a.coverCard(c, pl.Name, pl.ImageURL, playlistMoving(pl), playlistLine(pl), a.playlistPlay(id)) {
		a.open(route{page: pagePlaylist, id: id})
	}
}

// playlistLine is what a playlist's card says under its name: how many songs
// it has and who made it, as strangers' playlists are told apart by their
// makers.
func playlistLine(pl *music.Playlist) string {
	var meta []string
	if pl.TrackCount > 0 {
		meta = append(meta, i18n.Songs(pl.TrackCount))
	}
	if pl.Owner != "" {
		meta = append(meta, pl.Owner)
	}
	return strings.Join(meta, " · ")
}

// coverCard is a playlist's card, or a show's or a station's: its cover over
// its name and a line under that. The button on the cover, if there's something
// for it to do, does play; it reports whether the rest was clicked.
func (a *app) coverCard(c *ui.Context, name, cover string, mv moving, sub string, play *coverPlay) bool {
	return a.coverCardWith(c, name, cover, mv, iconNote, sub, play)
}

// coverCardWith is a card that shows icon if there's no cover to show, as a
// source needn't have one.
func (a *app) coverCardWith(c *ui.Context, name, cover string, mv moving, icon *ui.SVG, sub string, play *coverPlay) bool {
	p := a.pal
	// From the top: a grid stretches the cards of a row as tall as the
	// tallest, whose name or line under it takes two lines.
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).Justify(ui.Start).FocusRing(false).Label(name)
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
		art.Children(func() {
			a.coverFillWith(c, cover, mv, 400, 10, icon)
			if play != nil {
				a.coverPlayButton(c, hovered, play)
			}
		})
		ui.Text(c, name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		if sub != "" {
			ui.Text(c, sub).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
		}
	})
	return card.Clicked()
}

// coverFill fills its parent with a cover, its corners rounded by radius.
// The cover rounds itself rather than its parent clipping it: a clip moved
// by a transform, as a card lifting under the pointer, is drawn on the CPU.
func (a *app) coverFill(c *ui.Context, url string, mv moving, px int, radius float32) {
	a.coverFillWith(c, url, mv, px, radius, iconNote)
}

// coverFillWith is coverFill showing icon in the cover's place until it
// comes, or for good where there's none. Over it, the cover of mv plays
// moving where it moves: as a page's head, a cover asked for as large as
// one is (px over 400).
func (a *app) coverFillWith(c *ui.Context, url string, mv moving, px int, radius float32, icon *ui.SVG) {
	bmp := bitmap(a.art.get(sized(url, px), false))
	box := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius)
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(a.pal.blank)
	}
	box.Children(func() {
		if in < 1 {
			a.blankIcon(c, icon, 1-in)
		}
		artImage(c, bmp, radius, in)
		size := motionThumb
		if px > 400 {
			size = motionHead
		}
		a.motionOver(c, box, mv, float32(px)/2, float32(px)/2, size, radius)
	})
}

// blankIcon is icon in the middle of a picture's place, a third as wide, as
// far shown as shown.
func (a *app) blankIcon(c *ui.Context, icon *ui.SVG, shown float32) {
	ui.Row(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Justify(ui.Center).AlignItems(ui.Center).IgnoreInput(true).Children(func() {
		ui.Icon(c, icon).WidthPercent(30).AspectRatio(1).TextColor(a.pal.faint).Opacity(shown)
	})
}

// coverPlayButton plays a playlist or a show from its card, without
// opening it.
func (a *app) coverPlayButton(c *ui.Context, shown bool, cp *coverPlay) {
	white := ui.RGB(255, 255, 255)
	b := ui.ButtonBase(c.Key("play")).Absolute().Bottom(10).Right(10).Size(38, 38).Radius(999).Center().
		Background(a.pal.accent).TextColor(white).Label(T("Play")).FocusRing(false).
		Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.3))
	// Played, it stays and spins until the song starts, the pointer gone or not.
	waits := cp.waits != nil && cp.waits()
	pose := ui.Pose{Fade: 1, Scale: 0.8, Y: 6}
	if shown || b.Hovered() || waits {
		pose = ui.Pose{Scale: 1}
		if b.Hovered() {
			pose.Scale = 1.05
		}
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() {
		if waits {
			ui.Spinner(c).FontSize(16).TextColor(white)
		} else {
			ui.Icon(c, iconPlay).Size(15, 15).Margin(0, 0, 0, 2)
		}
	})
	if cp.ahead != nil {
		aheadOnHover(c, b, shown || b.Hovered(), cp.ahead)
	}
	if b.Clicked() {
		cp.play()
	}
}

// What a cover's play button plays is loaded as the pointer rests on the
// cover for aheadAfter, and with it the source gets its first songs ready
// (applemusic's prefetch.go): the button is then a click away, and the song
// a moment, where a song nobody played yet would wait seconds for its keys.
// Passing over covers on the way elsewhere loads nothing.
const aheadAfter = 200 * time.Millisecond

type hoverAhead struct {
	since time.Time
	done  bool
}

// aheadOnHover calls ahead once on has been true of b for aheadAfter, and
// again after it has been false.
func aheadOnHover(c *ui.Context, b ui.Element, on bool, ahead func()) {
	h := ui.Local(b, "ahead", func() hoverAhead { return hoverAhead{} })
	if h == nil {
		return
	}
	if !on {
		*h = hoverAhead{}
		return
	}
	if h.done {
		return
	}
	now := c.Now()
	if h.since.IsZero() {
		h.since = now
	}
	if wait := aheadAfter - now.Sub(h.since); wait > 0 {
		c.After(wait)
		return
	}
	h.done = true
	ahead()
}

// playlistPage shows a playlist as Apple Music does: its header, its songs
// in a table of their titles, artists, albums and lengths, and its sections
// after them.
func (a *app) playlistPage(c *ui.Context) {
	id := a.route.id
	pl := a.playlists[id]
	if pl == nil {
		if !a.loadingPl[id] {
			a.centered(c, a.plErr[id], func() { a.loadPlaylist(id, false) })
			return
		}
		a.playlistSkeleton(c, a.pageShimmer(c))
		return
	}
	shelves := a.shelfRows(c, pl.Sections)
	// The header, the table's head, the songs or what says there are none,
	// and the sections.
	songs := max(len(pl.Tracks), 1)
	a.detailList(c, 2+songs+len(shelves), pl.Colors, func(i int) {
		switch {
		case i == 0:
			a.playlistHeader(c, pl)
		case i == 1:
			a.playlistColumns(c)
		case i < 2+songs:
			if len(pl.Tracks) == 0 {
				a.empty(c, glyphNote, T("Empty Playlist"), detailCover+90)
				return
			}
			a.playlistRow(c, pl.Tracks, i-2, pl.ID)
		default:
			a.sectionsRow(c, pl.Sections, shelves, i-2-songs)
		}
	})
}

func (a *app) playlistHeader(c *ui.Context, pl *music.Playlist) {
	p := a.pal
	lines := func() {
		a.detailTitle(c, pl.Name, false)
		if pl.Owner != "" {
			ui.RichText(c).FontSize(22).TextColor(p.accent).MaxLines(1).Children(func() {
				if pl.OwnerPage != "" {
					a.link(c.Key("owner"), pl.Owner, p.accent, p.accent.Mix(p.text, 0.3), route{page: pageBrowse, id: pl.OwnerPage})
				} else {
					ui.Text(c, pl.Owner)
				}
			})
		}
		ui.Row(c).Wrap().Gap(8).GapY(5).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
			if u := updated(pl.Modified, time.Now()); u != "" {
				ui.Text(c, u).FontSize(11.5).FontWeight(600).TextColor(p.muted).SingleLine()
			}
			a.traitBadges(c, pl.Traits, p.muted)
		})
	}
	a.detailHeader(c, pl.ImageURL, playlistMoving(pl), lines, pl.Description, pl.Tracks, func(compact bool) {
		if !pl.Mine && !pl.Fixed && !a.ownList(pl.ID) {
			a.libraryButton(c, music.ItemPlaylist, pl.ID, pl.Saved || a.isSaved(pl), pl.Favorite, compact)
		}
		a.moreButton(c, a.playlistItemsOf(*pl))
	})
}

// playlistWide reports whether a playlist's table has room for its albums
// and artists in columns of their own.
func (a *app) playlistWide(c *ui.Context) bool {
	return a.shelfMetrics(c).inner > 760
}

// playlistColumns heads a playlist's table: Song, Artist, Album and Time.
func (a *app) playlistColumns(c *ui.Context) {
	p := a.pal
	wide := a.playlistWide(c)
	ui.Row(c).Height(30).Margin(0, 12).PaddingX(8).Gap(12).AlignItems(ui.Center).
		BorderWidth(0, 0, 0.5, 0).BorderColor(p.hairline).Children(func() {
		head := func(label string) ui.Element {
			return ui.Text(c, label).FontSize(11.5).FontWeight(600).TextColor(p.muted).SingleLine()
		}
		head(T("Song")).Grow(3).Basis(0).MinWidth(0)
		if wide {
			head(T("Artist")).Grow(2).Basis(0).MinWidth(0)
			head(T("Album")).Grow(2).Basis(0).MinWidth(0)
		}
		ui.Box(c).Width(30)
		head(T("Time")).Width(44).TextAlign(ui.End)
		ui.Box(c).Width(30)
	})
}

// playlistRow is song i of a playlist's table, pid: its cover, title,
// artists, album, length and menu. A click plays the playlist from it.
func (a *app) playlistRow(c *ui.Context, tracks []music.Track, i int, pid string) {
	p := a.pal
	tr := &tracks[i]
	wide := a.playlistWide(c)
	playing := sameTrack(a.pl.Current(), tr)
	row := ui.ButtonBase(c.Key(tr.ID)).Height(52).Margin(0, 12).PaddingX(8).Gap(12).Radius(6).AlignItems(ui.Center).
		FocusRing(false).Label(tr.Name).ContextMenu(a.songItems(tr, pid))
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
		if i > 0 {
			ui.Box(c).Absolute().Top(0).Left(8).Right(8).Height(0.5).Background(p.hairline)
		}
		ui.Row(c).Grow(3).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(40, 40).Shrink(0).Children(func() {
				a.cover(c, tr.Album.ImageURL, songMoving(tr), 96, 40, 4)
				if playing || hovered && !tr.Unavailable {
					ui.Box(c.Key("cover-shade")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(4).Center().Background(ui.RGBA(0, 0, 0, 0.4)).
						Transition(rowHoverIn).Children(func() {
						if playing {
							a.nowPlayingBarsIn(c, white)
						} else {
							ui.Icon(c, iconPlay).Size(13, 13).TextColor(white)
						}
					})
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					name := p.text
					if playing {
						name = p.accent
					}
					ui.Text(c, tr.Name).FontSize(13.5).TextColor(name).SingleLine().Shrink(1).MinWidth(0)
					if tr.Explicit {
						a.explicitMark(c)
					}
				})
				if !wide {
					a.artistLinks(c, tr, p.muted, p.text).FontSize(12.5).SingleLine()
				}
			})
		})
		if wide {
			a.artistLinks(c, tr, p.muted, p.text).Grow(2).Basis(0).MinWidth(0).FontSize(13).SingleLine()
			album := ui.RichText(c).Grow(2).Basis(0).MinWidth(0).FontSize(13).TextColor(p.muted).SingleLine()
			album.Children(func() {
				if tr.Album.ID == "" {
					ui.Text(c, tr.Album.Name)
				} else {
					a.link(c.Key("album"), tr.Album.Name, p.muted, p.text, route{page: pageAlbum, id: tr.Album.ID})
				}
			})
		}
		ui.Box(c).Size(30, 30).Center().Children(func() {
			if a.isLiked(tr) || hovered {
				ui.Box(c.Key("like-in")).Transition(rowHoverIn).Children(func() {
					a.likeButton(c.Key("like"), tr, 14, p.muted)
				})
			}
		})
		ui.Text(c, clock(tr.Length())).FontSize(13).FontFeatures("tnum").TextColor(p.muted).Width(44).TextAlign(ui.End)
		a.songMenuIn(c.Key("menu"), tr, pid, 15, p.muted).Margin(0, -6, 0, 0)
	})
	if row.Clicked() {
		a.playRow(tracks, i)
	}
}

// rowHoverIn fades in what a song's row shows under the pointer, its cover's
// play button and its star, and fades it out, as its background does.
var rowHoverIn = ui.ElementTransition{Duration: 100 * time.Millisecond, Enter: &ui.Motion{Opacity: 0}, Exit: &ui.Motion{Opacity: 0}}

// playButtons are the buttons of a page that play what it lists, in order or
// shuffled. tight, Shuffle is its icon alone, where a header hasn't the room
// for its word.
func (a *app) playButtons(c *ui.Context, tracks []music.Track, tight bool) {
	p := a.pal
	if pillButton(c.Key("play"), iconPlay, T("Play"), p.accent, p.onAccent).Clicked() {
		if a.pl.Shuffle() {
			a.pl.ToggleShuffle()
		}
		a.pl.PlayQueue(tracks, firstPlayable(tracks), a.pageOrigin())
	}
	var shuffle ui.Element
	if tight {
		shuffle = a.roundHeaderButton(c.Key("shuffle"), T("Shuffle"))
		shuffle.Tooltip(T("Shuffle")).Children(func() { ui.Icon(c, iconShuffle).Size(14, 14) })
	} else {
		shuffle = pillButton(c.Key("shuffle"), iconShuffle, T("Shuffle"), p.hover.Over(p.bg), p.text)
	}
	if shuffle.Clicked() {
		a.pl.PlayShuffled(tracks, a.pageOrigin())
	}
}

// likedPage lists the songs the listener likes, loaded a page at a time.
func (a *app) likedPage(c *ui.Context) {
	lib := a.library
	header := func(c *ui.Context) {
		p := a.pal
		// In from the page's edge as a playlist's header is.
		ui.Row(c).Gap(28).AlignItems(ui.End).Margin(0, 12, 26, 12).Children(func() {
			ui.Box(c).Size(212, 212).Radius(14).Center().Shadow(0, 18, 40, -10, p.cardShade).TextColor(white).
				LinearGradient(ui.LinearGradient{Angle: 135, Oklab: true, From: ui.Oklch(0.72, 0.17, 10), To: ui.Oklch(0.58, 0.22, 350)}).
				Children(func() { ui.Icon(c, iconStarFill).Size(84, 84) })
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
				ui.Text(c, T("Playlist")).FontSize(12).FontWeight(600).TextColor(p.accent)
				ui.Text(c, T("Favorite Songs")).FontSize(30).FontWeight(800).LetterSpacing(-0.6)
				if lib.likedPaged.loaded {
					ui.Text(c, i18n.Songs(len(lib.likedList))).FontSize(13).TextColor(p.muted)
				}
				if len(lib.likedList) > 0 {
					ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() { a.playButtons(c, lib.likedList, false) })
				}
			})
		})
	}
	if !lib.likedPaged.loaded {
		if lib.likedPaged.err != "" {
			a.centered(c, lib.likedPaged.err, func() { a.loadLikedPage() })
			return
		}
		a.trackSkeleton(c, a.pageShimmer(c), true, header)
		return
	}
	// Where the source says how many songs there are, blocks stand in for
	// those to come, as high as their rows, and the foot says nothing of
	// loading; else the foot does (as browseRows).
	var after []func(c *ui.Context)
	if pending := a.likedPending(); pending > 0 {
		s := a.rowShimmer(c)
		for i := range pending {
			after = append(after, func(c *ui.Context) { a.trackRowSkeleton(c, s, len(lib.likedList)+i, true) })
		}
	} else {
		after = append(after, func(c *ui.Context) {
			a.pageFoot(c, &lib.likedPaged, func() { a.loadLikedPage() })
			if len(lib.likedList) == 0 {
				a.empty(c, glyphStar, T("Songs you favorite will show here"), detailCover+60)
			}
		})
	}
	a.trackRows(c, lib.likedList, true, header, after...)
	if p := &lib.likedPaged; p.more && !p.loading && p.err == "" && a.wantsMoreOf(lib.likedTotal, len(lib.likedList), len(lib.likedList)+1) {
		a.loadLikedPage()
	}
}

// likedPending is how many rows stand in for the liked songs yet to come: as
// many as the source says there are left, but pendingItems at most, and none
// where it didn't say, or the next part failed.
func (a *app) likedPending() int {
	lib := a.library
	if p := &lib.likedPaged; !p.more || p.err != "" {
		return 0
	}
	return max(min(lib.likedTotal-len(lib.likedList), pendingItems), 0)
}

// loadLikedPage loads the next page of the songs the listener likes.
func (a *app) loadLikedPage() {
	lib := a.library
	if lib.likedPaged.loading {
		return
	}
	lib.likedPaged.loading, lib.likedPaged.err = true, ""
	offset := len(lib.likedList)
	if offset == 0 {
		lib.likedAt = time.Now()
	}
	go func() {
		page, err := answered(lib.src.Liked(offset))
		a.post(func() {
			lib.likedPaged.loading = false
			if err != nil {
				lib.likedPaged.err = a.problem(err, T("Couldn't load your favorite songs"))
				return
			}
			lib.likedList = append(lib.likedList, page.Tracks...)
			lib.likedPaged.loaded, lib.likedPaged.more = true, page.More && len(page.Tracks) > 0
			lib.likedTotal = page.Total
			if lib.liked == nil {
				lib.liked = map[string]bool{}
			}
			for i := range page.Tracks {
				lib.liked[page.Tracks[i].ID] = true
			}
			a.keepPages()
		})
	}()
}

// freshenLiked asks for the first page of the liked songs anew, a while after
// it was: it takes the place of what's listed, which shows until it comes.
// Songs listed before and not now are asked about again as their hearts show.
func (a *app) freshenLiked() {
	lib := a.library
	if lib.likedFresh || lib.likedPaged.loading || time.Since(lib.likedAt) <= listsFresh {
		return
	}
	a.refreshLiked(false, nil)
}

// setLiked takes page as the first part of the songs the listener likes,
// before any was loaded, as one asked for ahead (prefetch.go) is.
func (a *app) setLiked(page *music.Page, at time.Time) {
	lib := a.library
	lib.likedList = append([]music.Track(nil), page.Tracks...)
	lib.likedPaged.loaded, lib.likedPaged.more = true, page.More && len(page.Tracks) > 0
	lib.likedTotal = page.Total
	lib.likedAt = at
	if lib.liked == nil {
		lib.liked = map[string]bool{}
	}
	for i := range page.Tracks {
		lib.liked[page.Tracks[i].ID] = true
	}
}

// refreshLiked is freshenLiked, whatever its age. quiet asks as for the
// listener's sidebar before they open it (preloadLiked), and then is called
// when it's over.
func (a *app) refreshLiked(quiet bool, then func()) {
	lib := a.library
	lib.likedFresh, lib.likedAt = true, time.Now()
	was := lib.likedList
	who := lib.listener()
	go func() {
		var page *music.Page
		var err error
		if quiet {
			page, err = preloadLiked(lib.src)
		} else {
			page, err = answered(lib.src.Liked(0))
		}
		a.post(func() {
			if then != nil {
				defer then()
			}
			lib.likedFresh = false
			// Liked or not in the app meanwhile, or more of it loaded, or
			// forgotten: what's there is newer than what was asked for.
			if err != nil || !sameList(lib.likedList, was) || lib.likedPaged.loading || lib.listener() != who {
				return
			}
			lib.likedList = page.Tracks
			lib.likedPaged.more = page.More && len(page.Tracks) > 0
			lib.likedTotal = page.Total
			if lib.liked == nil {
				lib.liked = map[string]bool{}
			}
			now := make(map[string]bool, len(page.Tracks))
			for i := range page.Tracks {
				now[page.Tracks[i].ID] = true
				lib.liked[page.Tracks[i].ID] = true
			}
			for i := range was {
				if !now[was[i].ID] {
					delete(lib.liked, was[i].ID)
				}
			}
			a.keepPages()
		})
	}()
}

// trackRows lists tracks under a header, which scrolls with them; a click
// plays the list from the track clicked.
func (a *app) trackRows(c *ui.Context, tracks []music.Track, covers bool, header func(c *ui.Context), footer ...func(c *ui.Context)) {
	// A song's menu can take it out of the playlist it is listed in.
	var pid string
	if a.route.page == pagePlaylist {
		pid = a.route.id
	}
	top, bottom := a.insets(c)
	// Rows out of view are not built, their marks with them: a list
	// scrolled away from its start or end has content under the bars.
	a.underTop, a.underBottom = true, true
	rows := len(tracks) + 1 + len(footer)
	list := ui.List(c.Key(a.route), &a.trackList, rows, func(i int) {
		if i == 0 {
			ui.Column(c).Children(func() {
				a.startMark(c)
				header(c)
			})
			return
		}
		if i == rows-1 {
			defer a.endMark(c)
		}
		if i > len(tracks) {
			footer[i-len(tracks)-1](c)
			return
		}
		a.trackRow(c, tracks, i-1, covers, pid)
	}).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
	a.pageBars(c, list, rows)
}

// sameTrack reports whether two tracks are one.
func sameTrack(x, y *music.Track) bool {
	return x != nil && y != nil && x.ID == y.ID
}

// trackRow is a row of the list tracks, its track i, which plays the list from
// there as it's clicked.
func (a *app) trackRow(c *ui.Context, tracks []music.Track, i int, covers bool, pid string) {
	p := a.pal
	tr := &tracks[i]
	w, _ := c.Size()
	// An album's page has no column of the album its songs are on.
	wide := w-sidebarWidth > 820 && a.route.page != pageAlbum
	playing := sameTrack(a.pl.Current(), tr)
	row := ui.ButtonBase(c.Key(tr.ID)).Height(54).PaddingX(12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(tr.Name).
		ContextMenu(a.songItems(tr, pid))
	hovered := row.Hovered()
	bg := ui.Transparent
	switch {
	case playing:
		bg = p.accent.Alpha(0.09)
	case hovered:
		bg = p.hover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
	if tr.Unavailable {
		row.Opacity(0.42)
	}
	a.wantLike(tr)
	row.Children(func() {
		ui.Box(c).Width(26).Center().Children(func() {
			switch {
			case playing:
				a.nowPlayingBars(c)
			case hovered && !tr.Unavailable:
				ui.Icon(c, iconPlay).Size(13, 13).TextColor(p.text)
			default:
				ui.Textf(c, "%d", i+1).FontSize(13).FontFeatures("tnum").TextColor(p.faint)
			}
		})
		if covers {
			a.cover(c, tr.Album.ImageURL, songMoving(tr), 96, 38, 6)
		}
		ui.Column(c).Grow(3).Basis(0).MinWidth(0).Gap(3).Children(func() {
			name := p.text
			if playing {
				name = p.accent
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tr.Name).FontSize(14).FontWeight(500).TextColor(name).SingleLine().Shrink(1).MinWidth(0)
				if tr.Explicit {
					a.explicitMark(c)
				}
			})
			a.artistLinks(c, tr, p.muted, p.text).FontSize(12.5).SingleLine().Shrink(1).MinWidth(0)
		})
		if wide {
			album := ui.RichText(c).Grow(2).Basis(0).MinWidth(0).FontSize(12.5).TextColor(p.muted).SingleLine()
			album.Children(func() {
				if tr.Album.ID == "" {
					ui.Text(c, tr.Album.Name)
				} else {
					a.link(c.Key("album"), tr.Album.Name, p.muted, p.text, route{page: pageAlbum, id: tr.Album.ID})
				}
			})
		}
		// A liked song's heart shows; the others' as the pointer is on them.
		ui.Box(c).Size(32, 32).Center().Children(func() {
			if a.isLiked(tr) || hovered {
				a.likeButton(c.Key("like"), tr, 15, p.muted)
			}
		})
		ui.Text(c, clock(tr.Length())).FontSize(12.5).FontFeatures("tnum").TextColor(p.faint).Width(44).TextAlign(ui.End)
	})
	if row.Clicked() {
		a.playRow(tracks, i)
	}
}

// nowPlayingBars dance in the row of the track playing.
func (a *app) nowPlayingBars(c *ui.Context) { a.nowPlayingBarsIn(c, a.pal.accent) }

// nowPlayingBarsIn are nowPlayingBars in color.
func (a *app) nowPlayingBarsIn(c *ui.Context, color ui.Color) {
	ui.Box(c).Size(16, 14).Draw(func(p *ui.Painter, r ui.Rect) {
		lv, live := a.pl.Levels()
		a.rowBars.step(p.Now(), lv, live)
		drawBars(p, r, &a.rowBars, 4, color, 2.5)
		if live || a.rowBars.group(0, player.Bands) > 0.01 {
			p.AnimationFrame()
		}
	})
}

// sized names a cover by the size it shows at: the cache keeps it no bigger.
func sized(url string, px int) string {
	if url == "" {
		return ""
	}
	return fmt.Sprintf("%s#%d", url, px)
}
