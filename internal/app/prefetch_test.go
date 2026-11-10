package app

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mygo-music/internal/music"
)

// keepingApp is an app that keeps its pages in dir, with its source starting
// as the listener signed in as who, who being "" for none.
func keepingApp(dir, who string) *app {
	a := newTestApp()
	a.pagesDir = dir
	lib := a.library
	lib.started, lib.accountAsked, lib.account = false, false, nil
	fake(a).account = func() (*music.Account, error) {
		if who == "" {
			return nil, nil
		}
		return &music.Account{ID: who, Name: "Alex"}, nil
	}
	return a
}

// keptSample is a few pages of what a listener has, whose parts all say
// something, for the round trip through the file to lose none of it.
func keptSample() (home *music.Browse, albums *music.Browse, pl *music.Playlist, liked []music.Track) {
	tr := song("s1", "A Song", "An Artist")
	tr.Album = music.Album{ID: "al1", Name: "An Album", ImageURL: "https://example.com/art.jpg"}
	tr.Explicit, tr.Number = true, 3
	home = &music.Browse{Title: "Home", Sections: []music.Section{
		{Title: "Picks", Style: music.ShelfSquare, Items: []music.Item{{Kind: music.ItemAlbum, ID: "al1", Title: "An Album"}}},
		{Title: "Songs", Tracks: []music.Track{tr}},
	}}
	albums = &music.Browse{Total: 347, Next: "library:albums|100", Sections: []music.Section{
		{Style: music.ShelfGrid, Items: []music.Item{{Kind: music.ItemAlbum, ID: "al2", Title: "Another"}}},
	}}
	pl = &music.Playlist{ID: "p1", Name: "Mix", Mine: true, TrackCount: 1, Tracks: []music.Track{tr}}
	return home, albums, pl, []music.Track{tr}
}

// loadKept fills a source as it would be once its pages were loaded, and keeps
// them.
func loadKept(t *testing.T, a *app) {
	t.Helper()
	lib := a.library
	home, albums, pl, liked := keptSample()
	a.stagePages(lib)
	lib.account = &music.Account{ID: "the-listener-1234"}
	a.adoptPages(lib)
	lib.home, lib.homeKept = home, false
	lib.mine = []music.Playlist{{ID: "p1", Name: "Mix", Mine: true}}
	lib.browses[music.PageLibraryAlbums] = &browseData{page: albums, at: time.Now()}
	lib.playlists["p1"] = pl
	a.setLiked(lib, &music.Page{Tracks: liked, More: true}, time.Now())
	a.savePages(lib, true)
}

// What a listener's pages showed comes back as they were at the next start,
// before the source has said who's signed in, and stays for the same listener.
func TestKeptPagesComeBackAtTheNextStart(t *testing.T) {
	dir := t.TempDir()
	first := keepingApp(dir, "the-listener-1234")
	loadKept(t, first)
	home, albums, pl, liked := keptSample()

	a := keepingApp(dir, "the-listener-1234")
	a.stagePages(a.library)
	lib := a.library
	if !reflect.DeepEqual(lib.home, home) || !lib.homeKept {
		t.Errorf("Home came back as %+v", lib.home)
	}
	if d := lib.browses[music.PageLibraryAlbums]; d == nil || !reflect.DeepEqual(d.page, albums) || !d.next.more || !d.at.IsZero() {
		t.Errorf("the albums came back as %+v", d)
	}
	if !reflect.DeepEqual(lib.playlists["p1"], pl) || len(lib.mine) != 1 {
		t.Errorf("the playlists came back as %+v, %+v", lib.mine, lib.playlists["p1"])
	}
	if !reflect.DeepEqual(lib.likedList, liked) || !lib.likedPaged.more || !lib.likedAt.IsZero() {
		t.Errorf("the favorite songs came back as %+v", lib.likedList)
	}

	lib.account = &music.Account{ID: "the-listener-1234"}
	a.adoptPages(lib)
	if lib.home == nil || lib.browses[music.PageLibraryAlbums] == nil {
		t.Error("the same listener's pages went")
	}
}

