// Package applemusic is Apple Music, the app's source: the catalog, the
// listener's library and playlists, charts, recommendations, stations and
// word-timed lyrics, from the same API the web player at music.apple.com
// uses.
//
// Songs play whole, as the 256 kbps AAC the web player plays. Apple encrypts
// it with Widevine: the source asks Apple's license server for the key as the
// web player would (package widevine), and amhelper takes the encryption
// off, copying the audio as it is into an .m4a the app plays itself. Nothing
// is re-encoded. With the listener's wrapper, songs play lossless as ALAC
// instead (lossless.go).
//
// It began as the MyGo Music plugin of the same name, written in TypeScript;
// each file here is the module of that name there, and what it keeps on disk
// is under the keys the plugin kept it under.
package applemusic

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"mygo-music/internal/applemusic/amhelper"
	"mygo-music/internal/music"
)

// ID is the source's.
const ID = "apple-music"

// Options are what the app gives the source.
type Options struct {
	// DataDir is the source's own folder: storage.json, settings.json, the
	// lossless/ cache and playlist covers.
	DataDir string
	// WebSignIn shows Apple's sign-in page and answers the Cookie header of
	// CookiesOf's address once it's done.
	WebSignIn func(WebSignIn) (cookie string, err error)
	// Region is the listener's country or region, as two letters, such as
	// jp: signed out, the catalog is the storefront there, if Apple has one,
	// and the US's otherwise.
	Region string
}

// WebSignIn is a sign-in page to show: URL, until a cookie of each name in
// Until is set for CookiesOf's address, as UserAgent.
type WebSignIn struct {
	Name      string
	URL       string
	CookiesOf string
	Until     []string
	UserAgent string
}

// Source is Apple Music. Its methods may be called from several goroutines
// at once.
type Source struct {
	dataDir   string
	webSignIn func(WebSignIn) (string, error)
	region    string

	// Where Apple is, and how it's reached; tests put stand-ins here.
	client                                     *http.Client
	web, amp, playURL, licenseURL, activityURL string
	clk                                        clock
	startHelper                                func(amhelper.Config) (helperServer, error)
	newCDM                                     func(kid []byte) (cdm, error)

	ctx    context.Context
	cancel context.CancelFunc
	bg     sync.WaitGroup
	bgMu   sync.Mutex
	closed bool

	storage  *jsonFile
	settings *jsonFile

	devMu    sync.Mutex
	devToken string

	// settle signs in as the current account, once, before the first answer.
	settle sync.Once
	// migrated turns the one account kept before into the list, once.
	migrated      sync.Once
	acctMu        sync.Mutex
	wrapperFailed *wrapperFailure

	heldMu sync.Mutex
	held   *heldList

	renewMu  sync.Mutex
	renewing *renewCall

	askMu     sync.Mutex
	asking    int
	lastAsked time.Time

	keptMu  sync.Mutex
	entries *omap[*keptEntry]

	libMu sync.Mutex
	lib   libraryState
	// edits are changes to the listener's playlists that Apple took but may
	// not answer with yet, by playlist (playlistEdit).
	edits map[string]*playlistEdit

	warmMu      sync.Mutex
	warmTimer   *warmTimer
	warmRunning bool
	lastWarm    time.Time
	warmGen     int

	helperMu  sync.Mutex
	helper    helperServer
	helperCfg amhelper.Config

	coverMu       sync.Mutex
	coverQueue    []coverJob
	coverFetching int
	coverKeptMu   sync.Mutex

	songsMu   sync.Mutex
	songs     *omap[keptSong]
	resolving map[string]*resolveCall

	qualityMu sync.Mutex
	quality   *string

	llMu      sync.Mutex
	audio     *omap[audioInfo]
	known     *omap[*knownQ]
	keptLL    *omap[keptLossless]
	saving    func() bool
	preparing map[string]*prepareCall
	found     map[string]string // the level each song's playlists were read at
	wokeAt    time.Time

	pfMu      sync.Mutex
	playedAt  time.Time
	warmed    time.Time
	lists     *omap[*songList]
	runs      *omap[[]string]
	stationAt *omap[time.Time]
	queue     []string
	pumping   bool
	startedAt []time.Time

	sfMu   sync.Mutex
	sfInfo map[string]*storefrontInfo

	subMu      sync.Mutex
	subscribed *bool

	// What the player said, and what's left to tell Apple of it (actMu):
	// the song that plays, what the event before was of (for a start's hint),
	// and the events in the order to send them, by one drain at a time.
	actMu     sync.Mutex
	durations *omap[int64]
	reported  told
	hasLast   bool
	last      music.Origin
	outbox    []playEvent
	sending   bool
	flushed   []chan struct{}
}

