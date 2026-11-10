package app

import (
	"log"
	"runtime"
	"slices"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

const sidebarWidth = 232

func (a *app) view(c *ui.Context) {
	t := *c.Theme()
	a.pal = paletteFor(&t)
	p := a.pal
	useAccent(&t, p)
	t.Selection = p.accent.Alpha(0.3)
	a.bars.frame(&t)
	c.SetTheme(&t)

	a.shortcuts(c)

	// Back to the front, after the listener may have changed their
	// playlists elsewhere.
	active := c.WindowActive()
	if active && a.wasBehind {
		a.freshen()
	}
	a.wasBehind = !active

	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	root := ui.Box(c).Fill()
	if !c.Focused(a.searchBox) {
		// Space plays and pauses wherever the focus is, as in Music. On
		// the container around everything, it comes before a button the
		// pointer focused, which would take Space as a click; on the
		// window, it would come after.
		root.OnShortcut(0, ui.KeySpace, a.pl.TogglePlay)
		// And where nothing has the focus, as once Now Playing covers the
		// button that had it.
		c.OnShortcut(0, ui.KeySpace, a.pl.TogglePlay)
	}
	npOpen, npSettled := a.npState(c)
	root.Children(func() {
		page := ui.Row(c).Fill().AlignItems(ui.Stretch)
		page.Children(func() {
			if npOpen && npSettled {
				// Covered by Now Playing: not built, which would cost each
				// of its frames as much again as the stage itself. Where
				// it is scrolled is the app's (pageScroll, navScroll,
				// trackList), so it shows again as it was.
				return
			}
			side := ui.Column(c).Width(sidebarWidth)
			if !c.Vibrancy() {
				side.Background(p.sidebar)
			}
			side.Children(func() { a.sidebar(c) })
			// Keyed, so that a popover of the sidebar opening or closing
			// keeps what the page and its bars hold, their edges' fades.
			ui.Box(c.Key("main")).Grow(1).Background(p.bg).HandleInput(a.scrollInput).Children(func() {
				a.content(c)
				// The bars float over the page, which scrolls under them:
				// positioned, they paint over it.
				a.pageTop(c)
				a.playerBar(c)
			})
		})
		a.nowPlayingLayer(c, npOpen, npSettled)
	})
	a.signInDialog(c, a.pal, false)
	a.playlistNameDialog(c)
	a.deletePlaylistDialog(c)
	a.artistBioDialog(c)
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
	a.toasts(c, !npOpen)
	a.askLikes()
	a.pumpWarm()
	a.syncWindows()
}

// toastWidth is how wide a toast goes at most, its words taking two lines
// beyond.
const toastWidth = 480

// toastFade is how long a toast takes to fade in and out, as MyGo's own.
const toastFade = 200 * time.Millisecond

// toasts shows the window's toasts as MyGo would, but above the player,
// which they'd cover, and over the pages if the sidebar shows, not across
// it.
func (a *app) toasts(c *ui.Context, sidebar bool) {
	t := c.Theme()
	fill, text := t.Inverse, t.InverseText
	if fill == ui.Transparent {
		fill = t.Text
	}
	if text == ui.Transparent {
		text = t.Background
	}
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		left := float32(0)
		if sidebar {
			left = sidebarWidth
		}
		viewport.Padding(0, 16, playerBarHeight+12, left+16).AlignItems(ui.Center).Gap(t.Space(2))
		held := viewport.Hovered()
		for _, ts := range toasts {
			p := ui.ToastBase(c, ts)
			box := p.Root.Row().AlignItems(ui.Center).Gap(t.Space(4)).Padding(t.Space(2.5), t.Space(4)).Radius(t.Space(2)).
				MaxWidth(toastWidth).Background(fill).TextColor(text).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.25))
			// It fades in, and out at the end, as MyGo's does; its age is
			// what its time leaves of the four seconds of a plain one.
			life := ts.Timeout
			if life == 0 {
				life = 4 * time.Second
			}
			left, age := p.Left(), time.Duration(0)
			if left > 0 {
				age = life - left
			} else {
				age = toastFade
			}
			opacity := float32(1)
			switch {
			case held:
			case age < toastFade:
				opacity = float32(age) / float32(toastFade)
				c.AnimationFrame()
			case left > 0 && left < toastFade:
				opacity = float32(left) / float32(toastFade)
				c.AnimationFrame()
			case left > 0:
				c.After(left - toastFade)
			}
			box.Opacity(opacity)
			box.Children(func() {
				ui.Text(c, ts.Title).MaxLines(2).Shrink(1).MinWidth(0)
			})
		}
	})
}

