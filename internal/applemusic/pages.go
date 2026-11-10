package applemusic

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
	"sync"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Albums, artists and playlists, from the catalog or the library. Library
// IDs start with l. (albums) and p. (playlists); an artist the app only knows
// by name has the ID name:<name>, and one known as the artist of a song or an
// album, of:<songs|albums>:<id>:<name>. A catalog page is asked for as the web
// player asks, with the shelves that follow it (views) in the language asked
// for.

// withLibrary fetches a catalog resource and, at the same time, its library
// version, if it's in the listener's library, and whether it's a favorite.
func (s *Source) withLibrary(typ, id string, p params) (*body, *resource, bool, error) {
	var (
		wg       sync.WaitGroup
		own      *resource
		favorite bool
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		own, favorite, _ = s.personal(typ, id)
	}()
	b, err := s.catalog("/"+typ+"/"+id, p)
	wg.Wait()
	return b, own, favorite, err
}

// The shelves under an album, in the web player's order, and how each shows.
var albumViews = []struct{ name, style string }{
	{"more-by-artist", music.ShelfSquare},
	{"other-versions", music.ShelfSquare},
	{"appears-on", music.ShelfSquare},
	{"related-videos", music.ShelfBrick},
	{"you-might-also-like", music.ShelfSquare},
	{"audio-extras", music.ShelfSquare},
	{"video-extras", music.ShelfBrick},
}

func albumParams() params {
	p := params{
		"extend":                "editorialArtwork,editorialVideo,extendedAssetUrls",
		"art[url]":              "f",
		"include":               "tracks,artists,record-labels",
		"meta[albums:tracks]":   "popularity",
		"include[songs]":        "artists,composers",
		"include[music-videos]": "artists",
		"fields[artists]":       "name,url",
		"fields[record-labels]": "name,url",
		"views":                 "",
		"platform":              "web",
		"format[resources]":     "map",
	}
	var views []string
	for _, v := range albumViews {
		views = append(views, v.name)
	}
	p["views"] = strings.Join(views, ",")
	return p
}

// A song is starred on its album's page when Apple says it's played much
// more than the rest. Popularity is from 0 to 1, the album's most played at
// 1: on 1989 (US, 2026-11) the hits are at 0.86 to 1, two more at 0.7, and
// the rest at 0.15 to 0.42. The star goes to those at 0.7 or more and within
// 0.85 of the most played, so the clear hits only, and not every song of an
// album whose songs are all played alike.
const (
	popular      = 0.7
	nearMostPlay = 0.85
)

func markPopular(refs []*resource, songs []music.Track) {
	most := 0.0
	pop := map[string]float64{}
	for _, r := range refs {
		if r != nil && r.Meta != nil {
			pop[r.ID] = r.Meta.Popularity
			most = max(most, r.Meta.Popularity)
		}
	}
	for i := range songs {
		p := pop[songs[i].ID]
		songs[i].Popular = p >= popular && p >= most*nearMostPlay
	}
}

