package app

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
	"mygo-music/internal/player"
)

type pageKind int

const (
	pageHome pageKind = iota
	pagePlaylist
	pageSearch
	pageArtist
	pageAlbum
	pageLiked
	pageBrowse
)

type route struct {
	page pageKind
	// src is the ID of the source the page is of. Opened without one, a page
	// is of the active source's.
	src string
	// id is the playlist, artist, album or browse page shown.
	id string
	// query is what a search page searched for: none for what Search shows
	// before a word is typed.
	query string
}

// app is the state both windows show. Everything in it is read and changed
// on the main thread: by the views, and by goroutines through post.
type app struct {
	win   *mygo.Window
	notch *notch
	tray  *mygo.Tray // the icon in the menu bar or notification area, while it shows
	art   *artCache
	pl    *player.Player
	pal   palette

	// libs are the sources, of which the app has one, Apple Music, and what
	// it has of each. The active one's is embedded: a.mine is what it lists.
	// What comes back from a goroutine goes to the library it asked.
	libs []*library
	*library

	wasBehind bool // the window wasn't at the front as last built

	route   route
	history []route
	forward []route // the pages gone back from, the last first to go to

	accountOpen bool         // the dropdown of the account row shows
	signIn      signInDialog // signing in to a source
	// signInWin is the window of Apple's sign-in page, while it shows. Main
	// thread only.
	signInWin *mygo.Window
	// signInOver is the window the sign-in dialog is in, for the sign-in page
	// to open over: the settings window's, if asked there. Set on the main
	// thread, read on the source's.
	signInOver  *atomic.Pointer[mygo.Window]
	naming      nameDialog         // the name of a playlist asked for
	deleting    deleteDialog       // whether to delete a playlist, asked
	toast       string             // a message to show once
	settingText map[string]*string // the text settings, as being typed

	trackList ui.ListState
	// Where the page and the sidebar are scrolled, kept while Now Playing
	// covers them, unbuilt.
	pageScroll ui.ScrollState
	navScroll  ui.ScrollState
	scrolled   route // the page trackList and pageScroll are of
	// notesOpen is the page whose editors' notes or biography show whole,
	// past their first lines.
	notesOpen route
	// shelves are where the shelves of the page that scroll sideways are
	// scrolled, by their section: kept here, as a shelf scrolled out of the
	// list is not built.
	shelves map[int]*ui.ScrollState
	// shelfSongs are the songs of the page's sections, by their section.
	shelfSongs map[int]*shelfTracks

	libraryCollapsed, mineCollapsed bool

	query     string
	searchBox ui.Handle
	// searchFocused says the search field had the focus as last built: it
	// opens Search as it takes it.
	searchFocused bool

	settingsPage      string // the settings' page that shows
	settingsQuery     string // what's searched for in the settings
	settingsSearchBox ui.Handle
	settingsUnderTop  bool // the settings' page has scrolled under its top edge

	suggestions suggestState // what the search field's query may be after

	nowPlaying  bool
	npOpen      bool    // Now Playing as it last showed: open or not
	npT         float32 // how far open Now Playing last showed, 0 to 1
	npShift     float32 // how far down it last showed, pulled
	npPull      float32 // how far the pointer has pulled it down
	npHeld      bool    // whether the pointer presses on it, to pull it
	npStill     bool    // whether it comes and goes in place, sliding or fading, rather than out of the bar's cover
	npFrom      ui.Rect // the player bar's cover, which it grows out of
	npPane      int     // paneLyrics, paneQueue, paneStage or paneWall
	npPaneTop   float32 // how far the cover beside the pane starts below its top
	lyricScroll ui.ScrollState
	lyricGlide  lyricGlide
	lyricAt     int
	lyricTrack  string
	queueList   ui.ListState
	queueAt     int // the row of the queue pane last scrolled to, -1 while it is hidden

	radioStarting string // the radio asked for its first songs, or ""

	volume     float64
	volumeOpen bool // the volume popover of the player bar shows
	mutedFrom  float64

	rowBars barState
	// underTop and underBottom say whether the page has content under its
	// bars, which show their scroll edges then.
	underTop, underBottom bool
	barScrub              scrubState
	npScrub               scrubState
	backdropCache         backdropCache // the backdrop of Now Playing, as last painted
	npArt                 *artwork      // the cover whose colors Now Playing shows
	npCoverState          coverState    // the cover of Now Playing, as it is tilted and flicked
	wallState             wallState     // the wall of the queue's covers, as it is dragged
	sig                   playerSig
	quitting              bool

	settings    settings
	settingsWin *mygo.Window
	cacheUsed   int64 // the bytes the song cache takes, -1 until measured
	updates     bool  // the update plugin is used (useUpdater)

	restored     bool   // playback picked up where it was left
	savedSession string // the session as last kept
	sessionDue   bool   // a save of the session is due

	// postFn, if set, takes what post would run on the main thread, as in
	// tests, which have no window.
	postFn func(func())
}

// post runs fn on the main thread and redraws both windows. Without a
// window, as in tests, it drops fn.
func (a *app) post(fn func()) {
	if a.postFn != nil {
		a.postFn(fn)
		return
	}
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		fn()
		a.redraw()
	})
}

