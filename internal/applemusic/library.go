package applemusic

import (
	"errors"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"mygo-music/internal/i18n"
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
	s.edits = nil
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

// ratingsOf is where a song's rating is: an upload, known by its library ID,
// is rated as a library song.
func ratingsOf(id string) string {
	if isLibrarySong(id) {
		return "/ratings/library-songs"
	}
	return "/ratings/songs"
}

func (s *Source) isLiked(ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	byKind := map[string][]string{}
	for _, id := range ids {
		out[id] = false
		if isCatalogID(id) || isLibrarySong(id) {
			byKind[ratingsOf(id)] = append(byKind[ratingsOf(id)], id)
		}
	}
	if !s.signedIn() {
		return out, nil
	}
	for path, some := range byKind {
		for i := 0; i < len(some); i += 100 {
			b, err := s.me(path, params{"ids": strings.Join(some[i:min(i+100, len(some))], ",")}, reqOpts{})
			if err != nil {
				return nil, err
			}
			for _, r := range b.Data {
				if r != nil {
					out[r.ID] = r.attrs().Value == 1
				}
			}
		}
	}
	return out, nil
}

func (s *Source) like(id string, on bool) error {
	if !isCatalogID(id) && !isLibrarySong(id) {
		return fail("unsupported", "")
	}
	if on {
		if _, err := s.me(ratingsOf(id)+"/"+id, nil, reqOpts{method: "PUT", body: map[string]any{"type": "rating", "attributes": map[string]any{"value": 1}}}); err != nil {
			return err
		}
		if isCatalogID(id) {
			s.me("/library", params{"ids[songs]": id}, reqOpts{method: "POST"})
			s.forget("recentlyAdded")
		}
	} else if _, err := s.me(ratingsOf(id)+"/"+id, nil, reqOpts{method: "DELETE"}); err != nil {
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
		return &music.AlbumList{Albums: s.toAlbums(b.Data), More: b.Next != ""}, nil
	}, ahead)
}

var catalogOrPlaylistID = regexp.MustCompile(`^(\d+|pl\.[\w.-]+)$`)

// inLibrary is the library's own resource for what of the catalog is in the library.
func (s *Source) inLibrary(typ, id string) *resource {
	own, _, _ := s.personal(typ, id)
	return own
}

// inLibraryOf says which songs or music videos of ids are in the library, a
// hundred to a request, as the web player asks for its rows' "in library"
// state: the catalog's resources by ID, with nothing but inLibrary. Those
// that aren't come with it false, or without it, and those the storefront
// doesn't have don't come at all. The library's own IDs are in it.
func (s *Source) inLibraryOf(kind string, ids []string) (map[string]bool, error) {
	if kind != music.ItemSong && kind != music.ItemVideo {
		return nil, fail("unsupported", "")
	}
	typ := catalogTypes[kind]
	out := make(map[string]bool, len(ids))
	var ask []string
	for _, id := range ids {
		switch {
		case isCatalogID(id):
			ask = append(ask, id)
		case isLibrarySong(id):
			out[id] = true
		}
	}
	if !s.signedIn() {
		return out, nil
	}
	for i := 0; i < len(ask); i += 100 {
		b, err := s.catalog("", params{"ids[" + typ + "]": strings.Join(ask[i:min(i+100, len(ask))], ","), "fields[" + typ + "]": "inLibrary", "omit[resource]": "autos"})
		if err != nil {
			return nil, err
		}
		for _, r := range b.Data {
			if r != nil && r.attrs().InLibrary {
				out[r.ID] = true
			}
		}
	}
	return out, nil
}

// askInLibrary has a page's songs say whether they're in the library, which
// Apple adds to their attributes for an extend asked of songs, and reports
// whether it did: signed out, there's no library to ask about.
func (s *Source) askInLibrary(p params) bool {
	if !s.signedIn() {
		return false
	}
	p["extend[songs]"] = "extendedAssetUrls,inLibrary"
	return true
}

