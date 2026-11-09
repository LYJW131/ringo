// Command tray draws the app's icon as it shows small: internal/app/tray/
// tray-template.png, for the macOS menu bar, the mark in black on
// transparent for macOS to tint, at 2× of the menu bar's 18 points; and
// internal/app/tray/tray.png, in the mark's colors, for the notification
// area of Windows and Linux. The mark is the apple cut across of the app's
// icon: its edge as a ring, and the five chambers of its core. It draws by
// sampling the shapes, 16×16 to a pixel.
//
// Run it from the repository root:
//
//	go run ./icon/tray
//
// With -preview dir, it also writes the template over light and dark menu
// bars, enlarged, to check by eye.
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

// The mark, as parts of the radius of the slice, which is 1 around (0, 0).
const (
	// The template: a ring and the chambers, heavy enough to hold at 18
	// points.
	ring                               = 0.15
	chamberIn, chamberOut, chamberHalf = 0.10, 0.66, 0.175

	// In color: the skin, the flesh, and the chambers as the icon has them.
	skin                         = 0.10
	colorIn, colorOut, colorHalf = 0.10, 0.62, 0.15
)

var (
	skinColor    = color.NRGBA{0xEC, 0x3A, 0x4B, 0xFF}
	fleshColor   = color.NRGBA{0xFF, 0xF3, 0xD3, 0xFF}
	chamberColor = color.NRGBA{0xF2, 0x9A, 0x17, 0xFF}
)

// inChamber reports whether (x, y) is in one of the five chambers: each the
// part two discs share, between a point at in and one at out along its
// spoke, half wide at the middle.
func inChamber(x, y, in, out, half float64) bool {
	chord := out - in
	r := (chord*chord/4 + half*half) / (2 * half)
	mid := (in + out) / 2
	for k := range 5 {
		t := (-90 + 72*float64(k)) * math.Pi / 180
		ux, uy := math.Cos(t), math.Sin(t)
		mx, my := ux*mid, uy*mid
		// The discs' centers, either side of the spoke's middle.
		ax, ay := mx-uy*(r-half), my+ux*(r-half)
		bx, by := mx+uy*(r-half), my-ux*(r-half)
		if math.Hypot(x-ax, y-ay) < r && math.Hypot(x-bx, y-by) < r {
			return true
		}
	}
	return false
}

// template is the mark in one color: whether (x, y) is inked.
func template(x, y float64) (color.NRGBA, bool) {
	d := math.Hypot(x, y)
	return color.NRGBA{A: 0xFF}, d < 1 && d > 1-ring || inChamber(x, y, chamberIn, chamberOut, chamberHalf)
}

// colored is the mark in its colors.
func colored(x, y float64) (color.NRGBA, bool) {
	switch d := math.Hypot(x, y); {
	case d >= 1:
		return color.NRGBA{}, false
	case d > 1-skin:
		return skinColor, true
	case inChamber(x, y, colorIn, colorOut, colorHalf):
		return chamberColor, true
	}
	return fleshColor, true
}

// render draws the mark px pixels square, the slice filling fit of it, each
// pixel the mean of 16×16 samples.
func render(px int, fit float64, at func(x, y float64) (color.NRGBA, bool)) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	half := float64(px) / 2
	const n = 16
	for y := range px {
		for x := range px {
			var r, g, b, a float64
			for j := range n {
				for i := range n {
					c, ok := at(((float64(x)+(float64(i)+0.5)/n)-half)/(half*fit), ((float64(y)+(float64(j)+0.5)/n)-half)/(half*fit))
					if ok {
						r, g, b, a = r+float64(c.R), g+float64(c.G), b+float64(c.B), a+1
					}
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(math.Round(r / a)), uint8(math.Round(g / a)), uint8(math.Round(b / a)), uint8(math.Round(a * 255 / (n * n)))})
		}
	}
	return img
}

// preview puts the icon on a light menu bar and a dark one, tinted as
// macOS tints it, and enlarged with nearest pixels so each one shows.
func preview(icon *image.NRGBA, zoom int) *image.NRGBA {
	w := icon.Bounds().Dx()
	pad := w / 3
	side := w + 2*pad
	out := image.NewNRGBA(image.Rect(0, 0, side*2*zoom, side*zoom))
	grounds := []color.NRGBA{{236, 236, 238, 255}, {40, 40, 44, 255}}
	inks := []float64{0, 255}
	for g := range grounds {
		for y := range side {
			for x := range side {
				c := grounds[g]
				if ix, iy := x-pad, y-pad; ix >= 0 && iy >= 0 && ix < w && iy < w {
					a := float64(icon.NRGBAAt(ix, iy).A) / 255
					mix := func(v uint8) uint8 { return uint8(float64(v)*(1-a) + inks[g]*a) }
					c = color.NRGBA{mix(c.R), mix(c.G), mix(c.B), 255}
				}
				for dy := range zoom {
					for dx := range zoom {
						out.SetNRGBA((g*side+x)*zoom+dx, y*zoom+dy, c)
					}
				}
			}
		}
	}
	return out
}

func save(name string, img image.Image) {
	f, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	out := flag.String("o", "internal/app/tray", "the directory to write the icons to")
	dir := flag.String("preview", "", "a directory for previews")
	flag.Parse()
	// 15.6 of the menu bar's 18 points, as its other icons stand.
	const fit = 15.6 / 18
	icon := render(36, fit, template)
	save(filepath.Join(*out, "tray-template.png"), icon)
	save(filepath.Join(*out, "tray.png"), render(64, 0.94, colored))
	if *dir != "" {
		save(filepath.Join(*dir, "preview-2x.png"), preview(icon, 8))
		save(filepath.Join(*dir, "preview-1x.png"), preview(render(18, fit, template), 12))
		save(filepath.Join(*dir, "preview-color.png"), render(256, 0.94, colored))
	}
}
