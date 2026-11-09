// Command icon draws the app's icons and the files of its brand from one
// geometry: the layers of AppIcon.icon, the Icon Composer document that macOS
// 26 and later render in Liquid Glass, and resources/icon.png, the same icon
// as macOS draws it, on the grid of older macOS icons, for older macOS,
// Windows and Linux; from AppIcon-dev.icon, a green apple where the app's is
// red, resources/icon-dev.png, the icon of mygo dev builds; and brand/*.svg,
// the mark and the wordmark on their own.
//
// The mark is an apple cut across: round, as a record is, with the core
// showing as a star of five chambers, a seed in each, in flesh that has gone
// to honey around it, as a ripe Fuji's does.
//
// Run it from the repository root on a Mac with Xcode 26 or later:
//
//	go run ./icon
//
// The icon.json of each document, its background, glass and dark-mode
// colors, is edited by hand or in Icon Composer.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ictool = "/Applications/Xcode.app/Contents/Applications/Icon Composer.app/Contents/Executables/ictool"

// skin is the apple's, a gradient down its edge.
type skin struct{ top, bottom string }

var (
	red   = skin{"#FF6A5C", "#E3213F"}
	green = skin{"#D2E86A", "#8DBF2E"}
)

// icons are the documents drawn, with the PNG rendered from each.
var icons = []struct {
	document, output string
	skin             skin
}{
	{"icon/AppIcon.icon", "resources/icon.png", red},
	{"icon/AppIcon-dev.icon", "resources/icon-dev.png", green},
}

// The slice on Icon Composer's 1024-point canvas, and what's in it, as
// parts of its radius.
const (
	cx, cy = 512.0, 512.0
	radius = 344.0
	rim    = 0.068 // the skin's width

	chamberIn, chamberOut, chamberHalf = 0.10, 0.60, 0.128
	seedIn, seedOut, seedHalf          = 0.23, 0.47, 0.058
	honey                              = 0.74 // how far out the flesh has gone to honey
	veins                              = 0.72 // where the ten dots around the core are
)

func main() {
	for _, ic := range icons {
		layers := map[string]string{
			"slice.svg": svg(1024, sliceSVG("", cx, cy, radius, ic.skin)),
			"core.svg":  svg(1024, coreSVG("", cx, cy, radius)),
		}
		for name, s := range layers {
			if err := os.WriteFile(filepath.Join(ic.document, "Assets", name), []byte(s), 0o644); err != nil {
				log.Fatal(err)
			}
		}
		if err := render(ic.document, ic.output); err != nil {
			log.Fatal(err)
		}
	}
	for name, s := range brand() {
		if err := os.WriteFile(filepath.Join("brand", name), []byte(s), 0o644); err != nil {
			log.Fatal(err)
		}
	}
}

func svg(size float64, body string) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %[1]s %[1]s">`, n(size)) + "\n" + body + "\n</svg>\n"
}

