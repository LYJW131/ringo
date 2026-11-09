package applemusic

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mygo-music/internal/music"
)

// The listener's library: what they played lately, their songs, albums and
// playlists, and what they favorite.

// listKept is how long what can't tell when it changed is kept: favorites, albums, stations.
const listKept = 10 * time.Minute

// What the source knows of the listener's playlists: as last listed, each
// with when it last changed, which tells whether a playlist fetched before is
// still as it was; and Favorite Songs, the playlist Apple keeps of the songs
// they favorite, which can't be deleted and isn't in the catalog.
type libraryState struct {
	changed map[string]string
	// favoritesKnown says the list was read since the listener signed in;
	// favoritesID is then Favorite Songs' ID, or "" when there's none.
	favoritesKnown bool
	favoritesID    string
}

// forgetLibrary forgets what was kept of the listener, who signed out or in.
func (s *Source) forgetLibrary() {
	s.forgetAll()
	s.libMu.Lock()
	s.lib = libraryState{changed: map[string]string{}}
	s.libMu.Unlock()
	s.subMu.Lock()
	s.subscribed = nil
	s.subMu.Unlock()
}

func (s *Source) favorites() (id string, known bool) {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	return s.lib.favoritesID, s.lib.favoritesKnown
}

// libraryPlaylists is the listener's playlists, Favorite Songs among them, from Apple.
func (s *Source) libraryPlaylists(ahead bool) ([]*resource, error) {
	// The app's own ask always goes to Apple: the list is what says what changed.
	maxAge := time.Duration(0)
	if ahead {
		maxAge = time.Minute
	}
	return kept(s, "playlists", freshness{maxAge: maxAge}, func(pause func()) ([]*resource, error) {
		lists, err := s.allPages("/v1/me/library/playlists", params{"limit": "100"}, 500, pause)
		if err != nil {
			return nil, err
		}
		st := libraryState{changed: map[string]string{}, favoritesKnown: true}
		for _, p := range lists {
			if p == nil {
				continue
			}
			a := p.attrs()
			if a.LastModifiedDate != "" {
				st.changed[p.ID] = a.LastModifiedDate
			}
			if a.CanDelete != nil && !*a.CanDelete && !a.HasCatalog {
				st.favoritesID = p.ID
			}
		}
		s.libMu.Lock()
		s.lib = st
		s.libMu.Unlock()
		return lists, nil
	}, ahead)
}

// playlistFreshness is how a library playlist fetched before stays good: as long as it hasn't changed.
func (s *Source) playlistFreshness(id string) freshness {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	if v := s.lib.changed[id]; v != "" {
		return freshness{version: v, maxAge: forever}
	}
	return freshness{maxAge: listKept}
}

// recentlyChanged is the listener's playlists by when they last changed, the latest first, but Favorite Songs.
func (s *Source) recentlyChanged() []string {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	var ids []string
	for id := range s.lib.changed {
		if id != s.lib.favoritesID {
			ids = append(ids, id)
		}
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := s.lib.changed[ids[i]], s.lib.changed[ids[j]]
		if a != b {
			return a > b
		}
		return ids[i] < ids[j]
	})
	return ids
}

func (s *Source) recentTracks() ([]music.Track, error) {
	out := []music.Track{}
	for offset := 0; offset < 50; offset += 10 {
		b, err := s.me("/recent/played/tracks", params{"limit": "10", "offset": strconv.Itoa(offset), "types": "songs,library-songs"}, reqOpts{})
		if err != nil {
			return nil, err
		}
		out = append(out, s.songsOf(b.Data)...)
		if b.Next == "" {
			break
		}
	}
	return out, nil
}

// recent is what the app lists in place of home, when it has nothing, and
// loads as it loads home.
func (s *Source) recent() ([]music.Track, error) {
	tracks, err := s.recentTracks()
	if err != nil {
		return nil, err
	}
	s.shown(tracks)
	return tracks, nil
}

// Liked songs are Favorite Songs, newest first, as Apple's apps list them.
// Liking a song favorites it, as the star in Apple's apps does, which puts it
// in Favorite Songs and the library. An account without Favorite Songs lists
// its library's songs.

