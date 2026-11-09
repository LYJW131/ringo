package app

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
	"mygo-music/internal/player"
)

// mainThread takes what has to run on the main thread, which TestMain keeps
// serving: a test runs on another.
var mainThread = make(chan func())

// onMainThread runs fn on the main thread. Without a window nothing else serves
// it, and what MyGo does there, as changing the theme, would wait forever.
func onMainThread(fn func()) {
	done := make(chan struct{})
	mainThread <- func() {
		defer close(done)
		fn()
	}
	<-done
}

// TestMain keeps what the tests write out of the listener's own data, and serves
// the main thread while they run.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mygo-music-test")
	if err != nil {
		panic(err)
	}
	mygo.App.SetPath(mygo.PathUserData, dir)
	result := make(chan int)
	go func() { result <- m.Run() }()
	code := 0
	for running := true; running; {
		select {
		case fn := <-mainThread:
			fn()
		case code = <-result:
			running = false
		}
	}
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeSource is a music.Source that does what its tests set, and answers
// ErrUnsupported to the rest. Can says true of what's in caps, which
// newFakeSource fills with everything; a test deletes what it wants hidden.
type fakeSource struct {
	info music.Info
	caps map[string]bool
	vals map[string]string

	signIn      func() (*music.Account, error)
	signOut     func() error
	account     func() (*music.Account, error)
	search      func(query string, kind music.SearchKind, offset int) (*music.SearchResult, error)
	suggest     func(query string) ([]string, error)
	playlists   func() ([]music.Playlist, error)
	playlist    func(id string) (*music.Playlist, error)
	album       func(id string) (*music.AlbumPage, error)
	artist      func(id string) (*music.ArtistPage, error)
	liked       func(offset int) (*music.Page, error)
	recent      func() ([]music.Track, error)
	home        func() (*music.Browse, error)
	browse      func(page string) (*music.Browse, error)
	isLiked     func(ids []string) (map[string]bool, error)
	like        func(id string, on bool) error
	likeEpisode func(id string, on bool) error
	create      func(name string) (*music.Playlist, error)
	rename      func(id, name string) error
	remove      func(id string) error
	addTo       func(id string, tracks []string) error
	removeFrom  func(id string, tracks []string) error
	move        func(id string, from, to int) error
	save        func(id string, on bool) error
	follow      func(id string, on bool) error
	saveAlbum   func(id string, on bool) error
	savedAlbums func(offset int) (*music.AlbumList, error)
	shows       func() ([]music.Show, error)
	show        func(id string, offset int) (*music.ShowPage, error)
	subscribe   func(id string, on bool) error
	stations    func() ([]music.Station, error)
	stationNext func(id string) ([]music.Track, error)
	comments    func(track string, offset int) (*music.CommentPage, error)
	likeComment func(track, comment string, on bool) error
	postComment func(track, text string) (*music.Comment, error)
	lyrics      func(id string) (*music.Lyrics, error)
	stream      func(id, quality string) (*music.Stream, error)
	setSetting  func(key, value string) error
}

func newFakeSource(id string) *fakeSource {
	s := &fakeSource{info: music.Info{ID: id, Name: id, Playback: music.PlaybackStream}, caps: map[string]bool{}, vals: map[string]string{}}
	for _, name := range capabilities {
		s.caps[name] = true
	}
	return s
}

func (s *fakeSource) Info() music.Info     { return s.info }
func (s *fakeSource) Can(what string) bool { return s.caps[what] }
func (s *fakeSource) Setting(key string) string {
	return s.vals[key]
}
func (s *fakeSource) SetSetting(key, value string) error {
	if s.setSetting != nil {
		return s.setSetting(key, value)
	}
	s.vals[key] = value
	return nil
}