func (s *Source) album(id string) (*music.AlbumPage, error) {
	s.warm()
	if strings.HasPrefix(id, "l.") {
		b, err := s.me("/library/albums/"+id, params{"include": "catalog,tracks"}, reqOpts{})
		if err != nil {
			return nil, err
		}
		item, err := first(b)
		if err != nil {
			return nil, err
		}
		// Under a library album, Apple's app shows its catalog album's
		// shelves, and Show Complete Album when the library has only some
		// of its songs: that album is asked for while the songs are.
		var (
			wg         sync.WaitGroup
			whole      *body
			wholeAlbum *resource
		)
		wg.Add(1)
		go func() {
			defer wg.Done()
			whole, wholeAlbum = s.catalogAlbum(b, item)
		}()
		tracks, err := s.allPages("/v1/me/library/albums/"+id+"/tracks", params{"limit": "100"}, 1000, nil)
		wg.Wait()
		if err != nil {
			return nil, err
		}
		songs := s.songsOf(tracks)
		s.prefetch(songs, nil)
		a := s.toAlbum(item)
		a.ID, a.Saved = id, true
		for _, song := range songs {
			a.Uploaded = a.Uploaded || song.Uploaded
		}
		if wholeAlbum != nil {
			// The library's album may not say when it came out or how it
			// sounds; the catalog's does, and Music's page shows it.
			w := s.toAlbum(wholeAlbum)
			a.Released = cmp.Or(a.Released, w.Released)
			if a.Traits == (music.Traits{}) {
				a.Traits = w.Traits
			}
		}
		page := &music.AlbumPage{Album: a, Tracks: songs}
		if wholeAlbum != nil {
			page.Sections = s.albumSections(whole, wholeAlbum)
			if wholeAlbum.attrs().TrackCount > len(tracks) {
				page.Complete = wholeAlbum.ID
			}
		}
		return page, nil
	}
	b, own, favorite, err := s.withLibrary("albums", id, albumParams())
	if err != nil {
		return nil, err
	}
	item, err := first(b)
	if err != nil {
		return nil, err
	}
	if item = b.get(item); item == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	refs := item.related("tracks").Data
	tracks := b.all(refs)
	if item.related("tracks").Next != "" {
		path, err := s.catalogPath("/albums/" + id + "/tracks")
		if err != nil {
			return nil, err
		}
		if tracks, err = s.allPages(path, params{"limit": "300"}, 1000, nil); err != nil {
			return nil, err
		}
	}
	songs := s.songsOf(tracks)
	markPopular(refs, songs)
	s.prefetch(songs, nil)
	a := s.toAlbum(item)
	a.Saved, a.Favorite = own != nil, favorite
	// Music names the label as the label's page does, not as the album's
	// line of who released it may: of Fantasy, in China, that line says
	// JVR MUSIC INTERNATIONAL LTD., and Music the label's Chinese name.
	if labels := b.all(item.related("record-labels").Data); len(labels) > 0 {
		a.Label, a.LabelPage = cmp.Or(labels[0].attrs().Name, a.Label), "label:"+labels[0].ID
	}
	return &music.AlbumPage{Album: a, Tracks: songs, Sections: s.albumSections(b, item)}, nil
}

// albumSections are the shelves that follow a catalog album's songs.
func (s *Source) albumSections(b *body, item *resource) []music.Section {
	return s.pageSections(b, "albums", item, albumViews)
}

// pageSections are the shelves of a catalog page of typ that its views
// give, in order, each with See All when Apple has more of it than it gave,
// as Music's have.
func (s *Source) pageSections(b *body, typ string, item *resource, views []struct{ name, style string }) []music.Section {
	var out []music.Section
	for _, v := range views {
		view := item.Views[v.name]
		if sec, ok := s.viewSection(b, view, v.style); ok {
			if view.Next != "" {
				sec.MorePage = "view:" + typ + ":" + item.ID + ":" + v.name
			}
			out = append(out, sec)
		}
	}
	return out
}

// viewPage is all of one of the shelves of a catalog page of typ: the page
// asked for with that view alone, which has its title, then the view's
// next parts.
func (s *Source) viewPage(typ, id, view, next string) (*music.Browse, error) {
	base := params{"art[url]": "f", "include[albums]": "artists", "include[songs]": "artists", "fields[artists]": "name,url"}
	prefix := "view:" + typ + ":" + id + ":" + view
	if next != "" {
		return s.more(prefix, next, base)
	}
	p := s.mapped(withSongAssets(base))
	p["views"], p["limit[view."+view+"]"] = view, "30"
	b, err := s.catalog("/"+typ+"/"+id, p)
	if err != nil {
		return nil, err
	}
	item, err := first(b)
	if err != nil {
		return nil, err
	}
	if item = b.get(item); item == nil || item.Views[view] == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	v := item.Views[view]
	items := s.items(b, v.Data)
	out := &music.Browse{Title: v.title(), Sections: []music.Section{{Style: gridStyle(items), Items: items}}}
	if v.Next != "" {
		out.Next = prefix + "|" + v.Next
	}
	s.listed(songItems(items), nil)
	return out, nil
}

