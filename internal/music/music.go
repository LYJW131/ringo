// Package music is what the player and its views know of music: tracks,
// albums and the like, whichever source they came from, and what a source
// has to do. Sources are plugins: see internal/plugin.
package music

import (
	"errors"
	"strings"
	"time"
)

// IDs are the source's own and mean nothing outside it.

type Artist struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
	Genre    string `json:"genre,omitempty"`
	// Bio is what Apple Music writes of them, in paragraphs, and Born and
	// Origin when they were born or formed and where they're from.
	Bio    string `json:"bio,omitempty"`
	Born   string `json:"born,omitempty"`
	Origin string `json:"origin,omitempty"`
	// Group says they're a band or the like, for whom Born is when they
	// formed.
	Group bool `json:"group,omitempty"`
	// WideURL is the picture that heads their page, wider than tall (Wide).
	WideURL string `json:"wideUrl,omitempty"`
	// VideoURL is that picture moving, sixteen to nine, as the master
	// playlist of an HLS stream: the head of their page plays it.
	VideoURL string `json:"videoUrl,omitempty"`
	Colors   Colors `json:"colors,omitzero"`
	Favorite bool   `json:"favorite,omitempty"`
	URL      string `json:"url,omitempty"`
}

type Album struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	ImageURL string   `json:"imageUrl,omitempty"`
	Artists  []Artist `json:"artists,omitempty"`
	Released string   `json:"released,omitempty"` // as the source has it, such as 2024-03-01
	// Saved says the listener saved it, for a source with saveAlbum.
	Saved bool `json:"saved,omitempty"`
	// Uploaded says some of its songs are the listener's uploads (see
	// Track.Uploaded), so it stays in their library: deleted, those
	// couldn't be added back. Known on its page.
	Uploaded bool `json:"uploaded,omitempty"`

	Explicit bool   `json:"explicit,omitempty"`
	Traits   Traits `json:"traits,omitzero"`
	Colors   Colors `json:"colors,omitzero"`
	// Notes are what Apple Music's editors wrote of it, in paragraphs, and
	// Tagline a few words of theirs, such as "New Album".
	Notes   string `json:"notes,omitempty"`
	Tagline string `json:"tagline,omitempty"`
	Genre   string `json:"genre,omitempty"`
	// Label and Copyright are as the album's page ends: who released it,
	// and the line that starts with ℗.
	Label string `json:"label,omitempty"`
	// LabelPage is the Browse page of the record label, if it has one.
	LabelPage  string `json:"labelPage,omitempty"`
	Copyright  string `json:"copyright,omitempty"`
	TrackCount int    `json:"trackCount,omitempty"`
	Single     bool   `json:"single,omitempty"`
	// VideoURL is its cover moving, square, as the master playlist of an
	// HLS stream: the head of its page plays it over the cover.
	VideoURL string `json:"videoUrl,omitempty"`
	Favorite bool   `json:"favorite,omitempty"`
	URL      string `json:"url,omitempty"`
}

type Track struct {
	ID string `json:"id"`
	// Source is the ID of the source the track came from, which it plays
	// through. The source's own answers leave it out; it's filled in for them.
	Source   string   `json:"source,omitempty"`
	Name     string   `json:"name"`
	Artists  []Artist `json:"artists,omitempty"`
	Album    Album    `json:"album"`
	Duration int64    `json:"duration"` // milliseconds
	// URL is the track's page at its source, for sharing.
	URL string `json:"url,omitempty"`
	// Unavailable says the listener can't play it, for a source that knows.
	Unavailable bool `json:"unavailable,omitempty"`

	// Number and Disc are its place on its album, from 1, where the source
	// says.
	Number   int    `json:"number,omitempty"`
	Disc     int    `json:"disc,omitempty"`
	Explicit bool   `json:"explicit,omitempty"`
	Traits   Traits `json:"traits,omitzero"`
	// Popular marks the songs of an album most played, which its page stars.
	Popular  bool   `json:"popular,omitempty"`
	Composer string `json:"composer,omitempty"`
	// Video says it's a music video, whose picture is wider than tall.
	Video bool `json:"video,omitempty"`
	// Saved says it's in the listener's library. SavedKnown says the source
	// asked, so that Saved false means it isn't, rather than that no one
	// knows; it isn't kept with the track, as the library changes meanwhile.
	Saved      bool `json:"saved,omitempty"`
	SavedKnown bool `json:"-"`
	// Uploaded says it's a song the listener uploaded to their library,
	// which the source's catalog doesn't have: it plays, and goes on their
	// playlists, but deleted from the library it couldn't be added back.
	Uploaded bool `json:"uploaded,omitempty"`
	// Origin is where it was played from, set as it's queued: the source
	// says it to whoever it tells what plays (Recorder). Pages leave it out.
	Origin Origin `json:"origin,omitzero"`
}