func (a *app) shortcuts(c *ui.Context) {
	// The side buttons of a mouse, the keys of keyboards that have them
	// and swipes between pages, and the keys of Music.
	c.OnShortcut(0, ui.KeyBack, a.back)
	c.OnShortcut(0, ui.KeyForward, a.goForward)
	c.OnShortcut(ui.Cmd, ui.KeyBracketLeft, a.back)
	c.OnShortcut(ui.Cmd, ui.KeyBracketRight, a.goForward)
	c.OnShortcut(ui.Cmd, ui.KeyF, func() {
		a.nowPlaying = false
		a.searchBox.Focus(c)
	})
	if a.nowPlaying {
		c.OnShortcut(0, ui.KeyEscape, func() { a.nowPlaying = false })
		// ⌘ and a number choose Now Playing's tab of that place.
		if a.pl.Current() != nil {
			for i, t := range a.npTabs() {
				c.OnShortcut(ui.Cmd, ui.Key1+ui.Key(i), func() { a.choosePane(t.pane) })
			}
		}
	}
}

// open opens a page, remembering the one before for the back button, and
// closes Now Playing over it, though it is the page open.
func (a *app) open(r route) {
	a.nowPlaying = false
	if r == a.route {
		return
	}
	a.history, a.forward = append(a.history, a.route), nil
	a.show(r)
	// Opened anew, not gone back to, a page paged through starts over.
	if id := browseID(r); id != "" {
		if lib := a.library; lib.browses[id] != nil && lib.browses[id].paged {
			a.loadBrowse(id, true)
		}
	}
}

// show shows the page of r.
func (a *app) show(r route) {
	a.route = r
	if r.page == pageSearch {
		a.query, a.searched = r.query, r
	}
	a.load(r)
}

// load loads what the page of r shows, unless it has.
func (a *app) load(r route) {
	lib := a.library
	switch r.page {
	case pageHome:
		a.freshenHome()
	case pagePlaylist:
		a.loadPlaylist(r.id, false)
	case pageSearch, pageBrowse:
		a.loadBrowse(browseID(r), false)
	case pageArtist:
		a.loadArtist(r.id)
	case pageAlbum:
		a.loadAlbum(r.id)
	case pageLiked:
		if !lib.likedPaged.loaded {
			a.loadLikedPage()
		} else {
			a.freshenLiked()
		}
	}
}

// back goes to the page before, as the back button of a mouse does, or
// closes Now Playing, which covers the pages.
func (a *app) back() {
	if a.nowPlaying {
		a.nowPlaying = false
		return
	}
	if len(a.history) == 0 {
		return
	}
	a.forward = append(a.forward, a.route)
	r := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	a.show(r)
	a.returning = a.route
}

// goForward goes to the page back left, as the forward button of a mouse
// does.
func (a *app) goForward() {
	if len(a.forward) == 0 {
		return
	}
	a.history = append(a.history, a.route)
	r := a.forward[len(a.forward)-1]
	a.forward = a.forward[:len(a.forward)-1]
	a.nowPlaying = false
	a.show(r)
	a.returning = a.route
}

// clearSearch empties the field and forgets the search last shown, which
// the sidebar goes back to; on its page, it goes to what Search shows before
// a word is typed.
func (a *app) clearSearch() {
	a.searched, a.query = route{}, ""
	a.dismissSuggestions()
	if a.route.page == pageSearch && a.route.query != "" && !a.nowPlaying {
		a.open(route{page: pageSearch})
	}
}

// playerBarHeight is the height of the player along the foot of the pages.
const playerBarHeight = 84

// pageGap is the room between the bar over a page and its content.
const pageGap = 12

// insets returns the room the bars over a page take at its top and its
// bottom: pages pad their content by it, and scroll under the bars.
func (a *app) insets(c *ui.Context) (top, bottom float32) {
	top = topBar(c)
	if runtime.GOOS == "darwin" {
		// The edge reaches a little below the traffic lights' strip.
		top += topEdgePad
	}
	return top, playerBarHeight
}

// topEdgePad is the room the edge over a page leaves below the strip, on
// macOS.
const topEdgePad = 6

