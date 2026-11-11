package app

import (
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"ringo/internal/music"
	"ringo/internal/player"
)

// The deck is what Vinyl's lyrics turn around (ui_stage_vinyl.go): a record
// of colored vinyl, half of it off the stage's left edge, turning as the
// song plays; the tonearm over it, tracking in as the song goes; and around
// it a ring of ticks that move with the music, with a marker of how far the
// song is.
//
// A record is one of a few pressings (pressings): marbled, splattered,
// smoked through dark vinyl, or one color set in another, each in its own
// colors. An album has the one nearest its cover's color, of a few, and
// keeps it.
//
// Its sizes are those of a stage 599 DIPs high, the record's center on the
// stage's left edge, halfway down: the stage scales them to its own.
const (
	vinylBase   = 599   // the height its sizes are for
	vinylRecord = 282.3 // the record's radius
	vinylLabel  = 100   // its label's
	vinylRing   = 296   // the ring's the marker runs along
	vinylTick   = 304   // where the ticks start
	vinylTurn   = 39.5  // degrees a second the record turns
)

// vinylNoise is the noise the record's shader makes its vinyl of.
const vinylNoise = `
func mod289(x vec3) vec3 {
	return x - floor(x*(1.0/289.0))*289.0
}

func mod289v(x vec4) vec4 {
	return x - floor(x*(1.0/289.0))*289.0
}

func perm(x vec4) vec4 {
	return mod289v(((x * 34.0) + 1.0) * x)
}

// sn is simplex noise in three dimensions (Ashima's, as shaders have it).
func sn(v vec3) float {
	c := vec2(1.0/6.0, 1.0/3.0)
	i := floor(v + dot(v, vec3(c.y)))
	x0 := v - i + dot(i, vec3(c.x))
	g := step(x0.yzx, x0.xyz)
	l := 1.0 - g
	i1 := min(g.xyz, l.zxy)
	i2 := max(g.xyz, l.zxy)
	x1 := x0 - i1 + vec3(c.x)
	x2 := x0 - i2 + vec3(c.y)
	x3 := x0 - vec3(0.5)
	i = mod289(i)
	p := perm(perm(perm(i.z+vec4(0, i1.z, i2.z, 1)) + i.y + vec4(0, i1.y, i2.y, 1)) + i.x + vec4(0, i1.x, i2.x, 1))
	ns := vec3(0.285714285714, -0.928571428571, 0.142857142857)
	j := p - 49.0*floor(p*ns.z*ns.z)
	xf := floor(j * ns.z)
	yf := floor(j - 7.0*xf)
	x := xf*ns.x + vec4(ns.y)
	y := yf*ns.x + vec4(ns.y)
	h := 1.0 - abs(x) - abs(y)
	b0 := vec4(x.xy, y.xy)
	b1 := vec4(x.zw, y.zw)
	s0 := floor(b0)*2.0 + 1.0
	s1 := floor(b1)*2.0 + 1.0
	sh := -step(h, vec4(0))
	a0 := b0.xzyw + s0.xzyw*sh.xxyy
	a1 := b1.xzyw + s1.xzyw*sh.zzww
	p0 := vec3(a0.xy, h.x)
	p1 := vec3(a0.zw, h.y)
	p2 := vec3(a1.xy, h.z)
	p3 := vec3(a1.zw, h.w)
	nm := 1.79284291400159 - 0.85373472095314*vec4(dot(p0, p0), dot(p1, p1), dot(p2, p2), dot(p3, p3))
	p0 *= nm.x
	p1 *= nm.y
	p2 *= nm.z
	p3 *= nm.w
	m := max(0.6-vec4(dot(x0, x0), dot(x1, x1), dot(x2, x2), dot(x3, x3)), vec4(0))
	m = m * m
	return 42.0 * dot(m*m, vec4(dot(p0, x0), dot(p1, x1), dot(p2, x2), dot(p3, x3)))
}

func fbm(p vec3) float {
	a := 0.5
	s := 0.0
	for i := 0; i < 3; i++ {
		s += a * sn(p)
		p = p*2.03 + vec3(1.7, 9.2, 3.1)
		a *= 0.5
	}
	return s
}
`

