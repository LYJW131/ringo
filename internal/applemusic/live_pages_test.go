package applemusic

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Apple Music's own pages against the real Apple Music, so not run by
// default: LIVE=1 signed out, in the US in English and in China in Chinese,
// and with APPLE_MUSIC_TOKEN_FILE signed in, reading only. What's printed is
// the pages' shapes, never what's in a listener's library.

// shape is a page as styles and counts.
func shape(b *music.Browse) string {
	var out []string
	for _, s := range b.Sections {
		out = append(out, fmt.Sprintf("%s:%d", s.Style, len(s.Items)))
	}
	more := ""
	if b.Next != "" {
		more = " +next"
	}
	return fmt.Sprintf("%d sections [%s]%s", len(b.Sections), strings.Join(out, " "), more)
}

// checkPage fails a page without at least least sections that have a style
// and items, or with items that have no title, or shelves of covers none of
// which has a picture.
func checkPage(t *testing.T, name string, b *music.Browse, err error, least int) *music.Browse {
	t.Helper()
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return nil
	}
	full := 0
	for _, s := range b.Sections {
		if s.Style == "" {
			t.Errorf("%s: a section without a style", name)
		}
		if len(s.Items) == 0 && s.Style != music.ShelfText {
			t.Errorf("%s: an empty %s section", name, s.Style)
			continue
		}
		full++
		pictured := 0
		for _, it := range s.Items {
			if it.Title == "" {
				t.Errorf("%s: an untitled %s in a %s section", name, it.Kind, s.Style)
			}
			if it.ImageURL != "" || it.WideURL != "" {
				pictured++
			}
		}
		if s.Style != music.ShelfLinks && s.Style != music.ShelfText && pictured == 0 {
			t.Errorf("%s: a %s section without pictures", name, s.Style)
		}
	}
	if full < least {
		t.Errorf("%s: %d sections with items, want %d or more", name, full, least)
	}
	t.Logf("%s: %s", name, shape(b))
	return b
}

// firstPage is the first page of sections or items that starts with prefix.
func firstPage(b *music.Browse, prefix string) string {
	if b == nil {
		return ""
	}
	for _, s := range b.Sections {
		if strings.HasPrefix(s.MorePage, prefix) {
			return s.MorePage
		}
		for _, it := range s.Items {
			if strings.HasPrefix(it.Page, prefix) {
				return it.Page
			}
		}
	}
	return ""
}

// firstItem is the ID of the first item of kind on b.
func firstItem(b *music.Browse, kind string) string {
	if b == nil {
		return ""
	}
	for _, s := range b.Sections {
		for _, it := range s.Items {
			if it.Kind == kind {
				return it.ID
			}
		}
	}
	return ""
}

