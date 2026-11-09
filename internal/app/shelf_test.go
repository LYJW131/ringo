package app

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// shelfColors are the colors the shelves' pictures go between.
var shelfColors = []struct{ from, to ui.Color }{
	{ui.RGB(40, 90, 170), ui.RGB(220, 120, 160)},
	{ui.RGB(230, 150, 70), ui.RGB(120, 40, 90)},
	{ui.RGB(30, 130, 110), ui.RGB(210, 220, 140)},
	{ui.RGB(200, 40, 70), ui.RGB(250, 190, 90)},
	{ui.RGB(90, 60, 200), ui.RGB(60, 200, 220)},
}

// wideImage is a picture wider than tall, from one color to another across.
func wideImage(from, to ui.Color) *image.RGBA {
	const w, h = 192, 108
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		c := from.Mix(to, float32(x)/w)
		for y := range h {
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	return img
}

// shelfArt gives the pictures of shelfPage, square and wide, as if loaded.
func shelfArt(a *app) {
	for i, c := range shelfColors {
		giveCover(a, fmt.Sprintf("cover:%d", i), coverImage(c.from, c.to))
		wide := ui.NewBitmap(wideImage(c.from, c.to))
		a.art.art[sized(music.Wide(fmt.Sprintf("wide:%d:{w}x{h}", i), wideArt, wideArt*9/16), wideArt)] = &artwork{bmp: wide}
	}
}

// shelfSong is an item of a song, by an invented band.
func shelfSong(i int) music.Item {
	tr := music.Track{ID: fmt.Sprint("s", i), Source: "fake", Name: fmt.Sprint("Song ", i), Duration: 200000,
		Artists:  []music.Artist{{ID: "7", Name: "The Polar Lights"}},
		Album:    music.Album{ID: "70", Name: "Polar Night", ImageURL: fmt.Sprintf("cover:%d", i%5)},
		Explicit: i%4 == 1}
	return music.Item{Kind: music.ItemSong, ID: tr.ID, Title: tr.Name, Subtitle: "The Polar Lights", ImageURL: tr.Album.ImageURL, Explicit: tr.Explicit, Track: &tr}
}

// shelfPage is a page of New with a section of each style.
func shelfPage() *music.Browse {
	cover := func(i int) string { return fmt.Sprintf("cover:%d", i%5) }
	wide := func(i int) string { return fmt.Sprintf("wide:%d:{w}x{h}", i%5) }
	var songs, albums, artists, videos, playlists, links []music.Item
	for i := range 12 {
		songs = append(songs, shelfSong(i))
	}
	for i, name := range []string{"Young Manhood and Other Donatable Organs", "No Hay Ángeles Aquí", "Better Over Time", "Luh Leakyz Vol. 1", "Heavy Is The Crown", "Grand Tour", "Mosaic"} {
		albums = append(albums, music.Item{Kind: music.ItemAlbum, ID: fmt.Sprint("a", i), Title: name, Subtitle: "Zach Bryan", ImageURL: cover(i), Explicit: i%3 == 0,
			Album: &music.Album{ID: fmt.Sprint("a", i), Name: name}})
	}
	for i, name := range []string{"Taylor Swift", "Natalie Taylor", "Tayla Parx", "TAYLOR", "Teyana Taylor", "Roger Taylor", "James Taylor"} {
		artists = append(artists, music.Item{Kind: music.ItemArtist, ID: fmt.Sprint("r", i), Title: name, ImageURL: cover(i + 2), Artist: &music.Artist{ID: fmt.Sprint("r", i), Name: name}})
	}
	for i := range 5 {
		tr := shelfSong(i).Track
		tr.Video = true
		videos = append(videos, music.Item{Kind: music.ItemVideo, ID: fmt.Sprint("v", i), Title: fmt.Sprint("Video ", i), Subtitle: "The Polar Lights", WideURL: wide(i + 1), Track: tr})
	}
	for i, name := range []string{"2026 Best Songs", "C-Pop: 2026 Best", "Taiwan Today", "Today's Hits", "A-List Pop", "Mandopop", "Cantopop", "K-Pop", "R&B Now", "Rap Life"} {
		playlists = append(playlists, music.Item{Kind: music.ItemPlaylist, ID: fmt.Sprint("p", i), Title: name, Subtitle: "Apple Music", ImageURL: cover(i + 1)})
	}
	for _, name := range []string{"Concerts", "Spatial Audio", "Music Videos", "Charts", "Browse by Genre"} {
		links = append(links, music.Item{Kind: music.ItemLink, ID: name, Title: name, Page: "room:" + name})
	}
	links = append(links, music.Item{Kind: music.ItemLink, ID: "web", Title: "Apple Music on the Web", URL: "https://music.apple.com"})
	return &music.Browse{Sections: []music.Section{
		{Style: music.ShelfHero, Items: []music.Item{
			{Kind: music.ItemAlbum, ID: "a0", Badge: "NEW ALBUM", Title: "The year in Olivia Dean", Subtitle: "Olivia Dean", Note: "Her voice was 2026's warmest welcome.", WideURL: wide(0), Colors: music.Colors{BG: "2a4a7a"}},
			{Kind: music.ItemPlaylist, ID: "p3", Badge: "UPDATED PLAYLIST", Title: "Today's Hits: 2026's Best", Subtitle: "Apple Music Hits", WideURL: wide(3), Note: "Olivia Dean falls into love with So Easy."},
			{Kind: music.ItemStation, ID: "st1", Badge: "LISTEN NOW", Title: "Apple Music 1", Subtitle: "The new music that matters", ImageURL: cover(4)},
		}},
		{Title: "Best New Songs", MorePage: "room:songs", Style: music.ShelfTracks, Items: songs},
		{Title: "New Releases", MorePage: "room:albums", Style: music.ShelfSquare, Items: albums},
		{Title: "Artists", Style: music.ShelfCircle, Items: artists},
		{Title: "Music Videos", MorePage: "room:videos", Style: music.ShelfBrick, Items: videos},
		{Title: "On the Air", Style: music.ShelfLive, Items: []music.Item{
			{Kind: music.ItemStation, ID: "am1", Badge: "LIVE", Title: "Apple Music 1", Subtitle: "The Zane Lowe Show", Note: "4:00 – 6:00 PM", WideURL: wide(2)},
			{Kind: music.ItemStation, ID: "am2", Badge: "LIVE", Title: "Apple Music Hits", Subtitle: "Hits Hits Hits with Brooke Reese", Note: "6:00 – 8:00 PM", WideURL: wide(4)},
		}},
		{Title: "Latest Release", Style: music.ShelfFeatured, Items: []music.Item{
			{Kind: music.ItemAlbum, ID: "a5", Badge: "NOV 7, 2026", Title: "Grand Tour", Subtitle: "12 Songs", Note: "The band's road record, cut live in a week.", ImageURL: cover(3)},
		}},
		{Title: "More to Explore", Style: music.ShelfLinks, Items: links},
		{Title: "Playlists Updated", Style: music.ShelfGrid, Items: playlists},
		{Title: "About", Style: music.ShelfText, Subtitle: "The Polar Lights are a band from the far north.\nThey play slow songs about ice and light, and play them loud."},
	}}
}

// shelfApp is an app signed in, showing the page of New that shelfPage is,
// whose source answers the rest of its pages with nothing.
func shelfApp() (*app, chan string) {
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	without(a, "isLiked")
	asked := make(chan string, 16)
	fake(a).browse = func(page string) (*music.Browse, error) {
		asked <- page
		return &music.Browse{}, nil
	}
	a.browses[music.PageNew] = &browseData{page: shelfPage(), at: time.Now()}
	a.route = route{page: pageBrowse, src: "fake", id: music.PageNew}
	return a, asked
}

// A section's title opens the page with all of it.
func TestShelfTitleOpensMorePage(t *testing.T) {
	a, asked := shelfApp()
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("New Releases"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageBrowse, src: "fake", id: "room:albums"}) {
		t.Fatalf("opened %+v", a.route)
	}
	if page := wait(t, asked, "the page"); page != "room:albums" {
		t.Errorf("asked for %q", page)
	}
}

