package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// libraryApp is signed in and shows the library's Albums, which any way it
// is asked for answers with a few albums.
func libraryApp() *app {
	return libraryAppWith(nil)
}

func libraryAppWith(ch chan func()) *app {
	a := newTestApp()
	if ch != nil {
		a.postFn = func(fn func()) { ch <- fn }
	}
	a.account = &music.Account{ID: "1", Name: "Alex"}
	var items []music.Item
	for i := range 4 {
		items = append(items, music.Item{Kind: music.ItemAlbum, ID: fmt.Sprint("a", i), Title: fmt.Sprint("Album ", i), Subtitle: "Jay Chou"})
	}
	fake(a).browse = func(string) (*music.Browse, error) {
		return &music.Browse{Title: "Albums", Sections: []music.Section{{Style: music.ShelfGrid, Items: items}}}, nil
	}
	a.open(route{page: pageBrowse, id: a.libraryPage(music.PageLibraryAlbums)})
	return a
}

// The library's Albums opens with what was added last first. Its menu sorts
// it by title, which starts from A, the other way, and to favorites only,
// each its own page; how is kept, by title from A too, and the sidebar opens
// Albums that way again. Sorted by when added again, it's as it first opened.
func TestLibrarySortOptions(t *testing.T) {
	a := libraryApp()
	if a.route.id != "library:albums?sort=-dateAdded" {
		t.Fatalf("opened as %q", a.route.id)
	}
	tt := ui.NewTester(a.view, 1180, 800)
	settled(tt)
	choose := func(path ...string) {
		t.Helper()
		if err := tt.Click(T("Sort Options")); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(path...); err != nil {
			t.Fatalf("%v: %q", err, tt.Menu())
		}
		tt.Frame()
	}
	choose(T("Sort Options"), T("Title"))
	if a.route.id != music.PageLibraryAlbums || a.settings.LibraryViews[music.PageLibraryAlbums] != music.PageLibraryAlbums {
		t.Fatalf("by title, which starts from A: %q, kept %q", a.route.id, a.settings.LibraryViews)
	}
	choose(T("Sort Options"), T("Descending"))
	choose(T("Favorites Only"))
	want := "library:albums?favorites=1&sort=-name"
	if a.route.id != want || a.settings.LibraryViews[music.PageLibraryAlbums] != want {
		t.Fatalf("shown %q, kept %q", a.route.id, a.settings.LibraryViews)
	}
	a.open(route{page: pageHome})
	tt.Frame()
	if err := tt.Click(T("Albums")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.id != want {
		t.Fatalf("opened again as %q", a.route.id)
	}
	choose(T("All Albums"))
	choose(T("Sort Options"), T("Recently Added"))
	if _, kept := a.settings.LibraryViews[music.PageLibraryAlbums]; a.route.id != "library:albums?sort=-dateAdded" || kept {
		t.Fatalf("back to how it opens: %q, kept %q", a.route.id, a.settings.LibraryViews)
	}
}

// The field finds what has a word a moment after the last key, as it is
// sorted, and its clear button brings the whole list back at once.
func TestLibraryFind(t *testing.T) {
	a := libraryApp()
	a.settings.LibraryViews = map[string]string{music.PageLibraryAlbums: "library:albums?sort=dateAdded"}
	a.open(route{page: pageBrowse, id: a.libraryPage(music.PageLibraryAlbums)})
	tt := ui.NewTester(a.view, 1180, 800)
	settled(tt)
	if err := tt.Click(T("Find in Albums")); err != nil {
		t.Fatal(err)
	}
	tt.Type("jay")
	tt.Frame()
	if a.route.id != "library:albums?sort=dateAdded" {
		t.Fatalf("looked before the pause: %q", a.route.id)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.route.id != "library:albums?sort=dateAdded&term=jay" {
		t.Fatalf("looking: %q", a.route.id)
	}
	if a.settings.LibraryViews[music.PageLibraryAlbums] != "library:albums?sort=dateAdded" {
		t.Errorf("the word was kept: %q", a.settings.LibraryViews)
	}
	if err := tt.Click(T("Clear")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.id != "library:albums?sort=dateAdded" {
		t.Fatalf("cleared: %q", a.route.id)
	}
}

// With MM_FRAMES set, saves the library's Albums with its menu's button and
// its field, looking for a word, in the light and the dark.
func TestLibraryFrames(t *testing.T) {
	dir := framesDir(t)
	for _, dark := range []bool{false, true} {
		ch := make(chan func(), 64)
		a := libraryAppWith(ch)
		shelfArt(a)
		a.libFind = libraryFind{base: music.PageLibraryAlbums, text: "jay", seen: "jay", applied: "jay"}
		a.show(route{page: pageBrowse, id: "library:albums?term=jay"})
		tt := ui.NewTester(a.view, 1180, 700)
		tt.SetDark(dark)
		tt.Frame()
		for len(ch) > 0 {
			runPost(t, ch)
		}
		settled(tt)
		name := "library-albums"
		if dark {
			name += "-dark"
		}
		saveFrame(t, dir, name+".png", tt.Image())
	}
}
