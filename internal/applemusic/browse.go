package applemusic

import (
	"strings"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Home, and the source's own pages, by their names:
//
//	new, radio                        the editors' New and Radio (editorial.go)
//	grouping:<id>                     a genre's or a category's page
//	room:<id>                         all of an editors' row (See All)
//	multiroom:<id>                    a feature
//	curator:<id>                      a curator's or a show's page
//	charts, charts:<genre>            the charts, of all music or a genre
//	chart|<path>                      all of one chart (lists.go)
//	search                            what Search shows before a word is typed
//	search:<term>                     all a search finds
//	search-more:<group>:<term>        all it found of one kind
//	artist-view:<id>:<view>           all of a shelf of an artist's page
//	rec:<id>                          all of a shelf of Home
//	library:…                         the library's pages (libpages.go)
//	library-artist:<id>               an artist of the library
//	library-folder:<id>               a folder of playlists
//
// A page that comes in parts goes on at "<page>|<next>", where next is
// Apple's own address of the part after, as it gave it.

// Older names of pages, as the plugin's sidebar has them.
var pageAliases = map[string]string{"recentlyAdded": music.PageRecentlyAdded}

func orT(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

// home is Apple's recommendations for the listener, or New for one signed out.
func (s *Source) home() (*music.Browse, error) {
	s.warm()
	if s.signedIn() {
		return s.listenNow()
	}
	return s.browse(music.PageNew)
}

// pageKey is what a page is kept under: by the storefront and the language
// it was asked in, so that it isn't shown in another once either changes.
// The library's pages start with their own names, which a change to the
// library forgets.
func (s *Source) pageKey(page string) string {
	sf, _ := s.storefront()
	key := page
	if rest, ok := strings.CutPrefix(page, music.PageRecentlyAdded); ok {
		key = "recentlyAdded" + rest // as liking a song forgets it
	} else if !strings.HasPrefix(page, "library") {
		key = "page:" + page
	}
	return key + "#" + sf + "#" + s.language()
}

func (s *Source) browse(page string) (*music.Browse, error) {
	s.warm()
	return s.browseAt(page)
}

// browseAt is browse without the wrapper being woken (s.warm), for what the
// app asks for before the listener does.
func (s *Source) browseAt(page string) (*music.Browse, error) {
	if alias, ok := pageAliases[page]; ok {
		page = alias
	}
	if page == music.PageRecentlyAdded {
		return s.recentlyAdded(false)
	}
	if strings.HasPrefix(page, "library") || strings.HasPrefix(page, "rec:") {
		if err := s.needAccount(); err != nil {
			return nil, err
		}
	}
	// What the listener keeps in their library changes where they can't be
	// told of it, so a page of it is asked for again sooner than the editors'.
	maxAge := listKept
	if strings.HasPrefix(page, "library") {
		maxAge = libraryKept
	}
	return kept(s, s.pageKey(page), freshness{maxAge: maxAge}, func(func()) (*music.Browse, error) {
		return s.makePage(page)
	}, false)
}

// makePage asks Apple for page.
func (s *Source) makePage(page string) (*music.Browse, error) {
	page, next := page, ""
	if i := strings.Index(page, "|/"); i >= 0 {
		page, next = page[:i], page[i+1:]
	}
	kind, arg, _ := strings.Cut(page, ":")
	switch {
	case page == music.PageNew:
		b, err := s.groupingPage("music", "")
		if err == nil {
			b.Title = i18n.TC("New", "Apple Music")
		}
		return b, err
	case page == music.PageRadio:
		b, err := s.groupingPage("radio", "")
		if err == nil {
			b.Title = i18n.TC("Radio", "Apple Music")
		}
		return b, err
	case page == music.PageSearchLanding:
		return s.searchLanding()
	case strings.HasPrefix(page, music.PageSearch):
		return s.searchPage(strings.TrimPrefix(page, music.PageSearch))
	case kind == "search-more":
		group, term, _ := strings.Cut(arg, ":")
		return s.searchMore(group, term, next)
	case page == "charts":
		return s.charts("")
	case kind == "charts":
		return s.charts(arg)
	case kind == "chart" || page == "chart":
		return s.chart(next)
	case kind == "grouping":
		return s.groupingPage("", arg)
	case kind == "room":
		if next != "" {
			return s.more(page, next, roomParams())
		}
		return s.room(arg)
	case kind == "multiroom":
		return s.multiroom(arg)
	case kind == "curator":
		return s.curator(arg)
	case kind == "artist-view":
		id, view, _ := strings.Cut(arg, ":")
		return s.artistView(id, view, next)
	case kind == "rec":
		return s.seeAll(arg)
	case kind == "library-artist":
		return s.libraryArtist(arg, next)
	case strings.HasPrefix(page, "library"):
		return s.libraryPage(page, next)
	}
	return nil, fail("unavailable", i18n.T("No such page"))
}
