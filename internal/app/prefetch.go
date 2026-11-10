package app

import (
	"log"
	"slices"
	"time"

	"mygo-music/internal/music"
)

// What the app fetches before the listener asks, so that a page of the
// sidebar is there as it is clicked, and shows as it was while it is asked for
// anew (loadBrowse, loadPlaylist, freshenLiked).
//
// What: the first part of each page of the sidebar: Recently Added, Albums,
// Songs, Artists, Favorite Songs, All Playlists, Made for You, New, Radio and
// what Search shows before a word is typed, and the first warmPlaylists of the
// playlists the sidebar lists. The library's are fetched for a listener who is
// signed in only; Home is loaded with the library (startLibrary).
//
// When: once the source says who is signed in, and as another signs in
// (afterAccount); as the playlists are listed (loadMine); as the window comes
// back to the front (freshen); and every warmEvery while the window shows.
// Of those, what is missing is fetched, and what is older than warmStale is
// fetched anew: at most ten pages and twelve playlists a round, which the
// source answers from what it kept where it can.
//
// In what order: as the listener is likely to want them, the page they are
// on first, the library's before the editors', which are bigger and opened
// more seldom, the playlists' last.
//
// Only a source that has accounts is fetched from this way, as the pages named
// are Apple Music's; a plugin's are asked for as they open. Nothing is fetched
// ahead for a listener who is signed out but for the pages that need no
// account, and a round that finds nothing due asks for nothing.
//
// How: at most warmAtOnce requests at a time, none starting while the
// listener waits for the page they are on, and what the listener asks for
// themselves isn't held back by any of it. A request that fails is dropped:
// the page asks again as it is opened, and says why then.

const (
	warmAtOnce    = 4
	warmPlaylists = 12
	warmStale     = 5 * time.Minute
	warmEvery     = 10 * time.Minute
	// likedPart is how many favorite songs the source gives to a request.
	likedPart = 100
)

// warmQueue is what's to be fetched ahead, and what is being.
type warmQueue struct {
	jobs   []warmJob
	flying int
	keys   map[string]bool // those queued or flying
}

// A warmJob is one thing to fetch ahead. due says it is still wanted as its
// turn comes; start begins it, and calls done on the main thread once it has
// landed or is dropped.
type warmJob struct {
	key   string
	due   func() bool
	start func(done func())
}

func newWarmQueue() *warmQueue { return &warmQueue{keys: map[string]bool{}} }

// warmPages queues what is due of what the sidebar opens, and starts it, for a
// source that has accounts, whose pages these are: another's are its own.
func (a *app) warmPages(lib *library) {
	if lib == nil || lib != a.library || !lib.started || !lib.can("account") || lib.accountUnknown() {
		return
	}
	w := lib.warm
	for _, j := range a.warmJobs(lib) {
		if !w.keys[j.key] && j.due() {
			w.keys[j.key] = true
			w.jobs = append(w.jobs, j)
		}
	}
	// The page the listener is on first.
	if key := warmKey(a.route); key != "" {
		if i := slices.IndexFunc(w.jobs, func(j warmJob) bool { return j.key == key }); i > 0 {
			j := w.jobs[i]
			w.jobs = slices.Insert(slices.Delete(w.jobs, i, i+1), 0, j)
		}
	}
	a.pumpWarm(lib)
}

// warmKey is the job that fetches the page r shows.
func warmKey(r route) string {
	switch r.page {
	case pageBrowse, pageSearch:
		if id := browseID(r); id != "" {
			return "browse:" + id
		}
	case pageLiked:
		return "liked"
	case pagePlaylist:
		return "playlist:" + r.id
	}
	return ""
}

// warmJobs are all the pages the sidebar opens, in the order to fetch them.
func (a *app) warmJobs(lib *library) []warmJob {
	signedIn := lib.account != nil || !lib.can("signIn")
	var jobs []warmJob
	browse := func(page string) {
		if lib.can("browse") {
			jobs = append(jobs, a.warmBrowse(lib, page))
		}
	}
	playlists := func(from, to int) {
		if !lib.can("playlist") || !signedIn {
			return
		}
		lists := append(append([]music.Playlist(nil), lib.mine...), lib.saved...)
		for i := from; i < min(to, len(lists)); i++ {
			jobs = append(jobs, a.warmPlaylist(lib, lists[i].ID))
		}
	}
	if signedIn {
		for _, page := range []string{music.PageRecentlyAdded, music.PageLibraryAlbums, music.PageLibrarySongs, music.PageLibraryArtist} {
			browse(page)
		}
		if lib.can("liked") {
			jobs = append(jobs, a.warmLiked(lib))
		}
		for _, page := range []string{music.PageAllPlaylists, music.PageMadeForYou} {
			browse(page)
		}
	}
	playlists(0, 4)
	for _, page := range []string{music.PageNew, music.PageRadio, music.PageSearchLanding} {
		browse(page)
	}
	playlists(4, warmPlaylists)
	return jobs
}

// pumpWarm starts what is queued while there's room, unless the listener is
// waiting for the page they're on.
func (a *app) pumpWarm(lib *library) {
	w := lib.warm
	for len(w.jobs) > 0 && w.flying < warmAtOnce && !a.listenerWaits(lib) {
		j := w.jobs[0]
		w.jobs = w.jobs[1:]
		if !j.due() {
			delete(w.keys, j.key)
			continue
		}
		w.flying++
		j.start(func() {
			w.flying--
			delete(w.keys, j.key)
			a.pumpWarm(lib)
		})
	}
}

