package app

import (
	"log"
	"slices"
	"time"

	"ringo/internal/music"
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
// Nothing is fetched ahead for a listener who is signed out but for the pages
// that need no account, and a round that finds nothing due asks for nothing.
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

// warmPages queues what is due of what the sidebar opens, and starts it, once
// the source has said whose pages these are.
func (a *app) warmPages() {
	lib := a.library
	if lib == nil || !lib.started || lib.accountUnknown() {
		return
	}
	w := lib.warm
	for _, j := range a.warmJobs() {
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
	a.pumpWarm()
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
func (a *app) warmJobs() []warmJob {
	lib := a.library
	signedIn := lib.account != nil
	var jobs []warmJob
	browse := func(page string) { jobs = append(jobs, a.warmBrowse(page)) }
	playlists := func(from, to int) {
		if !signedIn {
			return
		}
		lists := append(append([]music.Playlist(nil), lib.mine...), lib.saved...)
		for i := from; i < min(to, len(lists)); i++ {
			jobs = append(jobs, a.warmPlaylist(lists[i].ID))
		}
	}
	if signedIn {
		// Albums and Songs as they were last sorted, which is the page the
		// sidebar opens.
		for _, page := range []string{music.PageRecentlyAdded, a.libraryPage(music.PageLibraryAlbums), a.libraryPage(music.PageLibrarySongs), music.PageLibraryArtist} {
			browse(page)
		}
		jobs = append(jobs, a.warmLiked())
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
func (a *app) pumpWarm() {
	w := a.library.warm
	for len(w.jobs) > 0 && w.flying < warmAtOnce && !a.listenerWaits() {
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
			a.pumpWarm()
		})
	}
}

// listenerWaits reports whether the page the listener is on has nothing to
// show yet, as it loads.
func (a *app) listenerWaits() bool {
	lib := a.library
	r := a.route
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
				a.warmPages()
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
func (a *app) warmBrowse(page string) warmJob {
	lib := a.library
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
						log.Printf("fetching %s ahead: %v", page, err)
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
					a.keepPages()
				})
			}()
		},
	}
}

// warmPlaylist fetches a playlist, or fetches it anew, if it's old.
func (a *app) warmPlaylist(id string) warmJob {
	lib := a.library
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
				a.refreshPlaylist(id, true, done)
				return
			}
			w, at := lib.warm, time.Now()
			go func() {
				pl, err := preloadPlaylist(lib.src, id)
				a.post(func() {
					defer done()
					if err != nil {
						log.Printf("fetching a playlist ahead: %v", err)
						return
					}
					if lib.warm != w || lib.playlists[id] != nil || lib.loadingPl[id] {
						return
					}
					lib.playlists[id], lib.plAt[id] = pl, at
					a.keepPages()
				})
			}()
		},
	}
}

// warmLiked fetches the first part of the favorite songs, or fetches it anew,
// if it's old and the listener hasn't gone on past it.
func (a *app) warmLiked() warmJob {
	lib := a.library
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
				a.refreshLiked(true, done)
				return
			}
			w, at := lib.warm, time.Now()
			go func() {
				page, err := preloadLiked(lib.src)
				a.post(func() {
					defer done()
					if err != nil {
						log.Printf("fetching the favorite songs ahead: %v", err)
						return
					}
					if lib.warm != w || lib.likedPaged.loaded || lib.likedPaged.loading {
						return
					}
					a.setLiked(page, at)
					a.keepPages()
				})
			}()
		},
	}
}