// The shelves of a record label's page, in the order Music's has them.
var labelViews = []struct{ name, style string }{
	{"top-releases", music.ShelfSquare},
	{"latest-releases", music.ShelfSquare},
}

// recordLabel is a record label's page: its banner, its name and what it
// says of itself, and its releases.
func (s *Source) recordLabel(id string) (*music.Browse, error) {
	var views []string
	for _, v := range labelViews {
		views = append(views, v.name)
	}
	b, err := s.catalog("/record-labels/"+id, s.mapped(params{"views": strings.Join(views, ","), "extend": "editorialArtwork", "art[url]": "f"}))
	if err != nil {
		return nil, err
	}
	item, err := first(b)
	if err != nil {
		return nil, err
	}
	if item = b.get(item); item == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	a := item.attrs()
	out := &music.Browse{Title: a.Name, Description: plain(a.Description.long())}
	if u, w := wide(a, "bannerUber"); u != "" {
		out.WideURL, out.Colors = u, colors(w)
	} else {
		out.ImageURL = art(a.Artwork)
	}
	out.Sections = s.pageSections(b, "record-labels", item, labelViews)
	return out, nil
}

// catalogAlbum is the catalog album a library album is of, asked for with
// its shelves, or nil when it can't be told. The library album says which
// only sometimes (of Ao No Hako's second soundtrack, in 2026-11, it
// doesn't); its songs always say theirs, and theirs is its.
func (s *Source) catalogAlbum(b *body, lib *resource) (*body, *resource) {
	id := ""
	if d := lib.related("catalog").Data; len(d) > 0 && d[0] != nil {
		id = d[0].ID
	}
	if id == "" {
		for _, song := range b.all(lib.related("tracks").Data) {
			if sid := songID(song); isCatalogID(sid) {
				sb, err := s.catalog("/songs/"+sid, params{"include": "albums"})
				if err != nil {
					return nil, nil
				}
				if r, err := first(sb); err == nil {
					id = albumIDOf(r)
				}
				break
			}
		}
	}
	if !isCatalogID(id) {
		return nil, nil
	}
	cb, err := s.catalog("/albums/"+id, albumParams())
	if err != nil {
		return nil, nil
	}
	r, err := first(cb)
	if err != nil {
		return nil, nil
	}
	if r = cb.get(r); r == nil {
		return nil, nil
	}
	return cb, r
}

// viewSection is a view of a page as a shelf, titled as Apple titles it.
func (s *Source) viewSection(b *body, v *relationship, style string) (music.Section, bool) {
	if v == nil {
		return music.Section{}, false
	}
	items := s.items(b, v.Data)
	if len(items) == 0 {
		return music.Section{}, false
	}
	if shelfStyle(items) == music.ShelfTracks {
		style = music.ShelfTracks
	}
	return music.Section{Title: v.title(), Style: style, Items: items}, true
}

func artistParams() params {
	return params{
		"views":                         "canvas,top-songs",
		"extend":                        "artistBio,bornOrFormed,editorialArtwork,editorialVideo,extendedAssetUrls,hasBiographyContent,hero,isGroup,keyColor,origin,plainEditorialNotes",
		"art[url]":                      "c,f",
		"include":                       "default-playable-content",
		"include[canvas-shelves]":       "contents,children",
		"include[songs]":                "albums",
		"extend[canvas-shelves-header]": "badge",
		"filter[canvas-shelves-collection:contents{type}]":        "albums,apple-curators,artists,music-movies,music-videos,playlists,songs,stations,uploaded-videos",
		"limit[canvas-shelves-collection:contents]":               "12",
		"limit[canvas-shelves-collection:contents{displayStyle}]": "artist-featured-track-lockup:24",
		"relate[canvas-shelves-collection]":                       "see-all",
		"omit[resource]":                                          "autos",
		"platform":                                                "web",
		"format[resources]":                                       "map",
	}
}