// What's kept holds nothing of the account but a hash, and no token.
func TestKeptPagesHoldNoAccount(t *testing.T) {
	dir := t.TempDir()
	loadKept(t, keepingApp(dir, "the-listener-1234"))
	data, err := os.ReadFile(pagesPath(dir, "fake"))
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"the-listener-1234", "token", "Token"} {
		if strings.Contains(string(data), word) {
			t.Errorf("what's kept says %q", word)
		}
	}
}

// Pages kept for one listener never show another's, whether they signed in as
// someone else or out, and are deleted.
func TestKeptPagesAreNotAnotherListeners(t *testing.T) {
	for _, who := range []string{"someone-else", ""} {
		dir := t.TempDir()
		loadKept(t, keepingApp(dir, "the-listener-1234"))

		a := keepingApp(dir, who)
		a.stagePages(a.library)
		lib := a.library
		if lib.home == nil {
			t.Fatal("nothing was kept")
		}
		if who != "" {
			lib.account = &music.Account{ID: who}
		}
		a.adoptPages(lib)
		if lib.home != nil || len(lib.browses) > 0 || len(lib.playlists) > 0 || len(lib.mine) > 0 || len(lib.likedList) > 0 {
			t.Errorf("as %q: the last listener's pages stayed: %+v", who, lib.home)
		}
		if _, err := os.Stat(pagesPath(dir, "fake")); err == nil {
			t.Errorf("as %q: the file stayed", who)
		}
	}
}

// Switching to another account deletes what was kept, and what's kept after is
// the new account's.
func TestKeptPagesGoWithTheAccount(t *testing.T) {
	dir := t.TempDir()
	a := keepingApp(dir, "the-listener-1234")
	loadKept(t, a)
	lib := a.library
	posts(a)

	lib.account = &music.Account{ID: "another"}
	a.afterAccount(lib)
	if _, err := os.Stat(pagesPath(dir, "fake")); err == nil {
		t.Fatal("the last account's file stayed")
	}
	if lib.pages.account != accountKey(lib, lib.account) {
		t.Error("what's loaded next is kept for the last account")
	}

	lib.account = nil
	a.afterAccount(lib)
	lib.home = &music.Browse{Title: "Anonymous"}
	a.keepPages(lib)
	if lib.pages.pending {
		t.Error("something signed out was kept")
	}
}

// A file that's too big for the cap leaves out the playlists, which are most of
// it, and a file of nothing but pages that don't fit isn't kept.
func TestKeptPagesStayWithinTheCap(t *testing.T) {
	a := keepingApp(t.TempDir(), "x")
	lib := a.library
	home, albums, _, _ := keptSample()
	lib.home = home
	lib.browses[music.PageLibraryAlbums] = &browseData{page: albums}
	long := strings.Repeat("x", 20000)
	for i := range 3 {
		pl := &music.Playlist{ID: fmt.Sprint("p", i), Name: "Mix"}
		for j := range pagesTracks {
			pl.Tracks = append(pl.Tracks, song(fmt.Sprint("s", j), long))
		}
		lib.playlists[pl.ID] = pl
		lib.mine = append(lib.mine, music.Playlist{ID: pl.ID, Mine: true})
	}
	data := encodePages(lib, "key")
	if data == nil || len(data) > pagesMaxBytes {
		t.Fatalf("%d bytes were kept", len(data))
	}
	if strings.Contains(string(data), long) || !strings.Contains(string(data), "An Album") {
		t.Error("it left out the pages instead of the playlists")
	}

	lib.home.Description = strings.Repeat("y", pagesMaxBytes)
	if data := encodePages(lib, "key"); data != nil {
		t.Errorf("%d bytes were kept past the cap", len(data))
	}
}