// vinylVinylSource paints a record's vinyl and the paper of its label, as
// they lie on the record, not turned: a square around it, its colors those
// of its pressing, Style the kind of pressing and Seed telling one record
// from another. The noise is slow around the record and quick across it,
// and smeared along the grooves, more or less by the kind; Flow moves it
// slowly. It is costly, and soft: it is painted small, a few times a
// second, for vinylSource to turn and light.
const vinylVinylSource = `package main

var Flow float
var Seed float
var Style float
var Deep vec4
var Base vec4
var Bright vec4
var Second vec4
var Third vec4
` + vinylNoise + `
func Fragment(dst vec4, pos vec2) vec4 {
	half := rectSize() / 2
	d := (pos - half) / half.x * 282.3
	r := length(d)
	if r > 285.0 {
		return vec4(0)
	}
	ph := atan2(d.y, d.x)
	rn := r / 282.3
	t := Flow
	o := vec3(Seed*3.1, Seed*1.7, Seed*2.3)
	smear := 0.1
	if Style > 0.5 {
		smear = 0.03
	}
	if Style > 1.5 {
		smear = 0.16
	}
	if Style > 2.5 {
		smear = 0.07
	}
	n0 := 0.0
	n1 := 0.0
	n2 := 0.0
	for s := 0; s < 5; s++ {
		a := ph + (float(s)-2.0)*smear*rn
		p := vec3(cos(a)*0.7, sin(a)*0.7, rn*3.4) + o
		w := vec3(sn(p*0.8+vec3(0, 0, t*0.10)), sn(p*0.8+vec3(5.2, 1.3, -t*0.08)), fbm(p*0.7+vec3(2.1, 7.7, t*0.06)))
		q := p + w*vec3(0.35, 0.35, 1.25)
		n0 += fbm(q*0.9 + vec3(0, 0, t*0.04))
		n1 += fbm(q*0.75 + vec3(5.2, 1.3, 3.7+t*0.03))
		n2 += sn(q*0.85 + vec3(2, 7, t*0.06))
	}
	n0 /= 5.0
	n1 /= 5.0
	n2 /= 5.0

	// The vinyl's own color, shaded.
	col := mix(Deep.rgb, Base.rgb, smoothstep(-0.45, -0.08, n0))
	col = mix(col, Bright.rgb, smoothstep(0.0, 0.35, n0))
	if Style < 0.5 {
		// Marbled: broad stains of the second color, veins of the third.
		in := smoothstep(0.23, 0.3, n1)
		stain := mix(mix(Second.rgb, Base.rgb, 0.3), Second.rgb, smoothstep(0.25, 0.36, n1))
		stain = mix(stain, Second.rgb*0.75, smoothstep(0.34, 0.5, n1))
		stain = mix(stain, mix(Second.rgb, vec3(1), 0.45), smoothstep(0.5, 0.62, n1)*0.4)
		col = mix(col, stain, in)
		vein := smoothstep(0.33, 0.41, n2) * (1.0 - smoothstep(0.5, 0.6, n2)) * (1.0 - in)
		col = mix(col, mix(Third.rgb, mix(Third.rgb, vec3(1), 0.3), smoothstep(0.42, 0.6, n2)), vein)
	} else if Style < 1.5 {
		// Splattered: blots of the second color, streaks of it flung from
		// the middle, and specks of the third.
		c := d / 282.3
		blot := sn(vec3(c*4.2, Seed)) + 0.35*sn(vec3(c*9.0, Seed+3.0))
		col = mix(col, Second.rgb, smoothstep(0.42, 0.5, blot))
		flung := sn(vec3(cos(ph)*7.0, sin(ph)*7.0, rn*1.4+Seed))
		col = mix(col, Second.rgb, smoothstep(0.52, 0.6, flung)*smoothstep(0.25, 0.6, rn)*0.85)
		col = mix(col, Third.rgb, smoothstep(0.56, 0.62, sn(vec3(c*17.0, Seed+7.0))))
	} else if Style < 2.5 {
		// Smoked: dark vinyl, with its color in wisps through it, lit in
		// places, and thin threads of the third color.
		wisp := smoothstep(-0.15, 0.45, n0)
		col = mix(Deep.rgb, Base.rgb, wisp*0.85)
		col = mix(col, Bright.rgb, smoothstep(0.2, 0.48, n1)*0.75*wisp)
		col = mix(col, Second.rgb, smoothstep(0.18, 0.45, -n1)*0.6)
		col = mix(col, Third.rgb, (1.0-smoothstep(0.0, 0.04, abs(n2-0.1)))*0.5)
	} else {
		// One color set in another: the second in the middle, its edge
		// wandering, a rim of the third between them.
		edge := 0.58 + 0.09*sn(vec3(cos(ph)*1.3, sin(ph)*1.3, Seed)) + 0.05*n2
		middle := mix(Second.rgb, Second.rgb*0.82, smoothstep(-0.2, 0.3, n1))
		col = mix(col, middle, 1.0-smoothstep(edge-0.04, edge+0.04, rn))
		col = mix(col, Third.rgb, (1.0-smoothstep(0.0, 0.05, abs(rn-edge)))*0.6*smoothstep(-0.1, 0.3, n2))
	}

	if r < 102.5 {
		// The label's paper: pale, of the record's colors.
		pl := vec3(vec2(cos(ph), sin(ph))*r*0.016, 4.2) + o
		m1 := fbm(pl*1.2 + vec3(5.2, 1.3, 3.7))
		m2 := fbm(pl + vec3(9, 2, 0))
		lab := mix(vec3(1), Base.rgb, 0.24)
		lab = mix(lab, mix(vec3(1), Base.rgb, 0.34), smoothstep(-0.1, -0.35, m1)*0.7)
		lab = mix(lab, mix(vec3(1), Second.rgb, 0.45), smoothstep(0.05, 0.3, m1)*0.5)
		lab = mix(lab, mix(vec3(1), Third.rgb, 0.5), smoothstep(0.2, 0.45, m2)*0.7)
		lab = mix(lab, mix(lab, Base.rgb, 0.3), smoothstep(80.0, 92.0, r)*0.5)
		col = lab
	}
	return vec4(col, 1)
}
`

