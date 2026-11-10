package music

import (
	"net/url"
	"strings"
)

// A list of the library that can be sorted and filtered, as Music's
// Albums and Songs can, is asked for as its page with how after a "?":
// library:songs?sort=-dateAdded&favorites=1&term=love. Its page alone is it
// as Apple keeps it, by title, everything.

// The ways a list of the library sorts, as Apple's library can: by title,
// or by when each was added.
const (
	LibrarySortTitle = "name"
	LibrarySortAdded = "dateAdded"
)

// LibraryView is how a list of the library is shown.
type LibraryView struct {
	// Sort is one of the LibrarySort constants; empty, by title.
	Sort string
	// Descending sorts the other way.
	Descending bool
	// Favorites keeps to the listener's favorites.
	Favorites bool
	// Term keeps to what has it in its name, its artist's or its album's.
	Term string
}

// LibraryPage is the page of the list base, shown as v.
func LibraryPage(base string, v LibraryView) string {
	q := url.Values{}
	if s := v.sortKey(); s != LibrarySortTitle {
		q.Set("sort", s)
	}
	if v.Favorites {
		q.Set("favorites", "1")
	}
	if t := strings.TrimSpace(v.Term); t != "" {
		q.Set("term", t)
	}
	if len(q) == 0 {
		return base
	}
	return base + "?" + q.Encode()
}

// ParseLibraryPage is a page's list and how it's shown, as LibraryPage made it.
func ParseLibraryPage(page string) (string, LibraryView) {
	base, query, ok := strings.Cut(page, "?")
	if !ok || !strings.HasPrefix(base, "library:") {
		return page, LibraryView{}
	}
	q, _ := url.ParseQuery(query)
	var v LibraryView
	sort := q.Get("sort")
	v.Descending = strings.HasPrefix(sort, "-")
	if s := strings.TrimPrefix(sort, "-"); s == LibrarySortAdded {
		v.Sort = s
	}
	v.Favorites = q.Get("favorites") == "1"
	v.Term = q.Get("term")
	return base, v
}

// SortKey is how Apple's library is asked to sort for v: the sort's name,
// after a "-" for the other way.
func (v LibraryView) SortKey() string { return v.sortKey() }

func (v LibraryView) sortKey() string {
	s := v.Sort
	if s == "" {
		s = LibrarySortTitle
	}
	if v.Descending {
		return "-" + s
	}
	return s
}