func (s *fakeSource) SignIn() (*music.Account, error) {
	if s.signIn == nil {
		return nil, music.ErrUnsupported
	}
	return s.signIn()
}
func (s *fakeSource) SignOut() error {
	if s.signOut == nil {
		return nil
	}
	return s.signOut()
}
func (s *fakeSource) Account() (*music.Account, error) {
	if s.account == nil {
		return nil, nil
	}
	return s.account()
}
func (s *fakeSource) Search(query string, kind music.SearchKind, offset int) (*music.SearchResult, error) {
	if s.search == nil {
		return &music.SearchResult{}, nil
	}
	return s.search(query, kind, offset)
}
func (s *fakeSource) Suggest(query string) ([]string, error) {
	if s.suggest == nil {
		return nil, nil
	}
	return s.suggest(query)
}
func (s *fakeSource) Playlists() ([]music.Playlist, error) {
	if s.playlists == nil {
		return nil, nil
	}
	return s.playlists()
}
func (s *fakeSource) Playlist(id string) (*music.Playlist, error) {
	if s.playlist == nil {
		return nil, music.ErrUnsupported
	}
	return s.playlist(id)
}
func (s *fakeSource) Album(id string) (*music.AlbumPage, error) {
	if s.album == nil {
		return nil, music.ErrUnsupported
	}
	return s.album(id)
}
func (s *fakeSource) Artist(id string) (*music.ArtistPage, error) {
	if s.artist == nil {
		return nil, music.ErrUnsupported
	}
	return s.artist(id)
}
func (s *fakeSource) Liked(offset int) (*music.Page, error) {
	if s.liked == nil {
		return &music.Page{}, nil
	}
	return s.liked(offset)
}
func (s *fakeSource) Recent() ([]music.Track, error) {
	if s.recent == nil {
		return nil, nil
	}
	return s.recent()
}
func (s *fakeSource) Home() (*music.Browse, error) {
	if s.home == nil {
		return nil, nil
	}
	return s.home()
}
func (s *fakeSource) Browse(page string) (*music.Browse, error) {
	if s.browse == nil {
		return nil, music.ErrUnsupported
	}
	return s.browse(page)
}
func (s *fakeSource) IsLiked(ids []string) (map[string]bool, error) {
	if s.isLiked == nil {
		return map[string]bool{}, nil
	}
	return s.isLiked(ids)
}
func (s *fakeSource) Like(id string, on bool) error {
	if s.like == nil {
		return nil
	}
	return s.like(id, on)
}
func (s *fakeSource) LikeEpisode(id string, on bool) error {
	if s.likeEpisode == nil {
		return nil
	}
	return s.likeEpisode(id, on)
}
func (s *fakeSource) CreatePlaylist(name string) (*music.Playlist, error) {
	if s.create == nil {
		return nil, music.ErrUnsupported
	}
	return s.create(name)
}
func (s *fakeSource) RenamePlaylist(id, name string) error {
	if s.rename == nil {
		return nil
	}
	return s.rename(id, name)
}
func (s *fakeSource) DeletePlaylist(id string) error {
	if s.remove == nil {
		return nil
	}
	return s.remove(id)
}
func (s *fakeSource) AddToPlaylist(id string, tracks []string) error {
	if s.addTo == nil {
		return nil
	}
	return s.addTo(id, tracks)
}
func (s *fakeSource) RemoveFromPlaylist(id string, tracks []string) error {
	if s.removeFrom == nil {
		return nil
	}
	return s.removeFrom(id, tracks)
}
func (s *fakeSource) MovePlaylistTrack(id string, from, to int) error {
	if s.move == nil {
		return nil
	}
	return s.move(id, from, to)
}
func (s *fakeSource) SavePlaylist(id string, on bool) error {
	if s.save == nil {
		return nil
	}
	return s.save(id, on)
}
func (s *fakeSource) FollowArtist(id string, on bool) error {
	if s.follow == nil {
		return nil
	}
	return s.follow(id, on)
}
func (s *fakeSource) SaveAlbum(id string, on bool) error {
	if s.saveAlbum == nil {
		return nil
	}
	return s.saveAlbum(id, on)
}
func (s *fakeSource) SavedAlbums(offset int) (*music.AlbumList, error) {
	if s.savedAlbums == nil {
		return &music.AlbumList{}, nil
	}
	return s.savedAlbums(offset)
}
func (s *fakeSource) Shows() ([]music.Show, error) {
	if s.shows == nil {
		return nil, nil
	}
	return s.shows()
}
func (s *fakeSource) Show(id string, offset int) (*music.ShowPage, error) {
	if s.show == nil {
		return nil, music.ErrUnsupported
	}
	return s.show(id, offset)
}
func (s *fakeSource) SubscribeShow(id string, on bool) error {
	if s.subscribe == nil {
		return nil
	}
	return s.subscribe(id, on)
}
func (s *fakeSource) Stations() ([]music.Station, error) {
	if s.stations == nil {
		return nil, nil
	}
	return s.stations()
}
func (s *fakeSource) StationNext(id string) ([]music.Track, error) {
	if s.stationNext == nil {
		return nil, music.ErrUnsupported
	}
	return s.stationNext(id)
}
func (s *fakeSource) Comments(track string, offset int) (*music.CommentPage, error) {
	if s.comments == nil {
		return &music.CommentPage{}, nil
	}
	return s.comments(track, offset)
}
func (s *fakeSource) LikeComment(track, comment string, on bool) error {
	if s.likeComment == nil {
		return nil
	}
	return s.likeComment(track, comment, on)
}
func (s *fakeSource) PostComment(track, text string) (*music.Comment, error) {
	if s.postComment == nil {
		return nil, music.ErrUnsupported
	}
	return s.postComment(track, text)
}
func (s *fakeSource) Lyrics(id string) (*music.Lyrics, error) {
	if s.lyrics == nil {
		return &music.Lyrics{}, nil
	}
	return s.lyrics(id)
}
func (s *fakeSource) Stream(id, quality string) (*music.Stream, error) {
	if s.stream == nil {
		return nil, music.ErrUnavailable
	}
	return s.stream(id, quality)
}
func (s *fakeSource) RemotePlay(ids []string, start int, position int64) error {
	return music.ErrUnsupported
}
func (s *fakeSource) RemotePause() error              { return music.ErrUnsupported }
func (s *fakeSource) RemoteResume() error             { return music.ErrUnsupported }
func (s *fakeSource) RemoteSeek(position int64) error { return music.ErrUnsupported }
func (s *fakeSource) RemoteVolume(percent int) error  { return music.ErrUnsupported }
func (s *fakeSource) RemoteState() (*music.RemoteState, error) {
	return nil, music.ErrUnsupported
}
func (s *fakeSource) RemoteDevices() ([]music.Device, error) { return nil, music.ErrUnsupported }
func (s *fakeSource) RemoteUse(deviceID string) error        { return music.ErrUnsupported }