// canvasStyle is how a shelf of an artist's page shows, by its
// display.style; "" leaves it out, as concerts, which the app doesn't have.
var canvasStyle = map[string]string{
	"artist-featured-content":                  music.ShelfFeatured,
	"artist-featured-track-lockup":             music.ShelfTracks,
	"square-lockup":                            music.ShelfSquare,
	"square-lockup-medium":                     music.ShelfSquare,
	"square-lockup-large":                      music.ShelfSquare,
	"adaptive-horizontal-square-lockup-medium": music.ShelfSquare,
	"brick-lockup":                             music.ShelfBrick,
	"brick-lockup-large":                       music.ShelfBrick,
	"circle-lockup":                            music.ShelfCircle,
	// More to hear: playlists and stations as small covers in rows; songs
	// among them show as songs.
	"horizontal-lockup":     music.ShelfSquare,
	"calendar-event-lockup": "",
	"artist-detail-header":  "",
}

// named is the artist among found whose name is name, to the letter if one
// is, else whatever the case, else the first: a search for LiSA finds LISA
// too, and may list her first.
func named(found []*resource, name string) *resource {
	var folded, first *resource
	for _, r := range found {
		if r == nil {
			continue
		}
		if first == nil {
			first = r
		}
		switch got := r.attrs().Name; {
		case got == name:
			return r
		case folded == nil && strings.EqualFold(got, name):
			folded = r
		}
	}
	if folded != nil {
		return folded
	}
	return first
}

func (s *Source) artist(id string) (*music.ArtistPage, error) {
	s.warm()
	if typ, of, name, ok := artistOfParts(id); ok {
		// The artists of the song or album itself; by name only if the
		// catalog doesn't say.
		id = "name:" + name
		if b, err := s.catalog("/"+typ+"/"+of+"/artists", params{"limit": "10"}); err == nil {
			if found := named(b.all(b.Data), name); found != nil {
				id = found.ID
			}
		}
	}
	if name, ok := strings.CutPrefix(id, "name:"); ok {
		var b struct {
			Results struct {
				Artists relationship `json:"artists"`
			} `json:"results"`
		}
		if err := s.catalogInto("/search", params{"term": name, "types": "artists", "limit": "10"}, &b); err != nil {
			return nil, err
		}
		found := named(b.Results.Artists.Data, name)
		if found == nil {
			return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
		}
		id = found.ID
	}
	b, _, favorite, err := s.withLibrary("artists", id, artistParams())
	if err != nil {
		return nil, err
	}
	item, err := first(b)
	if err != nil {
		return nil, err
	}
	if item = b.get(item); item == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	view := func(name string) []*resource {
		if v := item.Views[name]; v != nil {
			return b.all(v.Data)
		}
		return nil
	}
	page := &music.ArtistPage{Artist: toArtist(item)}
	page.Artist.Favorite = favorite
	var canvasTop []*resource
	if canvas := item.Views["canvas"]; canvas != nil {
		page.Sections, canvasTop = s.canvas(b, id, canvas.Data)
	}
	top := view("top-songs")
	if len(top) == 0 {
		top = canvasTop
	}
	page.Tracks = s.songsOf(top)
	s.prefetch(page.Tracks, nil)
	for _, sec := range page.Sections {
		for _, it := range sec.Items {
			if it.Album != nil && sec.Style != music.ShelfFeatured {
				page.Albums = append(page.Albums, *it.Album)
			}
		}
	}
	if a := page.Artist; a.Bio != "" {
		page.Sections = append(page.Sections, music.Section{Title: fmt.Sprintf(i18n.TC("About %s", "Apple Music"), a.Name), Style: music.ShelfText, Subtitle: a.Bio})
	}
	return page, nil
}