func n(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

// leaf is the shape of a chamber and of a seed: two arcs between a point at
// in and one at out along the spoke at angle deg, half wide at the middle.
func leaf(x, y, deg, in, out, half float64) string {
	t := deg * math.Pi / 180
	ux, uy := math.Cos(t), math.Sin(t)
	chord := out - in
	r := (chord*chord/4 + half*half) / (2 * half)
	return fmt.Sprintf("M%s %sA%s %s 0 0 1 %s %sA%s %s 0 0 1 %s %sZ",
		n(x+ux*in), n(y+uy*in), n(r), n(r), n(x+ux*out), n(y+uy*out), n(r), n(r), n(x+ux*in), n(y+uy*in))
}

// spokes are the angles of the five chambers, the first pointing up.
func spokes() []float64 {
	out := make([]float64, 5)
	for k := range out {
		out[k] = -90 + 72*float64(k)
	}
	return out
}

// sliceSVG is the apple cut across, around (x, y): its skin as a rim, its
// flesh, gone to honey around the core, and toward the skin the rings of a
// record, faintly. id tells its
// gradients from those of another drawing in the same file.
func sliceSVG(id string, x, y, r float64, sk skin) string {
	flesh := r * (1 - rim)
	var b strings.Builder
	fmt.Fprintf(&b, `<defs>
<linearGradient id="%[1]ss" x1="0" y1="0" x2="0" y2="1">
<stop offset="0" stop-color="%[6]s"/><stop offset="1" stop-color="%[7]s"/></linearGradient>
<radialGradient id="%[1]sf" cx="%[2]s" cy="%[3]s" r="%[5]s" gradientUnits="userSpaceOnUse">
<stop offset="0" stop-color="#FFFDF2"/><stop offset="0.6" stop-color="#FFF4D6"/><stop offset="1" stop-color="#F9E0A8"/></radialGradient>
<radialGradient id="%[1]sh" cx="%[2]s" cy="%[3]s" r="%[8]s" gradientUnits="userSpaceOnUse">
<stop offset="0" stop-color="#FFC247" stop-opacity="0.5"/><stop offset="0.7" stop-color="#FFC247" stop-opacity="0.2"/><stop offset="1" stop-color="#FFC247" stop-opacity="0"/></radialGradient>
</defs>
<circle cx="%[2]s" cy="%[3]s" r="%[4]s" fill="url(#%[1]ss)"/>
<circle cx="%[2]s" cy="%[3]s" r="%[5]s" fill="url(#%[1]sf)"/>
<circle cx="%[2]s" cy="%[3]s" r="%[8]s" fill="url(#%[1]sh)"/>
`, id, n(x), n(y), n(r), n(flesh), sk.top, sk.bottom, n(r*honey))
	// The ten dots a cut apple shows around its core, one off each
	// chamber's point and one between each two.
	for k := range 10 {
		t := (-90 + 36*float64(k)) * math.Pi / 180
		fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="#C98F2E" fill-opacity="0.5"/>`+"\n",
			n(x+math.Cos(t)*r*veins), n(y+math.Sin(t)*r*veins), n(r*0.013))
	}
	for _, at := range []float64{0.87, 0.94} {
		fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="none" stroke="#C98F2E" stroke-opacity="0.2" stroke-width="%s"/>`+"\n",
			n(x), n(y), n(flesh*at), n(r*0.008))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// coreSVG is the core, around (x, y) in a slice of radius r: its five
// chambers, and a seed in each.
func coreSVG(id string, x, y, r float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<defs>
<radialGradient id="%[1]sc" cx="%[2]s" cy="%[3]s" r="%[4]s" gradientUnits="userSpaceOnUse">
<stop offset="0.1" stop-color="#FFD76A"/><stop offset="0.65" stop-color="#F7A823"/><stop offset="1" stop-color="#EA8A0E"/></radialGradient>
</defs>
`, id, n(x), n(y), n(r*chamberOut))
	for _, deg := range spokes() {
		fmt.Fprintf(&b, `<path d="%s" fill="url(#%sc)"/>`+"\n", leaf(x, y, deg, r*chamberIn, r*chamberOut, r*chamberHalf), id)
	}
	for _, deg := range spokes() {
		fmt.Fprintf(&b, `<path d="%s" fill="#5A2A11"/>`+"\n", leaf(x, y, deg, r*seedIn, r*seedOut, r*seedHalf))
	}
	fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="#8C4A14"/>`, n(x), n(y), n(r*0.042))
	return b.String()
}

// brand is the mark and the wordmark as files of their own, by name.
func brand() map[string]string {
	const size = 512.0
	c, r := size/2, size/2-6
	mark := func(sk skin) string {
		return svg(size, sliceSVG("s", c, c, r, sk)+"\n"+coreSVG("c", c, c, r))
	}
	return map[string]string{
		"mark.svg":          mark(red),
		"mark-green.svg":    mark(green),
		"mark-mono.svg":     svg(size, monoSVG(c, c, r, "#000000")),
		"wordmark.svg":      wordmarkSVG("#2B1418"),
		"wordmark-dark.svg": wordmarkSVG("#FFF6E6"),
	}
}

// monoSVG is the mark in one color, for where it's tinted or small: the
// slice's edge as a ring, and the chambers, without their seeds.
func monoSVG(x, y, r float64, ink string) string {
	var b strings.Builder
	w := r * 0.13
	fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="%s"/>`+"\n", n(x), n(y), n(r-w/2), ink, n(w))
	for _, deg := range spokes() {
		fmt.Fprintf(&b, `<path d="%s" fill="%s"/>`+"\n", leaf(x, y, deg, r*0.12, r*0.64, r*0.165), ink)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// wordmarkSVG is the app's name in letters of one round stroke, drawn here
// rather than set in a typeface, with the mark for its o.
func wordmarkSVG(ink string) string {
	const (
		stroke = 20.0
		top    = 70.0  // of the letters without ascenders
		base   = 170.0 // the baseline
		bowl   = (base - top) / 2
		gap    = 46.0 // between the middles of strokes side by side
	)
	mid := top + bowl
	var b strings.Builder
	x := stroke / 2
	// r
	fmt.Fprintf(&b, `<path d="M%[1]s %[2]sV%[3]sM%[1]s %[4]sC%[1]s %[5]s %[6]s %[7]s %[8]s %[7]s"/>`+"\n",
		n(x), n(base), n(top), n(mid-4), n(mid-32), n(x+18), n(top+2), n(x+46))
	x += 46 + gap - 8
	// i
	fmt.Fprintf(&b, `<path d="M%[1]s %[2]sV%[3]s"/><circle cx="%[1]s" cy="%[4]s" r="%[5]s" fill="%[6]s" stroke="none"/>`+"\n",
		n(x), n(base), n(top), n(top-40), n(stroke*0.66), ink)
	x += gap
	// n
	fmt.Fprintf(&b, `<path d="M%[1]s %[2]sV%[3]sM%[1]s %[4]sA%[5]s %[5]s 0 0 1 %[6]s %[4]sV%[2]s"/>`+"\n",
		n(x), n(base), n(top), n(mid), n(bowl), n(x+2*bowl))
	x += 2*bowl + gap
	// g
	gx := x + bowl
	hook := base - 6
	fmt.Fprintf(&b, `<circle cx="%[1]s" cy="%[2]s" r="%[3]s"/><path d="M%[4]s %[5]sV%[6]sA%[3]s %[3]s 0 0 1 %[7]s %[8]s"/>`+"\n",
		n(gx), n(mid), n(bowl), n(gx+bowl), n(top), n(hook),
		n(gx+bowl*math.Cos(130*math.Pi/180)), n(hook+bowl*math.Sin(130*math.Pi/180)))
	x += 2*bowl + gap
	// o, the mark
	ox := x + bowl
	width := ox + bowl + stroke/2
	letters := b.String()
	r := bowl + stroke/2
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s 240">`+"\n"+
		`<g fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round">`+"\n%s</g>\n%s\n%s\n</svg>\n",
		n(width), ink, n(stroke), letters, sliceSVG("s", ox, mid, r, red), coreSVG("c", ox, mid, r))
}