// buttonsHeight is the height of the window buttons off macOS, which fill
// the edge over a page.
const buttonsHeight = topBarHeight + topEdgePad

// topBar returns the height of the strip along the top of the window, as
// tall as the window buttons and the room above and below them, which
// what sits in it centers on.
func topBar(c *ui.Context) float32 {
	if runtime.GOOS != "darwin" {
		// The whole edge, which the buttons fill: what sits in the strip
		// centers on their glyphs.
		return buttonsHeight
	}
	return max(c.TitleBar().Height, topBarHeight)
}

// topBarHeight is the least height of the strip along the top.
const topBarHeight = 40

// startMark marks where a page's content starts, first in what scrolls:
// once it has moved up, content is under the bar at the top.
func (a *app) startMark(c *ui.Context) {
	b := ui.Box(c.Key("start-mark")).Height(0).Bounds()
	if b.W > 0 {
		top, _ := a.insets(c)
		a.underTop = b.Y < top+pageGap-0.5
		a.marked = true
		a.markY, a.markBuilt = b.Y, true
	}
}

// endMark marks where a page's content ends, last in what scrolls: while it
// is below the top of the player, content is under the player.
func (a *app) endMark(c *ui.Context) {
	b := ui.Box(c.Key("end-mark")).Height(0).Bounds()
	if b.W > 0 {
		_, h := c.Size()
		_, bottom := a.insets(c)
		a.underBottom = b.Y > h-bottom+0.5
		a.endMarked = true
	}
}

// edgeShown returns how much of a scroll edge shows over el: fully while
// content is under its bar or the pointer is over it, and none at rest, as
// AppKit shows its edges.
func edgeShown(el ui.Element, under bool) float32 {
	target := float32(0)
	if under {
		target = 1
	}
	return el.AnimateTo("shown", target, ui.Animation{Duration: 180 * time.Millisecond})
}

// topEdge returns how much of the edge over the page shows, under says
// whether it should. It fades as content scrolls under it; but a page just
// turned to has its edge as it should be at once, rather than the last
// page's fading out over it: as soon as the page's start mark says where it
// is, or a frame on where it has none, the edge held as it was till then.
func (a *app) topEdge(c *ui.Context, edge ui.Element, under bool) float32 {
	if a.edgeSnap > 0 {
		a.edgeSnap--
		if !a.marked && a.edgeSnap > 0 {
			c.AnimationFrame()
			return edge.Jump("shown", a.topShown)
		}
		a.edgeSnap = 0
		a.topShown = 0
		if under {
			a.topShown = 1
		}
		return edge.Jump("shown", a.topShown)
	}
	a.topShown = edgeShown(edge, under)
	return a.topShown
}

// pageTop is the strip over the top of a page, frosting what scrolls under
// it as macOS 26's hard scroll edge does: it drags the window, and holds
// the back button once there is a page to go back to.
func (a *app) pageTop(c *ui.Context) {
	top, _ := a.insets(c)
	edge := ui.Box(c.Key("top-edge")).Absolute().Top(0).Left(0).Right(0).Height(top).DragWindow().
		Material(glass.ScrollEdge{Hard: true, Background: a.pal.bg})
	// The pointer over it brings the edge up too, as AppKit's does.
	edge.Opacity(a.topEdge(c, edge, a.underTop || edge.Hovered()))
	if len(a.history) > 0 || len(a.forward) > 0 {
		ui.Row(c).Absolute().Top(0).Left(20).Height(topBar(c)).AlignItems(ui.Center).Children(func() {
			back := iconButton(c.Key("back"), iconBack, 18, a.pal.muted, T("Back")).Disabled(len(a.history) == 0)
			if len(a.history) == 0 {
				back.Opacity(0.35)
			}
			if back.Clicked() {
				a.back()
			}
			if len(a.forward) > 0 && iconButton(c.Key("forward"), iconForward, 18, a.pal.muted, T("Forward")).Clicked() {
				a.goForward()
			}
		})
	}
}

// pagePlace is where a page was scrolled, down and its shelves across.
type pagePlace struct {
	list       ui.ListState
	scroll     ui.ScrollState
	shelves    map[int]*shelfScroll
	shelfSongs map[int]*shelfTracks
}

