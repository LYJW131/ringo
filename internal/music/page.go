package music

import (
	"strconv"
	"strings"
)

// Apple Music's pages are laid out by its service: a page is sections in
// the order it gives them, each with a style that says how its items show.
// A source answers them as a Browse of Sections with a Style and Items,
// and the app draws each style.

// The styles of a Section, as Section.Style has them.
const (
	// ShelfHero is the pictures that lead a page, wide and one at a time or
	// nearly, each with a badge, a title and a line or two over it.
	ShelfHero = "hero"
	// ShelfPortrait is cards taller than wide, three to four, in a row that
	// scrolls sideways: Home's picks for the listener. A card is its
	// picture, a tall one that fills it or a cover with its own color
	// below, and over its foot the Badge, then the Artists if it has them,
	// else the Title and Subtitle.
	ShelfPortrait = "portrait"
	// ShelfSquare is covers in a row that scrolls sideways, a title and a
	// line under each: albums, playlists, stations.
	ShelfSquare = "square"
	// ShelfGrid is covers in rows that wrap, all of them: what See All
	// opens, and a library's albums.
	ShelfGrid = "grid"
	// ShelfTracks is songs in short columns that scroll sideways, a few
	// rows to a column, as Top Songs on an artist's page.
	ShelfTracks = "tracks"
	// ShelfList is songs one to a row, all of them.
	ShelfList = "list"
	// ShelfBrick is pictures wider than tall in a row that scrolls
	// sideways, the title under or over each: music videos, categories.
	ShelfBrick = "brick"
	// ShelfCircle is round pictures in a row that scrolls sideways, a name
	// under each: artists.
	ShelfCircle = "circle"
	// ShelfLinks is a list of titles that open pages, a line each.
	ShelfLinks = "links"
	// ShelfLive is what's on the air: a wide picture, the station, the show
	// and its hours.
	ShelfLive = "live"
	// ShelfFeatured is one item shown large beside what follows: an
	// artist's latest release.
	ShelfFeatured = "featured"
	// ShelfText is the section's Subtitle as paragraphs under its Title,
	// without items: an artist's biography, an album's notes.
	ShelfText = "text"
)

// The kinds of Item.
const (
	ItemAlbum    = "album"
	ItemPlaylist = "playlist"
	ItemArtist   = "artist"
	ItemSong     = "song"
	ItemVideo    = "video"
	ItemStation  = "station"
	// ItemLink opens the Browse page Page, or the address URL in the
	// listener's browser when it has no Page.
	ItemLink = "link"
)

// Item is one thing a shelf shows, whatever it is: Kind says which of the
// pointers holds it, for playing it, opening its page and its menu. The
// rest is how it shows, which a shelf may word differently from the thing
// itself, as a hero does.
type Item struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Title and Subtitle are the two lines by its picture.
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	// Badge is a few words over it, such as "NEW ALBUM" or, on a card of
	// Home's picks, why it is there: "Listen Again".
	Badge string `json:"badge,omitempty"`
	// Artists is who is on a playlist, a line of names that Apple cut short
	// with "and more" itself.
	Artists string `json:"artists,omitempty"`
	// Note is what the editors say of it, a sentence or two, where a hero
	// has room for it.
	Note     string `json:"note,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	// WideURL is its picture wider than tall, for heroes and bricks (Wide).
	WideURL string `json:"wideUrl,omitempty"`
	// TallURL is its picture taller than wide, three to four, for the
	// cards of a portrait shelf: its own title and Apple's mark are drawn
	// in it. Its address is asked for as Wide's is.
	TallURL string `json:"tallUrl,omitempty"`
	// VideoURL is its picture moving, an HLS stream.
	VideoURL string `json:"videoUrl,omitempty"`
	Colors   Colors `json:"colors,omitzero"`
	Explicit bool   `json:"explicit,omitempty"`
	Page     string `json:"page,omitempty"`
	URL      string `json:"url,omitempty"`

	Track    *Track    `json:"track,omitempty"` // of ItemSong and ItemVideo
	Album    *Album    `json:"album,omitempty"`
	Playlist *Playlist `json:"playlist,omitempty"`
	Artist   *Artist   `json:"artist,omitempty"`
	Station  *Station  `json:"station,omitempty"`
}

// Colors are those Apple Music gives with a picture, as rrggbb without a #:
// the picture's ground, and four for text over it, the first the plainest
// to read. All empty when the source gave none.
type Colors struct {
	BG    string `json:"bg,omitempty"`
	Text1 string `json:"text1,omitempty"`
	Text2 string `json:"text2,omitempty"`
	Text3 string `json:"text3,omitempty"`
	Text4 string `json:"text4,omitempty"`
}

// Traits are how a song or an album sounds at its best, which its page
// shows as badges.
type Traits struct {
	Lossless      bool `json:"lossless,omitempty"`
	HiRes         bool `json:"hiRes,omitempty"`
	Atmos         bool `json:"atmos,omitempty"`
	DigitalMaster bool `json:"digitalMaster,omitempty"`
}

// A picture wider than tall is asked for by its width and height: its
// address has {w} and {h} where they go, and may have {c}, where a code for
// cropping goes, and {f}, the format. Wide fills them in. A square one has
// {size}, which the app fills in as it fetches it.
func Wide(url string, w, h int) string {
	return wideReplacer(w, h).Replace(url)
}

// The pages a source of Apple Music answers Browse for, by what its Page
// is, besides those it makes up itself and names in a Section's MorePage
// or an Item's Page.
const (
	PageNew           = "new"           // Apple Music's New
	PageRadio         = "radio"         // its Radio
	PageSearchLanding = "search"        // what Search shows before a word is typed
	PageRecentlyAdded = "library:added" // the library, by when things came
	PageLibraryArtist = "library:artists"
	PageLibraryAlbums = "library:albums"
	PageLibrarySongs  = "library:songs"
	PageMadeForYou    = "library:made-for-you"
	PageAllPlaylists  = "library:playlists"
	// PageSearch and a word searched for, as "search:" + query, is all that
	// Apple Music finds for it: its top results, then a section a kind.
	PageSearch = "search:"
)

func wideReplacer(w, h int) *strings.Replacer {
	return strings.NewReplacer("{w}", strconv.Itoa(w), "{h}", strconv.Itoa(h), "{c}", "sr", "{f}", "jpg")
}

// What follows is a source's to do or not, as Recorder is: the app asks Can
// by the method's name, with a small letter first, before it offers any.

// A Favorer keeps the listener's favorites: albums, playlists, artists and
// songs, by an Item's kind.
type Favorer interface {
	Favorite(kind, id string, on bool) error
}

// A Keeper adds what an Item's kind names to the listener's library, a
// song, an album, a playlist or a video, or takes it out.
type Keeper interface {
	AddToLibrary(kind, id string, on bool) error
}

// A LibraryChecker says which of ids, of an Item's kind, are in the
// listener's library, all at once: those it leaves out aren't.
type LibraryChecker interface {
	InLibrary(kind string, ids []string) (map[string]bool, error)
}

// An Autoplayer answers songs to go on with once the queue ends, like
// those of ids, the songs it ended with.
type Autoplayer interface {
	Autoplay(ids []string) ([]Track, error)
}
