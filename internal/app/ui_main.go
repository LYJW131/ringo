package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

const sidebarWidth = 232

func (a *app) view(c *ui.Context) {
	t := *c.Theme()
	a.pal = paletteFor(&t)
	p := a.pal
	useAccent(&t, p)
	t.Selection = p.accent.Alpha(0.3)
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
			ui.Box(c.Key("main")).Grow(1).Background(p.bg).Children(func() {
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
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
	a.askLikes()
	a.syncWindows()
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
// closes Now Playing over it, though it is the page open. A page of another
// source makes that source the active one.
func (a *app) open(r route) {
	a.nowPlaying = false
	if r.src == "" {
		r.src = a.route.src
	}
	if r == a.route {
		return
	}
	a.history, a.forward = append(a.history, a.route), nil
	a.show(r)
	// Opened anew, not gone back to, a page paged through starts over.
	if id := browseID(r); id != "" {
		if lib := a.libOf(r.src); lib != nil && lib.browses[id] != nil && lib.browses[id].paged {
			a.loadBrowse(lib, id, true)
		}
	}
}

// show shows the page of r.
func (a *app) show(r route) {
	if r.src == "" {
		r.src = a.route.src
	}
	lib := a.libOf(r.src)
	if lib == nil {
		// Its source went away since: the active one's Home instead.
		r = route{page: pageHome, src: a.library.id}
		lib = a.library
	}
	a.activate(lib)
	a.route = r
	if r.page == pageSearch {
		a.query, a.searched = r.query, r
	}
	a.load(r)
}

// load loads what the page of r shows, unless it has.
func (a *app) load(r route) {
	lib := a.libOf(r.src)
	if lib == nil {
		return
	}
	switch r.page {
	case pageHome:
		a.freshenHome()
	case pagePlaylist:
		a.loadPlaylist(lib, r.id, false)
	case pageSearch, pageBrowse:
		a.loadBrowse(lib, browseID(r), false)
	case pageArtist:
		a.loadArtist(lib, r.id)
	case pageAlbum:
		a.loadAlbum(lib, r.id)
	case pageLiked:
		if !lib.likedPaged.loaded {
			a.loadLikedPage(lib)
		} else {
			a.freshenLiked(lib)
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

// pageTop is the strip over the top of a page, frosting what scrolls under
// it as macOS 26's hard scroll edge does: it drags the window, and holds
// the back button once there is a page to go back to.
func (a *app) pageTop(c *ui.Context) {
	top, _ := a.insets(c)
	edge := ui.Box(c.Key("top-edge")).Absolute().Top(0).Left(0).Right(0).Height(top).DragWindow().
		Material(glass.ScrollEdge{Hard: true, Background: a.pal.bg})
	// The pointer over it brings the edge up too, as AppKit's does.
	edge.Opacity(edgeShown(edge, a.underTop || edge.Hovered()))
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

func (a *app) content(c *ui.Context) {
	// A new page starts at its top. Not as it is opened: a control in a
	// list, as a page's tabs in its header, opens it as the list builds.
	if a.scrolled != a.route {
		a.trackList, a.pageScroll, a.scrolled = ui.ListState{}, ui.ScrollState{}, a.route
		a.shelves, a.shelfSongs = nil, nil
	}
	// Pages mark where their content is, as they scroll.
	a.underTop, a.underBottom = false, false
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
	ui.Column(c).Grow(1).Padding(top, 0, bottom, 0).Center().Gap(12).Children(func() {
		ui.Text(c, err).TextColor(a.pal.muted).FontSize(13).TextAlign(ui.Center).MaxWidth(520)
		ui.Row(c).Gap(10).Children(func() {
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
	switch {
	case lib.needsSignIn && lib.signedOut():
		if ui.PrimaryButton(c.Key("sign-in"), T("Sign In")).Clicked() {
			a.promptSignIn(lib)
		}
	case lib.needsSetup:
		if ui.PrimaryButton(c.Key("setup"), T("Open Settings")).Clicked() {
			a.openAccountSettings()
		}
	}
}

// Loading what the pages show.

// homeFresh is how long what Home shows stays good. A plugin's picks may change
// by the hour; the app may stay open overnight.
const homeFresh = time.Hour

// loadHome loads what the Home of a source shows: its own page if it makes one
// up, else what it played lately, the playlists being the sidebar's.
func (a *app) loadHome(lib *library) {
	lib.homeLoading, lib.homeErr = true, ""
	at := time.Now()
	go func() {
		var home *music.Browse
		var recent []music.Track
		var homeErr, recentErr error
		if lib.can("home") {
			home, homeErr = lib.src.Home()
		}
		if home == nil || len(home.Sections) == 0 {
			home = nil
			if lib.can("recent") {
				recent, recentErr = lib.src.Recent()
			}
		}
		a.post(func() {
			lib.homeLoading = false
			lib.home, lib.recent = home, recent
			lib.homeAt = at
			switch {
			case homeErr != nil:
				lib.homeErr = a.problem(lib, homeErr, T("Couldn't load this page"))
			case recentErr != nil:
				lib.homeErr = a.problem(lib, recentErr, T("Couldn't load this page"))
			}
		})
	}()
}

// freshenHome loads the home page again a while after it last did.
func (a *app) freshenHome() {
	lib := a.library
	if !lib.homeLoading && !lib.homeAt.IsZero() && time.Since(lib.homeAt) > homeFresh {
		a.loadHome(lib)
	}
}

// listsFresh is how long the listener's playlists and liked songs, as last
// loaded, are taken to be what the source has. They change elsewhere too, in
// its own apps and on its website: opened again after it, or as the window
// comes back to the front, they are asked for anew.
const listsFresh = 30 * time.Second

// loadPlaylist loads a playlist's page, and plays it once it has, if thenPlay.
// Loaded a while ago, it shows as it was while it's asked for anew.
func (a *app) loadPlaylist(lib *library, id string, thenPlay bool) {
	if pl := lib.playlists[id]; pl != nil {
		if thenPlay {
			a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks))
		}
		if !lib.freshPl[id] && time.Since(lib.plAt[id]) > listsFresh {
			a.freshenPlaylist(lib, id)
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
				lib.plErr[id] = a.problem(lib, err, T("Couldn't load this playlist"))
				return
			}
			lib.playlists[id], lib.plAt[id] = pl, at
			if thenPlay {
				a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks))
			}
		})
	}()
}

// freshenPlaylist asks for a loaded playlist anew, which shows as it was until
// the answer takes its place. Failing, it stays as it was.
func (a *app) freshenPlaylist(lib *library, id string) {
	was := lib.playlists[id]
	before := *was
	lib.freshPl[id] = true
	at := time.Now()
	go func() {
		pl, err := answered(lib.src.Playlist(id))
		a.post(func() {
			delete(lib.freshPl, id)
			// Changed in the app meanwhile, its songs or name, or forgotten:
			// what's there is newer than what was asked for.
			if err != nil || lib.playlists[id] != was || !sameList(was.Tracks, before.Tracks) ||
				was.Name != before.Name || was.Saved != before.Saved {
				return
			}
			lib.playlists[id], lib.plAt[id] = pl, at
		})
	}()
}

// sameList reports whether x and y are one list, not two alike: the app's
// changes to a playlist's songs make a new one.
func sameList(x, y []music.Track) bool {
	return len(x) == len(y) && (len(x) == 0 || &x[0] == &y[0])
}

// freshen asks anew for what the listener keeps of the active source, as
// the window comes back to the front: their playlists, and the page showing
// if it's one of them.
func (a *app) freshen() {
	lib := a.library
	if lib == nil || !lib.started {
		return
	}
	if time.Since(lib.mineAt) > listsFresh {
		a.loadMine(lib)
	}
	if r := a.route; r.src == lib.id && (r.page == pagePlaylist || r.page == pageLiked) {
		a.load(r)
	}
}

func firstPlayable(tracks []music.Track) int {
	for i := range tracks {
		if !tracks[i].Unavailable {
			return i
		}
	}
	return 0
}
