package music

import "testing"

// A list of the library shown a way is a page that says that way back; as
// Apple keeps it, it is the list's page alone.
func TestLibraryPage(t *testing.T) {
	for _, v := range []LibraryView{{}, {Descending: true}, {Sort: LibrarySortAdded}, {Sort: LibrarySortAdded, Descending: true, Favorites: true, Term: "a b&c"}} {
		page := LibraryPage(PageLibrarySongs, v)
		base, got := ParseLibraryPage(page)
		if base != PageLibrarySongs || got != v {
			t.Errorf("%+v: page %q read back as %q %+v", v, page, base, got)
		}
	}
	if p := LibraryPage(PageLibraryAlbums, LibraryView{Sort: LibrarySortTitle}); p != PageLibraryAlbums {
		t.Errorf("by title, ascending: %q", p)
	}
	if base, v := ParseLibraryPage("search:a?b"); base != "search:a?b" || v != (LibraryView{}) {
		t.Errorf("not a list of the library: %q %+v", base, v)
	}
}