// vinylSource paints the record, a square around it, Unit DIPs to one of
// its own, from its vinyl as vinylVinylSource painted it, Cross of the way
// from the vinyl of the record before, the second image: turned by Spin,
// under a light that stays where it is, a bright lobe up and to the right,
// a fainter one below and a shade between; with the grooves' hairlines,
// five rings and a darker rim, and on the label its rings, its grain and
// the hole in its middle.
const vinylSource = `package main

var Spin float
var Unit float
var Cross float
var Deep vec4
var Bright vec4
var Ink vec4

func grain(p vec2) float {
	return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453)
}

func ring(r, at, w float) float {
	return 1.0 - smoothstep(0.0, w, abs(r-at))
}

func Fragment(dst vec4, pos vec2) vec4 {
	d := (pos - rectSize()/2) / Unit
	r := length(d)
	aa := 1.0 / (Unit * pixelScale())
	cover := 1.0 - smoothstep(282.3-aa, 282.3+aa, r)
	if cover <= 0.0 {
		return vec4(0)
	}
	th := atan2(d.y, d.x)
	ph := th - Spin
	at := vec2(cos(ph), sin(ph)) * (min(r, 281.0) / 282.3 / 2)
	col := image0Sample((at + 0.5) * image0Size()).rgb
	if Cross < 1.0 {
		// Another song's record, coming in over the one before's.
		col = mix(image1Sample((at+0.5)*image1Size()).rgb, col, Cross)
	}

	deg := th * 57.29577951
	l1 := exp(-0.5 * pow((deg+45.0)/10.0, 2.0))
	l2 := exp(-0.5 * pow((deg-52.0)/6.4, 2.0))
	dk := smoothstep(-6.0, 2.0, deg) * (1.0 - smoothstep(34.0, 42.0, deg))
	if r >= 100.0 {
		col = mix(col, mix(Bright.rgb, vec3(1), 0.4), l1*0.62)
		col = mix(col, mix(Bright.rgb, vec3(1), 0.12), l2*0.5)
		col *= 1.0 - dk*0.2
		hair := sin(r * 6.2831853 / 3.5)
		col *= 1.0 + hair*(0.012+0.05*l1+0.03*l2)
		col *= 1.0 + 0.03*(ring(r, 105.5, 0.9)+ring(r, 141.0, 0.9)+ring(r, 177.0, 0.9)+ring(r, 213.0, 0.9)+ring(r, 249.5, 0.9))
		col *= 1.0 + 0.06*smoothstep(275.0, 277.0, r)*(1.0-smoothstep(279.0, 280.5, r))
		col = mix(col, Deep.rgb*0.8, smoothstep(280.0, 281.0, r))
		col = mix(col, vec3(0.03, 0.03, 0.04), 1.0-smoothstep(105.0-aa, 105.0+aa, r))
	} else {
		col += (grain(floor(pos*pixelScale())) - 0.5) * 0.06
		col = mix(col, col*0.93, ring(r, 78.0, 0.6))
		col = mix(col, mix(col, Ink.rgb, 0.45), ring(r, 92.5, 0.55))
		col = mix(col, mix(col, Ink.rgb, 0.65), ring(r, 96.5, 0.95))
		col = mix(col, vec3(0.03, 0.03, 0.04), smoothstep(100.0-aa, 100.0+aa, r))
		col = mix(col, vec3(0.02, 0.02, 0.012), 1.0-smoothstep(3.8-aa, 3.8+aa, r))
	}
	return vec4(col*cover, cover)
}
`

var (
	vinylVinylShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(vinylVinylSource) })
	vinylShader      = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(vinylSource) })
)

// vinylMarbleOnCPU has the record's vinyl painted where no GPU draws the
// window too, which takes the CPU seconds: a test looking at it sets it,
// and vinylPressed to the pressing it looks at.
var (
	vinylMarbleOnCPU bool
	vinylPressed     = -1
)

// The kinds of pressing.
const (
	pressMarble = iota
	pressSplatter
	pressSmoke
	pressInset
)

// pressing is a record's vinyl: its kind and its colors, the vinyl's own
// from dark to light, and the two set in it.
type pressing struct {
	kind               int
	deep, base, bright ui.Color
	second, third      ui.Color
	hue                float32 // of base, in degrees; -1 for one without color
	seed               float32 // which tells one album's from another's
}

var pressings = []pressing{
	{kind: pressMarble, deep: ui.RGB(150, 38, 100), base: ui.RGB(212, 50, 132), bright: ui.RGB(236, 52, 142), second: ui.RGB(75, 38, 112), third: ui.RGB(232, 114, 60), hue: 330},
	{kind: pressMarble, deep: ui.RGB(20, 66, 150), base: ui.RGB(36, 118, 214), bright: ui.RGB(72, 164, 242), second: ui.RGB(18, 40, 104), third: ui.RGB(126, 232, 222), hue: 212},
	{kind: pressMarble, deep: ui.RGB(16, 104, 84), base: ui.RGB(28, 164, 128), bright: ui.RGB(84, 214, 170), second: ui.RGB(12, 58, 72), third: ui.RGB(240, 214, 120), hue: 164},
	{kind: pressSplatter, deep: ui.RGB(214, 150, 38), base: ui.RGB(245, 192, 62), bright: ui.RGB(255, 218, 104), second: ui.RGB(226, 70, 58), third: ui.RGB(34, 30, 40), hue: 43},
	{kind: pressSplatter, deep: ui.RGB(212, 218, 222), base: ui.RGB(238, 240, 240), bright: ui.RGB(252, 252, 250), second: ui.RGB(60, 120, 230), third: ui.RGB(240, 90, 130), hue: -1},
	{kind: pressSplatter, deep: ui.RGB(196, 76, 150), base: ui.RGB(244, 132, 190), bright: ui.RGB(255, 176, 214), second: ui.RGB(120, 220, 200), third: ui.RGB(250, 246, 240), hue: 325},
	{kind: pressSmoke, deep: ui.RGB(22, 14, 38), base: ui.RGB(128, 80, 222), bright: ui.RGB(192, 152, 255), second: ui.RGB(50, 24, 96), third: ui.RGB(255, 132, 200), hue: 262},
	{kind: pressSmoke, deep: ui.RGB(22, 12, 10), base: ui.RGB(228, 88, 40), bright: ui.RGB(255, 170, 82), second: ui.RGB(110, 28, 20), third: ui.RGB(255, 222, 152), hue: 16},
	{kind: pressSmoke, deep: ui.RGB(10, 10, 13), base: ui.RGB(46, 46, 54), bright: ui.RGB(96, 96, 110), second: ui.RGB(20, 20, 26), third: ui.RGB(150, 150, 170), hue: -1},
	{kind: pressInset, deep: ui.RGB(16, 120, 116), base: ui.RGB(30, 172, 162), bright: ui.RGB(96, 216, 204), second: ui.RGB(246, 236, 212), third: ui.RGB(240, 122, 62), hue: 176},
	{kind: pressInset, deep: ui.RGB(150, 18, 36), base: ui.RGB(204, 32, 52), bright: ui.RGB(240, 80, 90), second: ui.RGB(22, 20, 24), third: ui.RGB(240, 202, 122), hue: 353},
	{kind: pressInset, deep: ui.RGB(40, 44, 150), base: ui.RGB(70, 78, 220), bright: ui.RGB(130, 140, 250), second: ui.RGB(250, 196, 70), third: ui.RGB(250, 250, 250), hue: 236},
}