// An item of a shelf opens what it is: an album's page, an artist's or a
// playlist's, a link's page, or an address in the browser.
func TestShelfItemsOpenTheirPages(t *testing.T) {
	defer func(open func(string)) { openExternal = open }(openExternal)
	var opened []string
	openExternal = func(url string) { opened = append(opened, url) }
	for _, c := range []struct {
		label string
		want  route
	}{
		{"Better Over Time", route{page: pageAlbum, src: "fake", id: "a2"}},
		{"Natalie Taylor", route{page: pageArtist, src: "fake", id: "r1"}},
		{"Mandopop", route{page: pagePlaylist, src: "fake", id: "p5"}},
		{"Charts", route{page: pageBrowse, src: "fake", id: "room:Charts"}},
	} {
		a, _ := shelfApp()
		tt := ui.NewTester(a.view, 1180, 3400)
		tt.Frame()
		if err := tt.Click(c.label); err != nil {
			t.Fatalf("%s: %v", c.label, err)
		}
		if a.route != c.want {
			t.Errorf("%s opened %+v", c.label, a.route)
		}
	}
	a, _ := shelfApp()
	tt := ui.NewTester(a.view, 1180, 3400)
	tt.Frame()
	if err := tt.Click("Apple Music on the Web"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != "https://music.apple.com" {
		t.Errorf("opened %q in the browser", opened)
	}
}

// A song of a tracks shelf plays the whole section from it.
func TestTracksShelfPlaysFromSong(t *testing.T) {
	a, _ := shelfApp()
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if err := tt.Click("Song 2"); err != nil {
		t.Fatal(err)
	}
	cur := a.pl.Current()
	if cur == nil || cur.ID != "s2" || len(a.pl.Queue()) != 12 {
		t.Fatalf("playing %+v of %d", cur, len(a.pl.Queue()))
	}
}

// The arrow over a shelf's edge scrolls it a screen, and the one back shows
// once it has.
func TestShelfArrowsScroll(t *testing.T) {
	a, _ := shelfApp()
	tt := ui.NewTester(a.view, 1180, 1400)
	tt.Frame()
	r, ok := tt.Find("Better Over Time")
	if !ok {
		t.Fatal("no shelf")
	}
	tt.Move(r.X+r.W/2, r.Y+10)
	tt.Frame()
	if _, ok := tt.Find("Previous Page"); ok {
		t.Error("an arrow back at the shelf's start")
	}
	if err := tt.Click("Next Page"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	st := a.shelves[2]
	if st == nil || st.X <= 0 {
		t.Fatalf("the shelf is at %+v", st)
	}
	if _, ok := tt.Find("Previous Page"); !ok {
		t.Error("no arrow back once scrolled")
	}
}

// The wheel over a shelf scrolls the page down, not the shelf, which only
// scrolls sideways.
func TestWheelOverShelfScrollsPage(t *testing.T) {
	a, _ := shelfApp()
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	r, _ := tt.Find("Better Over Time")
	tt.Wheel(r.X+r.W/2, r.Y+10, 0, 300)
	tt.Advance(time.Second)
	tt.Frame()
	after, _ := tt.Find("Better Over Time")
	if after.Y >= r.Y-100 {
		t.Errorf("the page stayed: the item was at %v, is at %v", r.Y, after.Y)
	}
}

// A page in parts asks for its next part as its foot comes into view, and
// its items go on the last section's; failing, it offers to try again.
func TestBrowseLoadsNext(t *testing.T) {
	a, asked := shelfApp()
	ch := posts(a)
	fails := true
	fake(a).browse = func(page string) (*music.Browse, error) {
		asked <- page
		if fails {
			return nil, fmt.Errorf("offline")
		}
		return &music.Browse{Sections: []music.Section{{Style: music.ShelfGrid, Items: []music.Item{
			{Kind: music.ItemAlbum, ID: "4", Title: "Four"}, {Kind: music.ItemAlbum, ID: "5", Title: "Five"},
		}}}}, nil
	}
	page := &music.Browse{Next: "library:albums?2", Sections: []music.Section{{Style: music.ShelfGrid, Items: []music.Item{
		{Kind: music.ItemAlbum, ID: "1", Title: "One"}, {Kind: music.ItemAlbum, ID: "2", Title: "Two"}, {Kind: music.ItemAlbum, ID: "3", Title: "Three"},
	}}}}
	a.browses[music.PageLibraryAlbums] = &browseData{page: page, at: time.Now(), next: paged{loaded: true, more: true}}
	a.route = route{page: pageBrowse, src: "fake", id: music.PageLibraryAlbums}
	tt := ui.NewTester(a.view, 1180, 900)
	tt.Frame()
	if got := wait(t, asked, "the next part"); got != "library:albums?2" {
		t.Fatalf("asked for %q", got)
	}
	runPost(t, ch)
	tt.Frame()
	fails = false
	if err := tt.Click("Retry"); err != nil {
		t.Fatalf("no way to try again: %v", err)
	}
	wait(t, asked, "the next part again")
	runPost(t, ch)
	tt.Frame()
	d := a.browses[music.PageLibraryAlbums]
	if items := d.page.Sections[0].Items; len(items) != 5 || items[4].Title != "Five" || d.page.Next != "" {
		t.Fatalf("the page is %+v", d.page)
	}
	if len(page.Sections[0].Items) != 3 {
		t.Error("the part loaded first changed")
	}
	if _, ok := tt.Find("Five"); !ok {
		t.Error("the next part doesn't show")
	}
}

// The search field opens Search's categories as it takes the focus, and
// what's typed and entered opens what Apple Music finds for it.
func TestSearchOpensSearchPages(t *testing.T) {
	a, asked := shelfApp()
	tt := ui.NewTester(a.view, 1180, 800)
	tt.Frame()
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageSearch, src: "fake"}) {
		t.Fatalf("the field opened %+v", a.route)
	}
	if page := wait(t, asked, "the categories"); page != music.PageSearchLanding {
		t.Fatalf("asked for %q", page)
	}
	tt.Type("迷星叫")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.route != (route{page: pageSearch, src: "fake", query: "迷星叫"}) {
		t.Fatalf("searching opened %+v", a.route)
	}
	if page := wait(t, asked, "the search"); page != music.PageSearch+"迷星叫" {
		t.Errorf("asked for %q", page)
	}
	tt.Key(0, ui.KeyBack)
	if a.route != (route{page: pageSearch, src: "fake"}) {
		t.Errorf("back went to %+v", a.route)
	}
}

