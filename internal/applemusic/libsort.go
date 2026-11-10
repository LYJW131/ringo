package applemusic

import (
	"strings"

	"mygo-music/internal/music"
)

// The library's Albums and Songs sorted and filtered, as Music's are (see
// music.LibraryView). Apple's library sorts by name and by dateAdded, either
// way ("-" first), and keeps to favorites itself, so those are asked of it,
// and a list only partly loaded is in its order already. Other keys are
// refused (HTTP 400: artistName, releaseDate, genre, playCount, checked in
// 2026-11). Its search takes neither a sort nor a filter, so a word looked
// for is looked for in the whole list, sorted and filtered, kept once.

// sortable are the lists that sort and filter.
var sortable = map[string]bool{music.PageLibraryAlbums: true, music.PageLibrarySongs: true}

// viewParams asks Apple's library for its list as v shows it.
func viewParams(p params, v music.LibraryView) {
	p["sort"] = v.SortKey()
	if v.Favorites {
		p["filter[inFavorites]"] = "true"
	}
}

// libraryFound is what of a list of the library has v's Term, out of all
// of the list as v sorts and filters it, which is kept a while.
func (s *Source) libraryFound(page string, list libraryList, p params, v music.LibraryView) (*music.Browse, error) {
	base, _ := music.ParseLibraryPage(page)
	whole := v
	whole.Term = ""
	all, err := kept(s, s.pageKey(music.LibraryPage(base, whole))+"#all", freshness{maxAge: listKept}, func(pause func()) ([]music.Item, error) {
		b, err := s.me(list.path, p, reqOpts{})
		var items []music.Item
		for err == nil {
			items = append(items, s.items(b, b.Data)...)
			if b.Next == "" || len(items) >= 20000 {
				break
			}
			pause()
			b, err = s.follow(b.Next, p)
		}
		return items, err
	}, false)
	if err != nil {
		return nil, err
	}
	term := strings.ToLower(strings.TrimSpace(v.Term))
	found := []music.Item{}
	for _, it := range s.editedItems(all) {
		if itemHas(it, term) {
			found = append(found, it)
		}
	}
	s.listed(songItems(found), nil)
	return &music.Browse{Title: list.title(), Sections: []music.Section{{Style: list.style, Items: found}}}, nil
}

// itemHas says it has term, lowered, in its name, its artist's or its album's.
func itemHas(it music.Item, term string) bool {
	in := []string{it.Title, it.Subtitle, it.Artists}
	if tr := it.Track; tr != nil {
		in = append(in, tr.Name, tr.ArtistNames(), tr.Album.Name)
	}
	if al := it.Album; al != nil {
		in = append(in, al.Name)
		for _, ar := range al.Artists {
			in = append(in, ar.Name)
		}
	}
	for _, s := range in {
		if s != "" && strings.Contains(strings.ToLower(s), term) {
			return true
		}
	}
	return false
}