// hueOf is a color's hue in degrees, and how colorful it is, from 0 to 1.
func hueOf(c ui.Color) (hue, chroma float32) {
	r, g, b := float32(c.R)/255, float32(c.G)/255, float32(c.B)/255
	hi, lo := max(r, g, b), min(r, g, b)
	chroma = hi - lo
	if chroma == 0 {
		return 0, 0
	}
	switch hi {
	case r:
		hue = float32(math.Mod(float64((g-b)/chroma)+6, 6))
	case g:
		hue = (b-r)/chroma + 2
	default:
		hue = (r-g)/chroma + 4
	}
	return hue * 60, chroma
}

// pressingFor is the pressing of the record tr plays from: one of the
// three nearest its cover's color, accent, by its album, so that an album
// keeps its record, and albums of a color do not all get the same; a
// cover without color gets one of those without.
func pressingFor(accent ui.Color, tr *music.Track) pressing {
	key := tr.Album.ID
	if key == "" {
		key = tr.Album.Name + tr.ID
	}
	h := uint32(2166136261)
	for _, b := range []byte(key) {
		h = (h ^ uint32(b)) * 16777619
	}
	hue, chroma := hueOf(accent)
	far := func(p pressing) float32 {
		if p.hue < 0 || chroma < 0.12 {
			if p.hue < 0 && chroma < 0.12 {
				return 0
			}
			return 120
		}
		d := abs32(p.hue - hue)
		return min(d, 360-d)
	}
	// The three nearest, nearest first.
	near := make([]int, 0, len(pressings))
	for i := range pressings {
		near = append(near, i)
	}
	for i := 1; i < len(near); i++ {
		for j := i; j > 0 && far(pressings[near[j]]) < far(pressings[near[j-1]]); j-- {
			near[j], near[j-1] = near[j-1], near[j]
		}
	}
	p := pressings[near[h%3]]
	if vinylPressed >= 0 {
		p = pressings[vinylPressed]
	}
	p.seed = float32(h>>8%1000) / 100
	return p
}

func gray(v uint8) ui.Color { return ui.RGB(v, v, v) }

func stops(at ...any) []ui.GradientStop {
	s := make([]ui.GradientStop, 0, len(at)/2)
	for i := 0; i+1 < len(at); i += 2 {
		s = append(s, ui.GradientStop{At: float32(at[i].(float64)), Color: at[i+1].(ui.Color)})
	}
	return s
}

// The metals of the tonearm, across a tube lit from the left.
var (
	armTube   = stops(0.0, ui.RGB(142, 142, 145), 0.08, gray(214), 0.2, white, 0.3, gray(233), 0.42, gray(169), 0.5, ui.RGB(124, 124, 127), 0.58, gray(185), 0.7, gray(251), 0.82, gray(220), 0.93, ui.RGB(178, 176, 181), 1.0, ui.RGB(126, 126, 130))
	armChrome = stops(0.0, ui.RGB(111, 111, 114), 0.18, gray(217), 0.32, white, 0.5, ui.RGB(162, 162, 165), 0.68, gray(241), 0.85, ui.RGB(189, 189, 192), 1.0, ui.RGB(108, 108, 112))
	armYoke   = stops(0.0, ui.RGB(86, 86, 90), 0.15, ui.RGB(62, 62, 66), 0.85, ui.RGB(44, 44, 47), 1.0, ui.RGB(68, 68, 72))
	armShell  = stops(0.0, ui.RGB(21, 21, 23), 0.2, ui.RGB(47, 47, 51), 0.45, ui.RGB(56, 56, 60), 0.8, ui.RGB(35, 35, 38), 1.0, ui.RGB(20, 20, 22))
	armCopper = stops(0.0, ui.RGB(122, 60, 22), 0.35, ui.RGB(217, 130, 43), 0.55, ui.RGB(242, 176, 103), 0.8, ui.RGB(184, 98, 31), 1.0, ui.RGB(109, 52, 18))
	// The cap over the pivot, spun metal: bands of light around it.
	armSpun = func() []ui.GradientStop {
		const n = 48
		s := make([]ui.GradientStop, 0, n+1)
		for i := range n + 1 {
			a := 2 * math.Pi * float64(i) / n
			v := 92 + 38*(0.55*math.Cos(2*a+0.6)+0.3*math.Cos(4*a+1.26)+0.15*math.Sin(7*a+0.6))
			s = append(s, ui.GradientStop{At: float32(i) / n, Color: gray(uint8(min(255, max(0, v))))})
		}
		return s
	}()
)

// deck is the stage's deck as last painted: how high its ticks stood,
// where its clock is, and where its record, its tonearm and its marker are.
type deck struct {
	levels barState
	clock  vinylClock
	// How far the record has turned, in degrees, and its vinyl flowed,
	// and how far along the song the tonearm and the marker show: they
	// go on from where they are as another song starts, or playback is
	// moved, rather than follow its position.
	spin, flow float64
	along      float32
	turnedAt   time.Time
	// The record's vinyl, as last painted: for which pressing, with which
	// label, and when; and the record before's, which the one of another
	// song fades in over, since fadeAt.
	vinyl, before      *ui.Canvas
	press, pressBefore pressing
	label, labelBefore deckLabel
	fadeAt, vinylAt    time.Time
}

// deckLabel is what a record's label says: its song, the artist and the
// album.
type deckLabel struct{ name, artist, album string }

// arcPoints are the points of an arc of radius r around (cx, cy), from a0
// to a1 in degrees clockwise from the right.
func arcPoints(cx, cy, r, a0, a1 float32) [][2]float32 {
	n := max(2, int(abs32(a1-a0)/2)+1)
	pts := make([][2]float32, n+1)
	for i := range pts {
		a := float64(a0+(a1-a0)*float32(i)/float32(n)) * math.Pi / 180
		pts[i] = [2]float32{cx + r*float32(math.Cos(a)), cy + r*float32(math.Sin(a))}
	}
	return pts
}

func disc(p *ui.Painter, cx, cy, r float32, c ui.Color) {
	p.Fill(ui.Rect{X: cx - r, Y: cy - r, W: 2 * r, H: 2 * r}, c, r)
}