// turnPage keeps where the page left was scrolled, and starts the page now
// shown where it was left if back or forward went to it, else at its top,
// as Music does. Only the pages there is a way back or forward to are kept.
func (a *app) turnPage() {
	if a.pagePlaces == nil {
		a.pagePlaces = map[route]pagePlace{}
	}
	if a.scrolled != (route{}) {
		a.pagePlaces[a.scrolled] = pagePlace{a.trackList, a.pageScroll, a.shelves, a.shelfSongs}
	}
	var p pagePlace
	if a.returning == a.route {
		p = a.pagePlaces[a.route]
	}
	// What was kept goes back to the page, not shared with a copy here.
	delete(a.pagePlaces, a.route)
	a.trackList, a.pageScroll, a.shelves, a.shelfSongs = p.list, p.scroll, p.shelves, p.shelfSongs
	a.scrolled, a.returning = a.route, route{}
	// The edge over it goes where it should at once (topEdge), as soon as
	// the page says where that is: a list takes a build or two to lay out
	// its first row, and a page without marks says nothing.
	a.edgeSnap = 4
	for r := range a.pagePlaces {
		if !slices.Contains(a.history, r) && !slices.Contains(a.forward, r) {
			delete(a.pagePlaces, r)
		}
	}
}

func (a *app) content(c *ui.Context) {
	// A new page starts at its top, and one gone back to where it was left.
	// Not as it is opened: a control in a list, as a page's tabs in its
	// header, opens it as the list builds.
	if a.scrolled != a.route {
		a.turnPage()
	}
	// Pages mark where their content is, as they scroll.
	a.underTop, a.underBottom, a.marked = false, false, false
	// Whether the page shows its skeleton, from what it built alone: the
	// sidebar, built before it, may show one of its own.
	skeleton := a.skelShown == a.route
	a.skelOf, a.endMarked = route{}, false
	defer func() {
		a.skelShown = a.skelOf
		if skeleton && a.skelShown != a.route {
			// What it was loading arrived: the edge over its foot goes
			// where it should at once.
			a.footSnap = 4
		}
	}()
	switch a.route.page {
	case pageHome:
		a.homePage(c)
	case pagePlaylist:
		a.playlistPage(c)
	case pageSearch:
		a.searchPage(c)
	case pageArtist:
		a.artistPage(c)
	case pageAlbum:
		a.albumPage(c)
	case pageLiked:
		a.likedPage(c)
	case pageBrowse:
		a.browsePage(c)
	}
}

// centered shows a message, with a retry button if retry is set: what a
// page shows in place of what failed to load. Where the source wants the
// listener to sign in or set it up, it offers that.
func (a *app) centered(c *ui.Context, err string, retry func()) {
	top, bottom := a.insets(c)
	a.failure(ui.Column(c).Grow(1).Padding(top, 0, bottom, 0).Center(), c, err, retry)
}

// failure fills col with what centered shows: the message, and the buttons
// that may put it right.
func (a *app) failure(col ui.Element, c *ui.Context, err string, retry func()) {
	col.Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, err).TextColor(a.pal.muted).FontSize(13).TextAlign(ui.Center).MaxWidth(520)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			a.remedy(c)
			if retry != nil && ui.Button(c.Key("retry"), T("Try Again")).Clicked() {
				retry()
			}
		})
	})
}

// remedy is a button that takes the listener where the source's error can be
// fixed, if it said one.
func (a *app) remedy(c *ui.Context) {
	lib := a.library
	p := a.pal
	switch {
	case lib.needsSignIn && lib.signedOut():
		// As Home's own, which says the same.
		if pillButton(c.Key("sign-in"), iconUser, T("Sign In"), p.accent, p.onAccent).Clicked() {
			a.promptSignIn()
		}
	case lib.needsSetup:
		if ui.PrimaryButton(c.Key("setup"), T("Open Settings")).Clicked() {
			a.openAccountSettings()
		}
	}
}

// Loading what the pages show.

// homeFresh is how long what Home shows stays good. Apple's picks may change
// by the hour; the app may stay open overnight.
const homeFresh = time.Hour

