package applemusic

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"mygo-music/internal/music"
)

// Search, suggestions, and links from music.apple.com pasted into the search.

var searchTypes = map[music.SearchKind]string{
	music.SearchTracks:    "songs",
	music.SearchAlbums:    "albums",
	music.SearchArtists:   "artists",
	music.SearchPlaylists: "playlists",
}

func (s *Source) search(term string, kind music.SearchKind, offset int) (*music.SearchResult, error) {
	s.warm()
	typ := searchTypes[kind]
	if typ == "" {
		return &music.SearchResult{}, nil
	}
	if link := parseAppleURL(term); link != nil && offset == 0 {
		if found, err := s.fromLink(link, kind); err == nil && found != nil {
			return found, nil
		}
	}
	p := params{"term": term, "types": typ, "limit": "25"}
	if offset > 0 {
		p["offset"] = strconv.Itoa(offset)
	}
	var b struct {
		Results map[string]*relationship `json:"results"`
	}
	if err := s.catalogInto("/search", p, &b); err != nil {
		return nil, err
	}
	r := b.Results[typ]
	if r == nil {
		r = &relationship{}
	}
	out := &music.SearchResult{More: r.Next != ""}
	switch kind {
	case music.SearchTracks:
		out.Tracks = s.songsOf(r.Data)
		// The app adds the next pages to the songs it lists, which play on into them.
		run := &run{key: "search:" + term, start: offset == 0}
		if offset > 0 {
			s.listed(out.Tracks, run)
		} else {
			s.prefetch(out.Tracks, run)
		}
	case music.SearchAlbums:
		out.Albums = s.toAlbums(r.Data)
	case music.SearchArtists:
		out.Artists = []music.Artist{}
		for _, i := range r.Data {
			if i != nil {
				out.Artists = append(out.Artists, toArtist(i))
			}
		}
	case music.SearchPlaylists:
		out.Playlists = s.toPlaylists(r.Data)
	}
	return out, nil
}

// appleLink is a music.apple.com link.
type appleLink struct {
	kind string // album, song, playlist or artist
	id   string
	// song is the song an album link points at, by its ?i=.
	song string
}

var appleURL = regexp.MustCompile(`music\.apple\.com/([a-z]{2})/(album|song|playlist|artist)/(?:[^/?#]+/)?([^/?#]+)(?:\?([^#]*))?`)

// parseAppleURL is the link text is, if it's one.
func parseAppleURL(text string) *appleLink {
	m := appleURL.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return nil
	}
	q, _ := url.ParseQuery(m[4])
	return &appleLink{kind: m[2], id: m[3], song: q.Get("i")}
}

func (s *Source) fromLink(link *appleLink, kind music.SearchKind) (*music.SearchResult, error) {
	if link.kind == "song" || link.song != "" {
		if kind != music.SearchTracks {
			return nil, nil
		}
		id := link.song
		if id == "" {
			id = link.id
		}
		b, err := s.catalog("/songs/"+id, nil)
		if err != nil {
			return nil, err
		}
		return &music.SearchResult{Tracks: s.songsOf(b.Data)}, nil
	}
	one := func(path string) (*resource, error) {
		b, err := s.catalog(path, nil)
		if err != nil {
			return nil, err
		}
		return first(b)
	}
	switch {
	case link.kind == "album" && kind == music.SearchAlbums:
		r, err := one("/albums/" + link.id)
		if err != nil {
			return nil, err
		}
		return &music.SearchResult{Albums: []music.Album{s.toAlbum(r)}}, nil
	case link.kind == "album" && kind == music.SearchTracks:
		page, err := s.album(link.id)
		if err != nil {
			return nil, err
		}
		return &music.SearchResult{Tracks: page.Tracks}, nil
	case link.kind == "playlist" && kind == music.SearchPlaylists:
		r, err := one("/playlists/" + link.id)
		if err != nil {
			return nil, err
		}
		return &music.SearchResult{Playlists: []music.Playlist{s.toPlaylist(r)}}, nil
	case link.kind == "artist" && kind == music.SearchArtists:
		r, err := one("/artists/" + link.id)
		if err != nil {
			return nil, err
		}
		return &music.SearchResult{Artists: []music.Artist{toArtist(r)}}, nil
	}
	return nil, nil
}

func (s *Source) suggest(term string) ([]string, error) {
	s.warm()
	if strings.TrimSpace(term) == "" {
		return []string{}, nil
	}
	var b struct {
		Results struct {
			Suggestions []struct {
				DisplayTerm string `json:"displayTerm"`
				SearchTerm  string `json:"searchTerm"`
			} `json:"suggestions"`
		} `json:"results"`
	}
	if err := s.catalogInto("/search/suggestions", params{"term": term, "kinds": "terms", "limit": "10"}, &b); err != nil {
		return nil, err
	}
	out := []string{}
	for _, x := range b.Results.Suggestions {
		if t := orT(x.DisplayTerm, x.SearchTerm); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}