func circle(p *ui.Painter, cx, cy, r, width float32, c ui.Color) {
	p.Stroke(ui.Rect{X: cx - r, Y: cy - r, W: 2 * r, H: 2 * r}, c, r, width)
}

// vinylAccent is what the accents of the lyrics make of the deck at a
// moment: how far the wheel's pulse and the glow around the record are,
// from 0 to 1.
type vinylAccent struct{ pulse, glow float32 }

// paintDeck paints the deck in r, the stage, s of its DIPs to one of the
// deck's own: tr playing, pos into dur of it.
func (a *app) paintDeck(p *ui.Painter, r ui.Rect, s float32, tr *music.Track, press pressing, pos, dur time.Duration, playing bool, ac vinylAccent) {
	cx, cy := r.X, r.Y+r.H/2
	dk := &a.deck
	now := p.Now()
	moving := false // something of it goes on moving, whether or not the song plays

	// The record turns on from where it is, by the time since it was last
	// painted, and the tonearm and the marker glide to where the song is:
	// another song, or a seek, moves nothing at once.
	dt := now.Sub(dk.turnedAt).Seconds()
	first := dk.turnedAt.IsZero()
	if first || dt < 0 || dt > 0.25 {
		dt = 0
	}
	dk.turnedAt = now
	if playing {
		dk.spin = math.Mod(dk.spin+dt*vinylTurn, 360)
		dk.flow += dt * 0.9
	}
	progress := float32(0)
	if dur > 0 {
		progress = clamp01(float32(pos.Seconds() / dur.Seconds()))
	}
	if d := progress - dk.along; first || abs32(d) < 0.0005 {
		dk.along = progress
	} else {
		dk.along += d * float32(1-math.Exp(-8*dt))
		moving = true
	}
	spin := float32(dk.spin)

	// Another song's record fades in over the one before's, as its
	// backdrop does, and its label's words with it.
	label := deckLabel{short(tr.Name, 26), short(tr.ArtistNames(), 30), tr.Album.Name}
	if !first && (dk.press != press || dk.label != label) {
		dk.before, dk.pressBefore, dk.labelBefore, dk.fadeAt = dk.vinyl, dk.press, dk.label, now
		if dk.press != press {
			dk.vinyl = nil
		}
	}
	dk.label = label
	k := float32(1)
	if dk.before != nil || !dk.fadeAt.IsZero() {
		if k = float32(now.Sub(dk.fadeAt)) / float32(backdropFade); k < 1 && k >= 0 {
			k = 1 - (1-k)*(1-k)*(1-k)
			moving = true
		} else {
			k, dk.before, dk.fadeAt = 1, nil, time.Time{}
		}
	}
	was := dk.pressBefore
	if k == 1 {
		was = press
	}
	deep, base, bright, second, third := was.deep.Mix(press.deep, k), was.base.Mix(press.base, k), was.bright.Mix(press.bright, k), was.second.Mix(press.second, k), was.third.Mix(press.third, k)
	ink := base.Mix(ui.RGB(0, 0, 0), 0.7)

	// The dark the record sits in, and the glow an accent throws off it.
	reach := float32(365) * s
	p.FillRadialGradient(ui.Rect{X: cx - reach, Y: cy - reach, W: 2 * reach, H: 2 * reach}, ui.RadialGradient{
		Radius: reach,
		Stops:  stops(0.0, ui.RGB(12, 6, 10).Alpha(0.5), float64(vinylRecord/365), ui.RGB(12, 6, 10).Alpha(0.5), 1.0, ui.RGB(12, 6, 10).Alpha(0)),
	}, 0)
	if g := ac.glow; g > 0.01 {
		reach := float32(vinylRecord+130) * s
		at := func(d float64) float64 { return (vinylRecord + d) / (vinylRecord + 130) }
		hot, warm := bright, third
		p.FillRadialGradient(ui.Rect{X: cx - reach, Y: cy - reach, W: 2 * reach, H: 2 * reach}, ui.RadialGradient{
			Radius: reach,
			Stops:  stops(0.0, hot.Alpha(0.75*g), at(0), hot.Alpha(0.75*g), at(12), hot.Alpha(0.3*g), at(37), warm.Alpha(0.16*g), at(60), warm.Alpha(0.07*g), 1.0, warm.Alpha(0)),
		}, 0)
	}

	// The record. Its vinyl is painted small, a dozen times a second as
	// it flows, and turned and lit from that in every frame. Where the
	// CPU draws the window it is plain: the shaders are far too slow there.
	R := float32(vinylRecord) * s
	if p.Accelerated() || vinylMarbleOnCPU {
		n := int(min(900, max(256, 2*R)))
		if !p.Accelerated() {
			n = 256 // a test's look at it
		}
		if dk.vinyl != nil {
			if w, _ := dk.vinyl.Size(); w != n {
				dk.vinyl = nil
			}
		}
		if since := now.Sub(dk.vinylAt); dk.vinyl == nil || dk.press != press || playing && (since >= 80*time.Millisecond || since < 0) {
			if dk.vinyl == nil {
				dk.vinyl = ui.NewCanvas(n, n)
			}
			dk.press, dk.vinylAt = press, now
			dk.vinyl.Draw(p, func(cp *ui.Painter) {
				vinylVinylShader().Draw(cp, ui.Rect{W: float32(n), H: float32(n)}, shader.Options{Uniforms: map[string]any{
					"Flow": float32(dk.flow), "Seed": press.seed, "Style": float32(press.kind),
					"Deep": press.deep, "Base": press.base, "Bright": press.bright, "Second": press.second, "Third": press.third,
				}})
			})
		}
		images, cross := [4]*ui.Bitmap{dk.vinyl.Bitmap()}, float32(1)
		if dk.before != nil && k < 1 {
			images[1], cross = dk.before.Bitmap(), k
		}
		vinylShader().Draw(p, ui.Rect{X: cx - R - 2, Y: cy - R - 2, W: 2*R + 4, H: 2*R + 4}, shader.Options{
			Images: images,
			Uniforms: map[string]any{
				"Spin": float32(float64(spin) * math.Pi / 180), "Unit": s, "Cross": cross, "Deep": deep, "Bright": bright, "Ink": ink,
			},
		})
	} else {
		dk.press = press
		p.FillConicGradient(ui.Rect{X: cx - R, Y: cy - R, W: 2 * R, H: 2 * R}, ui.ConicGradient{
			Angle: spin,
			Stops: stops(0.0, base, 0.16, second, 0.3, base, 0.52, base.Mix(third, 0.7), 0.6, base, 0.8, second, 1.0, base),
		}, R)
		disc(p, cx, cy, float32(vinylLabel+5)*s, ui.RGB(8, 8, 11))
		disc(p, cx, cy, float32(vinylLabel)*s, white.Mix(base, 0.24))
		disc(p, cx, cy, 3.8*s, ui.RGB(5, 5, 3))
	}

	p.Transform(ui.Scaling(s, s), cx, cy, func() {
		p.Transform(ui.Rotation(spin), cx, cy, func() {
			if k < 1 {
				paintLabel(p, cx, cy, dk.labelBefore, ink, 1-k)
			}
			paintLabel(p, cx, cy, label, ink, k)
		})
		a.paintRing(p, cx, cy, bright, dk.along, playing, ac.pulse)
		// The tonearm's pivot is over the record's top, and its head tracks
		// in from the edge as the song goes.
		paintArm(p, cx+171, cy-286, lerp(17.7, -8.6, dk.along))
	})
	if moving {
		p.AnimationFrame()
	}
}

