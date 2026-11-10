package app

import (
	"errors"
	"log"
	"net/url"
	"time"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// T returns text in the listener's language; see package i18n.
func T(text string) string { return i18n.T(text) }

// TC is T of a text that reads differently by what it's about.
func TC(text, context string) string { return i18n.TC(text, context) }

// Tf is T of a format, formatted.
func Tf(format string, args ...any) string { return i18n.Tf(format, args...) }

// N marks text kept in a table for T to translate when it shows.
func N(text string) string { return i18n.N(text) }

// library is what the app has of Apple Music: what it answered, and who is
// signed in. forget drops what was loaded, as another listener signs in.
type library struct {
	src     music.Source
	started bool // what it lists was asked for

	account      *music.Account
	accountBusy  bool // it's being asked who's signed in
	accountAsked bool // it has said who's signed in, or failed to
	// needsSignIn and needsSetup say the source answered ErrSignIn or
	// ErrSetup, and asked says the listener was already taken there once.
	needsSignIn, needsSetup bool
	askedSignIn, askedSetup bool
	// setupReason is what the source said it needs setting up for, if it did.
	setupReason string

	// choices are the accounts a music.Switcher can switch to, as last
	// listed, and choicesErr what kept some from being listed. switching is
	// the one being switched to, and switchErr why the last switch failed.
	choices              []music.AccountChoice
	choicesErr           string
	choicesBusy          bool
	choicesAgain         bool // asked for while on its way
	choicesAt            time.Time
	switching, switchErr string

	mine, saved []music.Playlist // the listener's playlists: made, and saved
	mineAt      time.Time        // when they were last asked for

	liked      map[string]bool // the tracks known to be liked, or not
	wantLikes  []string        // the tracks a heart showed for, to ask about
	likedList  []music.Track   // the liked songs' page
	likedPaged paged
	likedAt    time.Time // when the liked songs' first page was asked for
	likedFresh bool      // it's being asked for anew

	recent      []music.Track
	home        *music.Browse // the source's own Home
	homeErr     string
	homeLoading bool
	homeAt      time.Time // when what the home page shows was asked for
	// homeKept says home is what was kept from last time (pagecache.go),
	// which stays if asking again fails.
	homeKept bool

	// warm is what's being fetched ahead (prefetch.go), forgotten with what
	// was loaded, as answers to it that come after aren't the new listener's.
	warm *warmQueue
	// pages is what's kept on this computer of what's loaded for the listener,
	// for the next time the app starts, and staged what was kept of the last
	// time, waiting for the source to say who's signed in (pagecache.go).
	pages  *pageStore
	staged *pageFile

	playlists map[string]*music.Playlist
	loadingPl map[string]bool
	plAt      map[string]time.Time // when each playlist loaded was asked for
	freshPl   map[string]bool      // the loaded playlists being asked for anew
	plErr     map[string]string
	artists   map[string]*artistData
	albums    map[string]*albumData
	browses   map[string]*browseData
	searched  route // the search last shown, which the sidebar goes back to

	// favorites and kept are what the listener made a favorite, or added to
	// their library, or undid, in the app, by itemKey: over what the
	// source's answers said, which it may say again only as they load
	// anew. busy are those on their way to the source.
	favorites, kept, busy map[string]bool
	// kept also holds what the source answered of songs shown whose own
	// word wasn't known (songInLibrary): wantKept are those of this frame,
	// askedKept those asked, which aren't asked again, and unsaid those
	// the source failed to answer of. doubted are the player's songs as
	// another listener signed in, whose word was theirs.
	wantKept  []keptSong
	askedKept map[string]bool
	unsaid    map[string]bool
	doubted   map[string]bool
}

func newLibrary(src music.Source) *library {
	lib := &library{src: src}
	lib.forget()
	return lib
}

// forget drops what was loaded of the source, whose answers may have changed.
func (l *library) forget() {
	l.playlists = map[string]*music.Playlist{}
	l.loadingPl = map[string]bool{}
	l.plAt = map[string]time.Time{}
	l.freshPl = map[string]bool{}
	l.plErr = map[string]string{}
	l.artists = map[string]*artistData{}
	l.albums = map[string]*albumData{}
	l.browses = map[string]*browseData{}
	l.liked = nil
	l.favorites, l.kept, l.busy = map[string]bool{}, map[string]bool{}, map[string]bool{}
	l.wantKept, l.askedKept, l.doubted = nil, map[string]bool{}, map[string]bool{}
	l.unsaid = map[string]bool{}
	l.likedList, l.likedPaged, l.likedAt = nil, paged{}, time.Time{}
	l.warm = newWarmQueue()
}

// accountUnknown reports whether the source is yet to say who's signed in.
func (l *library) accountUnknown() bool { return !l.accountAsked }

// signedOut reports whether no one is signed in, which isn't known until the
// source said so.
func (l *library) signedOut() bool { return l.account == nil && l.accountAsked }

// name is what the listener knows the source as.
func (l *library) name() string { return "Apple Music" }

// boot loads the source's pages, and picks up playback where it was left.
func (a *app) boot() {
	a.startLibrary(a.library)
	a.restoreSession()
}

// startLibrary loads what the sidebar and Home show, once.
func (a *app) startLibrary(lib *library) {
	if lib.started {
		return
	}
	lib.started = true
	a.stagePages(lib)
	a.loadAccount(lib)
	a.loadMine(lib)
	a.loadHome(lib)
}

// errNoAnswer stands for a source that answered nothing where it had to answer
// something, which shouldn't take the app down with it.
var errNoAnswer = errors.New("the source gave no answer")

// answered says errNoAnswer of a nil answer to a question without an error.
func answered[T any](v *T, err error) (*T, error) {
	if v == nil && err == nil {
		return nil, errNoAnswer
	}
	return v, err
}

// problem makes out what went wrong with what a source was asked, for a page's
// error state or a toast: msg, with what the source said. ErrSignIn and
// ErrSetup take the listener where they can fix it, the first time only if
// nobody asked for this in particular.
func (a *app) problem(lib *library, err error, msg string) string {
	log.Printf("%s: %v", msg, err)
	switch {
	case errors.Is(err, music.ErrSignIn):
		lib.needsSignIn = true
		if !lib.askedSignIn {
			lib.askedSignIn = true
			a.promptSignIn(lib)
		}
		return Tf("Sign in to %s to see this", lib.name())
	case errors.Is(err, music.ErrSetup):
		lib.needsSetup, lib.setupReason = true, music.Reason(err)
		if !lib.askedSetup {
			lib.askedSetup = true
			a.openAccountSettings()
		}
		if lib.setupReason != "" {
			return lib.setupReason
		}
		return Tf("%s needs setting up first", lib.name())
	case errors.Is(err, music.ErrUnsupported):
		return msg
	}
	// A request that never got an answer says the whole address it went
	// to, which the log above keeps: the listener reads why alone.
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return msg + ": " + err.Error()
}

// actionFailed says that something the listener did didn't work, taking them
// where they can fix it if that's the reason.
func (a *app) actionFailed(lib *library, err error, msg string) {
	log.Printf("%s: %v", msg, err)
	switch {
	case errors.Is(err, music.ErrSignIn):
		a.promptSignIn(lib)
	case errors.Is(err, music.ErrSetup):
		lib.needsSetup, lib.setupReason = true, music.Reason(err)
		a.openAccountSettings()
	case errors.Is(err, music.ErrUnsupported) && music.Reason(err) != "":
		a.toast = music.Reason(err) // the source's word for why it won't
	default:
		a.toast = msg
	}
}

// sourceFailed hears from the player that the source wants something of the
// listener before it plays.
func (a *app) sourceFailed(err error) {
	a.problem(a.library, err, T("Couldn't play this song"))
}

// libraryChanged has what the source answers asked for anew, as after a
// setting that changes it: other pages load anew as they open. The page
// showing stays as it is, rather than going blank, but for a page the source
// makes up, or Home, which take the new one in place as it comes.
func (a *app) libraryChanged(lib *library) {
	was := *lib
	lib.forget()
	lib.home, lib.browses, lib.liked = was.home, was.browses, was.liked
	switch r := a.route; r.page {
	case pagePlaylist:
		if pl := was.playlists[r.id]; pl != nil {
			lib.playlists[r.id] = pl
		}
	case pageAlbum:
		if d := was.albums[r.id]; d != nil && d.album != nil {
			lib.albums[r.id] = d
		}
	case pageArtist:
		if d := was.artists[r.id]; d != nil && d.page != nil {
			lib.artists[r.id] = d
		}
	case pageLiked:
		lib.likedList, lib.likedPaged = was.likedList, was.likedPaged
	}
	shown := browseID(a.route)
	for page := range lib.browses {
		if shown != page {
			delete(lib.browses, page)
		}
	}
	a.loadMine(lib)
	a.loadHome(lib)
	a.reload(a.route)
}

// reload loads what the page of r shows anew.
func (a *app) reload(r route) {
	if id := browseID(r); id != "" {
		a.loadBrowse(a.library, id, true)
		return
	}
	a.load(r)
}
