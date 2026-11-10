package app

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// speakNow is an album's page as Apple Music has it: its songs on two
// discs, the last a video, and a section after them.
func speakNow() *music.AlbumPage {
	ts := music.Artist{ID: "159260351", Name: "Taylor Swift", URL: "https://music.apple.com/cn/artist/159260351"}
	al := music.Album{ID: "1440755169", Name: "Speak Now", Artists: []music.Artist{ts}, ImageURL: "cover:1",
		Released: "2010-01-01", Genre: "Pop", Copyright: "℗ 2010 Taylor Swift", TrackCount: 6,
		Traits: music.Traits{Lossless: true, DigitalMaster: true}, Colors: music.Colors{BG: "2b1a14", Text1: "f7e9e1"},
		Notes: "Taylor Swift's third album is named for its songs' theme: things she should have said, and now says.\nIt was going to be called Enchanted.",
		URL:   "https://music.apple.com/cn/album/1440755169"}
	var tracks []music.Track
	for i, name := range []string{"Mine (POP Mix)", "Sparks Fly", "Back To December", "Speak Now", "Dear John", "Mine"} {
		disc, n := 1, i+1
		if i >= 4 {
			disc, n = 2, i-3
		}
		tracks = append(tracks, music.Track{ID: fmt.Sprint("t", i), Source: "fake", Name: name, Artists: []music.Artist{ts},
			Album: al, Duration: int64(200000 + 30000*i), Number: n, Disc: disc, Popular: i == 1 || i == 2, Video: i == 5})
	}
	more := []music.Item{}
	for i, name := range []string{"Speak Now (Taylor's Version)", "Speak Now World Tour Live", "Red", "1989"} {
		more = append(more, music.Item{Kind: music.ItemAlbum, ID: fmt.Sprint("m", i), Title: name, Subtitle: "Taylor Swift", ImageURL: fmt.Sprintf("cover:%d", i%5)})
	}
	return &music.AlbumPage{Album: al, Tracks: tracks, Sections: []music.Section{
		{Title: "More By Taylor Swift", Style: music.ShelfSquare, Items: more},
	}}
}

// albumApp is signed in and shows speakNow's page.
func albumApp() *app {
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	without(a, "isLiked")
	page := speakNow()
	a.albums[page.Album.ID] = &albumData{album: page}
	a.route = route{page: pageAlbum, src: "fake", id: page.Album.ID}
	return a
}

// The artist's name heading an album's page opens their page.
func TestAlbumArtistOpensTheirPage(t *testing.T) {
	a := albumApp()
	fake(a).artist = func(string) (*music.ArtistPage, error) { return nil, errors.New("not now") }
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	r, ok := tt.Find("Taylor Swift")
	if !ok {
		t.Fatal("no artist")
	}
	tt.ClickAt(r.X+4, r.Y+r.H/2)
	tt.Frame()
	if a.route != (route{page: pageArtist, src: "fake", id: "159260351"}) {
		t.Fatalf("opened %+v", a.route)
	}
}

// A click on a song of an album plays the album from it.
func TestAlbumSongPlaysAlbum(t *testing.T) {
	a := albumApp()
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("Back To December"); err != nil {
		t.Fatal(err)
	}
	cur := a.pl.Current()
	if cur == nil || cur.ID != "t2" || len(a.pl.Queue()) != 6 || a.pl.Queue()[0].ID != "t0" {
		t.Fatalf("playing %+v of %d", cur, len(a.pl.Queue()))
	}
}