// At most warmAtOnce things are asked for ahead at once, one more as each
// lands or fails, and none while the listener waits for the page they're on.
func TestWarmQueueRules(t *testing.T) {
	a := newTestApp()
	lib := a.library
	var started int
	var done []func()
	queue := func(n int) {
		for i := range n {
			key := fmt.Sprint("job", started, i, len(lib.warm.jobs))
			lib.warm.keys[key] = true
			lib.warm.jobs = append(lib.warm.jobs, warmJob{key: key, due: func() bool { return true }, start: func(d func()) {
				started++
				done = append(done, d)
			}})
		}
	}
	queue(10)
	a.pumpWarm(lib)
	if started != warmAtOnce {
		t.Fatalf("%d were started at once", started)
	}
	done[0]()
	done[1]()
	if started != warmAtOnce+2 || lib.warm.flying != warmAtOnce {
		t.Errorf("%d were started, %d at once, after two ended", started, lib.warm.flying)
	}

	// The page the listener is on is being asked for.
	a.route = route{page: pageBrowse, src: "fake", id: "waited"}
	lib.browses["waited"] = &browseData{loading: true}
	before := started
	done[2]()
	done[3]()
	if started != before {
		t.Error("something was asked for ahead while the listener waited")
	}
	lib.browses["waited"].page = &music.Browse{}
	a.pumpWarm(lib)
	if started == before {
		t.Error("nothing was asked for once the listener's page was up")
	}
}

// What was asked for before the listener switched accounts never lands among
// the new one's pages, and a signed-out listener has none of the library's
// asked for.
func TestWarmPagesAreOnlyTheListeners(t *testing.T) {
	p := newPace(t, 400*time.Millisecond)
	var who atomic.Value
	who.Store("old")
	f := fake(p.a)
	f.browse = func(page string) (*music.Browse, error) {
		name := who.Load().(string)
		p.answer("browse")
		return &music.Browse{Title: name, Sections: []music.Section{{Style: music.ShelfGrid, Items: []music.Item{{Kind: music.ItemAlbum, ID: "x", Title: name}}}}}, nil
	}
	lib := p.a.library
	p.a.startLibrary(lib)
	p.idle(time.Second)
	if p.ahead == 0 {
		t.Fatal("nothing was being asked for ahead")
	}
	who.Store("new")
	lib.account = &music.Account{ID: "2"}
	p.a.afterAccount(lib)
	p.idle(8 * time.Second)
	if len(lib.browses) == 0 {
		t.Fatal("nothing was asked for after")
	}
	for page, d := range lib.browses {
		if d.page != nil && d.page.Title != "new" {
			t.Errorf("%s is %q's", page, d.page.Title)
		}
	}

	p = newPace(t, 400*time.Millisecond)
	fake(p.a).account = func() (*music.Account, error) { p.answer("account"); return nil, nil }
	p.a.startLibrary(p.a.library)
	p.idle(8 * time.Second)
	if p.asked["liked"]+p.asked["playlist"]+p.asked["playlists"] > 0 || p.asked["browse"] != 3 {
		t.Errorf("signed out, asked for %v", p.asked)
	}
}

// A page that's old shows as it was while it's asked for anew, and is
// replaced without being drawn as loading, so nothing jumps.
func TestWarmRefreshReplacesInPlace(t *testing.T) {
	p := newPace(t, 400*time.Millisecond)
	lib := p.a.library
	p.a.startLibrary(lib)
	p.idle(6 * time.Second)
	page := music.PageNew
	d := lib.browses[page]
	if d == nil || d.page == nil {
		t.Fatal("the page wasn't asked for ahead")
	}
	old := d.page
	d.at = time.Now().Add(-time.Hour)
	p.a.warmPages(lib)
	for range 40 {
		p.step(16 * time.Millisecond)
		if d.loading || d.page == nil {
			t.Fatal("the page went to loading as it was asked for anew")
		}
	}
	if d.page == old || time.Since(d.at) > time.Minute {
		t.Error("the page wasn't replaced")
	}
}

// What was kept stays as it is if Home can't be loaded now.
func TestKeptHomeStaysWhenItFails(t *testing.T) {
	dir := t.TempDir()
	loadKept(t, keepingApp(dir, "the-listener-1234"))
	a := keepingApp(dir, "the-listener-1234")
	ch := posts(a)
	fake(a).home = func() (*music.Browse, error) { return nil, errors.New("offline") }
	a.startLibrary(a.library)
	until(t, ch, func() bool { return !a.homeLoading && a.accountAsked })
	if a.home == nil || a.homeErr != "" {
		t.Errorf("Home went: %+v, %q", a.home, a.homeErr)
	}
}
