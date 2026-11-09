package applemusic

import (
	"errors"
	"strings"
	"sync"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Albums, artists and playlists, from the catalog or the library. Library
// IDs start with l. (albums) and p. (playlists); an artist the app only knows
// by name has the ID name:<name>.

// withLibrary fetches a catalog resource and, at the same time, its library
// version, if it's in the listener's library.
func (s *Source) withLibrary(typ, id string, p params) (*body, *resource, error) {
	var (
		wg  sync.WaitGroup
		own *resource
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		own = s.inLibrary(typ, id)
	}()
	b, err := s.catalog("/"+typ+"/"+id, p)
	wg.Wait()
	return b, own, err
}

func (s *Source) album(id string) (*music.AlbumPage, error) {
	s.warm()
	if strings.HasPrefix(id, "l.") {
		b, err := s.me("/library/albums/"+id, params{"include": "tracks"}, reqOpts{})
		if err != nil {
			return nil, err
		}
		item, err := first(b)
		if err != nil {
			return nil, err
		}
		tracks, err := s.allPages("/v1/me/library/albums/"+id+"/tracks", params{"limit": "100"}, 1000, nil)
		if err != nil {
			return nil, err
		}
		songs := s.songsOf(tracks)
		s.prefetch(songs, nil)
		a := toAlbum(item)
		a.ID, a.Saved = id, true
		return &music.AlbumPage{Album: a, Tracks: songs}, nil
	}
	b, own, err := s.withLibrary("albums", id, params{"include": "tracks,artists"})
	if err != nil {
		return nil, err
	}
	item, err := first(b)
	if err != nil {
		return nil, err
	}
	tracks := item.related("tracks").Data
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
	s.prefetch(songs, nil)
	a := toAlbum(item)
	a.Saved = own != nil
	return &music.AlbumPage{Album: a, Tracks: songs}, nil
}

func (s *Source) artist(id string) (*music.ArtistPage, error) {
	s.warm()
	if name, ok := strings.CutPrefix(id, "name:"); ok {
		var b struct {
			Results struct {
				Artists relationship `json:"artists"`
			} `json:"results"`
		}
		if err := s.catalogInto("/search", params{"term": name, "types": "artists", "limit": "1"}, &b); err != nil {
			return nil, err
		}
		if len(b.Results.Artists.Data) == 0 || b.Results.Artists.Data[0] == nil {
			return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
		}
		id = b.Results.Artists.Data[0].ID
	}
	b, err := s.catalog("/artists/"+id, params{"views": "top-songs,full-albums,singles"})
	if err != nil {
		return nil, err
	}
	item, err := first(b)
	if err != nil {
		return nil, err
	}
	view := func(name string) []*resource {
		if v := item.Views[name]; v != nil {
			return v.Data
		}
		return nil
	}
	albums := toAlbums(append(view("full-albums"), view("singles")...))
	top := s.songsOf(view("top-songs"))
	s.prefetch(top, nil)
	return &music.ArtistPage{Artist: toArtist(item), Tracks: top, Albums: albums}, nil
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

func (s *Source) playlist(id string) (*music.Playlist, error) {
	s.warm()
	if strings.HasPrefix(id, "p.") {
		found, err := s.libraryPlaylist(id, false)
		if err != nil {
			return nil, err
		}
		s.prefetch(found.Tracks, nil)
		return found, nil
	}
	b, own, err := s.withLibrary("playlists", id, params{"include": "tracks"})
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
	tracks := item.related("tracks").Data
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
	p.Saved, p.TrackCount, p.Tracks = own != nil, len(songs), songs
	return &p, nil
}