// canvas is an artist's page as Apple lays it out, from the shelves refs
// point at, and the songs of its Top Songs.
func (s *Source) canvas(b *body, artist string, refs []*resource) ([]music.Section, []*resource) {
	var out []music.Section
	var top []*resource
	for _, shelf := range b.all(refs) {
		a := shelf.attrs()
		if shelf.Type == "canvas-shelves-group" || len(shelf.related("children").Data) > 0 {
			secs, t := s.canvas(b, artist, shelf.related("children").Data)
			out = append(out, secs...)
			if top == nil {
				top = t
			}
			continue
		}
		style := ""
		if a.Display != nil {
			style = a.Display.Style
		}
		layout, known := canvasStyle[style]
		if !known {
			unknown("artist shelf", style)
			continue
		}
		if layout == "" {
			continue
		}
		contents := shelf.related("contents")
		if a.Kind == "TOP_SONGS" {
			top = b.all(contents.Data)
		}
		items := s.items(b, contents.Data)
		if len(items) == 0 {
			continue
		}
		if layout == music.ShelfSquare && shelfStyle(items) == music.ShelfTracks {
			layout = music.ShelfTracks
		}
		sec := music.Section{Title: string(a.Title), Style: layout, Items: items}
		if v := seeAllView(shelf); v != "" {
			sec.MorePage = "artist-view:" + artist + ":" + v
		}
		out = append(out, sec)
	}
	return out, top
}

// artistViews are the views that have all of an artist's shelf, by the
// shelf's marker: the shelves' See All goes to the shelf itself, which the
// catalog's views give in full and in parts (checked against the US
// storefront: a radio show's shelf has none).
var artistViews = map[string]string{
	"TopSongs":          "top-songs",
	"Albums":            "full-albums",
	"ArtistSingles":     "singles",
	"LiveAlbums":        "live-albums",
	"CompilationAlbums": "compilation-albums",
	"AppearsOn":         "appears-on-albums",
	"Essentials":        "featured-albums",
	"ArtistPlaylists":   "playlists",
	"MusicVideos":       "top-music-videos",
	"SimilarArtists":    "similar-artists",
	"MoreToHear":        "more-to-hear",
	"MoreToSee":         "more-to-see",
}

// seeAllView is the view with all of an artist's shelf, or "".
func seeAllView(shelf *resource) string {
	_, marker, ok := strings.Cut(shelf.ID, ".marker.")
	if !ok {
		return ""
	}
	return artistViews[marker]
}

// artistView is all of one of an artist's shelves, or, with next, its next part.
func (s *Source) artistView(id, view, next string) (*music.Browse, error) {
	p := params{
		"include[songs]":        "artists,composers",
		"include[albums]":       "artists,composers",
		"include[music-videos]": "artists,composers",
		"limit":                 "20",
		"with":                  "attributes",
		"art[url]":              "f",
	}
	prefix := "artist-view:" + id + ":" + view
	if next != "" {
		return s.more(prefix, next, p)
	}
	full, err := s.catalogPath("/artists/" + id + "/view/" + view)
	if err != nil {
		return nil, err
	}
	var b struct {
		body
		Attributes *struct {
			Title string `json:"title"`
		} `json:"attributes"`
	}
	if err := s.requestJSON(s.amp+full+query(s.mapped(withSongAssets(p))), reqOpts{}, &b); err != nil {
		return nil, err
	}
	items := s.items(&b.body, b.Data)
	out := &music.Browse{Sections: []music.Section{{Style: gridStyle(items), Items: items}}}
	if b.Attributes != nil {
		out.Title = b.Attributes.Title
	}
	if b.Next != "" {
		out.Next = prefix + "|" + b.Next
	}
	s.listed(songItems(items), nil)
	return out, nil
}

