package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// likes are the songs the listener made favorites, which stars show filled,
// as Apple Music's: what a source calls liking a song. A source can't always list them all: those shown are asked about, a
// frame's at a time, once each.

// wantLike notes that the heart of tr may show, to ask its source about it.
func (a *app) wantLike(tr *music.Track) {
	lib := a.trackLib(tr)
	if !lib.can("isLiked") {
		return
	}
	if _, known := lib.liked[tr.ID]; known {
		return
	}
	if lib.liked == nil {
		lib.liked = map[string]bool{}
	}
	lib.wantLikes = append(lib.wantLikes, tr.ID)
}

// askLikes asks the sources about the hearts this frame showed, as it ends.
func (a *app) askLikes() {
	for _, lib := range a.libs {
		if len(lib.wantLikes) > 0 {
			a.askLiked(lib, lib.wantLikes)
			lib.wantLikes = nil
		}
	}
}

// askLiked asks a source about tracks, which show not liked until it answers.
func (a *app) askLiked(lib *library, ids []string) {
	seen := make(map[string]bool, len(ids))
	var ask []string
	for _, id := range ids {
		if _, known := lib.liked[id]; !known && !seen[id] {
			seen[id] = true
			ask = append(ask, id)
		}
	}
	if len(ask) == 0 {
		return
	}
	if lib.liked == nil {
		lib.liked = map[string]bool{}
	}
	// Asked: they don't come up again, whatever it answers.
	for _, id := range ask {
		lib.liked[id] = false
	}
	who := lib.listener()
	go func() {
		liked, err := lib.src.IsLiked(ask)
		if err != nil {
			return
		}
		a.post(func() {
			// Forgotten meanwhile, as the listener signed out, it's asked anew;
			// and it's no one else's who signed in since.
			if lib.liked == nil || lib.listener() != who {
				return
			}
			for _, id := range ask {
				if liked[id] {
					lib.liked[id] = true
				}
			}
		})
	}()
}

// listener is the ID of who is signed in to the source, or "".
func (l *library) listener() string {
	if l.account == nil {
		return ""
	}
	return l.account.ID
}

// isLiked reports whether the listener likes tr.
func (a *app) isLiked(tr *music.Track) bool { return a.trackLib(tr).liked[tr.ID] }

// canLike reports whether tr's source likes songs.
func (l *library) canLike(tr *music.Track) bool { return l.can("like") }

// toggleLike likes a song, or unlikes it, at once in the app
// and then with its source, which may refuse: the heart goes back then.
// Signed out, it asks the listener to sign in.
func (a *app) toggleLike(tr *music.Track) {
	lib := a.trackLib(tr)
	if lib.signedOut() {
		a.promptSignIn(lib)
		return
	}
	if !lib.canLike(tr) {
		return
	}
	if lib.liked == nil {
		lib.liked = map[string]bool{}
	}
	id, like := tr.ID, !lib.liked[tr.ID]
	lib.liked[id] = like
	liked := *tr
	go func() {
		err := lib.src.Like(id, like)
		a.post(func() {
			if err != nil {
				if lib.liked != nil {
					lib.liked[id] = !like
				}
				a.actionFailed(lib, err, T("That didn't work. Try again later."))
				return
			}
			a.likedChanged(lib, liked, like)
		})
	}()
}

// likedChanged shows on the liked songs' page what was liked or unliked.
func (a *app) likedChanged(lib *library, tr music.Track, like bool) {
	// A new list: the player may be playing the old one.
	var list []music.Track
	if like {
		list = append(list, tr)
	}
	for _, t := range lib.likedList {
		if t.ID != tr.ID {
			list = append(list, t)
		}
	}
	lib.likedList = list
}

// likeCurrent likes the song playing, or unlikes it, from the keyboard (⌘D),
// which shows no heart: a toast says which it did.
func (a *app) likeCurrent() {
	tr := a.pl.Current()
	if tr == nil {
		return
	}
	lib := a.trackLib(tr)
	if !lib.canLike(tr) {
		return
	}
	a.toggleLike(tr)
	switch {
	case lib.signedOut():
		return // asked to sign in
	case !a.isLiked(tr):
		a.toast = T("Removed from Favorites")
	default:
		a.toast = T("Added to Favorites")
	}
}