// labelGlyphs are the widths of the characters of texts set around the
// label, which are measured once: by the text, its size and its weight.
var labelGlyphs = map[labelText][]float32{}

type labelText struct {
	text   string
	size   float32
	weight int
	mono   bool
}

func (t labelText) span(s string, c ui.Color) ui.Span {
	sp := ui.Span{Text: s, Size: t.size, Weight: t.weight, Color: c}
	if t.mono {
		sp.Font = "monospace"
	}
	return sp
}

// arcText sets t around (cx, cy) at radius r, its middle at degrees
// clockwise from the top, each character spaced by tracking more than its
// width: over the top, reading clockwise, or under it, reading from the
// left, its characters' heads to the center.
func arcText(p *ui.Painter, cx, cy, r, at float32, t labelText, tracking float32, under bool, c ui.Color) {
	widths, ok := labelGlyphs[t]
	if !ok {
		if len(labelGlyphs) > 64 {
			clear(labelGlyphs)
		}
		for _, ch := range t.text {
			w, _ := p.MeasureText(0, t.span(string(ch), c))
			if ch == ' ' {
				w = t.size * 0.3 // a space alone measures nothing
			}
			widths = append(widths, w)
		}
		labelGlyphs[t] = widths
	}
	total := float32(0)
	for _, w := range widths {
		total += w + tracking
	}
	_, h := p.MeasureText(0, t.span("A", c))
	x, i := -total/2, 0
	for _, ch := range t.text {
		w := widths[i]
		i++
		mid := (x + (w+tracking)/2) / r * 180 / math.Pi
		x += w + tracking
		if under {
			p.Transform(ui.Rotation(at-mid), cx, cy, func() { p.RichText(cx-w/2, cy+r-h/2, 0, t.span(string(ch), c)) })
		} else {
			p.Transform(ui.Rotation(at+mid), cx, cy, func() { p.RichText(cx-w/2, cy-r-h/2, 0, t.span(string(ch), c)) })
		}
	}
}

// centered draws s with its middle at (x, y).
func centered(p *ui.Painter, x, y float32, sp ui.Span) {
	w, h := p.MeasureText(0, sp)
	p.RichText(x-w/2, y-h/2, 0, sp)
}

// short is s, cut to n characters at most.
func short(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n-1])) + "…"
}

// paintLabel prints the record's label around (cx, cy), in ink, as much
// of it showing as fade: the song over the top, its artist under, its
// album between, the side and the speed either side of the hole, and the
// three of them again around the rim.
func paintLabel(p *ui.Painter, cx, cy float32, l deckLabel, ink ui.Color, fade float32) {
	if fade <= 0.004 {
		return
	}
	at := func(a float32) ui.Color { return ink.Alpha(a * fade) }
	soft, pale := at(0.62), at(0.55)
	arcText(p, cx, cy, 71, 0, labelText{text: l.name, size: 14.5, weight: 700}, 0.3, false, at(1))
	if l.album != "" {
		centered(p, cx, cy-28, ui.Span{Text: short(l.album, 24), Size: 7.5, Color: soft})
	}
	centered(p, cx-33, cy-11, ui.Span{Text: "Side", Size: 6.5, Color: soft})
	centered(p, cx-33, cy+3, ui.Span{Text: "A", Size: 19, Weight: 800, Color: at(0.92)})
	centered(p, cx+33, cy-1, ui.Span{Text: "33⅓", Size: 15, Weight: 700, Color: at(0.85)})
	centered(p, cx+33, cy+11, ui.Span{Text: "rpm", Size: 6.5, Color: soft})
	if l.artist != "" {
		arcText(p, cx, cy, 71, 0, labelText{text: l.artist, size: 10, weight: 600}, 0.3, true, at(0.8))
	}
	// Around the rim, as often as fits.
	rim := strings.ToLower(l.name)
	for _, s := range []string{l.artist, l.album} {
		if s != "" {
			rim += " · " + strings.ToLower(s)
		}
	}
	rim += " · "
	if n := utf8.RuneCountInString(rim); n > 0 {
		rim = strings.Repeat(rim, max(1, 110/n))
		rim = string([]rune(rim)[:min(120, utf8.RuneCountInString(rim))])
		t := labelText{text: rim, size: 5.6, weight: 400, mono: true}
		// Spaced to go once around.
		widths := float32(0)
		for _, ch := range rim {
			w, _ := p.MeasureText(0, t.span(string(ch), pale))
			if ch == ' ' {
				w = t.size * 0.3
			}
			widths += w
		}
		tracking := max(0, (2*math.Pi*85-widths)/float32(utf8.RuneCountInString(rim)))
		arcText(p, cx, cy, 85, 180, t, tracking, false, pale)
	}
}