// OriginKind is what a song is played from.
type OriginKind int

const (
	// FromNothing is a song picked from a list that isn't a thing of its own,
	// such as search results, or Home's shelves.
	FromNothing OriginKind = iota
	FromAlbum
	FromPlaylist
	FromStation
	FromArtist
)

// Origin is where a song is played from: the album, playlist, station or
// artist it was started from, as Apple Music's own players tell it of
// what they play.
type Origin struct {
	Kind OriginKind `json:"kind,omitempty"`
	// ID is the thing's own, as its page has it: an album's catalog ID or
	// its library's, a playlist's of either, a station's, an artist's.
	ID string `json:"id,omitempty"`
	// CatalogID is the catalog's ID of a playlist from the listener's library,
	// where it has one.
	CatalogID string `json:"catalogId,omitempty"`
	// Version is a catalog playlist's version, which changes as it does.
	Version string `json:"version,omitempty"`
}

// Same says two songs were played from the same thing.
func (o Origin) Same(other Origin) bool { return o.Kind == other.Kind && o.ID == other.ID }

// ArtistNames joins the track's artists as the credits read.
func (t *Track) ArtistNames() string {
	names := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

func (t *Track) Length() time.Duration { return time.Duration(t.Duration) * time.Millisecond }

type Playlist struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ImageURL    string  `json:"imageUrl,omitempty"`
	Description string  `json:"description,omitempty"`
	Owner       string  `json:"owner,omitempty"`
	TrackCount  int     `json:"trackCount,omitempty"`
	Mine        bool    `json:"mine,omitempty"`  // the listener's own
	Saved       bool    `json:"saved,omitempty"` // saved from someone else
	Fixed       bool    `json:"fixed,omitempty"` // the listener's, but kept by the source: liked songs, say
	Tracks      []Track `json:"tracks,omitempty"`
	// CatalogID is the catalog's ID of one from the listener's library, where
	// it has one, and Version a catalog playlist's version, which changes as
	// it does. They're for Origin.
	CatalogID string `json:"catalogId,omitempty"`
	Version   string `json:"version,omitempty"`

	// OwnerPage is the Browse page of who made it, such as a curator's.
	OwnerPage string `json:"ownerPage,omitempty"`
	// Modified is when it last changed, as the source has it, such as
	// 2026-03-01T08:00:00Z.
	Modified string `json:"modified,omitempty"`
	Colors   Colors `json:"colors,omitzero"`
	Traits   Traits `json:"traits,omitzero"`
	// VideoURL is its cover moving, square, as the master playlist of an
	// HLS stream: the head of its page plays it over the cover.
	VideoURL string `json:"videoUrl,omitempty"`
	Favorite bool   `json:"favorite,omitempty"`
	URL      string `json:"url,omitempty"`
	// Sections follow its songs: its featured artists, more by who made it.
	Sections []Section `json:"sections,omitempty"`
}

