package applemusic

import (
	"strings"

	"ringo/internal/music"
)

// Motion answers the square covers that move of what ids are, by an Item's
// kind: a song's is its album's. Most pages come without them, and a song's
// album often without an ID at all, as Liked's songs do, so the app asks
// about the covers it shows, a song's by the song. Those without one, or not
// in the catalog, are left out.
func (s *Source) Motion(kind string, ids []string) (map[string]string, error) {
	var path, typ string
	batch := 100
	var p params
	switch kind {
	case music.ItemSong:
		// The song's album comes along, with no more of the song than its
		// name: what's asked for of the album is all that's wanted of it.
		path, typ, batch = "/songs", "songs", 300
		p = params{"include": "albums", "fields[songs]": "name", "extend[albums]": "editorialVideo", "fields[albums]": "editorialVideo"}
	case music.ItemAlbum:
		path, typ = "/albums", "albums"
		p = params{"extend": "editorialVideo", "fields[albums]": "editorialVideo"}
	case music.ItemPlaylist:
		path, typ = "/playlists", "playlists"
		p = params{"extend": "editorialVideo", "fields[playlists]": "editorialVideo"}
	default:
		return map[string]string{}, nil
	}
	var ask []string
	for _, id := range ids {
		if catalogOf(kind, id) {
			ask = append(ask, id)
		}
	}
	out := map[string]string{}
	for i := 0; i < len(ask); i += batch {
		q := params{"ids": strings.Join(ask[i:min(i+batch, len(ask))], ",")}
		for k, v := range p {
			q[k] = v
		}
		b, err := s.catalog(path, q)
		if err != nil {
			return out, err
		}
		for _, ref := range b.Data {
			r := b.get(ref)
			if r == nil || r.Type != typ {
				continue
			}
			moving := r
			if kind == music.ItemSong {
				albums := r.related("albums").Data
				if len(albums) == 0 {
					continue
				}
				if moving = b.get(albums[0]); moving == nil {
					continue
				}
			}
			if v := motion(moving.attrs(), squareMove...); v != "" {
				out[r.ID] = v
			}
		}
	}
	return out, nil
}

// catalogOf says id is the catalog's, which alone has covers that move: a
// playlist of the listener's own has none.
func catalogOf(kind, id string) bool {
	if kind == music.ItemPlaylist {
		return strings.HasPrefix(id, "pl.")
	}
	return isCatalogID(id)
}