func (a *app) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.notch != nil && a.notch.win != nil {
		a.notch.win.Invalidate()
	}
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}

// shotHook, when set, drives the app to check its design.
var shotHook func(a *app)

func (a *app) showMain() {
	if a.win != nil {
		if a.win.IsMinimized() {
			a.win.Restore()
		}
		a.win.Show()
		a.win.Focus()
		// From the menu bar alone the app may be inactive.
		mygo.App.Focus()
	}
}

// userDataDir is where the app keeps what's the listener's.
func userDataDir(sub string) string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		dir = filepath.Join(os.TempDir(), "ringo")
	}
	return filepath.Join(dir, sub)
}

// makeSources makes the app's source, not started yet.
func (a *app) makeSources() {
	a.library = newLibrary(a.newSource())
	a.libs = []*library{a.library}
	a.route = route{page: pageHome, src: a.library.id}
}

// sourceByID finds the source a track says it's of, for the player.
func (a *app) sourceByID(id string) music.Source {
	if lib := a.libOf(id); lib != nil {
		return lib.src
	}
	// A track that names no source is of the app's one source, which
	// doesn't sign what it answers.
	if id == "" && a.library != nil {
		return a.library.src
	}
	return nil
}

// closeSources stops what the source has running, which ends with the app.
func (a *app) closeSources() {
	for _, lib := range a.libs {
		if closer, ok := lib.src.(interface{ Close() }); ok {
			closer.Close()
		}
	}
}

// Run runs the app until it quits.
func Run() error {
	// Opening the app again shows the running one's window: Linux and
	// Windows would start another, with a second tray icon. Asked before
	// anything else, so the second one touches no data on its way out.
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	a := &app{
		signInOver:  new(atomic.Pointer[mygo.Window]),
		settingText: map[string]*string{},
		lyricAt:     -1,
		queueAt:     -1,
		cacheUsed:   -1,
	}
	a.useUpdater(i18n.Resolve(loadSettings().Language, systemLanguages()))
	mygo.App.WhenReady(func() {
		a.settings = loadSettings()
		a.settings.applyAppearance()
		i18n.SetLanguage(i18n.Resolve(a.settings.Language, systemLanguages()))
		a.npPane = a.lyricsPaneChosen()
		a.makeSources()
		// Before the window: on Windows it lays out its content as it is
		// made, and the view asks the player what plays.
		a.art = newArtCache(a.post)
		a.pl = player.New(a.post)
		a.pl.Quiet = func(fn func()) { mygo.RunOnMain(fn) }
		a.pl.Source = a.sourceByID
		a.pl.Failed = a.sourceFailed
		a.pl.Quality = func() string { return a.settings.quality() }
		a.pl.Normalize = func() bool { return a.settings.Normalize }
		a.pl.Cache = newSongCache(a.settings.CacheMB)
		a.pl.Changed = a.keepSession
		a.pl.More = a.autoplay
		a.volume = a.settings.Volume
		a.pl.SetVolume(a.volume)
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:         mygo.App.Name(),
			Width:         1180,
			Height:        780,
			MinWidth:      880,
			MinHeight:     580,
			StateKey:      "main",
			TitleBarStyle: mygo.TitleBarHidden,
			// As tall as the edge over a page (Linux, Windows).
			TitleBarHeight: buttonsHeight,
			// Inset from the corner, within the sidebar, as macOS 26's
			// apps with a sidebar place them.
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			Vibrancy:             mygo.VibrancySidebar,
			BackgroundColor:      "light-dark(#ffffff, #141416)",
			Content:              ui.View(a.view),
		})
		a.setMenu()
		a.boot()
		// The notch player is macOS's, over its menu bar: elsewhere it would
		// hang at the top of the screen as a black box.
		if runtime.GOOS == "darwin" {
			a.notch = newNotch(a)
		}
		// From the menu bar alone, the Dock icon shows while the window
		// does.
		a.syncDock()
		a.win.OnShow(a.syncDock)
		a.win.OnHide(a.syncDock)
		// Hidden, it draws nothing for a while: what the heap took for
		// the pages it showed goes back to the system, which Go would
		// keep for minutes.
		a.win.OnHide(func() { go debug.FreeOSMemory() })
		// Shown or switched to a while after, it shows what's new.
		a.win.OnFocus(a.freshenHome)
		a.applyTray()
		if shotHook != nil {
			shotHook(a)
		}
		// Closing the window hides it: music plays on, and the Dock icon
		// or the tray's brings it back. A Linux desktop without a tray
		// has neither, so there closing it quits.
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.quitting {
				return
			}
			e.PreventDefault()
			if runtime.GOOS == "darwin" || a.tray != nil {
				a.win.Hide()
			} else {
				mygo.App.Quit()
			}
		})
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		a.settings.Volume = a.volume
		a.settings.save()
		a.saveSession()
	})
	// Windows leaves an icon in the notification area after its app until
	// the pointer passes over it.
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.tray != nil {
			a.tray.Destroy()
		}
		a.closeSources()
	})
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { a.showMain() })
	mygo.App.OnSecondInstance(func([]string, string) { a.showMain() })
	return mygo.App.Run()
}
