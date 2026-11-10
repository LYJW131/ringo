package applemusic

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Apple Music's pages, from answers Apple gave (testdata/, cut short; see
// live_pages_test.go for the real thing): what each page's sections are,
// how they show, and where their See All and next parts lead.

// apple is a stand-in for amp-api that answers from testdata by path, and
// keeps what it was asked.
type apple struct {
	t      *testing.T
	mu     sync.Mutex
	asked  []*url.URL
	routes []route
}

type route struct {
	path   *regexp.Regexp
	answer func(r *http.Request) any // a testdata file's name, or what to answer as JSON
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (a *apple) serve(r *http.Request) *http.Response {
	if r.Method == "HEAD" || r.URL.Hostname() != "amp-api.music.apple.com" {
		return answer(404, "")
	}
	a.mu.Lock()
	a.asked = append(a.asked, r.URL)
	a.mu.Unlock()
	for _, rt := range a.routes {
		if rt.path.MatchString(r.URL.Path) {
			switch v := rt.answer(r).(type) {
			case string:
				if strings.HasSuffix(v, ".json") {
					return answer(200, fixture(a.t, v))
				}
				return answer(404, "")
			case *http.Response:
				return v
			default:
				return answer(200, v)
			}
		}
	}
	return answer(404, "")
}

// last is the last request made to a path matching p.
func (a *apple) last(p string) *url.URL {
	a.mu.Lock()
	defer a.mu.Unlock()
	re := regexp.MustCompile(p)
	for i := len(a.asked) - 1; i >= 0; i-- {
		if re.MatchString(a.asked[i].Path) {
			return a.asked[i]
		}
	}
	return nil
}

func fixed(name string) func(*http.Request) any { return func(*http.Request) any { return name } }

var usStorefront = map[string]any{"data": []any{map[string]any{"id": "us", "type": "storefronts", "attributes": map[string]any{
	"defaultLanguageTag": "en-US", "supportedLanguageTags": []string{"en-US", "es-MX"},
}}}}

// newApple is the catalog of the US, signed out, as testdata has it.
func newApple(t *testing.T) (*testSource, *apple) {
	a := &apple{t: t}
	a.routes = []route{
		{regexp.MustCompile(`^/v1/storefronts/us$`), func(*http.Request) any { return usStorefront }},
		{regexp.MustCompile(`^/v1/storefronts/cn$`), fixed("storefront-cn.json")},
		{regexp.MustCompile(`^/v1/editorial/\w+/groupings$`), func(r *http.Request) any {
			if r.URL.Query().Get("name") == "radio" {
				return "radio-us.json"
			}
			return "new-us.json"
		}},
		{regexp.MustCompile(`^/v1/editorial/us/rooms/\d+$`), fixed("room-us.json")},
		{regexp.MustCompile(`^/v1/editorial/us/multirooms/\d+$`), fixed("multiroom-us.json")},
		{regexp.MustCompile(`^/v1/catalog/us/charts$`), fixed("charts-us.json")},
		{regexp.MustCompile(`^/v1/catalog/us/search$`), func(r *http.Request) any {
			if r.URL.Query().Get("groups") != "" {
				return "search-more-us.json"
			}
			return "search-us.json"
		}},
		{regexp.MustCompile(`^/v1/recommendations/us$`), fixed("search-landing-us.json")},
		{regexp.MustCompile(`^/v1/catalog/us/artists/159260351$`), fixed("artist-us.json")},
		{regexp.MustCompile(`^/v1/catalog/us/albums/1440935467$`), fixed("album-us.json")},
	}
	return newTestSource(t, a.serve), a
}

func styles(b *music.Browse) []string {
	var out []string
	for _, s := range b.Sections {
		out = append(out, s.Style)
	}
	return out
}

func browse(t *testing.T, ts *testSource, page string) *music.Browse {
	t.Helper()
	b, err := ts.Browse(page)
	if err != nil {
		t.Fatalf("%s: %v", page, err)
	}
	return b
}

// New is its editors' elements in order, each in its style, the heroes with
// their badges over wide pictures, a row's See All its own room, and the
// genres' links their pages.
func TestNewPage(t *testing.T) {
	ts, a := newApple(t)
	b := browse(t, ts, music.PageNew)
	want := []string{"hero", "square", "tracks", "square", "square", "square", "square", "tracks", "square", "square", "square", "square", "square", "square", "square", "square", "brick", "square", "links", "links"}
	if got := styles(b); !slices.Equal(got, want) {
		t.Errorf("styles %q", got)
	}
	// The first a feature's own picture, the next a playlist's.
	heroes := b.Sections[0].Items
	if h := heroes[0]; h.Badge == "" || !strings.Contains(h.WideURL, "{w}x{h}") || h.Colors.BG == "" || h.Kind != music.ItemLink || h.Page != "multiroom:6820621614" {
		t.Errorf("hero %+v", h)
	}
	// A HeroCustom's words are its title, under its badge, as the web player
	// has them: it showed its badge twice, and its words over the picture.
	if h := heroes[0]; h.Badge != "THE WOMAN WE NEEDED" || h.Title != "In 2026, Olivia Dean was the sound of unconditional love." || h.Note != "" || h.Subtitle != "" {
		t.Errorf("HeroCustom: badge %q, title %q, subtitle %q, note %q", h.Badge, h.Title, h.Subtitle, h.Note)
	}
	if h := heroes[1]; h.Badge == "" || !strings.Contains(h.WideURL, "{w}x{h}") || h.Kind != music.ItemPlaylist || h.Playlist == nil || h.Title == "" {
		t.Errorf("hero %+v", h)
	}
	songs := b.Sections[2].Items
	if songs[0].Kind != music.ItemSong || songs[0].Track == nil || songs[0].Track.Name == "" {
		t.Errorf("a song of Best New Songs: %+v", songs[0])
	}
	if b.Sections[1].MorePage != "room:6827375194" {
		t.Fatalf("See All: %q", b.Sections[1].MorePage)
	}
	room := browse(t, ts, b.Sections[1].MorePage)
	if u := a.last(`/rooms/`); u == nil || u.Path != "/v1/editorial/us/rooms/6827375194" {
		t.Errorf("asked %v", u)
	}
	if len(room.Sections) != 1 || room.Sections[0].Style != music.ShelfGrid || len(room.Sections[0].Items) == 0 {
		t.Errorf("room %s", shape(room))
	}
	genres := b.Sections[len(b.Sections)-1].Items
	if genres[0].Page != "grouping:168981" {
		t.Errorf("a genre: %+v", genres[0])
	}
	// Signed out, the page for those who don't subscribe.
	if u := a.last(`/groupings$`); u.Query().Get("tabs") != "nonsubscriber" || u.Query().Get("format[resources]") != "map" {
		t.Errorf("asked %v", u)
	}
}

// Radio leads with Apple Music 1 as a hero, and what's on the air as a live shelf.
func TestRadioPage(t *testing.T) {
	ts, _ := newApple(t)
	b := browse(t, ts, music.PageRadio)
	if got := styles(b); len(got) < 2 || got[0] != music.ShelfHero || got[1] != music.ShelfLive {
		t.Fatalf("styles %q", got)
	}
	for _, it := range b.Sections[1].Items {
		if it.Kind != music.ItemStation || it.Station == nil || !it.Station.Live {
			t.Errorf("on the air: %+v", it)
		}
	}
}

// A feature is headed by its own picture, kept as a template, in its colors.
func TestMultiroom(t *testing.T) {
	ts, _ := newApple(t)
	b := browse(t, ts, "multiroom:6820621614")
	if !strings.Contains(b.WideURL, "{w}x{h}") || b.Colors.BG != "92a8b6" || b.Title == "" {
		t.Errorf("head %q %+v %q", b.WideURL, b.Colors, b.Title)
	}
	if got := styles(b); len(got) == 0 || got[0] != music.ShelfText || b.Sections[0].Subtitle == "" {
		t.Errorf("styles %q", got)
	}
}

// The charts are in Apple's order, a city's and the world's daily charts
// playlists, each with all of it a page of its own, from its start.
func TestCharts(t *testing.T) {
	ts, _ := newApple(t)
	b := browse(t, ts, "charts")
	want := []string{"tracks", "square", "square", "square", "square", "brick"}
	if got := styles(b); !slices.Equal(got, want) {
		t.Errorf("styles %q", got)
	}
	kinds := []string{music.ItemSong, music.ItemPlaylist, music.ItemPlaylist, music.ItemPlaylist, music.ItemAlbum, music.ItemVideo}
	for i, s := range b.Sections[:min(len(b.Sections), len(kinds))] {
		if s.Items[0].Kind != kinds[i] {
			t.Errorf("%d: %s", i, s.Items[0].Kind)
		}
		if !strings.HasPrefix(s.MorePage, "chart|/v1/catalog/us/charts?") || strings.Contains(s.MorePage, "offset") {
			t.Errorf("%d: See All %q", i, s.MorePage)
		}
	}
}

// A search is its groups in Apple's order, each in its style; a group's See
// All asks for that group, and its next part is asked for at the address
// Apple gave, whose offset isn't the count so far.
func TestSearchPage(t *testing.T) {
	ts, a := newApple(t)
	b := browse(t, ts, music.PageSearch+"taylor swift")
	want := []string{"square", "circle", "square", "tracks", "square", "square", "square", "brick", "brick"}
	if got := styles(b); !slices.Equal(got, want) {
		t.Errorf("styles %q", got)
	}
	kinds := map[string]bool{}
	for _, it := range b.Sections[0].Items {
		kinds[it.Kind] = true
	}
	if len(kinds) < 2 {
		t.Errorf("top results of one kind: %v", kinds)
	}
	more := ""
	for _, s := range b.Sections {
		if strings.HasPrefix(s.MorePage, "search-more:album:") {
			more = s.MorePage
		}
	}
	if more != "search-more:album:taylor swift" {
		t.Fatalf("See All of the albums: %q", more)
	}
	all := browse(t, ts, more)
	if u := a.last(`/search$`); u.Query().Get("groups") != "album" || u.Query().Get("term") != "taylor swift" || u.Query().Get("offset") != "" {
		t.Errorf("asked %v", u)
	}
	if all.Sections[0].Style != music.ShelfGrid || all.Sections[0].Items[0].Kind != music.ItemAlbum {
		t.Errorf("all albums: %s", shape(all))
	}
	// Apple's next, as it gave it.
	var given struct {
		Results map[string]struct{ Next string } `json:"results"`
	}
	jsonOf(t, fixture(t, "search-more-us.json"), &given)
	next := given.Results["album"].Next
	if next == "" || all.Next != more+"|"+next {
		t.Fatalf("next %q, Apple's %q", all.Next, next)
	}
	browse(t, ts, all.Next)
	asked := a.last(`/search$`)
	nu, _ := url.Parse(next)
	for k, v := range nu.Query() {
		if asked.Query().Get(k) != v[0] {
			t.Errorf("%s: asked %q, Apple's next %q", k, asked.Query().Get(k), v[0])
		}
	}
}

// Search's categories are wide, each opening its page.
func TestSearchLanding(t *testing.T) {
	ts, _ := newApple(t)
	b := browse(t, ts, music.PageSearchLanding)
	if len(b.Sections) == 0 {
		t.Fatal("no categories")
	}
	for _, s := range b.Sections {
		if s.Style != music.ShelfBrick {
			t.Errorf("style %s", s.Style)
		}
		for _, it := range s.Items {
			if it.Kind != music.ItemLink || it.Page == "" {
				t.Errorf("category %+v", it)
			}
		}
	}
}

// An artist's page is Apple's shelves in order, in their styles, each See
// All its view, and the biography last; its Play button plays the top songs.
func TestArtistPage(t *testing.T) {
	ts, _ := newApple(t)
	page, err := ts.Artist("159260351")
	if err != nil {
		t.Fatal(err)
	}
	b := &music.Browse{Sections: page.Sections}
	want := []string{"featured", "tracks", "square", "square", "square", "brick", "square", "square", "square", "square", "square", "circle", "text"}
	if got := styles(b); !slices.Equal(got, want) {
		t.Errorf("styles %q", got)
	}
	views := map[string]bool{}
	for _, s := range page.Sections {
		if v, ok := strings.CutPrefix(s.MorePage, "artist-view:159260351:"); ok {
			views[v] = true
		}
	}
	for _, v := range []string{"top-songs", "full-albums", "similar-artists"} {
		if !views[v] {
			t.Errorf("no See All of %s: %v", v, views)
		}
	}
	a := page.Artist
	if len(page.Tracks) == 0 || a.Bio == "" || a.Born == "" || a.Origin == "" || !strings.Contains(a.WideURL, "{w}") || a.Colors.BG == "" || a.Genre == "" || a.Group {
		t.Errorf("artist %+v, %d top songs", a, len(page.Tracks))
	}
	// Their square photo, cut wide around them by a crop Apple made for it:
	// cut from its middle, it showed only their body.
	if !strings.HasSuffix(a.WideURL, "/{w}x{h}vb.{f}") {
		t.Errorf("the head's picture is %q", a.WideURL)
	}
}

// An album's songs have their place on it, the clear hits are starred and
// no others, and the shelves after it are Apple's views, in order.
func TestAlbumPage(t *testing.T) {
	ts, _ := newApple(t)
	page, err := ts.Album("1440935467")
	if err != nil {
		t.Fatal(err)
	}
	var popular []int
	for i, tr := range page.Tracks {
		if tr.Number != i+1 || tr.Disc != 1 {
			t.Errorf("song %d: number %d disc %d", i, tr.Number, tr.Disc)
		}
		if tr.Popular {
			popular = append(popular, i)
		}
	}
	// 1989's popularity: 0.42, 1, 0.86, 0.42, 0.31, 0.97, 0.18, 0.73, 0.70, …
	if !slices.Equal(popular, []int{1, 2, 5}) {
		t.Errorf("popular %v", popular)
	}
	a := page.Album
	if a.Colors.BG == "" || a.Copyright == "" || a.Label == "" || a.Genre == "" || !a.Traits.Lossless || a.TrackCount != 13 {
		t.Errorf("album %+v", a)
	}
	var titles []string
	for _, s := range page.Sections {
		titles = append(titles, s.Title)
	}
	if len(titles) == 0 || titles[0] != "More By Taylor Swift" {
		t.Errorf("shelves %q", titles)
	}
}

// What a song or an album says of itself: explicit, and how it sounds at its best.
func TestTraits(t *testing.T) {
	s := New(Options{DataDir: t.TempDir()})
	tr := s.toTrack(resourceOf(t, `{"id":"1","type":"songs","attributes":{"name":"x","contentRating":"explicit","audioTraits":["lossy-stereo","lossless","hi-res-lossless","atmos","spatial"],"isAppleDigitalMaster":true,"trackNumber":3,"discNumber":2,"composerName":"C","playParams":{"id":"1","kind":"song"}}}`))
	if !tr.Explicit || tr.Traits != (music.Traits{Lossless: true, HiRes: true, Atmos: true, DigitalMaster: true}) || tr.Number != 3 || tr.Disc != 2 || tr.Composer != "C" {
		t.Errorf("song %+v", tr)
	}
	al := (&Source{}).toAlbum(resourceOf(t, `{"id":"2","type":"albums","attributes":{"name":"y","contentRating":"clean","audioTraits":["lossy-stereo"],"isMasteredForItunes":true,"artwork":{"url":"u/{w}x{h}bb.{f}","bgColor":"101010","textColor1":"f0f0f0","textColor4":"a0a0a0"},"editorialNotes":{"standard":"<i>One</i> &amp; two","tagline":"New Album"}}}`))
	if al.Explicit || al.Traits != (music.Traits{DigitalMaster: true}) || al.Colors != (music.Colors{BG: "101010", Text1: "f0f0f0", Text4: "a0a0a0"}) || al.Notes != "One & two" || al.Tagline != "New Album" {
		t.Errorf("album %+v", al)
	}
}

// The catalog speaks the app's language among those of the storefront.
func TestPickLanguage(t *testing.T) {
	cn := []string{"zh-Hans-CN", "en-GB"}
	for _, c := range []struct {
		app, def string
		tags     []string
		want     string
	}{
		{i18n.Chinese, "zh-Hans-CN", cn, "zh-Hans-CN"},
		{i18n.English, "zh-Hans-CN", cn, "en-GB"},
		{i18n.Chinese, "en-US", []string{"en-US", "es-MX"}, "en-US"},
		{i18n.English, "en-US", []string{"en-US", "es-MX"}, "en-US"},
		{i18n.Chinese, "ja-JP", []string{"ja-JP", "en-US", "zh-Hans-CN"}, "zh-Hans-CN"},
		{i18n.Chinese, "zh-Hant-TW", []string{"zh-Hant-TW", "en-GB"}, "zh-Hant-TW"},
		{i18n.Chinese, "en-SG", []string{"en-SG", "zh-Hans-SG"}, "zh-Hans-SG"},
	} {
		if got := pickLanguage(c.app, c.def, c.tags); got != c.want {
			t.Errorf("%s in %v: %s", c.app, c.tags, got)
		}
	}
}

// Signed out, the catalog is the region's, if Apple has a storefront there,
// in the app's language; a page asked for again in another language is
// asked for again.
func TestRegionAndLanguage(t *testing.T) {
	defer i18n.SetLanguage(i18n.English)
	i18n.SetLanguage(i18n.Chinese)
	a := &apple{t: t}
	a.routes = []route{
		{regexp.MustCompile(`^/v1/storefronts/cn$`), fixed("storefront-cn.json")},
		{regexp.MustCompile(`^/v1/storefronts/us$`), func(*http.Request) any { return usStorefront }},
		{regexp.MustCompile(`^/v1/storefronts/`), fixed("none")},
		{regexp.MustCompile(`^/v1/editorial/\w+/groupings$`), fixed("new-us.json")},
	}
	ts := newTestSource(t, a.serve)
	ts.region = "CN"
	browse(t, ts, music.PageNew)
	if u := a.last(`/groupings$`); u.Path != "/v1/editorial/cn/groupings" || u.Query().Get("l") != "zh-Hans-CN" {
		t.Errorf("asked %v", u)
	}
	n := len(a.asked)
	browse(t, ts, music.PageNew)
	if len(a.asked) != n {
		t.Errorf("asked again in the same language: %v", a.asked[n:])
	}
	i18n.SetLanguage(i18n.English)
	browse(t, ts, music.PageNew)
	if u := a.last(`/groupings$`); u.Query().Get("l") != "en-GB" {
		t.Errorf("in English: %v", u)
	}

	other := newTestSource(t, a.serve)
	other.region = "xx"
	browse(t, other, music.PageNew)
	if u := a.last(`/groupings$`); u.Path != "/v1/editorial/us/groupings" {
		t.Errorf("a region without a storefront: %v", u)
	}
}

func jsonOf(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}
