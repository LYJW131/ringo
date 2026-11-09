package applemusic

import (
	"strings"

	"mygo-music/internal/music"
)

// Favorites, the library, and songs to go on with: written as the web
// player's script writes them (api.md §3). A song's favorite is its rating,
// as Like has it; the rest have favorites of their own.

// catalogTypes are Apple's names of the kinds of Item.
var catalogTypes = map[string]string{
	music.ItemSong:     "songs",
	music.ItemAlbum:    "albums",
	music.ItemPlaylist: "playlists",
	music.ItemArtist:   "artists",
	music.ItemVideo:    "music-videos",
	music.ItemStation:  "stations",
}

// libraryTypes are what a library ID's start says it is.
var libraryTypes = map[string]string{"i.": "library-songs", "l.": "library-albums", "p.": "library-playlists", "r.": "library-artists"}

// typeOf is Apple's type of id, of kind.
func typeOf(kind, id string) string {
	for prefix, typ := range libraryTypes {
		if strings.HasPrefix(id, prefix) {
			return typ
		}
	}
	return catalogTypes[kind]
}

func (s *Source) favorite(kind, id string, on bool) error {
	if kind == music.ItemSong {
		return s.like(id, on)
	}
	typ := typeOf(kind, id)
	if typ == "" || kind == music.ItemVideo || kind == music.ItemStation {
		return fail("unsupported", "")
	}
	method := "POST"
	if !on {
		method = "DELETE"
	}
	_, err := s.me("/favorites", params{"ids[" + typ + "]": id}, reqOpts{method: method})
	return err
}

// addToLibrary adds what id is to the library, or takes it out. An album or
// a playlist goes as SaveAlbum and SavePlaylist have it, which never take out
// a playlist the listener made.
func (s *Source) addToLibrary(kind, id string, on bool) error {
	switch kind {
	case music.ItemAlbum:
		return s.saveAlbum(id, on)
	case music.ItemPlaylist:
		return s.savePlaylist(id, on)
	case music.ItemSong, music.ItemVideo:
	default:
		return fail("unsupported", "")
	}
	defer s.forget("library")
	defer s.forget("recentlyAdded")
	typ := catalogTypes[kind]
	if on {
		if !isCatalogID(id) {
			return nil // a library song is in the library
		}
		_, err := s.me("/library", params{"ids[" + typ + "]": id, "representation": "ids"}, reqOpts{method: "POST"})
		return err
	}
	own := id
	if isCatalogID(id) {
		own = ""
		if r := s.inLibrary(typ, id); r != nil {
			own = r.ID
		}
	}
	if own == "" {
		return nil
	}
	// By the catalog's type name, as an album's and a playlist's go.
	_, err := s.me("/library/"+typ+"/"+own, nil, reqOpts{method: "DELETE"})
	return err
}

// autoplayMost is how many songs to go on with are asked for at a time.
const autoplayMost = "10"

// autoplay is songs to go on with after ids, from the station Apple makes
// of them, as the web player's autoplay does.
func (s *Source) autoplay(ids []string) ([]music.Track, error) {
	var data []map[string]string
	for _, id := range ids {
		if isCatalogID(id) {
			data = append(data, map[string]string{"id": id, "type": "songs"})
		}
	}
	if len(data) == 0 {
		return []music.Track{}, nil
	}
	if err := s.needAccount(); err != nil {
		return nil, err
	}
	// Apple answers the station it made, and its first songs beside it.
	var b struct {
		Results struct {
			Station *resource   `json:"station"`
			Tracks  []*resource `json:"tracks"`
		} `json:"results"`
	}
	u := s.amp + "/v1/me/stations/continuous" + query(params{"with": "tracks", "limit[results:tracks]": autoplayMost})
	if err := s.requestJSON(u, reqOpts{method: "POST", body: map[string]any{"data": data}}, &b); err != nil {
		return nil, err
	}
	songs := b.Results.Tracks
	tracks := s.songsOf(songs)
	s.listed(tracks, nil)
	return tracks, nil
}
