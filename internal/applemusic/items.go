package applemusic

import (
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"ringo/internal/music"
)

// What Apple's pages hold, as the app's shelves show it: each of Apple's
// resources as an Item, and the links of its editors as the Browse pages
// they open.

// item is r as a shelf shows it, or false for what the app can't show or
// open, such as TV episodes.
func (s *Source) item(b *body, r *resource) (music.Item, bool) {
	if r == nil {
		return music.Item{}, false
	}
	a := r.attrs()
	it := music.Item{
		ID:       r.ID,
		Title:    a.Name,
		ImageURL: art(a.Artwork),
		Colors:   colors(a.Artwork),
		Explicit: explicit(a),
		URL:      a.URL,
	}
	if t, err := time.Parse(time.RFC3339, a.DateAdded); err == nil {
		it.Added = t
	}
	switch r.Type {
	case "albums", "library-albums":
		album := s.toAlbum(r)
		it.Kind, it.ID, it.Album, it.Subtitle, it.ImageURL = music.ItemAlbum, album.ID, &album, a.ArtistName, album.ImageURL
		it.VideoURL = album.VideoURL
		it.TallVideoURL, it.TallURL = tallMotion(a, "")
	case "playlists", "library-playlists":
		p := s.toPlaylist(r)
		it.Kind, it.Playlist, it.Subtitle, it.ImageURL = music.ItemPlaylist, &p, a.CuratorName, p.ImageURL
		it.VideoURL = p.VideoURL
		it.Artists = a.ArtistNames
		it.TallURL, _ = wide(a, tallArt...)
		it.TallVideoURL, it.TallURL = tallMotion(a, it.TallURL)
	case "artists":
		artist := toArtist(r)
		it.Kind, it.Artist = music.ItemArtist, &artist
	case "library-artists":
		artist := toArtist(r)
		// What the library knows of them is their albums there.
		it.Kind, it.Artist, it.Page = music.ItemArtist, &artist, "library-artist:"+r.ID
		if cat := b.all(r.related("catalog").Data); len(cat) > 0 {
			it.ImageURL = art(cat[0].attrs().Artwork)
		}
	case "songs", "library-songs":
		t := s.toTrack(r)
		it.Kind, it.ID, it.Track, it.Subtitle = music.ItemSong, t.ID, &t, a.ArtistName
	case "music-videos", "library-music-videos":
		t := s.toTrack(r)
		t.Video = true
		it.Kind, it.ID, it.Track, it.Subtitle = music.ItemVideo, t.ID, &t, a.ArtistName
		it.WideURL = a.Artwork.wideURL()
	case "stations":
		st := toStation(r)
		if shows := b.all(r.related("radio-show").Data); len(shows) > 0 && st.Subtitle == "" {
			st.Subtitle = shows[0].attrs().Name
		}
		it.Kind, it.Station, it.Subtitle, it.WideURL = music.ItemStation, &st, st.Subtitle, st.WideURL
		it.TallURL, _ = wide(a, tallArt...)
		it.TallVideoURL, it.TallURL = tallMotion(a, it.TallURL)
	case "apple-curators", "curators":
		it.Kind, it.Page = music.ItemLink, "curator:"+r.ID
		it.Subtitle = a.ShowHostName
		it.WideURL, _ = wide(a, brickArt...)
	case "editorial-items":
		it.Kind = music.ItemLink
		it.Title = orT(a.Name, a.EditorialNotes.nameOf())
		if a.Link != nil {
			it.URL = a.Link.URL
			it.Page = linkPage(a.Link.URL)
		}
		if it.Page == "" {
			it.Page = linkPage(a.URL)
		}
		var w *artwork
		it.WideURL, w = wide(a, brickArt...)
		it.TallURL, _ = wide(a, tallArt...)
		it.TallVideoURL, it.TallURL = tallMotion(a, it.TallURL)
		if w != nil {
			it.Colors = colors(w)
		}
		if it.ImageURL == "" {
			it.ImageURL = art(w)
		}
	case "library-playlist-folders":
		it.Kind, it.Page = music.ItemLink, "library-folder:"+r.ID
	default:
		return music.Item{}, false
	}
	if it.Note == "" {
		it.Note = plain(a.PlainEditorialNotes.brief())
	}
	return it, true
}

