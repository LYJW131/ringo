package app

import (
	"fmt"
	"image"
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// tallImage is a picture taller than wide, three to four, from one color
// to another down it, as a playlist's tall one comes from Apple.
func tallImage(from, to ui.Color) *image.RGBA {
	const w, h = 96, 128
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		c := from.Mix(to, float32(y)/h)
		for x := range w {
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	return img
}

func hexOf(c ui.Color) string { return fmt.Sprintf("%02x%02x%02x", c.R, c.G, c.B) }

// homePicks is a signed-in Home as Apple Music lays it out: cards three to
// four for its picks, then covers, a section made from one album, more cards
// three to four with who is on each, and stations. The pictures are blocks of
// color, the names invented.
func homePicks(a *app) *music.Browse {
	cover := func(i int) string { return fmt.Sprintf("cover:%d", i%5) }
	tall := func(i int) string { return fmt.Sprintf("tall:%d:{w}x{h}", i%5) }
	for i, c := range shelfColors {
		giveCover(a, cover(i), coverImage(c.from, c.to))
		a.art.art[sized(music.Wide(tall(i), 900, 1200), 900)] = &artwork{bmp: ui.NewBitmap(tallImage(c.from, c.to))}
	}
	// An album's colors, as Apple gives them with its cover: the cover's
	// ground, and the words that read on it.
	album := func(i int, title, artist, reason string) music.Item {
		c := shelfColors[i%5]
		ground := c.to
		text := "f5f5f7"
		if luminance(ground) > 0.4 {
			text = "1d1d1f"
		}
		return music.Item{Kind: music.ItemAlbum, ID: fmt.Sprint("a", i), Title: title, Subtitle: artist, Badge: reason, ImageURL: cover(i),
			Colors: music.Colors{BG: hexOf(ground), Text1: text}, Album: &music.Album{ID: fmt.Sprint("a", i), Name: title}}
	}
	playlist := func(i int, reason, artists string) music.Item {
		return music.Item{Kind: music.ItemPlaylist, ID: fmt.Sprint("p", i), Title: "Mix", Subtitle: "Apple Music", Badge: reason, Artists: artists,
			ImageURL: cover(i), TallURL: tall(i), Colors: music.Colors{BG: hexOf(shelfColors[i%5].from)}, Playlist: &music.Playlist{ID: fmt.Sprint("p", i), Name: "Mix"}}
	}
	station := func(i int, name string) music.Item {
		return music.Item{Kind: music.ItemStation, ID: fmt.Sprint("r", i), Title: name, ImageURL: cover(i), Colors: music.Colors{BG: hexOf(shelfColors[i%5].from)},
			Station: &music.Station{ID: fmt.Sprint("r", i), Name: name}}
	}
	var recent, similar []music.Item
	for i, name := range []string{"Young Manhood", "Better Over Time", "Grand Tour", "Mosaic", "Heavy Is The Crown", "No Hay Angeles", "Luh Leakyz", "Hollow Season"} {
		recent = append(recent, music.Item{Kind: music.ItemAlbum, ID: fmt.Sprint("n", i), Title: name, Subtitle: "The Polar Lights", ImageURL: cover(i + 2), Album: &music.Album{ID: fmt.Sprint("n", i)}})
		similar = append(similar, music.Item{Kind: music.ItemAlbum, ID: fmt.Sprint("m", i), Title: name + " II", Subtitle: "Quiet Hours", ImageURL: cover(i + 1), Album: &music.Album{ID: fmt.Sprint("m", i)}})
	}
	names := "The Polar Lights, Quiet Hours, Paper Sails, Harbor Choir, North Wind Radio and more"
	return &music.Browse{Sections: []music.Section{
		{Title: "Top Picks for You", MorePage: "rec:1", Style: music.ShelfPortrait, Items: []music.Item{
			album(0, "Harbor Lights (feat. Paper Sails) - Single", "The Polar Lights", "Listen Again"),
			album(1, "Slow Tide", "Quiet Hours", "Made for You"),
			album(2, "A Long Way North", "The Polar Lights", "Listen Again"),
			playlist(3, "Made for You", ""),
			playlist(4, "Made for You", ""),
			album(3, "Last Ferry Home", "Paper Sails", "Because You Like Quiet Hours"),
		}},
		{Title: "Recently Played", MorePage: "rec:2", Style: music.ShelfSquare, Items: recent},
		{Title: "More Like Paper Boats", Kicker: "More Like", Seed: &music.Item{Kind: music.ItemAlbum, ID: "a9", Title: "Paper Boats", ImageURL: cover(1)}, MorePage: "rec:3", Style: music.ShelfSquare, Items: similar},
		{Title: "Playlists Made for You", MorePage: "rec:4", Style: music.ShelfPortrait, Items: []music.Item{
			playlist(0, "", names), playlist(1, "", "Quiet Hours, Paper Sails and more"), playlist(2, "", names), playlist(3, "", names), playlist(4, "", "The Polar Lights"),
		}},
		{Title: "Stations for You", Style: music.ShelfSquare, Items: []music.Item{
			station(0, "The Polar Lights Station"), station(1, "Quiet Hours Station"), station(2, "Paper Sails Station"), station(3, "Harbor Choir Station"), station(4, "Slow Tide Station"), station(5, "Mosaic Station"),
		}},
	}}
}

// With MM_FRAMES set, saves a signed-in Home, light and dark, at the window's
// size and a wide one, on screen and whole down the page, and the smallest
// window, dark.
func TestHomePicksFrames(t *testing.T) {
	dir := framesDir(t)
	save := func(name string, w, h int, dark bool) {
		a := newTestApp()
		a.account = &music.Account{ID: "1", Name: "Alex"}
		a.home = homePicks(a)
		tt := ui.NewTester(a.view, w, h)
		tt.SetDark(dark)
		settled(tt)
		theme := "light"
		if dark {
			theme = "dark"
		}
		saveFrame(t, dir, fmt.Sprintf("home-%s-%s.png", name, theme), tt.Image())
	}
	for _, dark := range []bool{false, true} {
		save("1180x780", 1180, 780, dark)
		save("1600x900", 1600, 900, dark)
		save("1180-full", 1180, 2000, dark)
		save("1600-full", 1600, 2000, dark)
	}
	save("880x580", 880, 580, true)

	// The pointer over a card of the picks, with its play button.
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	a.home = homePicks(a)
	tt := ui.NewTester(a.view, 1600, 900)
	tt.SetDark(true)
	settled(tt)
	tt.Move(500, 300)
	settled(tt)
	saveFrame(t, dir, "home-hover-dark.png", tt.Image())
}