// A click on a greyed-out song plays nothing, and says why, rather than
// doing nothing at all.
func TestAlbumUnavailableSongSaysWhy(t *testing.T) {
	a := albumApp()
	a.albums["1440755169"].album.Tracks[2].Unavailable = true
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("Back To December"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if a.pl.Current() != nil {
		t.Fatalf("playing %+v", a.pl.Current())
	}
	if _, ok := tt.Find("This song isn't available in your country or region"); !ok {
		t.Fatal("no word of why")
	}
}

// An album of several discs has each disc's number over its songs, and one
// of a disc none.
func TestAlbumRowsByDisc(t *testing.T) {
	tracks := speakNow().Tracks
	var got []string
	for _, r := range albumRows(tracks, 2) {
		got = append(got, fmt.Sprint(r.kind, ":", r.i))
	}
	want := []string{"0:1", "1:0", "1:1", "1:2", "1:3", "0:2", "1:4", "1:5", "2:0", "3:0", "3:1"}
	if !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	for i := range tracks {
		tracks[i].Disc = 1
	}
	for _, r := range albumRows(tracks, 0) {
		if r.kind == albumDisc {
			t.Fatal("a disc's number on an album of one")
		}
	}
}

// Add to Library adds the album with the source at once, reading Added;
// one the source refuses goes back, saying so.
func TestAlbumAddToLibrary(t *testing.T) {
	type call struct {
		kind, id string
		on       bool
	}
	calls := make(chan call, 4)
	var answer error
	a := albumApp()
	ch := posts(a)
	fake(a).addToLibrary = func(kind, id string, on bool) error { calls <- call{kind, id, on}; return answer }
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("Add to Library"); err != nil {
		t.Fatal(err)
	}
	if c := wait(t, calls, "the adding"); c != (call{music.ItemAlbum, "1440755169", true}) {
		t.Fatalf("asked for %+v", c)
	}
	tt.Frame()
	if _, ok := tt.Find("Added"); !ok {
		t.Fatal("not Added at once")
	}
	runPost(t, ch)
	tt.Frame()

	answer = errors.New("offline")
	a.toast = ""
	if err := tt.Click("Added"); err != nil {
		t.Fatal(err)
	}
	if c := wait(t, calls, "the deleting"); c.on {
		t.Fatalf("asked for %+v", c)
	}
	runPost(t, ch)
	if a.toast == "" {
		t.Error("a delete the source refused said nothing")
	}
	tt.Frame()
	if _, ok := tt.Find("Added"); !ok {
		t.Error("a delete the source refused stayed")
	}
}

// In a narrow window, the header's buttons all stay in it, Add to Library
// down to its icon: they ran past the window's edge, which hid the page's
// menu.
func TestNarrowHeaderKeepsItsButtons(t *testing.T) {
	a := albumApp()
	added := make(chan bool, 1)
	fake(a).addToLibrary = func(kind, id string, on bool) error { added <- on; return nil }
	const w = 880
	tt := ui.NewTester(a.view, w, 580)
	tt.Frame()
	for _, label := range []string{"Play", "Shuffle", "Add to Library", "More"} {
		r, ok := tt.Find(label)
		if !ok || r.X+r.W > w-pagePad+1 {
			t.Fatalf("%s at %+v, past the page's edge at %d", label, r, w-pagePad)
		}
	}
	if err := tt.Click("Add to Library"); err != nil {
		t.Fatal(err)
	}
	if !wait(t, added, "the adding") {
		t.Fatal("deleted, not added")
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if len(tt.Menu()) == 0 {
		t.Error("the page's menu didn't open")
	}
}

// A toast shows above the player, which it covered at the foot of the
// window.
func TestToastClearsThePlayer(t *testing.T) {
	a := albumApp()
	const h = 580
	tt := ui.NewTester(a.view, 880, h)
	tt.Frame()
	a.toast = "Link Copied"
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	r, ok := tt.Find("Link Copied")
	if !ok {
		t.Fatal("no toast")
	}
	if r.Y+r.H > h-playerBarHeight {
		t.Errorf("toast at %+v, over the player from %d", r, h-playerBarHeight)
	}
}

// A page that failed for want of an answer says why without the address it
// asked, which ran across the page.
func TestProblemLeavesOutTheAddress(t *testing.T) {
	a := newTestApp()
	err := fmt.Errorf("browse: %w", &url.Error{Op: "Get", URL: "https://amp-api.music.apple.com/v1/editorial/cn/groupings?platform=web", Err: errors.New("no such host")})
	if msg := a.problem(a.library, err, "Couldn't load this page"); strings.Contains(msg, "https://") || !strings.Contains(msg, "no such host") {
		t.Errorf("says %q", msg)
	}
}

// Favorite in an album's menu makes it a favorite with the source, and the
// menu then offers to undo it.
func TestAlbumFavorite(t *testing.T) {
	type call struct {
		kind, id string
		on       bool
	}
	calls := make(chan call, 4)
	a := albumApp()
	fake(a).favorite = func(kind, id string, on bool) error { calls <- call{kind, id, on}; return nil }
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Favorite"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	if c := wait(t, calls, "the favorite"); c != (call{music.ItemAlbum, "1440755169", true}) {
		t.Fatalf("asked for %+v", c)
	}
	tt.Frame()
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Undo Favorite") {
		t.Errorf("the menu shows %q", tt.Menu())
	}
	tt.CloseMenu()
}