func (s *Source) liked(offset int, ahead bool) (*music.Page, error) {
	if _, known := s.favorites(); !known {
		if _, err := s.libraryPlaylists(ahead); err != nil {
			return nil, err
		}
	}
	favorites, _ := s.favorites()
	version := ""
	if favorites != "" {
		s.libMu.Lock()
		version = s.lib.changed[favorites]
		s.libMu.Unlock()
	}
	page, err := kept(s, "liked:"+strconv.Itoa(offset), freshness{version: version, maxAge: listKept}, func(func()) (*music.Page, error) {
		path := "/library/songs"
		if favorites != "" {
			path = "/library/playlists/" + favorites + "/tracks"
		}
		p := params{"limit": "100", "include": "catalog"}
		if offset > 0 {
			p["offset"] = strconv.Itoa(offset)
		}
		b, err := s.me(path, p, reqOpts{})
		if errors.Is(err, music.ErrUnavailable) {
			b, err = &body{}, nil // none yet
		}
		if err != nil {
			return nil, err
		}
		return &music.Page{Tracks: s.songsOf(b.Data), More: b.Next != ""}, nil
	}, ahead)
	if err != nil {
		return nil, err
	}
	if !ahead {
		s.prefetch(page.Tracks, nil)
	}
	return page, nil
}

func (s *Source) isLiked(ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	var catalogIDs []string
	for _, id := range ids {
		out[id] = false
		if isCatalogID(id) {
			catalogIDs = append(catalogIDs, id)
		}
	}
	if !s.signedIn() || len(catalogIDs) == 0 {
		return out, nil
	}
	for i := 0; i < len(catalogIDs); i += 100 {
		b, err := s.me("/ratings/songs", params{"ids": strings.Join(catalogIDs[i:min(i+100, len(catalogIDs))], ",")}, reqOpts{})
		if err != nil {
			return nil, err
		}
		for _, r := range b.Data {
			if r != nil {
				out[r.ID] = r.attrs().Value == 1
			}
		}
	}
	return out, nil
}

func (s *Source) like(id string, on bool) error {
	if !isCatalogID(id) {
		return fail("unsupported", "")
	}
	if on {
		if _, err := s.me("/ratings/songs/"+id, nil, reqOpts{method: "PUT", body: map[string]any{"type": "rating", "attributes": map[string]any{"value": 1}}}); err != nil {
			return err
		}
		s.me("/library", params{"ids[songs]": id}, reqOpts{method: "POST"})
		s.forget("recentlyAdded")
	} else if _, err := s.me("/ratings/songs/"+id, nil, reqOpts{method: "DELETE"}); err != nil {
		return err
	}
	s.forget("liked:")
	if favorites, _ := s.favorites(); favorites != "" {
		s.forget("playlist:" + favorites)
	}
	return nil
}

func (s *Source) savedAlbums(offset int, ahead bool) (*music.AlbumList, error) {
	return kept(s, "albums:"+strconv.Itoa(offset), freshness{maxAge: listKept}, func(func()) (*music.AlbumList, error) {
		p := params{"limit": "100"}
		if offset > 0 {
			p["offset"] = strconv.Itoa(offset)
		}
		b, err := s.me("/library/albums", p, reqOpts{})
		if err != nil {
			return nil, err
		}
		return &music.AlbumList{Albums: toAlbums(b.Data), More: b.Next != ""}, nil
	}, ahead)
}

var catalogOrPlaylistID = regexp.MustCompile(`^(\d+|pl\.[\w.-]+)$`)

// inLibrary is the library's own resource for what of the catalog is in the library.
func (s *Source) inLibrary(typ, id string) *resource {
	own, _, _ := s.personal(typ, id)
	return own
}