// loadHome loads what Home shows: Apple Music's page, or what was played
// lately if that has nothing, the playlists being the sidebar's.
func (a *app) loadHome() {
	lib := a.library
	lib.homeLoading, lib.homeErr = true, ""
	at := time.Now()
	go func() {
		var home *music.Browse
		var recent []music.Track
		var homeErr, recentErr error
		home, homeErr = lib.src.Home()
		if home == nil || len(home.Sections) == 0 {
			home = nil
			recent, recentErr = lib.src.Recent()
		}
		a.post(func() {
			lib.homeLoading = false
			// What was kept from last time stays, if Home can't be loaded now.
			if homeErr != nil && lib.homeKept && lib.home != nil {
				log.Printf("Home: %v", homeErr)
				return
			}
			lib.home, lib.recent, lib.homeKept = home, recent, false
			lib.homeAt = at
			switch {
			case homeErr != nil:
				lib.homeErr = a.problem(homeErr, T("Couldn't load this page"))
			case recentErr != nil:
				lib.homeErr = a.problem(recentErr, T("Couldn't load this page"))
			default:
				a.keepPages()
			}
		})
	}()
}

// freshenHome loads the home page again a while after it last did.
func (a *app) freshenHome() {
	lib := a.library
	if !lib.homeLoading && !lib.homeAt.IsZero() && time.Since(lib.homeAt) > homeFresh {
		a.loadHome()
	}
}

// listsFresh is how long the listener's playlists and liked songs, as last
// loaded, are taken to be what the source has. They change elsewhere too, in
// its own apps and on its website: opened again after it, or as the window
// comes back to the front, they are asked for anew.
const listsFresh = 30 * time.Second

// loadPlaylist loads a playlist's page, and plays it once it has, if thenPlay.
// Loaded a while ago, it shows as it was while it's asked for anew.
func (a *app) loadPlaylist(id string, thenPlay bool) {
	lib := a.library
	if pl := lib.playlists[id]; pl != nil {
		if thenPlay {
			a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks), playlistOrigin(pl))
		}
		if !lib.freshPl[id] && time.Since(lib.plAt[id]) > listsFresh {
			a.freshenPlaylist(id)
		}
		return
	}
	if lib.loadingPl[id] {
		return
	}
	lib.loadingPl[id] = true
	delete(lib.plErr, id)
	at := time.Now()
	go func() {
		pl, err := answered(lib.src.Playlist(id))
		a.post(func() {
			delete(lib.loadingPl, id)
			if err != nil {
				lib.plErr[id] = a.problem(err, T("Couldn't load this playlist"))
				return
			}
			lib.playlists[id], lib.plAt[id] = pl, at
			if thenPlay {
				a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks), playlistOrigin(pl))
			}
			a.keepPages()
		})
	}()
}

// freshenPlaylist asks for a loaded playlist anew (refreshPlaylist).
func (a *app) freshenPlaylist(id string) { a.refreshPlaylist(id, false, nil) }

// refreshPlaylist asks for a loaded playlist anew, which shows as it was until
// the answer takes its place. Failing, it stays as it was. quiet asks as for
// the listener's sidebar before they open it (preloadPlaylist), and then is
// called when it's over.
func (a *app) refreshPlaylist(id string, quiet bool, then func()) {
	lib := a.library
	was := lib.playlists[id]
	before := *was
	lib.freshPl[id] = true
	at := time.Now()
	go func() {
		var pl *music.Playlist
		var err error
		if quiet {
			pl, err = preloadPlaylist(lib.src, id)
		} else {
			pl, err = answered(lib.src.Playlist(id))
		}
		a.post(func() {
			if then != nil {
				defer then()
			}
			delete(lib.freshPl, id)
			// Changed in the app meanwhile, its songs or name, or forgotten:
			// what's there is newer than what was asked for.
			if err != nil || lib.playlists[id] != was || !sameList(was.Tracks, before.Tracks) ||
				was.Name != before.Name || was.Saved != before.Saved {
				return
			}
			lib.playlists[id], lib.plAt[id] = pl, at
			a.keepPages()
		})
	}()
}

// sameList reports whether x and y are one list, not two alike: the app's
// changes to a playlist's songs make a new one.
func sameList(x, y []music.Track) bool {
	return len(x) == len(y) && (len(x) == 0 || &x[0] == &y[0])
}

// freshen asks anew for what the listener keeps in Apple Music, as
// the window comes back to the front: their playlists, and the page showing
// if it's one of them.
func (a *app) freshen() {
	lib := a.library
	if lib == nil || !lib.started {
		return
	}
	if time.Since(lib.mineAt) > listsFresh {
		a.loadMine()
	}
	if r := a.route; r.page == pagePlaylist || r.page == pageLiked {
		a.load(r)
	}
	a.warmPages()
}

func firstPlayable(tracks []music.Track) int {
	for i := range tracks {
		if !tracks[i].Unavailable {
			return i
		}
	}
	return 0
}