// libraryPlaylist is one of the listener's playlists, kept until it changes.
func (s *Source) libraryPlaylist(id string, ahead bool) (*music.Playlist, error) {
	return kept(s, "playlist:"+id, s.playlistFreshness(id), func(pause func()) (*music.Playlist, error) {
		b, err := s.me("/library/playlists/"+id, nil, reqOpts{})
		if err != nil {
			return nil, err
		}
		pause()
		tracks, err := s.allPages("/v1/me/library/playlists/"+id+"/tracks", params{"limit": "100"}, 5000, pause)
		if errors.Is(err, music.ErrUnavailable) {
			tracks, err = nil, nil // an empty playlist has no tracks to find
		}
		if err != nil {
			return nil, err
		}
		item, err := first(b)
		if err != nil {
			return nil, err
		}
		songs := s.songsOf(tracks)
		p := s.toPlaylist(item)
		p.TrackCount, p.Tracks = len(songs), songs
		return &p, nil
	}, ahead)
}

func playlistParams() params {
	return params{
		"extend":                       "editorialArtwork,editorialVideo,trackCount",
		"art[url]":                     "f",
		"include":                      "tracks,curator",
		"include[songs]":               "artists",
		"include[music-videos]":        "artists",
		"fields[artists]":              "name,artwork,url",
		"fields[curators]":             "name,url",
		"fields[apple-curators]":       "name,url",
		"limit[tracks]":                "300",
		"views":                        "featured-artists,more-by-curator",
		"limit[view.more-by-curator]":  "15",
		"limit[view.featured-artists]": "15",
		"omit[resource]":               "autos",
		"platform":                     "web",
		"format[resources]":            "map",
	}
}

func (s *Source) playlist(id string) (*music.Playlist, error) {
	s.warm()
	if strings.HasPrefix(id, "p.") {
		found, err := s.libraryPlaylist(id, false)
		if err != nil {
			return nil, err
		}
		// As the listener last changed it, which Apple may not answer with yet.
		if e := s.edited(id); e != nil {
			if e.deleted {
				return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
			}
			edited := *found
			if e.name != "" {
				edited.Name = e.name
			}
			if e.tracks != nil {
				edited.Tracks = s.songsOf(e.tracks)
				edited.TrackCount = len(edited.Tracks)
			}
			found = &edited
		}
		s.prefetch(found.Tracks, nil)
		return found, nil
	}
	b, own, favorite, err := s.withLibrary("playlists", id, playlistParams())
	if err != nil {
		return nil, err
	}
	// The listener's own playlist by its catalog ID, as Apple's Recently
	// Played lists it, is opened as their library's: the app then offers to
	// edit it, and not to unsave it, which would delete it.
	if own != nil && strings.HasPrefix(id, "pl.u-") {
		if theirs, err := s.me("/library/playlists/"+own.ID, nil, reqOpts{}); err == nil && len(theirs.Data) > 0 && theirs.Data[0] != nil && theirs.Data[0].attrs().CanEdit {
			return s.playlist(own.ID)
		}
	}
	item, err := first(b)
	if err != nil {
		return nil, err
	}
	if item = b.get(item); item == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	tracks := b.all(item.related("tracks").Data)
	if item.related("tracks").Next != "" {
		path, err := s.catalogPath("/playlists/" + id + "/tracks")
		if err != nil {
			return nil, err
		}
		if tracks, err = s.allPages(path, params{"limit": "300"}, 5000, nil); err != nil {
			return nil, err
		}
	}
	songs := s.songsOf(tracks)
	s.prefetch(songs, nil)
	p := s.toPlaylist(item)
	p.Saved, p.Favorite, p.TrackCount, p.Tracks = own != nil, favorite, len(songs), songs
	if c := item.related("curator").Data; len(c) > 0 && c[0] != nil {
		p.OwnerPage = "curator:" + c[0].ID
		if cur := b.get(c[0]); cur != nil && p.Owner == "" {
			p.Owner = cur.attrs().Name
		}
	}
	for _, v := range []struct{ name, style string }{{"featured-artists", music.ShelfCircle}, {"more-by-curator", music.ShelfSquare}} {
		if sec, ok := s.viewSection(b, item.Views[v.name], v.style); ok {
			p.Sections = append(p.Sections, sec)
		}
	}
	return &p, nil
}