// likeButton is the star that makes tr a favorite: filled in the accent once
// it is. Starring pops it in on a spring while a ring swells around it and
// confetti flies out; unstarring lets it give a little. A source that has no
// likes shows none.
func (a *app) likeButton(c *ui.Context, tr *music.Track, size float32, color ui.Color) ui.Element {
	lib := a.trackLib(tr)
	if !lib.canLike(tr) {
		return ui.Element{}
	}
	a.wantLike(tr)
	liked := a.isLiked(tr)
	icon, label := iconStar, T("Favorite")
	if liked {
		icon, color, label = iconStarFill, a.pal.accent, T("Undo Favorite")
	}
	// The burst goes on a wrapper, which the star's pop does not scale.
	wrap := ui.Box(c).Center()
	var b ui.Element
	wrap.Children(func() { b = iconButton(c, icon, size, color, label) })
	scale := b.AnimateTo("pop", 1, ui.Animation{Spring: ui.SpringOptions{Duration: 550 * time.Millisecond, Bounce: 0.55}})
	b.Scale(scale, scale)
	burst := ui.Local(wrap, "burst", func() time.Time { return time.Time{} })
	if b.Clicked() {
		switch {
		case lib.signedOut():
		case liked:
			b.Jump("pop", 0.8)
		default:
			b.Jump("pop", 0.2)
			*burst = c.Now()
		}
		a.toggleLike(tr)
	}
	if !burst.IsZero() {
		start, accent := *burst, a.pal.accent
		wrap.DrawOver(func(p *ui.Painter, r ui.Rect) {
			if drawBurst(p, r, size, p.Now().Sub(start), accent) {
				p.AnimationFrame()
			}
		})
	}
	return wrap
}

// burstColors are the confetti's colors, around the heart's red.
var burstColors = []ui.Color{
	ui.Hex("#ff4d6d"), ui.Hex("#ff9f1c"), ui.Hex("#ffd166"),
	ui.Hex("#c77dff"), ui.Hex("#4cc9f0"), ui.Hex("#ff70a6"),
}

const burstTime = 750 * time.Millisecond

// drawBurst draws the burst of a heart of size in r, at elapsed since the
// like, and reports whether it goes on.
func drawBurst(p *ui.Painter, r ui.Rect, size float32, elapsed time.Duration, accent ui.Color) bool {
	t := float32(elapsed) / float32(burstTime)
	if t >= 1 || t < 0 {
		return false
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	out := func(x float32) float32 { return 1 - (1-x)*(1-x)*(1-x) } // ease out

	// The ring swells from the heart and thins away, in the first half.
	if rt := t * 2; rt < 1 {
		radius := size * (0.35 + 0.65*out(rt))
		width := size * 0.22 * (1 - rt)
		ring := (&ui.Path{}).Circle(cx, cy, radius)
		p.StrokePath(ring, width, accent.Alpha(0.9*(1-rt)))
	}

	// Two rings of confetti fly out, slowing, shrinking and fading; the
	// inner one a little behind and turned, as if thrown at once.
	for ring := range 2 {
		lag := float32(ring) * 0.08
		pt := (t - 0.12 - lag) / (0.88 - lag)
		if pt <= 0 {
			continue
		}
		pt = min(pt, 1)
		const n = 7
		dist := size * (0.55 + (1.15-0.25*float32(ring))*out(pt))
		dot := size * (0.16 - 0.04*float32(ring)) * (1 - pt*pt)
		alpha := 1 - pt*pt
		for i := range n {
			angle := (float64(i)/n + float64(ring)*0.5/n - 0.25) * 2 * math.Pi
			x := cx + dist*float32(math.Cos(angle))
			y := cy + dist*float32(math.Sin(angle))
			color := burstColors[(i+ring*3)%len(burstColors)].Alpha(alpha)
			p.Fill(ui.Rect{X: x - dot, Y: y - dot, W: 2 * dot, H: 2 * dot}, color, dot)
		}
	}
	return true
}