// shelfScenes are the pages the frames below show.
var shelfScenes = []struct {
	name string
	set  func(a *app)
}{
	{"new", func(a *app) {}},
	{"search-landing", func(a *app) {
		var bricks []music.Item
		for i, name := range []string{"Artists of the Year", "Mandopop", "K-Pop", "Pop", "Cantopop", "Hip-Hop/Rap", "Apple Music Exclusive", "C-Pop", "R&B", "Sleep", "Up Next", "Work Time", "Classical", "Chill", "Spatial Audio", "Hits"} {
			bricks = append(bricks, music.Item{Kind: music.ItemLink, ID: name, Title: name, Page: "grouping:" + name, WideURL: fmt.Sprintf("wide:%d:{w}x{h}", i%5)})
		}
		a.browses[music.PageSearchLanding] = &browseData{page: &music.Browse{Sections: []music.Section{
			{Title: "Browse Categories", Style: music.ShelfBrick, Items: bricks},
		}}, at: time.Now()}
		a.route = route{page: pageSearch, src: "fake"}
	}},
	{"library-albums", func(a *app) {
		var albums []music.Item
		for i := range 30 {
			albums = append(albums, music.Item{Kind: music.ItemAlbum, ID: fmt.Sprint("l", i), Title: fmt.Sprintf("Album with a Title Long Enough to Wrap %d", i), Subtitle: "The Polar Lights", ImageURL: fmt.Sprintf("cover:%d", i%5), Explicit: i%4 == 0})
		}
		a.browses[music.PageLibraryAlbums] = &browseData{page: &music.Browse{Next: "library:albums?31", Sections: []music.Section{
			{Style: music.ShelfGrid, Items: albums},
		}}, at: time.Now(), next: paged{loaded: true, more: true, err: "Couldn't load this page: offline"}}
		a.route = route{page: pageBrowse, src: "fake", id: music.PageLibraryAlbums}
	}},
}

