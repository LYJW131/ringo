package app

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// searchRequest is a search asked of a source.
type searchRequest struct {
	query string
}

// fakeSearch makes the source's pages of searches report what they were
// asked, and find nothing, which lands nowhere without a window.
func fakeSearch(a *app) chan searchRequest {
	asked := make(chan searchRequest, 8)
	fake(a).browse = func(page string) (*music.Browse, error) {
		if q, ok := strings.CutPrefix(page, music.PageSearch); ok {
			asked <- searchRequest{q}
		}
		return nil, errors.New("offline")
	}
	return asked
}

// A search before, from the empty field's popup, searches again, clicked
// or chosen with the arrows; it was lost when its option was keyed anew.
func TestSearchHistoryChoice(t *testing.T) {
	for _, keys := range []bool{false, true} {
		a := newTestApp()
		asked := fakeSearch(a)
		a.settings.SearchHistory = []string{"迷星叫", "壱雫空"}
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		if err := tt.Click("Search"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if keys {
			tt.Key(0, ui.KeyDown)
			tt.Key(0, ui.KeyDown)
			tt.Key(0, ui.KeyEnter)
		} else if err := tt.Click("壱雫空"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := wait(t, asked, "the search"); got.query != "壱雫空" || a.query != "壱雫空" {
			t.Fatalf("keys %v: searched %+v, field %q", keys, got, a.query)
		}
		if !slices.Equal(a.settings.SearchHistory, []string{"壱雫空", "迷星叫"}) {
			t.Errorf("keys %v: history %q", keys, a.settings.SearchHistory)
		}
	}
}

// A click anywhere on the sidebar's search field focuses it: only the text
// in the middle took the click, not the icon, the padding or the edges.
func TestSearchFieldClickFocuses(t *testing.T) {
	for _, at := range []struct {
		name   string
		dx, dy float32 // from the field's center
	}{
		{"icon", -(sidebarWidth-28)/2 + 12, 0},
		{"end", (sidebarWidth-28)/2 - 4, 0},
		{"top", 0, -13},
		{"bottom", 0, 13},
	} {
		a := newTestApp()
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		r, ok := tt.Find("Search")
		if !ok {
			t.Fatal("no search field")
		}
		tt.ClickAt(sidebarWidth/2+at.dx, r.Y+r.H/2+at.dy)
		tt.Frame()
		tt.Type("迷星叫")
		tt.Frame()
		if a.query != "迷星叫" {
			t.Errorf("a click on the field's %s left it unfocused", at.name)
		}
	}
}

// The × in the search field empties it, leaving the results' page for what
// Search shows before a word is typed, but not another page.
func TestClearSearch(t *testing.T) {
	for _, onPage := range []bool{true, false} {
		a := newTestApp()
		fakeSearch(a)
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		tt.Click("Search")
		tt.Type("迷星叫")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if !onPage {
			a.open(route{page: pageLiked})
		}
		before := a.route
		tt.Frame()
		if err := tt.Click("Clear Search"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.query != "" || a.searched.query != "" {
			t.Errorf("on page %v: field %q, search kept %+v", onPage, a.query, a.searched)
		}
		want := before
		if onPage {
			want = route{page: pageSearch, src: "fake"}
		}
		if a.route != want {
			t.Errorf("on page %v: showing %+v, want %+v", onPage, a.route, want)
		}
	}
}

// Clear Recent Searches in the empty field's popup forgets the searches
// before, and closes the popup without searching.
func TestClearRecentSearches(t *testing.T) {
	a := newTestApp()
	asked := fakeSearch(a)
	a.settings.SearchHistory = []string{"迷星叫", "壱雫空"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Clear Recent Searches"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if len(a.settings.SearchHistory) != 0 {
		t.Errorf("history %q", a.settings.SearchHistory)
	}
	if tt.HasText("迷星叫") {
		t.Error("the popup still lists the searches before")
	}
	select {
	case got := <-asked:
		t.Errorf("clearing searched %+v", got)
	default:
	}
}

// With MM_FRAMES set, saves the search field's popup, with the searches
// before and with suggestions, light and dark.
func TestSearchPopupFrames(t *testing.T) {
	dir := os.Getenv("MM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		for _, typed := range []bool{false, true} {
			a := newTestApp()
			ch := posts(a)
			a.settings.SearchHistory = []string{"迷星叫", "壱雫空", "a"}
			fake(a).suggest = func(q string) ([]string, error) { return []string{"迷跡波", "迷跡波 Live"}, nil }
			tt := ui.NewTester(a.view, 1000, 600)
			tt.SetDark(dark)
			tt.Frame()
			tt.Click("Search")
			if typed {
				tt.Type("迷跡")
				tt.Frame()
				until(t, ch, func() bool { return len(a.suggestions.items) > 0 })
			}
			tt.Advance(time.Second)
			tt.Frame()
			img := tt.Image()
			s := float32(img.Bounds().Dx()) / 1000
			crop := image.Rect(0, 0, int((sidebarWidth+80)*s), int(300*s))
			out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
			draw.Draw(out, out.Bounds(), img, crop.Min, draw.Src)
			name := fmt.Sprintf("search-dark%v-typed%v.png", dark, typed)
			saveFrame(t, dir, name, out)
		}
	}
}

// Typing asks for suggestions once it pauses, and a suggestion searches for
// what it says, remembering that.
func TestSuggestionSearches(t *testing.T) {
	asked := make(chan string, 4)
	a := newTestApp()
	ch := posts(a)
	fake(a).suggest = func(q string) ([]string, error) { asked <- q; return []string{"迷跡波 Live"}, nil }
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Click("Search")
	tt.Type("迷跡")
	tt.Frame()
	if q := wait(t, asked, "suggestions"); q != "迷跡" {
		t.Fatalf("suggestions for %q", q)
	}
	// Search's categories, opened as the field took the focus, may come
	// first.
	until(t, ch, func() bool { return len(a.suggestions.items) > 0 })
	tt.Frame()
	if err := tt.Click("迷跡波 Live"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route.page != pageSearch || a.route.query != "迷跡波 Live" || !slices.Equal(a.settings.SearchHistory, []string{"迷跡波 Live"}) {
		t.Fatalf("opened %+v, history %q", a.route, a.settings.SearchHistory)
	}
}

// A song row's album opens its page, not playing the song as the row's
// click would.
func TestAlbumLinkInRow(t *testing.T) {
	a := newTestApp()
	tr := music.Track{ID: "2", Name: "起风了", Artists: []music.Artist{{ID: "13", Name: "丙"}}, Album: music.Album{ID: "8", Name: "迷跡波"}, Duration: 200000}
	a.artists["13"] = &artistData{page: &music.ArtistPage{Artist: music.Artist{ID: "13", Name: "丙"}, Tracks: []music.Track{tr}}}
	a.open(route{page: pageArtist, id: "13"})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	// Its name, at the start of the column it fills.
	r, ok := tt.Find("迷跡波")
	if !ok {
		t.Fatal("no album")
	}
	tt.ClickAt(r.X+8, r.Y+r.H/2)
	tt.Frame()
	if a.route != (route{page: pageAlbum, src: "fake", id: "8"}) || a.pl.Current() != nil {
		t.Fatalf("opened %+v, playing %+v", a.route, a.pl.Current())
	}
}

// catalogApp is an app with an artist and an album loaded, and searches
// for each kind, to look at.
func catalogApp() *app {
	a := newTestApp()
	band := music.Artist{ID: "7", Name: "MyGO!!!!!"}
	al := music.Album{ID: "8", Name: "迷跡波"}
	var songs []music.Track
	for i, name := range []string{"詩超絆", "迷星叫", "壱雫空", "碧天伴走", "音一会", "栞"} {
		songs = append(songs, music.Track{ID: fmt.Sprint(i + 1), Name: name, Artists: []music.Artist{band}, Album: al, Duration: int64(210000 + 16000*i)})
	}
	var albums []music.Album
	for i, name := range []string{"迷跡波", "跡暖空", "壱雫空", "音一会", "迷星叫", "焚音打"} {
		albums = append(albums, music.Album{ID: fmt.Sprint(20 + i), Name: name, Artists: []music.Artist{band}, Released: "2026-10-01"})
	}
	a.artists["7"] = &artistData{page: &music.ArtistPage{Artist: band, Tracks: songs, Albums: albums}}
	a.albums["8"] = &albumData{album: &music.AlbumPage{Album: music.Album{ID: "8", Name: "迷跡波", Artists: []music.Artist{band}, Released: "2026-10-01"}, Tracks: songs}}
	return a
}

// catalogScenes are the pages of catalogApp.
var catalogScenes = []struct {
	name string
	r    route
	tab  int
}{
	{"artist", route{page: pageArtist, id: "7"}, artistSongs},
	{"artist-albums", route{page: pageArtist, id: "7"}, artistAlbums},
	{"album", route{page: pageAlbum, id: "8"}, 0},
}

// With MM_FRAMES set, saves the pages of artists, albums and shows
// in the light and the dark.
func TestCatalogFrames(t *testing.T) {
	dir := os.Getenv("MM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, s := range catalogScenes {
		for _, dark := range []bool{false, true} {
			a := catalogApp()
			a.artists["7"].tab = s.tab
			a.open(s.r)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(time.Second)
			tt.Frame()
			name := s.name
			if dark {
				name += "-dark"
			}
			saveFrame(t, dir, "catalog-"+name+".png", tt.Image())
		}
	}
}

// The pages of artists, albums and shows ask for no frames once
// shown.
func TestCatalogFrameBudget(t *testing.T) {
	for _, s := range catalogScenes {
		a := catalogApp()
		a.artists["7"].tab = s.tab
		a.open(s.r)
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Advance(time.Second)
		tt.Frame()
		tt.Advance(time.Second)
		if tt.FramePending() {
			t.Errorf("%s asks for frames while idle", s.name)
		}
	}
}
