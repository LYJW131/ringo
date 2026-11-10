package applemusic

import (
	"strings"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// The listener's own pages: Home's recommendations, and their library, a
// page at a time, as the web player lists it.

// listenNowParams are the web player's for Home, of a listener who isn't
// on the plan with voice only.
func listenNowParams() params {
	return params{
		"name":                             "listen-now",
		"extend[playlists]":                "artistNames,editorialArtwork",
		"extend[stations]":                 "airTime,supportsAirTimeUpdates,editorialArtwork",
		"meta[stations]":                   "inflectionPoints",
		"include[albums]":                  "artists",
		"include[library-playlists]":       "catalog",
		"include[stations]":                "radio-show",
		"include[personal-recommendation]": "primary-content",
		"fields[artists]":                  "name,artwork,url",
		"displayFilter[kind]":              "MusicCircleCoverShelf,MusicConcertsEmptyShelf,MusicCoverShelf,MusicCoverGrid,MusicNotesHeroShelf,MusicSuperHeroShelf,MusicSocialCardShelf",
		"types":                            "activities,albums,apple-curators,artists,curators,editorial-items,library-albums,library-playlists,music-movies,music-videos,playlists,stations,uploaded-audios,uploaded-videos",
		"art[url]":                         "f",
		"omit[resource]":                   "autos",
	}
}

// listenNow is Home signed in: Apple's recommendations for the listener.
func (s *Source) listenNow() (*music.Browse, error) {
	p := listenNowParams()
	p["timezone"] = s.timezone()
	b, err := s.me("/recommendations", s.mapped(p), reqOpts{})
	if err != nil {
		return nil, err
	}
	out := &music.Browse{Title: i18n.T("Home"), Sections: s.recommendations(b)}
	// The app loads Home again by itself, every hour, songs playing or not:
	// what's on it is noted, and worked out only as it plays, so that it
	// doesn't wake the wrapper.
	for _, sec := range out.Sections {
		s.shown(songItems(sec.Items))
	}
	return out, nil
}

// timezone is the listener's offset from UTC, as +08:00, without which
// Apple doesn't answer recommendations: they change at midnight there.
func (s *Source) timezone() string { return s.clk.Now().Local().Format("-07:00") }

// seeAll is all of one of Home's shelves.
func (s *Source) seeAll(id string) (*music.Browse, error) {
	p := params{
		"action":                           "refresh",
		"name":                             "listen-now",
		"extend":                           "artistUrl",
		"include[personal-recommendation]": "contents",
		"include[library-playlists]":       "catalog",
		"limit[contents]":                  "100",
		"types":                            listenNowParams()["types"],
		"timezone":                         s.timezone(),
	}
	b, err := s.me("/recommendations/"+id+"/view/see-all", s.mapped(p), reqOpts{})
	if err != nil {
		return nil, err
	}
	out := &music.Browse{}
	var refs []*resource
	for _, r := range b.all(b.Data) {
		if r.Type == "personal-recommendation" {
			out.Title = string(r.attrs().Title)
			refs = append(refs, r.related("contents").Data...)
		} else {
			refs = append(refs, r)
		}
	}
	items := s.items(b, refs)
	out.Sections = []music.Section{{Style: gridStyle(items), Items: items}}
	s.listed(songItems(items), nil)
	return out, nil
}

// A library page: where it is, how it's asked for, and how it shows.
type libraryList struct {
	path   string
	params params
	title  func() string
	style  string
	// empty is what the page says with nothing in it.
	empty func() string
}

// What the library's pages say with nothing in them, in Apple Music's words.
func emptyLibrary() string { return i18n.T("There are no items in your library.") }
func emptyMixes() string   { return i18n.T("Personal mixes you add will appear here.") }

var albumFields = "artistName,artistUrl,artwork,contentRating,editorialArtwork,name,playParams,releaseDate,url,trackCount"

var libraryLists = map[string]libraryList{
	music.PageRecentlyAdded: {"/library/recently-added", params{
		"limit": "25", "includeOnly": "catalog,artists", "include[library-albums]": "artists", "include[library-artists]": "catalog",
		"fields[artists]": "name,url", "fields[albums]": albumFields, "omit[resource]": "autos",
	}, func() string { return i18n.T("Recently Added") }, music.ShelfGrid, emptyLibrary},
	music.PageLibraryAlbums: {"/library/albums", params{
		"limit": "100", "includeOnly": "catalog,artists", "include[library-albums]": "artists", "include[albums]": "artists",
		"fields[artists]": "name,url", "fields[albums]": albumFields,
	}, func() string { return i18n.T("Albums") }, music.ShelfGrid, emptyLibrary},
	music.PageLibraryArtist: {"/library/artists", params{"limit": "100", "include": "catalog"},
		func() string { return i18n.TC("Artists", "Apple Music") }, music.ShelfGrid, emptyLibrary},
	music.PageLibrarySongs: {"/library/songs", params{"limit": "100", "include[library-songs]": "catalog"},
		func() string { return i18n.T("Songs") }, music.ShelfList, emptyLibrary},
	music.PageMadeForYou: {"/library/playlists", params{
		"limit": "100", "filter[featured]": "made-for-you", "include[library-playlists]": "catalog",
		"fields[library-playlists]": "artwork,dateAdded,name,playParams",
	}, func() string { return i18n.T("Made for You") }, music.ShelfGrid, emptyMixes},
	music.PageAllPlaylists: {"/library/playlist-folders/p.playlistsroot/children", params{
		"limit": "100", "include": "catalog", "extend": "hasCollaboration", "fields[playlists]": "curatorName", "omit[resource]": "autos",
	}, func() string { return i18n.T("Playlists") }, music.ShelfGrid, emptyLibrary},
}

// libraryPage is the first part of a library page, or, with next, the part
// that goes on there.
func (s *Source) libraryPage(page, next string) (*music.Browse, error) {
	if err := s.needAccount(); err != nil {
		return nil, err
	}
	full := page
	page, view := music.ParseLibraryPage(page)
	list, ok := libraryLists[page]
	if folder, isFolder := strings.CutPrefix(page, "library-folder:"); isFolder {
		list, ok = libraryList{"/library/playlist-folders/" + folder + "/children", params{"limit": "100"}, func() string { return "" }, music.ShelfGrid, func() string { return "" }}, true
	}
	if !ok {
		return nil, fail("unavailable", i18n.T("No such page"))
	}
	p := s.mapped(list.params)
	if full != page {
		if !sortable[page] {
			return nil, fail("unavailable", i18n.T("No such page"))
		}
		viewParams(p, view)
		if view.Term != "" {
			return s.libraryFound(full, list, p, view)
		}
	}
	var b *body
	var err error
	if next != "" {
		b, err = s.follow(next, p)
	} else {
		b, err = s.me(list.path, p, reqOpts{})
	}
	if err != nil {
		return nil, err
	}
	items := s.editedItems(s.items(b, b.Data))
	out := &music.Browse{Sections: []music.Section{{Style: list.style, Items: items}}}
	if next == "" {
		out.Title = list.title()
		if !view.Favorites {
			out.Sections[0].Empty = list.empty()
		}
	}
	if b.Next != "" {
		out.Next = full + "|" + b.Next
	}
	s.listed(songItems(items), nil)
	return out, nil
}

// libraryArtist is an artist of the library: their albums there.
func (s *Source) libraryArtist(id, next string) (*music.Browse, error) {
	p := s.mapped(params{
		"include[library-artists]": "albums,catalog",
		"include[library-albums]":  "catalog",
		"fields[library-artists]":  "name,url",
		"omit[resource]":           "autos",
	})
	if next != "" {
		return s.more("library-artist:"+id, next, p)
	}
	b, err := s.me("/library/artists/"+id, p, reqOpts{})
	if err != nil {
		return nil, err
	}
	a, err := first(b)
	if err != nil {
		return nil, err
	}
	if a = b.get(a); a == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	albums := a.related("albums")
	items := s.items(b, albums.Data)
	out := &music.Browse{Title: a.attrs().Name, Sections: []music.Section{{Style: music.ShelfGrid, Items: items}}}
	if cat := b.all(a.related("catalog").Data); len(cat) > 0 {
		out.ImageURL = art(cat[0].attrs().Artwork)
	}
	if albums.Next != "" {
		out.Next = "library-artist:" + id + "|" + albums.Next
	}
	return out, nil
}

// recentlyAdded is what the listener added lately, kept a while: the first
// part of its page, which the source fetches ahead.
func (s *Source) recentlyAdded(ahead bool) (*music.Browse, error) {
	if err := s.needAccount(); err != nil {
		return nil, err
	}
	return kept(s, s.pageKey(music.PageRecentlyAdded), freshness{maxAge: listKept}, func(func()) (*music.Browse, error) {
		return s.libraryPage(music.PageRecentlyAdded, "")
	}, ahead)
}
