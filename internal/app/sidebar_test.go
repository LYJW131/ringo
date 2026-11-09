package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// The Playlists header hides the playlists' rows without changing the page
// or queue, or the library's group, and Enter shows them again.
func TestSidebarPlaylistCollapse(t *testing.T) {
	a := editTestApp()
	a.playlists["11"] = &music.Playlist{ID: "11", Name: "睡前"}
	a.pl.Cue(a.playlists["10"].Tracks)
	before := a.route
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.Click("Playlists"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, row := range []string{"睡前", "飙升榜", "All Playlists"} {
		if _, ok := tt.Find(row); ok {
			t.Errorf("the collapsed group still builds %s", row)
		}
	}
	if _, ok := tt.Find("Recently Added"); !ok {
		t.Error("collapsing the playlists hid the library")
	}
	if a.route != before || len(a.pl.Queue()) != 2 || len(a.mine) != 2 || len(a.saved) != 1 {
		t.Error("collapsing the group changed the page, queue or playlists")
	}

	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if err := tt.Click("睡前"); err != nil {
		t.Fatalf("the restored playlist cannot be opened: %v", err)
	}
	tt.Frame()
	if a.route != (route{page: pagePlaylist, src: "fake", id: "11"}) {
		t.Errorf("opened %+v, want playlist 11", a.route)
	}
}

// The plus still opens the naming dialog with the playlists collapsed,
// without opening them.
func TestNewPlaylistInCollapsedSidebar(t *testing.T) {
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Playlists"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.mineCollapsed {
		t.Fatal("the group did not collapse")
	}
	if err := tt.Click("New Playlist"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.naming.open {
		t.Error("the plus did not open the naming dialog")
	}
	if !a.mineCollapsed {
		t.Error("the plus also opened the group")
	}
}

// Each entry of the sidebar opens its page: Apple Music's own, and those of
// the listener's library, which are the source's to make up.
func TestSidebarOpensPages(t *testing.T) {
	a := editTestApp()
	asked := make(chan string, 16)
	fake(a).browse = func(page string) (*music.Browse, error) {
		asked <- page
		return &music.Browse{}, nil
	}
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	for _, entry := range []struct {
		label string
		r     route
	}{
		{"New", route{page: pageBrowse, id: music.PageNew}},
		{"Radio", route{page: pageBrowse, id: music.PageRadio}},
		{"Recently Added", route{page: pageBrowse, id: music.PageRecentlyAdded}},
		{"Artists", route{page: pageBrowse, id: music.PageLibraryArtist}},
		{"Albums", route{page: pageBrowse, id: music.PageLibraryAlbums}},
		{"Songs", route{page: pageBrowse, id: music.PageLibrarySongs}},
		{"Made for You", route{page: pageBrowse, id: music.PageMadeForYou}},
		{"All Playlists", route{page: pageBrowse, id: music.PageAllPlaylists}},
		{"Favorite Songs", route{page: pageLiked}},
		{"Home", route{page: pageHome}},
	} {
		if err := tt.Click(entry.label); err != nil {
			t.Fatalf("%s: %v", entry.label, err)
		}
		tt.Frame()
		entry.r.src = "fake"
		if a.route != entry.r {
			t.Fatalf("%s opened %+v", entry.label, a.route)
		}
		if entry.r.page == pageBrowse {
			if page := wait(t, asked, entry.label); page != entry.r.id {
				t.Errorf("%s asked for %q", entry.label, page)
			}
		}
	}
}

// Signed out, the sidebar has no library and no playlists, which it has
// once the listener signs in.
func TestSidebarLibraryOnceSignedIn(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	for _, entry := range []string{"Library", "Recently Added", "Playlists", "All Playlists"} {
		if _, ok := tt.Find(entry); ok {
			t.Errorf("signed out, the sidebar has %s", entry)
		}
	}
	if _, ok := tt.Find("New"); !ok {
		t.Error("signed out, the sidebar has no New")
	}
	a.account = &music.Account{ID: "1", Name: "小明"}
	tt.Frame()
	if err := tt.Click("Recently Added"); err != nil {
		t.Fatal(err)
	}
	if a.route.id != music.PageRecentlyAdded {
		t.Errorf("opened %+v", a.route)
	}
}