// ArtistPage is what an artist's page shows.
type ArtistPage struct {
	Artist Artist  `json:"artist"`
	Tracks []Track `json:"tracks,omitempty"`
	Albums []Album `json:"albums,omitempty"`
	// Sections are the page as Apple Music lays it out, in order: the latest
	// release, top songs, albums, singles, videos, similar artists and the
	// rest. With any, the page shows them, and Tracks and Albums are only
	// what its Play button plays and what older views list.
	Sections []Section `json:"sections,omitempty"`
}

// AlbumPage is what an album's page shows.
type AlbumPage struct {
	Album  Album   `json:"album"`
	Tracks []Track `json:"tracks,omitempty"`
	// Sections follow the songs: more by the artist, other versions, what
	// it's featured on, and what else a listener might like.
	Sections []Section `json:"sections,omitempty"`
	// Complete is, for an album of the listener's library that has only
	// some of its songs, the ID of the whole album, which Show Complete
	// Album opens.
	Complete string `json:"complete,omitempty"`
}

// Account is who is signed in to a source.
type Account struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
	// Page is the Browse page of the listener's own, such as their profile,
	// which the account menu opens as My Page.
	Page string `json:"page,omitempty"`
	// Shared says the account is lent by something else, such as a wrapper
	// that holds it: the listener can switch to another account, but has
	// nothing to sign out of.
	Shared bool `json:"shared,omitempty"`
}

// A Switcher is a source a listener can be signed in to with several
// accounts at once, and switch between them without signing in again.
// Switching is the source's own business, done on this computer: Apple, or
// whoever holds an account, isn't told.
type Switcher interface {
	// Accounts lists what can be switched to: the accounts signed in to
	// here, and those the source can borrow, the current one among them.
	// Others it borrows are listed as last known for a while unless fresh.
	// An error says what kept some of them from being listed, such as a
	// wrapper not answering; the rest are listed all the same.
	Accounts(fresh bool) ([]AccountChoice, error)
	// UseAccount switches to the account of that ID, answering it as
	// Account would. On an error, whoever was signed in stays.
	UseAccount(id string) (*Account, error)
	// AddAccount signs in with a media-user-token, or the cookies it was
	// copied with, and switches to the account. Signing in to one that's
	// there already gives it the new token.
	AddAccount(token string) (*Account, error)
	// RemoveAccount forgets an account signed in to here. Removing the
	// current one is signing out of it.
	RemoveAccount(id string) error
}

// AccountChoice is an account a Switcher can switch to.
type AccountChoice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
	// Region is the account's storefront, or the country or region it's
	// in as whoever lends it says, in lower case; "" when not known.
	Region string `json:"region,omitempty"`
	// Shared says it's lent, as Account.Shared: it can't be removed.
	Shared  bool `json:"shared,omitempty"`
	Current bool `json:"current,omitempty"`
	// Used is when it was last switched to, for a menu to keep to the
	// recent ones; zero if never.
	Used time.Time `json:"used,omitzero"`
}

// SearchKind is what a search looks for.
type SearchKind string

const (
	SearchTracks    SearchKind = "tracks"
	SearchAlbums    SearchKind = "albums"
	SearchArtists   SearchKind = "artists"
	SearchPlaylists SearchKind = "playlists"
)

// SearchResult is a page of what a search found; only the kind searched for
// is filled in.
type SearchResult struct {
	Tracks    []Track    `json:"tracks,omitempty"`
	Albums    []Album    `json:"albums,omitempty"`
	Artists   []Artist   `json:"artists,omitempty"`
	Playlists []Playlist `json:"playlists,omitempty"`
	More      bool       `json:"more,omitempty"` // there is a next page
}

// Len is how many things the page holds.
func (r *SearchResult) Len() int {
	return len(r.Tracks) + len(r.Albums) + len(r.Artists) + len(r.Playlists)
}

// Page is a page of tracks.
type Page struct {
	Tracks []Track `json:"tracks,omitempty"`
	More   bool    `json:"more,omitempty"`
}