// render has ictool draw document as macOS does, and lays it on the
// 1024-pixel grid of macOS icons at output: an 824-pixel body, centered,
// over a soft shadow.
func render(document, output string) error {
	const canvas, body = 1024, 824
	dir, err := os.MkdirTemp("", "icon")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	drawn := filepath.Join(dir, "icon.png")
	size := fmt.Sprint(body)
	if out, err := exec.Command(ictool, document, "--export-image", "--output-file", drawn,
		"--platform", "macOS", "--rendition", "Default", "--width", size, "--height", size, "--scale", "1").CombinedOutput(); err != nil {
		return fmt.Errorf("ictool (it comes with Xcode 26 or later): %v\n%s", err, out)
	}
	// ictool draws in Display P3, and the PNG this writes carries no
	// profile, which readers take as sRGB: convert, or the reds go dull.
	if out, err := exec.Command("sips", "--matchTo", "/System/Library/ColorSync/Profiles/sRGB Profile.icc", drawn).CombinedOutput(); err != nil {
		return fmt.Errorf("sips: %v\n%s", err, out)
	}
	f, err := os.Open(drawn)
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return err
	}

	at := image.Pt((canvas-body)/2, (canvas-body)/2)
	dst := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	shadow := image.NewAlpha(dst.Bounds())
	draw.Draw(shadow, src.Bounds().Add(at).Add(image.Pt(0, 12)), src, src.Bounds().Min, draw.Src)
	blur(shadow, 10)
	draw.DrawMask(dst, dst.Bounds(), image.NewUniform(color.NRGBA{A: 72}), image.Point{}, shadow, image.Point{}, draw.Over)
	draw.Draw(dst, src.Bounds().Add(at), src, src.Bounds().Min, draw.Over)

	out, err := os.Create(output)
	if err != nil {
		return err
	}
	if err := png.Encode(out, dst); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// blur approximates a Gaussian blur of the given radius with three box
// blurs in each direction.
func blur(m *image.Alpha, radius int) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	buf := make([]uint8, len(m.Pix))
	pass := func(src, dst []uint8, count, length, stride, step int) {
		for line := range count {
			base := line * stride
			sum := 0
			for i := -radius; i <= radius; i++ {
				sum += int(src[base+min(max(i, 0), length-1)*step])
			}
			for i := range length {
				dst[base+i*step] = uint8(sum / (2*radius + 1))
				sum += int(src[base+min(i+radius+1, length-1)*step]) - int(src[base+max(i-radius, 0)*step])
			}
		}
	}
	for range 3 {
		pass(m.Pix, buf, h, w, m.Stride, 1)
		pass(buf, m.Pix, w, h, 1, m.Stride)
	}
}
