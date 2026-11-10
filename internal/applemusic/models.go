package applemusic

import (
	"bytes"
	"encoding/json"
	"html"
	"regexp"
	"slices"
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

// text is a title Apple gives as a string in one place (rooms, the
// artist's shelves) and as {stringForDisplay} in another (recommendations).
type text string

func (t *text) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '{' {
		var o struct {
			StringForDisplay string `json:"stringForDisplay"`
		}
		if err := json.Unmarshal(b, &o); err != nil {
			return err
		}
		*t = text(o.StringForDisplay)
		return nil
	}
	var s string
	json.Unmarshal(b, &s)
	*t = text(s)
	return nil
}

// notes are what editors wrote: an object of a short and a standard text
// on albums and playlists, a plain string on a page's paragraph.
type notes struct {
	Short    string `json:"short"`
	Standard string `json:"standard"`
	Name     string `json:"name"`
	Tagline  string `json:"tagline"`
}

func (n *notes) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &n.Standard)
	}
	type plain notes
	return json.Unmarshal(b, (*plain)(n))
}

// long is the fuller of the two texts.
func (n *notes) long() string {
	if n == nil {
		return ""
	}
	return orT(n.Standard, n.Short)
}

func (n *notes) tag() string {
	if n == nil {
		return ""
	}
	return n.Tagline
}

func (n *notes) brief() string {
	if n == nil {
		return ""
	}
	return orT(n.Short, n.Standard)
}