// AlbumList is a page of albums.
type AlbumList struct {
	Albums []Album `json:"albums,omitempty"`
	More   bool    `json:"more,omitempty"`
}

// Station is an endless radio: it goes on with what StationNext gives.
type Station struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`

	// Live says it's on the air now, as Apple Music 1 is, rather than made
	// of songs, and Subtitle what's on, or who hosts it.
	Live     bool   `json:"live,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	WideURL  string `json:"wideUrl,omitempty"`
	Colors   Colors `json:"colors,omitzero"`
}

// Browse is a page a source makes up itself, of sections it chooses: what its
// Home shows, and any other opened from a Link or a MorePage.
type Browse struct {
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	// ImageURL is a picture of whom the page is about, such as a listener's
	// avatar, which heads it round, as an artist's photo heads theirs.
	ImageURL string `json:"imageUrl,omitempty"`
	// Description goes under the title, a line to each line of it.
	Description string    `json:"description,omitempty"`
	Sections    []Section `json:"sections,omitempty"`

	// WideURL is a picture that heads the page across its width (Wide), and
	// Colors go with it.
	WideURL string `json:"wideUrl,omitempty"`
	Colors  Colors `json:"colors,omitzero"`
	// Next is the Browse page with what follows this one, for a page that
	// comes in parts, as a library's songs do: its sections go on from the
	// last of this one's, items after items.
	Next string `json:"next,omitempty"`
	// Total is how many items the page has in all, those of Next's parts
	// included, if the source knows. The app sizes what is yet to come by it,
	// and asks for the parts ahead of the listener's scrolling.
	Total int `json:"total,omitempty"`
}

// Section is a heading over what it lists. A section may hold several kinds,
// shown in the order of the fields.
type Section struct {
	Title     string     `json:"title,omitempty"`
	Tracks    []Track    `json:"tracks,omitempty"`
	Albums    []Album    `json:"albums,omitempty"`
	Artists   []Artist   `json:"artists,omitempty"`
	Playlists []Playlist `json:"playlists,omitempty"`
	Stations  []Station  `json:"stations,omitempty"`
	Links     []Link     `json:"links,omitempty"`
	// MorePage is the Browse page with the rest of it, if there is one.
	MorePage string `json:"morePage,omitempty"`
	// Empty is what the section says while it holds nothing, such as "No
	// public playlists". Without it, an empty section shows only its title.
	Empty string `json:"empty,omitempty"`
	// Subtitle is a line under the title. Of a section of text (ShelfText),
	// it is the text.
	Subtitle string `json:"subtitle,omitempty"`

	// Style is how the section is laid out (the Shelf constants). With one,
	// it shows Items, in a shelf that scrolls sideways unless the style says
	// otherwise, and the fields above but Title, Subtitle and MorePage are
	// left alone.
	Style string `json:"style,omitempty"`
	Items []Item `json:"items,omitempty"`

	// Seed is the one thing the section is made from, as "More Like" a
	// certain album: its title shows as Kicker over the Seed's own, beside
	// the Seed's small cover. Title stays the whole sentence.
	Seed   *Item  `json:"seed,omitempty"`
	Kicker string `json:"kicker,omitempty"`
}

// Link opens another Browse page.
type Link struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	Page     string `json:"page"`
}

// Stream is audio the app plays itself: what a source of PlaybackStream
// answers for a track.
type Stream struct {
	URL  string `json:"url"`
	Type string `json:"type"` // m4a: ALAC or AAC in an MP4
	// Quality is the level of Qualities that the stream satisfies, which is
	// the one it was asked for when it plays as that, or "high" where it fell
	// back to AAC: a song asked for at Hi-Res Lossless is "hires" even if it
	// only has 44.1 kHz. The cache keeps a song under it, and offers it for
	// that level and the ones below, so a song kept as "hires" serves a
	// request for "lossless" too, and not the other way round.
	Quality string `json:"quality,omitempty"`
	BitRate int    `json:"bitRate,omitempty"`
	// SampleRate, in Hz, and BitDepth are the lossless audio's, where the
	// source knows them: 0 where it doesn't, as of AAC.
	SampleRate int     `json:"sampleRate,omitempty"`
	BitDepth   int     `json:"bitDepth,omitempty"`
	Size       int64   `json:"size,omitempty"`
	Peak       float64 `json:"peak,omitempty"`
	Gain       float64 `json:"gain,omitempty"`
}