// tallMotion is an item's picture three to four moving, if Apple has one,
// and the still under it: still, the one made for the card, else the
// video's first frame, which makes the card a tall one, as the web
// player's is. Without a video, still is as it was.
func tallMotion(a *attributes, still string) (string, string) {
	v, first := moving(a, tallMove...)
	if still == "" {
		still = first
	}
	return v, still
}

// items are those of refs the app can show.
func (s *Source) items(b *body, refs []*resource) []music.Item {
	out := []music.Item{}
	for _, r := range b.all(refs) {
		if it, ok := s.item(b, r); ok {
			out = append(out, it)
		}
	}
	return out
}

func (n *notes) nameOf() string {
	if n == nil {
		return ""
	}
	return n.Name
}

// wideURL is a picture wider than tall as its template, for Wide.
func (a *artwork) wideURL() string {
	if a == nil {
		return ""
	}
	return a.URL
}

// shelfStyle is how a row of items shows when Apple says no more than
// what's in it: songs in columns, videos and categories wide, artists
// round, and the rest as covers.
func shelfStyle(items []music.Item) string {
	if len(items) == 0 {
		return music.ShelfSquare
	}
	count := map[string]int{}
	for _, it := range items {
		count[it.Kind]++
	}
	switch len(items) {
	case count[music.ItemSong]:
		return music.ShelfTracks
	case count[music.ItemVideo], count[music.ItemLink]:
		return music.ShelfBrick
	case count[music.ItemArtist]:
		return music.ShelfCircle
	}
	return music.ShelfSquare
}

// gridStyle is shelfStyle for all of something, on a page of its own: songs
// a row each, the rest in rows that wrap.
func gridStyle(items []music.Item) string {
	if shelfStyle(items) == music.ShelfTracks {
		return music.ShelfList
	}
	return music.ShelfGrid
}

// songItems are the songs of items, which the app plays on from the one
// picked on.
func songItems(items []music.Item) []music.Track {
	var out []music.Track
	for _, it := range items {
		if it.Track != nil && it.Kind == music.ItemSong {
			out = append(out, *it.Track)
		}
	}
	return out
}

var storeID = regexp.MustCompile(`/(?:room|multi-room|curator|apple-curator|grouping)/(?:[^/?#]+/)?(\d+)`)

// linkPage is the Browse page an editor's link opens, by the store's
// addresses Apple uses in them, or "" for one that's no page of ours.
func linkPage(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return ""
	}
	q := u.Query()
	switch path := u.Path; {
	case strings.HasSuffix(path, "/viewGrouping") && q.Get("id") != "":
		return "grouping:" + q.Get("id")
	case strings.HasSuffix(path, "/viewRoom") && q.Get("fcId") != "":
		return "room:" + q.Get("fcId")
	case strings.HasSuffix(path, "/viewMultiRoom") && q.Get("fcId") != "":
		return "multiroom:" + q.Get("fcId")
	case strings.HasSuffix(path, "/viewTop"):
		// 34 is all of music; another genre has charts of its own.
		if g := q.Get("genreId"); g != "" && g != "34" {
			return "charts:" + g
		}
		return "charts"
	case strings.HasSuffix(path, "/viewCurator") && q.Get("id") != "":
		return "curator:" + q.Get("id")
	case q.Get("fcId") != "":
		return "multiroom:" + q.Get("fcId")
	}
	if m := storeID.FindStringSubmatch(u.Path); m != nil {
		switch {
		case strings.Contains(u.Path, "/multi-room/"):
			return "multiroom:" + m[1]
		case strings.Contains(u.Path, "/room/"):
			return "room:" + m[1]
		case strings.Contains(u.Path, "/grouping/"):
			return "grouping:" + m[1]
		}
		return "curator:" + m[1]
	}
	return ""
}

var unknownOnce sync.Map

// unknown notes once a run what of Apple's the source doesn't show, for
// whoever reads the log to add it.
func unknown(what, value string) {
	if _, seen := unknownOnce.LoadOrStore(what+":"+value, true); !seen {
		log.Printf("Apple Music: leaving out %s %q", what, value)
	}
}