// listenerWaits reports whether the page the listener is on has nothing to
// show yet, as it loads.
func (a *app) listenerWaits(lib *library) bool {
	r := a.route
	if r.src != lib.id {
		return false
	}
	switch r.page {
	case pageHome:
		return lib.homeLoading && lib.home == nil
	case pagePlaylist:
		return lib.loadingPl[r.id] && lib.playlists[r.id] == nil
	case pageLiked:
		return lib.likedPaged.loading && !lib.likedPaged.loaded
	case pageArtist:
		d := lib.artists[r.id]
		return d != nil && d.loading && d.page == nil
	case pageAlbum:
		d := lib.albums[r.id]
		return d != nil && d.loading && d.album == nil
	case pageBrowse, pageSearch:
		d := lib.browses[browseID(r)]
		return d != nil && d.loading && d.page == nil
	}
	return false
}

// warmTick refreshes the pages every warmEvery, while the window shows: not
// hidden to the menu bar or minimized, where nobody sees them change.
func (a *app) warmTick() {
	time.AfterFunc(warmEvery, func() {
		a.post(func() {
			if a.win != nil && a.win.IsVisible() && !a.win.IsMinimized() {
				for _, lib := range a.libs {
					a.warmPages(lib)
				}
			}
			a.warmTick()
		})
	})
}

// preloadBrowse asks a source for a page before the listener does, as a
// music.Preloader answers it, or as it would the listener.
func preloadBrowse(src music.Source, page string) (*music.Browse, error) {
	if p, ok := src.(music.Preloader); ok {
		return answered(p.PreloadBrowse(page))
	}
	return answered(src.Browse(page))
}

func preloadPlaylist(src music.Source, id string) (*music.Playlist, error) {
	if p, ok := src.(music.Preloader); ok {
		return answered(p.PreloadPlaylist(id))
	}
	return answered(src.Playlist(id))
}

func preloadLiked(src music.Source) (*music.Page, error) {
	if p, ok := src.(music.Preloader); ok {
		return answered(p.PreloadLiked())
	}
	return answered(src.Liked(0))
}

// warmBrowse fetches the first part of a page. It takes the place of the
// page's, unless the listener's own asking is on its way, or got there first,
// or they've paged through what it is, which would go back to its start.
func (a *app) warmBrowse(lib *library, page string) warmJob {
	return warmJob{
		key: "browse:" + page,
		due: func() bool {
			d := lib.browses[page]
			return d == nil || !d.loading && (d.page == nil || !d.paged && time.Since(d.at) > warmStale)
		},
		start: func(done func()) {
			w, start := lib.warm, time.Now()
			go func() {
				b, err := preloadBrowse(lib.src, page)
				a.post(func() {
					defer done()
					if err != nil {
						log.Printf("source %s: fetching %s ahead: %v", lib.id, page, err)
						return
					}
					d := lib.browses[page]
					if d == nil {
						d = &browseData{}
						lib.browses[page] = d
					}
					if lib.warm != w || d.loading || d.page != nil && (d.paged || !d.at.Before(start)) {
						return
					}
					d.page, d.at, d.err = b, time.Now(), ""
					d.next, d.paged = paged{loaded: true, more: b.Next != ""}, false
					a.keepPages(lib)
				})
			}()
		},
	}
}

// warmPlaylist fetches a playlist, or fetches it anew, if it's old.
func (a *app) warmPlaylist(lib *library, id string) warmJob {
	return warmJob{
		key: "playlist:" + id,
		due: func() bool {
			if lib.loadingPl[id] || lib.freshPl[id] {
				return false
			}
			return lib.playlists[id] == nil || time.Since(lib.plAt[id]) > warmStale
		},
		start: func(done func()) {
			if lib.playlists[id] != nil {
				a.refreshPlaylist(lib, id, true, done)
				return
			}
			w, at := lib.warm, time.Now()
			go func() {
				pl, err := preloadPlaylist(lib.src, id)
				a.post(func() {
					defer done()
					if err != nil {
						log.Printf("source %s: fetching a playlist ahead: %v", lib.id, err)
						return
					}
					if lib.warm != w || lib.playlists[id] != nil || lib.loadingPl[id] {
						return
					}
					lib.playlists[id], lib.plAt[id] = pl, at
					a.keepPages(lib)
				})
			}()
		},
	}
}

// warmLiked fetches the first part of the favorite songs, or fetches it anew,
// if it's old and the listener hasn't gone on past it.
func (a *app) warmLiked(lib *library) warmJob {
	return warmJob{
		key: "liked",
		due: func() bool {
			if lib.likedPaged.loading || lib.likedFresh {
				return false
			}
			return !lib.likedPaged.loaded || len(lib.likedList) <= likedPart && time.Since(lib.likedAt) > warmStale
		},
		start: func(done func()) {
			if lib.likedPaged.loaded {
				a.refreshLiked(lib, true, done)
				return
			}
			w, at := lib.warm, time.Now()
			go func() {
				page, err := preloadLiked(lib.src)
				a.post(func() {
					defer done()
					if err != nil {
						log.Printf("source %s: fetching the favorite songs ahead: %v", lib.id, err)
						return
					}
					if lib.warm != w || lib.likedPaged.loaded || lib.likedPaged.loading {
						return
					}
					a.setLiked(lib, page, at)
					a.keepPages(lib)
				})
			}()
		},
	}
}