// paintRing paints the ticks around the record at (cx, cy), in the deck's
// own sizes: short and dim around most of it, long and lit where the
// lyrics are, in the middle, as loud as the music is there, low notes
// nearest the middle; and along the ring within them, how far the song is.
func (a *app) paintRing(p *ui.Painter, cx, cy float32, lit ui.Color, progress float32, playing bool, pulse float32) {
	lv, live := a.pl.Levels()
	st := &a.deck.levels
	st.step(p.Now(), lv, live && playing)
	loud := st.group(0, player.Bands)

	p.StrokePolyline(arcPoints(cx, cy, vinylRing, -50, 50), 1, white.Alpha(0.1))
	for k := range 51 {
		deg := float32(-50 + 2*k)
		off := abs32(deg)
		// How much a tick belongs to the lit middle.
		w := clamp01((22 - off) / 6)
		length, color := float32(7), white.Alpha(0.1)
		switch {
		case off >= 28:
			v := clamp01(loud * 2 * (0.75 + 0.5*st.h[min(player.Bands-1, int(off/2))]))
			length, color = 4.5+5.5*v, white.Alpha(0.16+0.12*v)
		case w > 0:
			level := float32(math.Pow(float64(st.h[min(player.Bands-1, int(2+off*1.15+0.5))]), 0.9))
			length = 6 + w*(4+19*level) + w*pulse*5
			color = white.Alpha(0.25).Mix(white.Mix(lit, 0.6), clamp01(w*(0.25+level)))
		}
		rad := float64(deg) * math.Pi / 180
		dx, dy := float32(math.Cos(rad)), float32(math.Sin(rad))
		p.StrokePolyline([][2]float32{{cx + dx*vinylTick, cy + dy*vinylTick}, {cx + dx*(vinylTick+length), cy + dy*(vinylTick+length)}}, 1.5, color)
	}

	at := -50 + 100*progress
	if at > -49.5 {
		p.StrokePolyline(arcPoints(cx, cy, vinylRing, -50, at), 1.5, lit.Mix(white, 0.2))
	}
	rad := float64(at) * math.Pi / 180
	mx, my := cx+vinylRing*float32(math.Cos(rad)), cy+vinylRing*float32(math.Sin(rad))
	disc(p, mx, my, 6, ui.RGB(11, 9, 8))
	circle(p, mx, my, 6, 1.5, lit.Mix(white, 0.2))
	disc(p, mx, my, 2, lit.Mix(white, 0.35))
}

// screw is the head of a slotted screw at (x, y), its slot turned.
func screw(p *ui.Painter, x, y, r, turn float32) {
	disc(p, x, y, r+0.6, gray(21))
	disc(p, x, y, r, gray(196))
	disc(p, x-r*0.25, y-r*0.3, r*0.45, gray(240))
	p.Transform(ui.Rotation(turn), x, y, func() {
		p.Fill(ui.Rect{X: x - r*0.75, Y: y - r*0.19, W: r * 1.5, H: r * 0.38}, ui.RGB(60, 60, 62), 0)
	})
}