var _ music.Source = (*fakeSource)(nil)

// addFake adds a library of src to the app, started as if it had answered.
func addFake(a *app, src music.Source) *library {
	lib := newLibrary(src)
	lib.started, lib.accountAsked = true, true
	a.libs = append(a.libs, lib)
	return lib
}

// newTestApp is an app with one source, a fakeSource called "fake": see fake.
func newTestApp() *app {
	a := &app{signInOver: new(atomic.Pointer[mygo.Window]), settings: defaultSettings(), settingText: map[string]*string{}, lyricAt: -1, queueAt: -1}
	a.library = addFake(a, newFakeSource("fake"))
	a.route = route{page: pageHome, src: "fake"}
	a.art = newArtCache(func(func()) {})
	a.pl = player.New(func(func()) {})
	a.pl.Source = a.sourceByID
	a.pl.SetVolume(0.5)
	return a
}

// fake is the source of an app newTestApp made.
func fake(a *app) *fakeSource { return a.libs[0].src.(*fakeSource) }

// posts collects what the app posts to the main thread, for a test to run with
// runPost once what it waits for is on its way.
func posts(a *app) chan func() {
	ch := make(chan func(), 64)
	a.postFn = func(fn func()) { ch <- fn }
	return ch
}

// runPost runs the next thing posted, waiting for it.
func runPost(t *testing.T, ch chan func()) {
	t.Helper()
	select {
	case fn := <-ch:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was posted")
	}
}

// until runs what's posted until cond holds, for what posts more as it lands.
func until(t *testing.T, ch chan func(), cond func() bool) {
	t.Helper()
	for !cond() {
		runPost(t, ch)
	}
}

// wait returns the next value of ch, which something on its way sends.
func wait[T any](t *testing.T, ch chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatalf("%s wasn't asked for", what)
		panic("unreachable")
	}
}

// without takes capabilities away from the source of an app newTestApp made.
func without(a *app, names ...string) {
	f := fake(a)
	for _, name := range names {
		delete(f.caps, name)
	}
	a.libs[0].caps = probeCaps(f)
}

// song is a track of a source's, as its answers have them.
func song(id, name string, artists ...string) music.Track {
	tr := music.Track{ID: id, Name: name, Duration: 200000}
	for _, ar := range artists {
		tr.Artists = append(tr.Artists, music.Artist{Name: ar})
	}
	return tr
}

// saveFrame saves img as a PNG named name in dir, for an MM_FRAMES test.
func saveFrame(t *testing.T, dir, name string, img image.Image) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// settled shows the view of tt at rest, whatever is on its way in.
func settled(tt *ui.Tester) {
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
}

// coverImage is a cover of its own, top to bottom from one color to another.
func coverImage(from, to ui.Color) *image.RGBA {
	const size = 96
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		c := from.Mix(to, float32(y)/size)
		for x := range size {
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	return img
}

// giveCover makes img the cover at url in the sizes the views ask for, as if
// it had loaded, its backdrop with it.
func giveCover(a *app, url string, img *image.RGBA) {
	art := &artwork{bmp: ui.NewBitmap(img), tint: dominantColor(img)}
	for _, px := range []int{80, 96, 120, 160, 240, 400, 600} {
		a.art.art[sized(url, px)] = art
	}
	bd := *art
	bd.backdrop = ui.NewBitmap(blurImage(img))
	a.art.art[sized(url, 600)+"#backdrop"] = &bd
}