// Lyrics are a track's words, as timed Lines, which can time each word.
type Lyrics struct {
	Lines []LyricLine `json:"lines,omitempty"`
}

// LyricLine is a line of timed lyrics, in milliseconds from the track's start.
type LyricLine struct {
	Start       int64       `json:"start"`
	End         int64       `json:"end,omitempty"`
	Text        string      `json:"text,omitempty"` // its words joined, if left out
	Translation string      `json:"translation,omitempty"`
	Words       []LyricWord `json:"words,omitempty"`
}

// LyricWord is a word of a line, as it's sung.
type LyricWord struct {
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
}

// Playback is how a source's tracks get heard.
type Playback string

const (
	// PlaybackStream sources give audio for the app to play.
	PlaybackStream Playback = "stream"
)

// Info is how a source introduces itself.
type Info struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Version  string   `json:"version,omitempty"`
	Playback Playback `json:"playback"`
}

var (
	// ErrSignIn says the listener has to sign in for this.
	ErrSignIn = errors.New("sign in first")
	// ErrSetup says a setting the source needs is missing.
	ErrSetup = errors.New("the plugin needs setting up")
	// ErrUnavailable says the track can't be played.
	ErrUnavailable = errors.New("not available")
	// ErrUnsupported says the source doesn't do this.
	ErrUnsupported = errors.New("not supported by this source")
)

// Fail is an error of a kind above, with what the source said of it, for the
// listener: what to fill in, or what to install. Reason gives it back.
func Fail(kind error, reason string) error {
	if reason == "" {
		return kind
	}
	return &failure{kind, reason}
}

type failure struct {
	kind   error
	reason string
}

func (f *failure) Error() string { return f.kind.Error() + ": " + f.reason }
func (f *failure) Unwrap() error { return f.kind }

// Reason is what the source said of why err happened, if it said anything.
func Reason(err error) string {
	var f *failure
	if errors.As(err, &f) {
		return f.reason
	}
	return ""
}

// Qualities are the levels of audio the listener can ask a source for, worst
// first, as Apple Music names them: High Quality (AAC, 256 kbps), Lossless
// (ALAC, up to 24-bit/48 kHz) and Hi-Res Lossless (ALAC, up to 24-bit/192 kHz).
var Qualities = []string{"high", "lossless", "hires"}

// IsLossless says whether a quality is one of the ALAC levels.
func IsLossless(quality string) bool { return quality == "lossless" || quality == "hires" }

// RepeatMode is whether the queue plays again, as the player keeps it.
type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatAll
	RepeatOne
)

// Mode is how the player is set to play, which Recorder is told with
// what it's told.
type Mode struct {
	Shuffle  bool
	Repeat   RepeatMode
	Autoplay bool
}

// PlayAction is what happened to the song that plays.
type PlayAction int

const (
	// Started: the song begins, from Position.
	Started PlayAction = iota
	// Resumed: it plays on from Position after a pause.
	Resumed
	// Stopped: it plays no more, for Why, at Position.
	Stopped
	// Sought: it jumped from From to Position, and plays on from there.
	Sought
)

// StopReason is why a song stopped.
type StopReason int

const (
	StopPaused StopReason = iota
	// StopEnded: it played to its end.
	StopEnded
	// StopNext and StopPrev: the listener skipped on to the next song, or
	// back to the one before.
	StopNext
	StopPrev
	// StopChanged: the listener chose something else to play, from another
	// album or playlist, or a song of their own pick.
	StopChanged
	// StopQuit: the app is closing.
	StopQuit
	// StopOther: the source is to hear no more of it, as when the listener
	// turns their history off.
	StopOther
)