var (
	_ music.Source     = (*Source)(nil)
	_ music.Recorder   = (*Source)(nil)
	_ music.Flusher    = (*Source)(nil)
	_ music.Favorer    = (*Source)(nil)
	_ music.Keeper     = (*Source)(nil)
	_ music.Autoplayer = (*Source)(nil)
	_ music.Switcher   = (*Source)(nil)
)

// New makes the source. It asks nothing and starts nothing until it's first called.
func New(o Options) *Source {
	ctx, cancel := context.WithCancel(context.Background())
	return &Source{
		dataDir:     o.DataDir,
		webSignIn:   o.WebSignIn,
		region:      o.Region,
		sfInfo:      map[string]*storefrontInfo{},
		client:      &http.Client{},
		web:         webBase,
		amp:         ampBase,
		playURL:     playbackURL,
		licenseURL:  licenseURL,
		activityURL: activityURL,
		clk:         realClock{},
		startHelper: startHelper,
		newCDM:      newWidevine,
		ctx:         ctx,
		cancel:      cancel,
		storage:     &jsonFile{path: filepath.Join(o.DataDir, "storage.json")},
		settings:    &jsonFile{path: filepath.Join(o.DataDir, "settings.json")},
		entries:     newOmap[*keptEntry](),
		lib:         libraryState{changed: map[string]string{}},
		resolving:   map[string]*resolveCall{},
		audio:       newOmap[audioInfo](),
		known:       newOmap[*knownQ](),
		preparing:   map[string]*prepareCall{},
		found:       map[string]string{},
		lists:       newOmap[*songList](),
		runs:        newOmap[[]string](),
		stationAt:   newOmap[time.Time](),
		durations:   newOmap[int64](),
	}
}