type artwork struct {
	URL        string `json:"url,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	BgColor    string `json:"bgColor,omitempty"`
	TextColor1 string `json:"textColor1,omitempty"`
	TextColor2 string `json:"textColor2,omitempty"`
	TextColor3 string `json:"textColor3,omitempty"`
	TextColor4 string `json:"textColor4,omitempty"`
	// RecommendedCropCodes are the crops Apple made for the picture, of an
	// artist's hero: see heroCrop.
	RecommendedCropCodes []string `json:"recommendedCropCodes,omitempty"`
}

type playParams struct {
	ID          string     `json:"id,omitempty"`
	Kind        string     `json:"kind,omitempty"`
	CatalogID   flexString `json:"catalogId,omitempty"`
	ReportingID string     `json:"reportingId,omitempty"`
	// Reporting is false for a song the listener uploaded, whose plays Apple
	// isn't told of.
	Reporting *bool `json:"reporting,omitempty"`
}

type video struct {
	Video        string   `json:"video"`
	PreviewFrame *artwork `json:"previewFrame"`
}

type link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
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
	// Description is a playlist's, and a page's paragraph (a plain string).
	Description         *notes `json:"description"`
	EditorialNotes      *notes `json:"editorialNotes"`
	PlainEditorialNotes *notes `json:"plainEditorialNotes"`
	CuratorName         string `json:"curatorName"`
	TrackCount          int    `json:"trackCount"`
	CanEdit             bool   `json:"canEdit"`
	CanDelete           *bool  `json:"canDelete"`
	HasCatalog          bool   `json:"hasCatalog"`
	LastModifiedDate    string `json:"lastModifiedDate"`
	IsLive              bool   `json:"isLive"`
	// Title is a recommendation's, a room's or a shelf's. A recommendation
	// made from one thing the listener has (More Like …) also has its title
	// without that thing's name.
	Title             text     `json:"title"`
	TitleWithoutName  text     `json:"titleWithoutName"`
	ArtistNames       string   `json:"artistNames"` // of a playlist
	Value             int      `json:"value"`
	TTML              string   `json:"ttml"`
	TTMLLocalizations string   `json:"ttmlLocalizations"`
	AudioTraits       []string `json:"audioTraits"` // such as lossless, hi-res-lossless, atmos
	AvatarArtwork     *artwork `json:"avatarArtwork"`
	ExtendedAssetUrls *struct {
		EnhancedHls string `json:"enhancedHls"`
	} `json:"extendedAssetUrls"`

	ContentRating        string              `json:"contentRating"`
	IsAppleDigitalMaster bool                `json:"isAppleDigitalMaster"`
	IsMasteredForItunes  bool                `json:"isMasteredForItunes"`
	TrackNumber          int                 `json:"trackNumber"`
	DiscNumber           int                 `json:"discNumber"`
	ComposerName         string              `json:"composerName"`
	GenreNames           []string            `json:"genreNames"`
	Copyright            string              `json:"copyright"`
	RecordLabel          string              `json:"recordLabel"`
	IsSingle             bool                `json:"isSingle"`
	EditorialArtwork     map[string]*artwork `json:"editorialArtwork"`
	EditorialVideo       map[string]*video   `json:"editorialVideo"`

	// An artist's.
	ArtistBio    string `json:"artistBio"`
	BornOrFormed string `json:"bornOrFormed"`
	IsGroup      bool   `json:"isGroup"`
	Origin       string `json:"origin"`
	Hero         []struct {
		Content []struct {
			Artwork *artwork `json:"artwork"`
		} `json:"content"`
	} `json:"hero"`

	// An editorial element's: what it is, and the words over it.
	EditorialElementKind string   `json:"editorialElementKind"`
	DesignBadge          string   `json:"designBadge"`
	DesignTag            string   `json:"designTag"`
	ResourceTypes        []string `json:"resourceTypes"`
	Links                []link   `json:"links"`
	Link                 *link    `json:"link"`
	// A multiroom's head.
	Uber *struct {
		MasterArt        *artwork `json:"masterArt"`
		BackgroundColor  string   `json:"backgroundColor"`
		PrimaryTextColor string   `json:"primaryTextColor"`
		TitleTextColor   string   `json:"titleTextColor"`
	} `json:"uber"`
	// How a recommendation (kind) or an artist's shelf (style) shows.
	Display *struct {
		Kind  string `json:"kind"`
		Style string `json:"style"`
	} `json:"display"`
	HasSeeAll bool `json:"hasSeeAll"`
	// Kind is a station's (streaming, editorial, artistOnly) or a shelf's (TOP_SONGS).
	Kind         string `json:"kind"`
	ShowHostName string `json:"showHostName"`

	// What the listener has of it, asked for by name (personal).
	InFavorites bool `json:"inFavorites"`
	InLibrary   bool `json:"inLibrary"`

	// A storefront's.
	DefaultLanguageTag    string   `json:"defaultLanguageTag"`
	SupportedLanguageTags []string `json:"supportedLanguageTags"`
}

type relationship struct {
	Data []*resource `json:"data"`
	Next string      `json:"next"`
	Href string      `json:"href"`
	// Attributes are a view's: its title, in the language asked for.
	Attributes *struct {
		Title string `json:"title"`
	} `json:"attributes"`
}

func (r *relationship) title() string {
	if r == nil || r.Attributes == nil {
		return ""
	}
	return r.Attributes.Title
}

type resource struct {
	ID            string                   `json:"id"`
	Type          string                   `json:"type"`
	Href          string                   `json:"href"`
	Attributes    *attributes              `json:"attributes"`
	Relationships map[string]*relationship `json:"relationships"`
	Views         map[string]*relationship `json:"views"`
	// Meta is what a pointer says of what it points at in its place, such
	// as how much an album's song is played, or why a recommendation has it.
	Meta *struct {
		Popularity float64 `json:"popularity"`
		Reason     text    `json:"reason"`
	} `json:"meta"`
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

// body is most answers: a page of resources. Asked with
// format[resources]=map, as the web player asks, data and relationships
// only point into Resources, by type and ID.
type body struct {
	Data      []*resource                     `json:"data"`
	Next      string                          `json:"next"`
	Resources map[string]map[string]*resource `json:"resources"`
	Meta      *struct {
		Results struct {
			Order []string `json:"order"`
		} `json:"results"`
	} `json:"meta"`
}

// get is what ref points at: itself, when Apple put it in place (no map),
// or nil when it points at nothing the answer has, as an answer cut short
// does. Nothing reads a nil.
func (b *body) get(ref *resource) *resource {
	if ref == nil {
		return nil
	}
	if b != nil {
		if r := b.Resources[ref.Type][ref.ID]; r != nil {
			return r
		}
	}
	if ref.Attributes != nil {
		return ref
	}
	return nil
}

// all is what refs point at, leaving out what they don't.
func (b *body) all(refs []*resource) []*resource {
	out := make([]*resource, 0, len(refs))
	for _, ref := range refs {
		if r := b.get(ref); r != nil {
			out = append(out, r)
		}
	}
	return out
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

// uploaded says item is a song the listener uploaded to their library,
// which the catalog doesn't have: it plays by its library ID alone. A song
// the catalog no longer has comes without playParams, and doesn't play.
func uploaded(item *resource) bool {
	play := item.attrs().PlayParams
	return item.Type == "library-songs" && play != nil && play.Kind == "song" &&
		isLibrarySong(play.ID) && play.CatalogID == "" && !isCatalogID(play.ReportingID)
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

// colors are those Apple gives with a picture.
func colors(a *artwork) music.Colors {
	if a == nil {
		return music.Colors{}
	}
	return music.Colors{BG: a.BgColor, Text1: a.TextColor1, Text2: a.TextColor2, Text3: a.TextColor3, Text4: a.TextColor4}
}

// traits are how it sounds at its best: the web player shows a badge for
// each of these audioTraits, and Digital Master for either flag.
func traits(a *attributes) music.Traits {
	var t music.Traits
	for _, x := range a.AudioTraits {
		switch x {
		case "lossless":
			t.Lossless = true
		case "hi-res-lossless":
			t.HiRes = true
		case "atmos":
			t.Atmos = true
		}
	}
	t.DigitalMaster = a.IsAppleDigitalMaster || a.IsMasteredForItunes
	return t
}

func explicit(a *attributes) bool { return a.ContentRating == "explicit" }

func genre(a *attributes) string {
	if len(a.GenreNames) > 0 {
		return a.GenreNames[0]
	}
	return ""
}

// wide is the first of the editorial pictures named that there is, as its
// template with {w}, {h}, {c} and {f} left for music.Wide.
func wide(a *attributes, names ...string) (string, *artwork) {
	for _, n := range names {
		if w := a.EditorialArtwork[n]; w != nil && w.URL != "" {
			return w.URL, w
		}
	}
	return "", nil
}

// motion is the first of the moving pictures named that there is.
func motion(a *attributes, names ...string) string {
	for _, n := range names {
		if v := a.EditorialVideo[n]; v != nil && v.Video != "" {
			return v.Video
		}
	}
	return ""
}

// The pictures, by what shows them: wider than tall, across a hero or a
// page's head; taller than wide, for a card of Home's picks (superHeroTall
// has the picture's title and Apple's mark drawn in it, staticDetailTall
// neither); and the cover moving, square.
var (
	heroArt    = []string{"subscriptionHero", "superHeroWide", "storeFlowcase", "bannerUber", "epicStageWide", "centeredFullscreenBackground"}
	tallArt    = []string{"superHeroTall", "staticDetailTall"}
	brickArt   = []string{"subscriptionCover", "superHeroWide", "subscriptionHero", "storeFlowcase", "bannerUber"}
	squareMove = []string{"motionDetailSquare", "motionSquareVideo1x1", "motionArtistSquare1x1"}
	wideMove   = []string{"motionWideVideo21x9", "motionMediumVideo16x9", "motionArtistWide16x9", "motionArtistFullscreen16x9", "motionDetailTall"}
)

var tags = regexp.MustCompile(`<[^>]*>`)

// plain is editors' HTML as text: tags off, entities read, paragraphs as
// they are.
func plain(s string) string {
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	return strings.TrimSpace(html.UnescapeString(tags.ReplaceAllString(s, "")))
}

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
	own := uploaded(item)
	return music.Track{
		ID:      id,
		Name:    a.Name,
		Artists: artists,
		Album: music.Album{
			ID:       albumIDOf(item),
			Name:     a.AlbumName,
			ImageURL: s.keptArt(a.Artwork),
			Artists:  albumArtists,
		},
		Duration:    a.DurationInMillis,
		URL:         a.URL,
		Unavailable: !own && (!isCatalogID(id) || (a.PlayParams == nil && !strings.HasPrefix(item.ID, "i."))),
		Uploaded:    own,
		Number:      a.TrackNumber,
		Disc:        a.DiscNumber,
		Explicit:    explicit(a),
		Traits:      traits(a),
		Composer:    a.ComposerName,
		Video:       strings.HasSuffix(item.Type, "music-videos"),
	}
}

// songsOf is the songs among items, music videos left out.
func (s *Source) songsOf(items []*resource) []music.Track {
	out := []music.Track{}
	for _, i := range items {
		if i != nil && (i.Type == "songs" || i.Type == "library-songs") {
			out = append(out, s.toTrack(i))
		}
	}
	return out
}

func (s *Source) toAlbum(item *resource) music.Album {
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
	written := a.EditorialNotes
	if written == nil {
		written = a.PlainEditorialNotes
	}
	tagline := a.EditorialNotes.tag()
	return music.Album{
		ID: id, Name: a.Name, ImageURL: s.keptArt(a.Artwork), Artists: artists, Released: a.ReleaseDate,
		Explicit:   explicit(a),
		Traits:     traits(a),
		Colors:     colors(a.Artwork),
		Notes:      plain(written.long()),
		Tagline:    tagline,
		Genre:      genre(a),
		Label:      a.RecordLabel,
		Copyright:  a.Copyright,
		TrackCount: a.TrackCount,
		Single:     a.IsSingle,
		VideoURL:   motion(a, squareMove...),
		URL:        a.URL,
	}
}

func (s *Source) toAlbums(items []*resource) []music.Album {
	out := []music.Album{}
	for _, i := range items {
		if i != nil {
			out = append(out, s.toAlbum(i))
		}
	}
	return out
}

func toArtist(item *resource) music.Artist {
	a := item.attrs()
	out := music.Artist{ID: item.ID, Name: a.Name, ImageURL: art(a.Artwork), Genre: genre(a), URL: a.URL, Colors: colors(a.Artwork)}
	out.Bio = plain(a.ArtistBio)
	if out.Bio == "" {
		out.Bio = plain(a.PlainEditorialNotes.long())
	}
	out.Born, out.Origin, out.Group = a.BornOrFormed, a.Origin, a.IsGroup
	// The head of their page, as the web player picks it: the editors'
	// background made for it, else their photo cropped around them by a crop
	// Apple made for it, kept as its template; else another editorial one.
	if u, w := wide(a, "centeredFullscreenBackground"); u != "" {
		out.WideURL, out.Colors = u, colors(w)
	}
	for _, h := range a.Hero {
		for _, c := range h.Content {
			if c.Artwork != nil && c.Artwork.URL != "" && out.WideURL == "" {
				out.WideURL, out.Colors = heroCrop(c.Artwork), colors(c.Artwork)
			}
		}
	}
	if out.WideURL == "" {
		if u, w := wide(a, "bannerUber", "superHeroWide", "subscriptionHero", "storeFlowcase"); u != "" {
			out.WideURL, out.Colors = u, colors(w)
		}
	}
	out.VideoURL = motion(a, wideMove...)
	return out
}

// heroCrops are the crops of an artist's photo that keep them in a wide
// one, the best first: the web player's for its hero (HeroGalleryCrop),
// then the others Apple recommends. A photo is square, and cut wide as
// music.Wide does (sr), from its middle, it showed only their body.
var heroCrops = []string{"vf", "vb", "vn", "va"}

// heroCrop is an artist's photo as a template of a wide picture, its crop
// one Apple recommends for it, if any.
func heroCrop(w *artwork) string {
	for _, c := range heroCrops {
		if slices.Contains(w.RecommendedCropCodes, c) {
			return strings.Replace(w.URL, "{c}", c, 1)
		}
	}
	return w.URL
}

func (s *Source) toPlaylist(item *resource) music.Playlist {
	a := item.attrs()
	library := item.Type == "library-playlists"
	image := art(a.Artwork)
	if library {
		image = s.keptArt(a.Artwork)
	}
	description := a.Description.long()
	if description == "" {
		description = a.EditorialNotes.brief()
	}
	return music.Playlist{
		ID:          item.ID,
		Name:        a.Name,
		ImageURL:    image,
		Description: plain(description),
		Owner:       a.CuratorName,
		TrackCount:  a.TrackCount,
		Mine:        library && a.CanEdit,
		Saved:       library && !a.CanEdit,
		Modified:    a.LastModifiedDate,
		Colors:      colors(a.Artwork),
		Traits:      traits(a),
		VideoURL:    motion(a, squareMove...),
		URL:         a.URL,
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
	out := music.Station{ID: item.ID, Name: a.Name, ImageURL: art(a.Artwork), Description: plain(a.EditorialNotes.brief()), Colors: colors(a.Artwork)}
	// Live radio is a stream, as Apple Music 1 is, not songs picked one by one.
	out.Live = a.IsLive || a.Kind == "streaming"
	out.Subtitle = orT(a.EditorialNotes.tag(), a.ShowHostName)
	if out.Subtitle == "" && out.Live {
		out.Subtitle = out.Description
	}
	if u, w := wide(a, "bannerUber", "subscriptionHero", "superHeroWide", "storeFlowcase", "backgroundStatic1x1"); u != "" {
		out.WideURL = u
		if out.Colors.BG == "" {
			out.Colors = colors(w)
		}
	}
	return out
}
