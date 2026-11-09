package applemusic

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"mygo-music/internal/music"
)

// What Apple's API answers, as far as the source reads it, and what Apple
// calls things, as the app calls them.

// flexString is an ID Apple gives as a string in one place and a number in another.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	if bytes.Equal(b, []byte("null")) {
		*f = ""
		return nil
	}
	*f = flexString(b)
	return nil
}

type artwork struct {
	URL    string `json:"url,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type playParams struct {
	ID          string     `json:"id,omitempty"`
	Kind        string     `json:"kind,omitempty"`
	CatalogID   flexString `json:"catalogId,omitempty"`
	ReportingID string     `json:"reportingId,omitempty"`
}

type attributes struct {
	Name             string      `json:"name"`
	ArtistName       string      `json:"artistName"`
	AlbumName        string      `json:"albumName"`
	AlbumArtistName  string      `json:"albumArtistName"`
	Artwork          *artwork    `json:"artwork"`
	DurationInMillis int64       `json:"durationInMillis"`
	URL              string      `json:"url"`
	PlayParams       *playParams `json:"playParams"`
	ReleaseDate      string      `json:"releaseDate"`
	Description      *struct {
		Standard string `json:"standard"`
		Short    string `json:"short"`
	} `json:"description"`
	EditorialNotes *struct {
		Short string `json:"short"`
	} `json:"editorialNotes"`
	CuratorName      string `json:"curatorName"`
	TrackCount       int    `json:"trackCount"`
	CanEdit          bool   `json:"canEdit"`
	CanDelete        *bool  `json:"canDelete"`
	HasCatalog       bool   `json:"hasCatalog"`
	LastModifiedDate string `json:"lastModifiedDate"`
	IsLive           bool   `json:"isLive"`
	Title            *struct {
		StringForDisplay string `json:"stringForDisplay"`
	} `json:"title"`
	Value             int      `json:"value"`
	TTML              string   `json:"ttml"`
	TTMLLocalizations string   `json:"ttmlLocalizations"`
	AudioTraits       []string `json:"audioTraits"` // such as lossless, hi-res-lossless, atmos
	AvatarArtwork     *artwork `json:"avatarArtwork"`
	ExtendedAssetUrls *struct {
		EnhancedHls string `json:"enhancedHls"`
	} `json:"extendedAssetUrls"`
}

type relationship struct {
	Data []*resource `json:"data"`
	Next string      `json:"next"`
}

type resource struct {
	ID            string                   `json:"id"`
	Type          string                   `json:"type"`
	Attributes    *attributes              `json:"attributes"`
	Relationships map[string]*relationship `json:"relationships"`
	Views         map[string]*relationship `json:"views"`
}

// attrs never answers nil.
func (r *resource) attrs() *attributes {
	if r.Attributes == nil {
		return &attributes{}
	}
	return r.Attributes
}

func (r *resource) related(name string) *relationship {
	if rel := r.Relationships[name]; rel != nil {
		return rel
	}
	return &relationship{}
}

// body is most answers: a page of resources.
type body struct {
	Data []*resource `json:"data"`
	Next string      `json:"next"`
}

// songID is a song's catalog ID, which is what plays: library songs carry it in their playParams.
func songID(item *resource) string {
	if play := item.attrs().PlayParams; play != nil {
		if play.CatalogID != "" {
			return string(play.CatalogID)
		}
		if play.ReportingID != "" && isCatalogID(play.ReportingID) {
			return play.ReportingID
		}
	}
	return item.ID
}

var albumInURL = regexp.MustCompile(`/album/[^/]+/(\d+)`)

func albumIDOf(item *resource) string {
	if d := item.related("albums").Data; len(d) > 0 && d[0] != nil {
		return d[0].ID
	}
	if m := albumInURL.FindStringSubmatch(item.attrs().URL); m != nil {
		return m[1]
	}
	return ""
}

func artistIDOf(item *resource) string {
	if d := item.related("artists").Data; len(d) > 0 && d[0] != nil {
		return d[0].ID
	}
	return ""
}

// byName is an artist without an ID, by name: the artist page looks them up.
func byName(name string) music.Artist { return music.Artist{ID: "name:" + name, Name: name} }

func (s *Source) toTrack(item *resource) music.Track {
	a := item.attrs()
	id := songID(item)
	if item.Type == "songs" {
		s.noteAudio(id, a)
	}
	s.noteDuration(id, a.DurationInMillis)
	artistID := artistIDOf(item)
	if artistID == "" {
		artistID = "name:" + a.ArtistName
	}
	artists := []music.Artist{{ID: artistID, Name: a.ArtistName}}
	albumArtists := artists
	if a.AlbumArtistName != "" {
		albumArtists = []music.Artist{byName(a.AlbumArtistName)}
	}
	return music.Track{
		ID:      id,
		Name:    a.Name,
		Artists: artists,
		Album: music.Album{
			ID:       albumIDOf(item),
			Name:     a.AlbumName,
			ImageURL: art(a.Artwork),
			Artists:  albumArtists,
		},
		Duration:    a.DurationInMillis,
		URL:         a.URL,
		Unavailable: !isCatalogID(id) || (a.PlayParams == nil && !strings.HasPrefix(item.ID, "i.")),
	}
}

// songsOf is the songs among items: music videos and uploads that aren't in the catalog don't play.
func (s *Source) songsOf(items []*resource) []music.Track {
	out := []music.Track{}
	for _, i := range items {
		if i != nil && (i.Type == "songs" || i.Type == "library-songs") {
			out = append(out, s.toTrack(i))
		}
	}
	return out
}

func toAlbum(item *resource) music.Album {
	a := item.attrs()
	id := item.ID
	if a.PlayParams != nil && a.PlayParams.CatalogID != "" {
		id = string(a.PlayParams.CatalogID)
	}
	var artists []music.Artist
	if a.ArtistName != "" {
		artistID := artistIDOf(item)
		if artistID == "" {
			artistID = "name:" + a.ArtistName
		}
		artists = []music.Artist{{ID: artistID, Name: a.ArtistName}}
	}
	return music.Album{ID: id, Name: a.Name, ImageURL: art(a.Artwork), Artists: artists, Released: a.ReleaseDate}
}

func toAlbums(items []*resource) []music.Album {
	out := []music.Album{}
	for _, i := range items {
		if i != nil {
			out = append(out, toAlbum(i))
		}
	}
	return out
}

func toArtist(item *resource) music.Artist {
	a := item.attrs()
	return music.Artist{ID: item.ID, Name: a.Name, ImageURL: art(a.Artwork)}
}

func (s *Source) toPlaylist(item *resource) music.Playlist {
	a := item.attrs()
	library := item.Type == "library-playlists"
	image := art(a.Artwork)
	if library {
		image = s.playlistArt(a.Artwork)
	}
	description := ""
	if a.Description != nil {
		description = a.Description.Standard
		if description == "" {
			description = a.Description.Short
		}
	}
	if description == "" && a.EditorialNotes != nil {
		description = a.EditorialNotes.Short
	}
	return music.Playlist{
		ID:          item.ID,
		Name:        a.Name,
		ImageURL:    image,
		Description: description,
		Owner:       a.CuratorName,
		TrackCount:  a.TrackCount,
		Mine:        library && a.CanEdit,
		Saved:       library && !a.CanEdit,
	}
}

func (s *Source) toPlaylists(items []*resource) []music.Playlist {
	out := []music.Playlist{}
	for _, i := range items {
		if i != nil {
			out = append(out, s.toPlaylist(i))
		}
	}
	return out
}

func toStation(item *resource) music.Station {
	a := item.attrs()
	description := ""
	if a.EditorialNotes != nil {
		description = a.EditorialNotes.Short
	}
	return music.Station{ID: item.ID, Name: a.Name, ImageURL: art(a.Artwork), Description: description}
}