// Close stops what runs in the background, amhelper among it, and waits a
// little for it to end.
func (s *Source) Close() {
	s.stopWarming()
	s.bgMu.Lock()
	s.closed = true
	s.bgMu.Unlock()
	s.cancel()
	s.llMu.Lock()
	pending := s.saving != nil && s.saving()
	s.saving = nil
	s.llMu.Unlock()
	if pending {
		s.flushKnown()
	}
	done := make(chan struct{})
	go func() {
		s.bg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	s.stopHelper()
}

func (s *Source) Info() music.Info {
	return music.Info{
		ID:       ID,
		Name:     "Apple Music",
		Playback: music.PlaybackStream,
	}
}

// can is what the plugin exported: what the app may call.
var can = map[string]bool{
	"account": true, "accounts": true, "addToLibrary": true, "addToPlaylist": true, "album": true, "artist": true, "autoplay": true, "browse": true,
	"createPlaylist": true, "deletePlaylist": true, "favorite": true, "home": true, "isLiked": true, "like": true, "liked": true,
	"lyrics": true, "movePlaylistTrack": true, "played": true, "playlist": true, "playlists": true, "recent": true,
	"removeFromPlaylist": true, "renamePlaylist": true, "saveAlbum": true, "savePlaylist": true, "savedAlbums": true, "search": true,
	"setSetting": true, "signIn": true, "signOut": true, "stationNext": true,
	"stations": true, "stream": true, "suggest": true,
}

func (s *Source) Can(what string) bool { return can[what] }

func (s *Source) Setting(key string) string { return s.settings.getString(key) }

// SetSetting keeps the setting. The app asks who's signed in after. None
// signs anyone in or out, but a new wrapper is listed and asked anew; the
// rest is read as it's used.
func (s *Source) SetSetting(key, value string) error {
	s.settings.set(key, value)
	defer s.asked(true)()
	if key == "wrapper" || key == "wrapperKey" {
		s.retryWrapper()
	}
	return nil
}

// SignIn signs in with the window, adding the account and switching to it.
func (s *Source) SignIn() (*music.Account, error) {
	defer s.asked(true)()
	s.stopWarming()
	return s.signIn()
}

// SignOut signs out of the current account, forgetting it, and goes on as
// the one used last of those signed in to here, if there's another.
func (s *Source) SignOut() error {
	defer s.asked(true)()
	s.stopWarming()
	return s.leave()
}

func (s *Source) Accounts(fresh bool) ([]music.AccountChoice, error) {
	defer s.asked(false)()
	return s.accounts(fresh)
}

func (s *Source) UseAccount(id string) (*music.Account, error) {
	defer s.asked(true)()
	s.stopWarming()
	return s.useAccount(id)
}

func (s *Source) AddAccount(token string) (*music.Account, error) {
	defer s.asked(true)()
	s.stopWarming()
	return s.addAccount(tokenIn(token))
}

func (s *Source) RemoveAccount(id string) error {
	defer s.asked(true)()
	if s.current() == id {
		s.stopWarming()
	}
	return s.removeAccount(id)
}

func (s *Source) Account() (*music.Account, error) {
	defer s.asked(true)()
	return s.account()
}

func (s *Source) Search(q string, kind music.SearchKind, offset int) (*music.SearchResult, error) {
	defer s.asked(true)()
	return s.search(q, kind, offset)
}

func (s *Source) Suggest(q string) ([]string, error) {
	defer s.asked(true)()
	return s.suggest(q)
}

func (s *Source) Playlists() ([]music.Playlist, error) {
	defer s.asked(true)()
	lists, err := s.playlists()
	// The app lists them as it comes to the front: what changed is fetched again.
	s.warmLibrary(time.Second)
	return lists, err
}

func (s *Source) Playlist(id string) (*music.Playlist, error) {
	defer s.asked(true)()
	return s.playlist(id)
}

func (s *Source) Album(id string) (*music.AlbumPage, error) {
	defer s.asked(true)()
	return s.album(id)
}

func (s *Source) Artist(id string) (*music.ArtistPage, error) {
	defer s.asked(true)()
	return s.artist(id)
}

func (s *Source) Liked(offset int) (*music.Page, error) {
	defer s.asked(true)()
	return s.liked(offset, false)
}

func (s *Source) Recent() ([]music.Track, error) {
	defer s.asked(true)()
	return s.recent()
}

func (s *Source) Home() (*music.Browse, error) {
	defer s.asked(true)()
	return s.home()
}

func (s *Source) Browse(page string) (*music.Browse, error) {
	defer s.asked(true)()
	return s.browse(page)
}

func (s *Source) Favorite(kind, id string, on bool) error {
	defer s.asked(true)()
	return s.favorite(kind, id, on)
}

func (s *Source) AddToLibrary(kind, id string, on bool) error {
	defer s.asked(true)()
	return s.addToLibrary(kind, id, on)
}

func (s *Source) Autoplay(ids []string) ([]music.Track, error) {
	defer s.asked(true)()
	return s.autoplay(ids)
}

func (s *Source) IsLiked(ids []string) (map[string]bool, error) {
	defer s.asked(true)()
	return s.isLiked(ids)
}

func (s *Source) Like(id string, on bool) error {
	defer s.asked(true)()
	return s.like(id, on)
}

func (s *Source) CreatePlaylist(name string) (*music.Playlist, error) {
	defer s.asked(true)()
	return s.createPlaylist(name)
}

func (s *Source) AddToPlaylist(id string, trackIDs []string) error {
	defer s.asked(true)()
	return s.addToPlaylist(id, trackIDs)
}

func (s *Source) SavePlaylist(id string, on bool) error {
	defer s.asked(true)()
	return s.savePlaylist(id, on)
}

func (s *Source) SaveAlbum(id string, on bool) error {
	defer s.asked(true)()
	return s.saveAlbum(id, on)
}

func (s *Source) SavedAlbums(offset int) (*music.AlbumList, error) {
	defer s.asked(true)()
	return s.savedAlbums(offset, false)
}

func (s *Source) Stations() ([]music.Station, error) {
	defer s.asked(true)()
	return s.stations(false)
}

func (s *Source) StationNext(id string) ([]music.Track, error) {
	defer s.asked(true)()
	return s.stationNext(id)
}

// Lyrics is asked for as a song starts, even one the app plays from its
// cache, without asking for its audio: a song plays, and the wrapper is left
// to sleep.
func (s *Source) Lyrics(id string) (*music.Lyrics, error) {
	defer s.asked(false)()
	s.heard()
	return s.lyrics(id)
}

func (s *Source) Stream(id, quality string) (*music.Stream, error) {
	defer s.asked(false)()
	// After the quality asked for is noted, which says how the songs coming up play.
	defer s.playing(id)
	return s.stream(id, quality)
}

func (s *Source) RenamePlaylist(id, name string) error {
	defer s.asked(true)()
	return s.renamePlaylist(id, name)
}

func (s *Source) DeletePlaylist(id string) error {
	defer s.asked(true)()
	return s.deletePlaylist(id)
}

func (s *Source) RemoveFromPlaylist(id string, trackIDs []string) error {
	defer s.asked(true)()
	return s.removeFromPlaylist(id, trackIDs)
}

func (s *Source) MovePlaylistTrack(id string, from, to int) error {
	defer s.asked(true)()
	return s.movePlaylistTrack(id, from, to)
}

// What Apple Music doesn't do, or the web player's API doesn't let it.

func (s *Source) FollowArtist(string, bool) error { return music.ErrUnsupported }