// personal is what the listener has of what of the catalog: its library's
// own resource, if it's in the library, and whether it's a favorite. Apple
// answers these only when the fields are asked for by name.
func (s *Source) personal(typ, id string) (own *resource, favorite bool, err error) {
	if !s.signedIn() || !catalogOrPlaylistID.MatchString(id) {
		return nil, false, nil
	}
	b, err := s.catalog("/"+typ+"/"+id, params{"relate": "library", "fields[" + typ + "]": "inFavorites,inLibrary"})
	if err != nil {
		return nil, false, err
	}
	if len(b.Data) == 0 || b.Data[0] == nil {
		return nil, false, nil
	}
	favorite = b.Data[0].attrs().InFavorites
	if d := b.Data[0].related("library").Data; len(d) > 0 {
		return d[0], favorite, nil
	}
	return nil, favorite, nil
}

func (s *Source) saveAlbum(id string, on bool) error {
	s.forget("albums:")
	s.forget("recentlyAdded")
	s.forget("library")
	if on {
		_, err := s.me("/library", params{"ids[albums]": id}, reqOpts{method: "POST"})
		return err
	}
	own := id
	if !strings.HasPrefix(id, "l.") {
		own = ""
		if r := s.inLibrary("albums", id); r != nil {
			own = r.ID
		}
	}
	if own == "" {
		return nil
	}
	// Its songs first: Apple answers 204 for an album taken out with its
	// songs still in the library, and goes on listing it, as it lists an
	// album for the songs of it the library holds.
	songs, err := s.allPages("/v1/me/library/albums/"+own+"/tracks", nil, 1000, nil)
	if err != nil {
		return err
	}
	for _, song := range songs {
		if song.Type != "library-songs" {
			continue
		}
		if _, err := s.me("/library/songs/"+song.ID, nil, reqOpts{method: "DELETE"}); err != nil {
			return err
		}
	}
	_, err = s.me("/library/albums/"+own, nil, reqOpts{method: "DELETE"})
	return err
}

func (s *Source) savePlaylist(id string, on bool) error {
	s.forget("recentlyAdded")
	s.forget("library")
	if on {
		_, err := s.me("/library", params{"ids[playlists]": id}, reqOpts{method: "POST"})
		return err
	}
	own := id
	if !strings.HasPrefix(id, "p.") {
		own = ""
		if r := s.inLibrary("playlists", id); r != nil {
			own = r.ID
		}
	}
	if own == "" {
		return nil
	}
	// Unsaving takes a playlist out of the library, which for one the
	// listener made deletes it: only one saved from someone else goes.
	b, err := s.me("/library/playlists/"+own, nil, reqOpts{})
	if err != nil {
		return err
	}
	item, err := first(b)
	if err != nil {
		return err
	}
	if item.attrs().CanEdit {
		return fail("unsupported", "")
	}
	_, err = s.me("/library/playlists/"+own, nil, reqOpts{method: "DELETE"})
	return err
}

// playlists is the sidebar's: Favorite Songs is the app's own Liked page.
func (s *Source) playlists() ([]music.Playlist, error) {
	lists, err := s.libraryPlaylists(false)
	if err != nil {
		return nil, err
	}
	favorites, _ := s.favorites()
	out := []music.Playlist{}
	for _, p := range lists {
		if p != nil && p.ID != favorites {
			out = append(out, s.toPlaylist(p))
		}
	}
	return out, nil
}

func (s *Source) createPlaylist(name string) (*music.Playlist, error) {
	b, err := s.me("/library/playlists", nil, reqOpts{method: "POST", body: map[string]any{"attributes": map[string]any{"name": name}}})
	if err != nil {
		return nil, err
	}
	if len(b.Data) == 0 || b.Data[0] == nil {
		return nil, errors.New("Apple Music didn't make the playlist")
	}
	p := s.toPlaylist(b.Data[0])
	p.Mine = true
	p.Tracks = []music.Track{}
	return &p, nil
}

func (s *Source) addToPlaylist(id string, trackIDs []string) error {
	var data []map[string]string
	for _, song := range trackIDs {
		if isCatalogID(song) {
			data = append(data, map[string]string{"id": song, "type": "songs"})
		}
	}
	if len(data) == 0 {
		return nil
	}
	if _, err := s.me("/library/playlists/"+id+"/tracks", nil, reqOpts{method: "POST", body: map[string]any{"data": data}}); err != nil {
		return err
	}
	s.forget("playlist:" + id)
	return nil
}