func TestLivePages(t *testing.T) {
	if os.Getenv("LIVE") != "1" {
		t.Skip("LIVE=1 asks the real Apple Music")
	}
	for _, c := range []struct{ region, lang, term string }{{"", i18n.English, "taylor swift"}, {"cn", i18n.Chinese, "taylor swift"}} {
		t.Run(orT(c.region, "us")+"-"+c.lang, func(t *testing.T) {
			i18n.SetLanguage(c.lang)
			defer i18n.SetLanguage(i18n.English)
			s := New(Options{DataDir: t.TempDir(), Region: c.region})
			defer s.Close()
			t.Logf("storefront %s, language %s", must(s.storefront()), s.language())

			newPage := openPage(t, s, "new", music.PageNew, 5)
			home, err := s.Home()
			checkPage(t, "home", home, err, 5)
			openPage(t, s, "radio", music.PageRadio, 3)
			if room := firstPage(newPage, "room:"); room != "" {
				r := openPage(t, s, room, room, 1)
				if r != nil && r.Next != "" {
					openPage(t, s, "its next part", r.Next, 1)
				}
			} else {
				t.Error("no room on New")
			}
			if g := firstPage(newPage, "grouping:"); g != "" {
				openPage(t, s, g, g, 2)
			} else {
				t.Error("no genre on New")
			}
			if m := firstPage(newPage, "multiroom:"); m != "" {
				openPage(t, s, m, m, 1)
			}
			charts := openPage(t, s, "charts", "charts", 3)
			if all := firstPage(charts, "chart|"); all != "" {
				c := openPage(t, s, "a chart", all, 1)
				if c != nil && c.Next != "" {
					openPage(t, s, "its next part", c.Next, 1)
				}
			}
			openPage(t, s, "charts of a genre", "charts:20", 2)
			landing := openPage(t, s, "search", music.PageSearchLanding, 1)
			if cur := firstPage(landing, "curator:"); cur != "" {
				openPage(t, s, cur, cur, 1)
			}
			found := openPage(t, s, "search:"+c.term, music.PageSearch+c.term, 4)
			if more := firstPage(found, "search-more:"); more != "" {
				m := openPage(t, s, more, more, 1)
				if m != nil && m.Next != "" {
					openPage(t, s, "its next part", m.Next, 1)
				}
			}

			artist, err := s.Artist("159260351")
			if err != nil {
				t.Fatal(err)
			}
			a := artist.Artist
			t.Logf("artist: %d top songs, bio %v, genre %q, born %q, origin %q, wide %v, video %v, colors %v",
				len(artist.Tracks), a.Bio != "", a.Genre, a.Born, a.Origin, a.WideURL != "", a.VideoURL != "", a.Colors.BG != "")
			checkPage(t, "artist", &music.Browse{Sections: artist.Sections}, err, 4)
			if len(artist.Tracks) == 0 {
				t.Error("no top songs")
			}
			if v := firstPage(&music.Browse{Sections: artist.Sections}, "artist-view:"); v != "" {
				openPage(t, s, v, v, 1)
			} else {
				t.Error("no See All on the artist's page")
			}

			albumID := firstItem(found, music.ItemAlbum)
			album, err := s.Album(albumID)
			if err != nil {
				t.Fatal(err)
			}
			popular, numbered := 0, 0
			for _, tr := range album.Tracks {
				if tr.Popular {
					popular++
				}
				if tr.Number > 0 {
					numbered++
				}
			}
			al := album.Album
			t.Logf("album: %d songs, %d numbered, %d popular, notes %v, label %v, copyright %v, traits %+v, explicit %v, colors %v, video %v",
				len(album.Tracks), numbered, popular, al.Notes != "", al.Label != "", al.Copyright != "", al.Traits, al.Explicit, al.Colors.BG != "", al.VideoURL != "")
			checkPage(t, "album", &music.Browse{Sections: album.Sections}, nil, 1)
			if numbered != len(album.Tracks) || len(album.Tracks) == 0 {
				t.Error("songs without their number")
			}

			listID := firstItem(newPage, music.ItemPlaylist)
			list, err := s.Playlist(listID)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("playlist: %d songs, owner %v, owner page %q, modified %v, description %v, colors %v, video %v",
				len(list.Tracks), list.Owner != "", list.OwnerPage, list.Modified != "", list.Description != "", list.Colors.BG != "", list.VideoURL != "")
			checkPage(t, "playlist", &music.Browse{Sections: list.Sections}, nil, 0)
			if len(list.Tracks) == 0 {
				t.Error("a playlist without songs")
			}
		})
	}
}

// Signed in, reading only: Home, the library's pages and their next parts,
// and the catalog's pages as the listener sees them.
func TestLivePagesSignedIn(t *testing.T) {
	file := os.Getenv("APPLE_MUSIC_TOKEN_FILE")
	if file == "" {
		t.Skip("APPLE_MUSIC_TOKEN_FILE names a file holding a media-user-token")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	t.Logf("storefront %s, language %s, subscriber %v", must(s.storefront()), s.language(), s.subscriber())

	home, err := s.Home()
	checkPage(t, "home", home, err, 3)
	if rec := firstPage(home, "rec:"); rec != "" {
		openPage(t, s, "a shelf of home", rec, 1)
	}
	openPage(t, s, "new", music.PageNew, 5)
	openPage(t, s, "radio", music.PageRadio, 3)
	for _, page := range []string{music.PageRecentlyAdded, music.PageLibraryAlbums, music.PageLibraryArtist, music.PageLibrarySongs, music.PageMadeForYou, music.PageAllPlaylists} {
		b := openPage(t, s, page, page, 1)
		if b != nil && b.Next != "" {
			openPage(t, s, page+" next", b.Next, 1)
		}
		if page == music.PageLibraryArtist {
			if a := firstPage(b, "library-artist:"); a != "" {
				openPage(t, s, "an artist of the library", a, 1)
			}
		}
	}
	found := openPage(t, s, "search", music.PageSearch+"taylor swift", 4)
	artist, err := s.Artist("159260351")
	if err == nil {
		checkPage(t, "artist", &music.Browse{Sections: artist.Sections}, nil, 4)
	} else {
		t.Error(err)
	}
	if album, err := s.Album(firstItem(found, music.ItemAlbum)); err == nil {
		checkPage(t, "album", &music.Browse{Sections: album.Sections}, nil, 1)
	} else {
		t.Error(err)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// openPage is checkPage of the page id.
func openPage(t *testing.T, s *Source, name, id string, least int) *music.Browse {
	t.Helper()
	b, err := s.Browse(id)
	return checkPage(t, name, b, err, least)
}