// With MM_FRAMES set, saves New with a section of each style, Search's
// categories and the library's albums, light and dark.
func TestShelfFrames(t *testing.T) {
	dir := framesDir(t)
	for _, s := range shelfScenes {
		for _, dark := range []bool{false, true} {
			a, _ := shelfApp()
			a.mine = []music.Playlist{{ID: "10", Name: "Evening Walks", ImageURL: "cover:0", Mine: true}, {ID: "11", Name: "Focus", ImageURL: "cover:1", Mine: true}}
			shelfArt(a)
			s.set(a)
			tt := ui.NewTester(a.view, 1180, 2400)
			tt.SetDark(dark)
			settled(tt)
			name := s.name
			if dark {
				name += "-dark"
			}
			saveFrame(t, dir, "shelf-"+name+".png", tt.Image())
		}
	}
	// The shelves under the pointer, with their arrows and play buttons.
	a, _ := shelfApp()
	shelfArt(a)
	tt := ui.NewTester(a.view, 1180, 800)
	tt.SetDark(true)
	settled(tt)
	if r, ok := tt.Find("Her voice was 2026's warmest welcome."); ok {
		tt.Move(r.X+r.W/2, r.Y-40)
	}
	settled(tt)
	saveFrame(t, dir, "shelf-hover-dark.png", tt.Image())
}