// An album in the library with songs the listener uploaded isn't offered to
// be deleted from it, from its menu or its Added button: the uploads
// couldn't be added back. One without is.
func TestAlbumWithUploadsStays(t *testing.T) {
	for _, uploads := range []bool{true, false} {
		asked := make(chan bool, 4)
		a := albumApp()
		page := a.albums["1440755169"].album
		page.Album.Saved, page.Album.Uploaded = true, uploads
		fake(a).addToLibrary = func(kind, id string, on bool) error { asked <- on; return nil }
		tt := ui.NewTester(a.view, 1180, 900)
		tt.Frame()
		if err := tt.Click("More"); err != nil {
			t.Fatal(err)
		}
		if offered := slices.Contains(tt.Menu(), "Delete From Library"); offered == uploads {
			t.Errorf("uploads %v: the menu shows %q", uploads, tt.Menu())
		}
		tt.CloseMenu()
		tt.Frame()
		if !uploads {
			continue
		}
		if err := tt.Click("Added"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		select {
		case on := <-asked:
			t.Errorf("asked the source to set it in the library: %v", on)
		default:
		}
		if _, ok := tt.Find("Songs you uploaded can't be added back once deleted, so this album stays in your library"); !ok {
			t.Error("no word of why")
		}
	}
}

// The star by an artist's name makes them a favorite with the source.
func TestArtistFavorite(t *testing.T) {
	asked := make(chan string, 1)
	a := artistApp(false)
	fake(a).favorite = func(kind, id string, on bool) error { asked <- kind + " " + id; return nil }
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("Favorite"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, asked, "the favorite"); got != "artist 159260351" {
		t.Errorf("asked for %q", got)
	}
}

// Play Next puts a song after the one playing, and Play Last after the rest
// of the queue, the song playing playing on.
func TestPlayNextAndLast(t *testing.T) {
	a := albumApp()
	queue := []music.Track{song("q1", "一"), song("q2", "二"), song("q3", "三")}
	for i := range queue {
		queue[i].Source = "fake"
	}
	a.pl.Cue(queue)
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	for _, c := range []struct {
		item, song string
		want       []string
	}{
		{"Play Next", "Sparks Fly", []string{"q1", "t1", "q2", "q3"}},
		{"Play Last", "Dear John", []string{"q1", "t1", "q2", "q3", "t4"}},
	} {
		if err := tt.RightClick(c.song); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
		order, at := a.pl.PlayOrder()
		var got []string
		for _, i := range order {
			got = append(got, a.pl.Queue()[i].ID)
		}
		if !slices.Equal(got, c.want) || at != 0 || a.pl.Current().ID != "q1" {
			t.Errorf("%s: the queue plays %q from %d, playing %+v", c.item, got, at, a.pl.Current())
		}
	}
}

// Near the end of the queue, AutoPlay asks the source for songs like the
// last ones and queues them after; switched off, it asks for none.
func TestAutoplay(t *testing.T) {
	asked := make(chan []string, 4)
	a := newTestApp()
	ch := posts(a)
	fake(a).caps["autoplay"] = true
	a.libs[0].caps = probeCaps(fake(a))
	fake(a).autoplay = func(ids []string) ([]music.Track, error) {
		asked <- ids
		return []music.Track{song("a1", "像这样"), song("q2", "二"), song("a2", "也像")}, nil
	}
	queue := []music.Track{song("q1", "一"), song("q2", "二")}
	for i := range queue {
		queue[i].Source = "fake"
	}
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	a.pl.PlayQueue(queue, 0)
	tt.Frame()
	if ids := wait(t, asked, "songs to go on with"); !slices.Equal(ids, []string{"q1", "q2"}) {
		t.Fatalf("asked with %q", ids)
	}
	until(t, ch, func() bool { return len(a.pl.Queue()) > 2 })
	var got []string
	for _, tr := range a.pl.Queue() {
		got = append(got, tr.ID)
	}
	if !slices.Equal(got, []string{"q1", "q2", "a1", "a2"}) {
		t.Errorf("the queue is %q", got)
	}
	if a.station() != "" {
		t.Errorf("autoplay reads as station %q", a.station())
	}

	// Off, a new queue asks for nothing.
	a.settings.Autoplay = false
	a.pl.Cue(queue)
	tt.Frame()
	a.pl.PlayQueue(queue, 1)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	select {
	case ids := <-asked:
		t.Errorf("asked with %q while off", ids)
	default:
	}
}

// artistApp shows Taylor Swift's page, its sections as Apple Music lays them
// out, headed by her wide picture if wide.
func artistApp(wide bool) *app {
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	without(a, "isLiked")
	page := speakNow()
	ar := page.Album.Artists[0]
	ar.ImageURL, ar.Genre, ar.Origin, ar.Born = "cover:3", "Pop", "West Reading, PA", "December 13, 1989"
	ar.Bio = "Taylor Swift writes songs about her life, and the world sings them back.\nShe began in country music, in Nashville, at fourteen."
	if wide {
		ar.WideURL, ar.Colors = "wide:2:{w}x{h}", music.Colors{BG: "202a3a", Text1: "ffffff"}
	}
	var top []music.Item
	for i := range page.Tracks {
		tr := page.Tracks[i]
		top = append(top, music.Item{Kind: music.ItemSong, ID: tr.ID, Title: tr.Name, Subtitle: "Speak Now · 2010", ImageURL: "cover:1", Track: &tr})
	}
	var albums, videos, similar []music.Item
	for i, name := range []string{"folklore (deluxe version)", "1989", "Red", "Fearless", "evermore", "Lover"} {
		albums = append(albums, music.Item{Kind: music.ItemAlbum, ID: fmt.Sprint("al", i), Title: name, Subtitle: fmt.Sprint(2008 + i*2), ImageURL: fmt.Sprintf("cover:%d", i%5)})
	}
	for i := range 4 {
		tr := page.Tracks[i]
		tr.Video = true
		videos = append(videos, music.Item{Kind: music.ItemVideo, ID: fmt.Sprint("v", i), Title: tr.Name, Subtitle: "2026", WideURL: fmt.Sprintf("wide:%d:{w}x{h}", i%5), Track: &tr})
	}
	for i, name := range []string{"Olivia Rodrigo", "Sabrina Carpenter", "Gracie Abrams", "Conan Gray", "Lorde", "Lana Del Rey"} {
		similar = append(similar, music.Item{Kind: music.ItemArtist, ID: fmt.Sprint("r", i), Title: name, ImageURL: fmt.Sprintf("cover:%d", i%5)})
	}
	a.artists[ar.ID] = &artistData{page: &music.ArtistPage{Artist: ar, Tracks: page.Tracks}}
	a.artists[ar.ID].page.Sections = []music.Section{
		{Title: "Latest Release", Style: music.ShelfFeatured, Items: []music.Item{
			{Kind: music.ItemAlbum, ID: "pz", Badge: "OCT 16, 2026", Title: "Patient Zero - Single", Subtitle: "3 Songs", ImageURL: "cover:4"},
		}},
		{Title: "Top Songs", MorePage: "top", Style: music.ShelfTracks, Items: top},
		{Title: "Albums", MorePage: "albums", Style: music.ShelfSquare, Items: albums},
		{Title: "Music Videos", MorePage: "videos", Style: music.ShelfBrick, Items: videos},
		{Title: "Similar Artists", Style: music.ShelfCircle, Items: similar},
	}
	d := a.artists[ar.ID]
	d.secs = artistSections(d.page)
	a.route = route{page: pageArtist, src: "fake", id: ar.ID}
	return a
}

// An artist's page with sections shows them, and its Play button plays
// their top songs.
func TestArtistPageSections(t *testing.T) {
	a := artistApp(false)
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if _, ok := tt.Find("folklore (deluxe version)"); !ok {
		t.Fatal("no shelf of albums")
	}
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != "t0" || len(a.pl.Queue()) != 6 {
		t.Fatalf("playing %+v of %d", cur, len(a.pl.Queue()))
	}
}

// A playlist's maker on its page opens their page.
func TestPlaylistOwnerOpensTheirPage(t *testing.T) {
	a := newTestApp()
	fake(a).browse = func(string) (*music.Browse, error) { return &music.Browse{}, nil }
	a.playlists["pl.1"] = &music.Playlist{ID: "pl.1", Name: "Today's Hits", Owner: "Apple Music Hits", OwnerPage: "curator:976439548",
		Tracks: []music.Track{song("1", "Hong Kong")}}
	a.route = route{page: pagePlaylist, src: "fake", id: "pl.1"}
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	r, ok := tt.Find("Apple Music Hits")
	if !ok {
		t.Fatal("no maker")
	}
	tt.ClickAt(r.X+4, r.Y+r.H/2)
	tt.Frame()
	if a.route != (route{page: pageBrowse, src: "fake", id: "curator:976439548"}) {
		t.Fatalf("opened %+v", a.route)
	}
}

// A page paged through stays as it is, gone back to past the time pages
// load anew in; opened anew, it starts over. It went back to its first part
// as the listener came back to it from a song's album.
func TestPagedPageStays(t *testing.T) {
	asked := make(chan string, 8)
	a := newTestApp()
	ch := posts(a)
	fake(a).browse = func(page string) (*music.Browse, error) {
		asked <- page
		return &music.Browse{Sections: []music.Section{{Title: page}}, Next: "songs:2"}, nil
	}
	songs := route{page: pageBrowse, src: "fake", id: music.PageLibrarySongs}
	a.open(songs)
	wait(t, asked, "the first part")
	runPost(t, ch)
	d := a.browses[music.PageLibrarySongs]
	a.loadNext(a.library, d)
	wait(t, asked, "the next part")
	runPost(t, ch)
	if !d.paged {
		t.Fatal("not paged")
	}
	d.at = time.Now().Add(-time.Hour)

	a.open(route{page: pageAlbum, src: "fake", id: "8"})
	a.back()
	select {
	case page := <-asked:
		t.Fatalf("asked for %q anew, going back", page)
	default:
	}
	a.open(route{page: pageHome, src: "fake"})
	a.open(songs)
	if page := wait(t, asked, "the page anew"); page != music.PageLibrarySongs {
		t.Errorf("asked for %q", page)
	}
}

// todaysHits is a playlist of Apple Music's, tinted gold.
func todaysHits() *music.Playlist {
	var tracks []music.Track
	for i, s := range []struct{ name, artist, album string }{
		{"Hong Kong (feat. Faye Wong)", "The Weeknd", "Hong Kong - Single"},
		{"Choosin' Texas", "Ella Langley", "Choosin' Texas - Single"},
		{"NEON", "A2O, A2O LITE", "NEON - Single"},
		{"SUPERSTAR", "A2O, A2O SOUL", "SUPERSTAR - EP"},
		{"With", "RESCENE", "PULSE - EP"},
		{"Been By Now", "Morgan Wallen", "Been By Now - Single"},
		{"CRASH", "ALLDAY PROJECT", "CRASH - EP"},
		{"You're Under Arrest", "ILLIT", "BREAK EVEN - EP"},
		{"Moonride", "Hearts2Hearts", "Moonride - Single"},
		{"FALLEN ANGEL", "JENNIE", "Fallen Angel - EP"},
	} {
		tracks = append(tracks, music.Track{ID: fmt.Sprint("h", i), Source: "fake", Name: s.name, Artists: []music.Artist{{ID: fmt.Sprint("ar", i), Name: s.artist}},
			Album: music.Album{ID: fmt.Sprint("al", i), Name: s.album, ImageURL: fmt.Sprintf("cover:%d", i%5)}, Duration: int64(150000 + 13000*i), Explicit: i == 6})
	}
	return &music.Playlist{ID: "pl.f4d106fed2bd41149aaacabb233eb5eb", Name: "Today's Hits", Owner: "Apple Music Hits", OwnerPage: "curator:1",
		Modified: time.Now().Add(-26 * time.Hour).UTC().Format(time.RFC3339), ImageURL: "cover:1", Colors: music.Colors{BG: "e2b14a", Text1: "1d1d1f"},
		Description: "We search high and low for the songs everyone will be talking about, from pop to folk to hip-hop and R&B. Add this playlist to your library to keep up with what's new, updated every week.",
		Tracks:      tracks, Sections: []music.Section{{Title: "More from Apple Music Hits", Style: music.ShelfSquare, Items: speakNow().Sections[0].Items}}}
}

// With MM_FRAMES set, saves the pages of an album, an artist with and
// without a wide picture, and a playlist, in the light and the dark, to
// set beside Apple Music's (refs/web-player/shots).
func TestDetailFrames(t *testing.T) {
	dir := framesDir(t)
	for _, c := range []struct {
		name string
		app  func() *app
	}{
		{"album", albumApp},
		{"artist-wide", func() *app { return artistApp(true) }},
		{"artist", func() *app { return artistApp(false) }},
		{"playlist", func() *app {
			a := newTestApp()
			a.account = &music.Account{ID: "1", Name: "Alex"}
			without(a, "isLiked")
			pl := todaysHits()
			a.playlists[pl.ID] = pl
			a.route = route{page: pagePlaylist, src: "fake", id: pl.ID}
			return a
		}},
	} {
		for _, dark := range []bool{false, true} {
			a := c.app()
			shelfArt(a)
			h := 1000
			if c.name != "playlist" && c.name != "album" {
				// Down to the artist's biography.
				h = 1700
			}
			tt := ui.NewTester(a.view, 1180, h)
			tt.SetDark(dark)
			settled(tt)
			name := c.name
			if dark {
				name += "-dark"
			}
			saveFrame(t, dir, name+".png", tt.Image())
		}
	}
}