// PlayEvent is something that happened to the song that plays, as the
// player knows it, for a Recorder.
type PlayEvent struct {
	Action PlayAction
	Track  Track
	// Position is where the song is in the song itself, not by the clock.
	Position time.Duration
	// From is where a Sought one jumped from.
	From time.Duration
	Why  StopReason
	Mode Mode
}

// A Recorder is a source that wants to know what's played, for its Recent.
// A source that keeps that itself, as a service does, needn't be one. It's
// told on the main thread, in the order things happen, and answers at once:
// what takes a while is its own to do after.
type Recorder interface {
	Played(ev PlayEvent)
}

// A Flusher is a Recorder that can be waited on, for what it's still to
// send as the app quits.
type Flusher interface {
	// Flush returns once what the source was told is sent, or after d.
	Flush(d time.Duration)
}

// A Preloader answers what the app asks for before the listener does, to
// show it as they open it: the pages of the sidebar, their playlists and
// their favorite songs. It answers as the source would to the listener's own
// asking, but without what that sets off: working out the songs on the page
// to play, and waking what plays them. An answer may be kept a while, so
// that the listener's asking for it then is quick.
type Preloader interface {
	PreloadBrowse(page string) (*Browse, error)
	PreloadPlaylist(id string) (*Playlist, error)
	// PreloadLiked is the first page of Liked.
	PreloadLiked() (*Page, error)
}

// Source is where music comes from. Everything but Info and Can may take a
// while and is called off the main thread. A source that doesn't do something
// answers ErrUnsupported, and Can says so beforehand for the views to leave it
// out.
type Source interface {
	Info() Info
	// Can reports whether the source does what's named, by the name of its
	// plugin's function, such as "lyrics" or "like". It answers at once, on
	// the main thread, without starting anything.
	Can(what string) bool

	Setting(key string) string
	SetSetting(key, value string) error

	SignIn() (*Account, error)
	SignOut() error
	Account() (*Account, error)

	Search(query string, kind SearchKind, offset int) (*SearchResult, error)
	// Suggest says what a search being typed may be after.
	Suggest(query string) ([]string, error)
	// Playlists lists those the listener made and those they saved.
	Playlists() ([]Playlist, error)
	Playlist(id string) (*Playlist, error)
	Album(id string) (*AlbumPage, error)
	Artist(id string) (*ArtistPage, error)
	Liked(offset int) (*Page, error)
	Recent() ([]Track, error)

	// Home is what the Home page shows, for a source that makes up its own.
	Home() (*Browse, error)
	// Browse is any other page it makes up.
	Browse(page string) (*Browse, error)

	IsLiked(ids []string) (map[string]bool, error)
	Like(id string, on bool) error

	CreatePlaylist(name string) (*Playlist, error)
	RenamePlaylist(id, name string) error
	DeletePlaylist(id string) error
	AddToPlaylist(id string, trackIDs []string) error
	// RemoveFromPlaylist takes the tracks out, every time each is listed.
	RemoveFromPlaylist(id string, trackIDs []string) error
	// MovePlaylistTrack moves the track at index from to index to, of the
	// tracks as Playlist lists them.
	MovePlaylistTrack(id string, from, to int) error
	// SavePlaylist saves someone else's playlist, or lets it go.
	SavePlaylist(id string, on bool) error

	FollowArtist(id string, on bool) error
	SaveAlbum(id string, on bool) error
	SavedAlbums(offset int) (*AlbumList, error)

	Stations() ([]Station, error)
	// StationNext answers the next tracks of a station.
	StationNext(id string) ([]Track, error)

	Lyrics(id string) (*Lyrics, error)

	// Stream answers audio for a track.
	Stream(id, quality string) (*Stream, error)
}