// libraryKnown marks songs as having said whether they're in the library,
// if they were asked.
func libraryKnown(songs []music.Track, asked bool) {
	for i := range songs {
		songs[i].SavedKnown = songs[i].SavedKnown || asked
	}
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
	// An album with songs the listener uploaded stays: those deleted
	// couldn't be added back. Its menu on a card, which doesn't know, still
	// offers it.
	for _, song := range songs {
		if uploaded(song) {
			return fail("unsupported", uploadsStay())
		}
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

func uploadsStay() string {
	return i18n.T("Songs you uploaded can't be added back once deleted, so this album stays in your library")
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
		if p == nil || p.ID == favorites {
			continue
		}
		if pl, ok := s.asEdited(s.toPlaylist(p)); ok {
			out = append(out, pl)
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
		switch {
		case isCatalogID(song):
			data = append(data, map[string]string{"id": song, "type": "songs"})
		case isLibrarySong(song):
			// An upload, which goes by its library ID, as Apple's API takes.
			data = append(data, map[string]string{"id": song, "type": "library-songs"})
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

// Editing the listener's own playlists, as the web player does: a new name
// is PATCHed, a playlist is DELETEd, a song is taken out by its library ID,
// and a new order is PUT as the whole list of its tracks.
//
// Apple takes a change at once but may answer with the playlist as it was
// for a minute or two after. Until it catches up, the source answers with
// what the listener made of it, so that what the app shows isn't undone by
// reading it back, and the next change starts from it, not from the past.

// editSettles is how long the source keeps answering with a change: by then
// Apple has it, or someone changed the playlist again elsewhere.
const editSettles = 5 * time.Minute

// playlistEdit is what the listener changed of one of their playlists.
type playlistEdit struct {
	at      time.Time
	name    string // its new name, if it was renamed
	deleted bool
	// tracks are all its tracks as they are now, music videos among them,
	// if they changed; nil if they didn't.
	tracks []*resource
}

// edited is what the listener lately changed of playlist id, or nil.
func (s *Source) edited(id string) *playlistEdit {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	e := s.edits[id]
	if e == nil {
		return nil
	}
	if s.clk.Now().Sub(e.at) > editSettles {
		delete(s.edits, id)
		return nil
	}
	c := *e
	return &c
}

// noteEdit keeps what change made of playlist id, which Apple took, and
// forgets the playlist as it was kept.
func (s *Source) noteEdit(id string, change func(e *playlistEdit)) {
	s.libMu.Lock()
	if s.edits == nil {
		s.edits = map[string]*playlistEdit{}
	}
	e := s.edits[id]
	if e == nil || s.clk.Now().Sub(e.at) > editSettles {
		e = &playlistEdit{}
		s.edits[id] = e
	}
	change(e)
	e.at = s.clk.Now()
	s.libMu.Unlock()
	s.forget("playlist:" + id)
}

// asEdited is pl with the name the listener last gave it, and false for one
// they deleted. Its tracks are left as they are, for a list of playlists.
func (s *Source) asEdited(pl music.Playlist) (music.Playlist, bool) {
	e := s.edited(pl.ID)
	switch {
	case e == nil:
	case e.deleted:
		return pl, false
	case e.name != "":
		pl.Name = e.name
	}
	return pl, true
}

// editedItems are a page's items with the listener's playlists as they last
// changed them: renamed, or gone.
func (s *Source) editedItems(items []music.Item) []music.Item {
	out := make([]music.Item, 0, len(items))
	for _, it := range items {
		if it.Kind == music.ItemPlaylist && it.Playlist != nil {
			pl, ok := s.asEdited(*it.Playlist)
			if !ok {
				continue
			}
			if pl.Name != it.Playlist.Name {
				it.Title, it.Playlist = pl.Name, &pl
			}
		}
		out = append(out, it)
	}
	return out
}

// changedElsewhere says a playlist isn't as the app showed it: it changed on
// another device, or Apple hasn't caught up with a song added here.
func changedElsewhere() error {
	return fail("unsupported", i18n.T("The playlist changed elsewhere. Try again in a moment."))
}

// playlistTracks are all of playlist id's tracks as they are now: as the
// listener last changed them, or else as Apple answers, asked anew.
func (s *Source) playlistTracks(id string) ([]*resource, error) {
	if e := s.edited(id); e != nil && e.tracks != nil {
		return e.tracks, nil
	}
	all, err := s.allPages("/v1/me/library/playlists/"+id+"/tracks", params{"limit": "100"}, 5000, nil)
	if errors.Is(err, music.ErrUnavailable) {
		return nil, nil // an empty playlist has no tracks to find
	}
	if err != nil {
		return nil, err
	}
	tracks := make([]*resource, 0, len(all))
	for _, t := range all {
		if t != nil {
			tracks = append(tracks, t)
		}
	}
	return tracks, nil
}

func (s *Source) renamePlaylist(id, name string) error {
	if _, err := s.me("/library/playlists/"+id, nil, reqOpts{method: "PATCH", body: map[string]any{"attributes": map[string]any{"name": name}}}); err != nil {
		return err
	}
	s.noteEdit(id, func(e *playlistEdit) { e.name = name })
	return nil
}

func (s *Source) deletePlaylist(id string) error {
	if _, err := s.me("/library/playlists/"+id, nil, reqOpts{method: "DELETE"}); err != nil {
		return err
	}
	s.noteEdit(id, func(e *playlistEdit) { e.deleted = true })
	s.forget("recentlyAdded")
	s.forget("library")
	return nil
}

// removeFromPlaylist takes songs out by their library IDs, which the
// playlist's tracks are: the app knows a song by its catalog ID, and an
// upload by its library ID already. mode=all takes out every time a song is
// listed, as the app does.
func (s *Source) removeFromPlaylist(id string, trackIDs []string) error {
	tracks, err := s.playlistTracks(id)
	if err != nil {
		return err
	}
	out := map[string]bool{}
	for _, t := range trackIDs {
		out[t] = true
	}
	var gone []string
	for _, t := range tracks {
		if t.Type == "library-songs" && (out[songID(t)] || out[t.ID]) && !slices.Contains(gone, t.ID) {
			gone = append(gone, t.ID)
		}
	}
	if len(gone) == 0 {
		s.forget("playlist:" + id)
		return changedElsewhere()
	}
	var done []string
	for _, lid := range gone {
		if _, err = s.me("/library/playlists/"+id+"/tracks", params{"ids[library-songs]": lid, "mode": "all"}, reqOpts{method: "DELETE"}); err != nil {
			break
		}
		done = append(done, lid)
	}
	if len(done) > 0 {
		left := slices.DeleteFunc(slices.Clone(tracks), func(t *resource) bool { return slices.Contains(done, t.ID) })
		s.noteEdit(id, func(e *playlistEdit) { e.tracks = left })
	}
	return err
}

// movePlaylistTrack moves a song from and to places among the songs, which
// are what the app lists: music videos stay where they are.
func (s *Source) movePlaylistTrack(id string, from, to int) error {
	tracks, err := s.playlistTracks(id)
	if err != nil {
		return err
	}
	var at []int
	for i, t := range tracks {
		if t.Type == "songs" || t.Type == "library-songs" {
			at = append(at, i)
		}
	}
	if from < 0 || to < 0 || from >= len(at) || to >= len(at) {
		s.forget("playlist:" + id)
		return changedElsewhere()
	}
	songs := make([]*resource, len(at))
	for i, j := range at {
		songs[i] = tracks[j]
	}
	moving := songs[from]
	songs = slices.Insert(slices.Delete(songs, from, from+1), to, moving)
	order := slices.Clone(tracks)
	for i, j := range at {
		order[j] = songs[i]
	}
	data := make([]map[string]string, len(order))
	for i, t := range order {
		data[i] = map[string]string{"id": t.ID, "type": t.Type}
	}
	if _, err := s.me("/library/playlists/"+id+"/tracks", nil, reqOpts{method: "PUT", body: map[string]any{"data": data}}); err != nil {
		return err
	}
	s.noteEdit(id, func(e *playlistEdit) { e.tracks = order })
	return nil
}