// paintArm paints the tonearm, its pivot at (px, py), leaning lean degrees
// from straight down, its head to the right of the pivot when it leans so:
// the plinth it turns on, with its clips and screws; the yoke its tube
// passes through, the tube, thinner by the pivot, and its collar; the
// headshell, askew, with its finger lift, the cartridge's wires and screws,
// and the cartridge under it; and over the pivot, a cap of spun metal.
func paintArm(p *ui.Painter, px, py, lean float32) {
	shadow := ui.RGB(0, 0, 0).Alpha(0.45)
	across := func(s []ui.GradientStop) ui.LinearGradient { return ui.LinearGradient{Stops: s, Angle: 90} }

	// The plinth, which stays.
	p.Shadow(ui.Rect{X: px - 40.5, Y: py - 40.5, W: 81, H: 81}, 40.5, 6, 4, 14, 0, shadow)
	p.FillRadialGradient(ui.Rect{X: px - 40.5, Y: py - 40.5, W: 81, H: 81}, ui.RadialGradient{
		Center: ui.Point{X: 0.45, Y: 0.4}, Radius: 48,
		Stops: stops(0.0, ui.RGB(84, 84, 87), 0.75, ui.RGB(63, 63, 66), 1.0, ui.RGB(43, 43, 45)),
	}, 40.5)
	circle(p, px, py, 40.5, 1, ui.RGB(98, 98, 101))
	circle(p, px, py, 34.7, 3, gray(205))
	circle(p, px, py, 34.7, 0.8, gray(247))
	disc(p, px, py, 31.4, ui.RGB(38, 38, 40))
	p.StrokePolyline(arcPoints(px, py, 29.5, 140, 172), 3.2, ui.RGB(15, 15, 16))
	for _, deg := range []float32{-128, -52} {
		rad := float64(deg) * math.Pi / 180
		x, y := px+28.5*float32(math.Cos(rad)), py+28.5*float32(math.Sin(rad))
		p.Transform(ui.Rotation(deg+90), x, y, func() {
			p.FillGradient(ui.Rect{X: x - 1.6, Y: y - 3.6, W: 3.2, H: 7.2}, across(armChrome), 0.6)
		})
	}
	screw(p, px+27*float32(math.Cos(2.27)), py+27*float32(math.Sin(2.27)), 2.4, 30)
	screw(p, px+27*float32(math.Cos(0.87)), py+27*float32(math.Sin(0.87)), 2.4, -20)

	p.Transform(ui.Rotation(-lean), px, py, func() {
		at := func(x, y, w, h float32) ui.Rect { return ui.Rect{X: px + x, Y: py + y, W: w, H: h} }
		// What it throws on the record under it.
		p.Shadow(at(-6.5, 60, 13, 220), 3, 7, 4, 12, 0, shadow)
		p.Shadow(at(-9, 296, 19, 62), 5, 7, 4, 12, 0, shadow)

		yoke := new(ui.Path).MoveTo(px-12.5, py+8).LineTo(px+12.5, py+8).LineTo(px+12.5, py+80).
			QuadTo(px+12.5, py+88, px+5, py+89).LineTo(px-5, py+89).QuadTo(px-12.5, py+88, px-12.5, py+80).Close()
		p.FillPathGradient(yoke, across(armYoke))
		p.FillGradient(at(-5.6, 6, 11.2, 84), across(armTube), 0)
		p.Fill(at(4.4, 78, 4, 9.5), gray(21), 0.8)
		p.Fill(at(-8.4, 78, 4, 9.5), gray(21), 0.8)
		p.Fill(at(4.9, 78.6, 1, 8.3), ui.RGB(58, 58, 61), 0)
		p.Fill(at(-7.9, 78.6, 1, 8.3), ui.RGB(58, 58, 61), 0)
		p.FillGradient(at(-6.5, 88, 13, 190), across(armTube), 1.2)
		p.Fill(at(-3.2, 90, 0.6, 186), white.Alpha(0.65), 0)
		p.Fill(at(-6.5, 88, 13, 3), ui.RGB(0, 0, 0).Alpha(0.18), 0)
		p.FillGradient(at(-7.8, 274, 15.6, 19), across(armChrome), 1.6)
		p.Fill(at(-7.8, 278.5, 15.6, 0.8), ui.RGB(109, 109, 112), 0)
		p.Fill(at(-7.8, 289.5, 15.6, 0.8), gray(232), 0)

		// The headshell, askew on the tube's end.
		hx, hy := px, py+292
		p.Transform(ui.Rotation(-7), hx, hy, func() {
			wire := func(c ui.Color, x0, y0, c1x, c1y, c2x, c2y, mx, my, c3x, c3y, x1, y1 float32) {
				w := new(ui.Path).MoveTo(hx+x0, hy+y0).CubeTo(hx+c1x, hy+c1y, hx+c2x, hy+c2y, hx+mx, hy+my).
					CubeTo(hx+c3x, hy+c3y, hx+c3x, hy+c3y+4, hx+x1, hy+y1)
				p.StrokePath(w, 0.75, c)
			}
			wire(ui.RGB(233, 226, 218), 8, 4, 15, 6, 16, 13, 11.5, 18, 9.5, 20.5, 12.5, 27)
			wire(ui.RGB(196, 64, 47), 7.5, 6, 17, 9, 17.5, 15, 12.6, 19.5, 10.8, 21.6, 13.4, 28)
			wire(ui.RGB(42, 42, 44), 7, 8, 14, 11, 14.5, 16, 11, 20, 9.6, 22, 11.6, 27.5)
			shell := new(ui.Path).MoveTo(hx-6.4, hy).LineTo(hx+6.4, hy).LineTo(hx+7.4, hy+10).QuadTo(hx+10, hy+12, hx+10.2, hy+16).
				LineTo(hx+10.6, hy+50).QuadTo(hx+10.6, hy+58, hx+6, hy+61).LineTo(hx-7.5, hy+61).QuadTo(hx-10.6, hy+58, hx-10.6, hy+50).
				LineTo(hx-10.2, hy+16).QuadTo(hx-9.6, hy+12, hx-7.4, hy+10).Close()
			p.FillPathGradient(shell, across(armShell))
			lift := new(ui.Path).MoveTo(hx+10, hy+30).LineTo(hx+34, hy+46).QuadTo(hx+37.5, hy+48.5, hx+35.2, hy+51.2).
				QuadTo(hx+33.4, hy+53, hx+30.8, hy+51).LineTo(hx+9.8, hy+37.6).Close()
			p.FillPathGradient(lift, across(armShell))
			p.StrokePolyline([][2]float32{{hx + 11, hy + 30.8}, {hx + 34, hy + 46.2}}, 0.6, ui.RGB(84, 84, 88))
			for _, at := range [][2]float32{{-4.6, 38}, {4.4, 41.5}} {
				x, y := hx+at[0], hy+at[1]
				p.Transform(ui.Rotation(-12), x, y, func() {
					p.Fill(ui.Rect{X: x - 2.6, Y: y - 6, W: 5.2, H: 12}, ui.RGB(11, 11, 12), 2.6)
				})
				p.StrokePolyline([][2]float32{{x, y + 1}, {x + 0.6, y + 6}}, 0.9, ui.RGB(191, 191, 194))
				screw(p, x, y-1, 2.3, 35)
			}
			p.Fill(ui.Rect{X: hx - 8.8, Y: hy + 52, W: 15.6, H: 14}, ui.RGB(27, 27, 29), 1.4)
			p.FillGradient(ui.Rect{X: hx - 4.8, Y: hy + 56.5, W: 7.6, H: 8.5}, across(armCopper), 0.8)
			p.Fill(ui.Rect{X: hx - 8.8, Y: hy + 52, W: 15.6, H: 1.2}, ui.RGB(59, 59, 62), 0)
			disc(p, hx-1, hy+66.5, 0.9, ui.RGB(207, 207, 210))
		})

		disc(p, px, py, 26, ui.RGB(26, 26, 27))
		p.FillConicGradient(ui.Rect{X: px - 25, Y: py - 25, W: 50, H: 50}, ui.ConicGradient{Stops: armSpun}, 25)
		p.FillRadialGradient(ui.Rect{X: px - 25, Y: py - 25, W: 50, H: 50}, ui.RadialGradient{
			Center: ui.Point{X: 0.38, Y: 0.3}, Radius: 35,
			Stops: stops(0.0, white.Alpha(0.28), 0.5, white.Alpha(0.06), 1.0, ui.RGB(0, 0, 0).Alpha(0.25)),
		}, 25)
		circle(p, px, py, 25, 0.8, ui.RGB(32, 32, 34))
		disc(p, px, py, 3.4, ui.RGB(59, 59, 62))
		disc(p, px, py, 1.4, gray(21))
	})
}
