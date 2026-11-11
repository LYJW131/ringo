package app

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
	"ringo/internal/video"
)

// Animated Art: the covers Apple Music has moving, played without sound and
// over and over wherever their still shows large: an album's or a
// playlist's at the head of its page and on its cards, a song's album's in
// Now Playing, an artist's across the head of their page, and Home's picks
// on their cards three to four. Small ones, in rows, bars, the sidebar and
// the notch, stay still: there they would only flicker, and take players
// from the large ones. The still shows first, and the video fades in over
// it once it has a frame.
// Most of what the source answers doesn't say which covers move, so those
// that show are asked about, a frame's at a time, once each (motionKnown).

// motionSize is the biggest variant fetched for a place, in pixels. Past
// these, the next sizes up took about twice the CPU to decode for no
// difference at the size they show: a card at
// 436×582 takes some 6% of a core, a page's head at 486² as much, an
// artist's at 768×432 some 8%.
// motionLeast is the fewest pixels across a cover is asked for at, to move:
// a card's, some 200 points.
const motionLeast = 400

// prime marks the places a cover is the most of: one of them that shows
// takes a player from a smaller cover that shows, where all are taken.
type motionSize struct {
	w, h  int
	prime bool
}

var (
	// motionSquare is a card's cover, some 200 points across: Apple's 456
	// square is as sharp as that is on a Retina display.
	motionSquare = motionSize{456, 456, false}
	motionCard   = motionSize{436, 582, false}
	motionHead   = motionSize{486, 486, true}
	motionArtist = motionSize{768, 432, true}
	// motionArtistMac is an artist's head where the Mac's own decoder plays
	// it (video.Hardware), as large as Music on the Mac plays it: there it
	// takes almost no CPU, where FFmpeg's took half a core.
	motionArtistMac = motionSize{1920, 1080, true}
	// motionStage is the cover of Now Playing, some 450 points across;
	// motionStageMac, as sharp as that is on a Retina display, where the
	// Mac's decoder plays it.
	motionStage    = motionSize{486, 486, true}
	motionStageMac = motionSize{960, 960, true}
)

// artistMotion is the biggest variant fetched for the head of an artist's
// page on this machine.
func artistMotion() motionSize {
	if video.Hardware() {
		return motionArtistMac
	}
	return motionArtist
}

// stageMotion is the biggest variant fetched for the cover of Now Playing
// on this machine.
func stageMotion() motionSize {
	if video.Hardware() {
		return motionStageMac
	}
	return motionStage
}

// motionPlayers is how many covers decode at once with FFmpeg's decoder:
// five cards took 20–40% of a core between them on an M4 Pro, so four keep
// it near a third at most, and a screen of Home shows three or four cards.
// Others show their still. motionPlayersMac is as many where the Mac's own
// decoder plays them, which takes almost no CPU: a screen of cards shows a
// dozen covers.
const (
	motionPlayers    = 4
	motionPlayersMac = 12
)

// motionMost is how many covers decode at once on this machine.
func motionMost() int {
	if video.Hardware() {
		return motionPlayersMac
	}
	return motionPlayers
}

// motionKeep is how long a cover out of sight keeps its player, to play on
// at once as it comes back; then it lets go of it and its memory.
const motionKeep = 5 * time.Second

// motionStale is how long since a cover last showed before another may
// take its player: a cover that shows is built every frame it plays.
const motionStale = 250 * time.Millisecond

// motionRetry is how long a cover whose video didn't load shows its still
// before it's tried again: what failed may have been the network.
const motionRetry = time.Minute

// motionCacheBytes is how much the videos fetched may take on disk.
const motionCacheBytes = 200 << 20

// motionClip is a cover's video, for a place of a size.
type motionClip struct {
	player  *ui.VideoPlayer
	loading bool
	failed  bool
	prime   bool // of a prime place (motionSize)
	seen    time.Time
}

// motionArt keeps the covers' players, and starts them as they show. Its
// methods run on the main thread.
type motionArt struct {
	clips map[string]*motionClip
	// load fetches and opens a cover's video, on a goroutine; post runs
	// what it found on the main thread, and redraws.
	load func(url string, w, h int, size motionSize) (ui.VideoDecoder, error)
	post func(func())
	// later runs a function on the main thread after a while, without
	// redrawing, to let go of what stopped showing; nil leaves that to the
	// next cover asked for.
	later    func(time.Duration, func())
	sweeping bool
	// players is how many decode at once, if not motionMost: for tests.
	players int
}

func newMotionArt(post func(func())) *motionArt {
	return &motionArt{clips: map[string]*motionClip{}, load: loadMotion, post: post}
}

var motionCache = sync.OnceValue(func() *video.Cache {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return nil
	}
	video.SetCacheDir(filepath.Join(dir, "wazero"))
	return video.NewCache(filepath.Join(dir, "motion"), motionCacheBytes)
})

// loadMotion fetches the variant of a cover's video for w×h pixels, from
// the cache if it's there, and opens it, for the Mac's own decoder where
// there is one. The cache keeps it by the variant, which places of other
// sizes may pick too; the master playlist that says which is a few
// kilobytes.
func loadMotion(url string, w, h int, size motionSize) (ui.VideoDecoder, error) {
	cache := motionCache()
	v, err := video.Choose(url, w, h, size.w, size.h, music.Fetch)
	if err != nil {
		return nil, err
	}
	data, ok := cache.Get(v.URI)
	if !ok {
		if data, err = video.FetchVariant(v, music.Fetch); err != nil {
			return nil, err
		}
		cache.Put(v.URI, data)
	}
	return video.Play(data)
}

// motionOn reports whether covers move: the listener wants them to, the
// desktop doesn't ask for less motion, and this machine can decode them.
func (a *app) motionOn(c *ui.Context) bool {
	return a.settings.AnimatedArt && !c.Preferences().ReduceMotion && video.Supported()
}

func (a *app) motionArt() *motionArt {
	if a.motion == nil {
		a.motion = newMotionArt(a.post)
		if a.postFn == nil {
			a.motion.later = func(d time.Duration, fn func()) {
				time.AfterFunc(d, func() { mygo.RunOnMain(fn) })
			}
		}
	}
	return a.motion
}

// motionOver plays the cover of mv moving over the still that el shows,
// w×h, its corners rounded by radius, once it has a frame, and only while
// el shows. Nothing is fetched or decoded while covers don't move.
func (a *app) motionOver(c *ui.Context, el ui.Element, mv moving, w, h float32, size motionSize, radius float32, focus ...float32) {
	var p *ui.VideoPlayer
	if a.motionOn(c) {
		if url := a.movingURL(mv); url != "" {
			// Sharp on a Retina display: twice the place, as far as size goes.
			p = a.motionArt().player(c.Now(), url, int(w*2), int(h*2), size, el.InView())
		}
	}
	if p == nil {
		el.Jump("motion", 0)
		return
	}
	in := el.AnimateTo("motion", 1, artFade)
	if in <= 0 {
		return
	}
	v := ui.Video(c.Key("motion"), p).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).Radius(radius)
	if len(focus) == 2 {
		v.ObjectPosition(focus[0], focus[1])
	}
	if in < 1 {
		v.Opacity(in)
	}
}

// player is the player of the video at url for a place of w×h pixels, nil
// until it has a frame. A cover asked for the first time starts loading
// whether it shows or not, as what shows isn't known until it has been
// laid out once; after that, only one that shows is played, or takes a
// player of those that stopped showing.
func (m *motionArt) player(now time.Time, url string, w, h int, size motionSize, shows bool) *ui.VideoPlayer {
	key := fmt.Sprintf("%s|%dx%d", url, size.w, size.h)
	cl := m.clips[key]
	if cl != nil && cl.failed && now.Sub(cl.seen) >= motionRetry {
		delete(m.clips, key)
		cl = nil
	}
	switch {
	case cl != nil && cl.failed:
		return nil
	case cl != nil && !shows:
		return nil
	case cl == nil:
		if !m.room(now, size.prime) {
			return nil
		}
		cl = &motionClip{loading: true, prime: size.prime}
		m.clips[key] = cl
		go func() {
			dec, err := m.load(url, min(w, size.w), min(h, size.h), size)
			var p *ui.VideoPlayer
			if err == nil {
				// The first frame decodes here, off the main thread.
				if p, err = ui.NewVideoPlayer(dec); err == nil {
					p.SetLoop(true)
				}
			}
			m.post(func() {
				cl.loading = false
				if m.clips[key] != cl {
					// Let go of while it loaded.
					if p != nil {
						p.Close()
					}
					return
				}
				// A cover that doesn't play shows its still, and isn't
				// tried again for motionRetry.
				cl.player, cl.failed = p, err != nil
			})
		}()
	}
	cl.seen = now
	m.sweepLater()
	return cl.player
}

// room makes room for another player, letting go of the one that showed
// longest ago if as many as can decode at once have one. While all of
// those show, a prime place takes the player of a smaller cover that
// does, and another finds none free.
func (m *motionArt) room(now time.Time, prime bool) bool {
	busy := 0
	var oldest, minor string
	for key, cl := range m.clips {
		if cl.player == nil && !cl.loading {
			continue
		}
		busy++
		if now.Sub(cl.seen) >= motionStale && (oldest == "" || cl.seen.Before(m.clips[oldest].seen)) {
			oldest = key
		}
		if !cl.prime && !cl.loading && (minor == "" || cl.seen.Before(m.clips[minor].seen)) {
			minor = key
		}
	}
	if busy < m.most() {
		return true
	}
	switch {
	case oldest != "":
		m.drop(oldest)
	case prime && minor != "":
		// That cover shows its still, and finds no player free until one
		// stops showing.
		m.drop(minor)
	default:
		return false
	}
	return true
}

// most is how many covers decode at once.
func (m *motionArt) most() int {
	if m.players > 0 {
		return m.players
	}
	return motionMost()
}

// drop lets go of a cover's player, and of its decoder and memory.
func (m *motionArt) drop(key string) {
	if cl := m.clips[key]; cl != nil && cl.player != nil {
		cl.player.Close()
	}
	delete(m.clips, key)
}

// sweep lets go of the covers that haven't shown for motionKeep; those that
// failed stay, so as not to be tried again before motionRetry.
func (m *motionArt) sweep(now time.Time) {
	for key, cl := range m.clips {
		if !cl.failed && !cl.loading && now.Sub(cl.seen) >= motionKeep {
			m.drop(key)
		}
	}
}

// sweepLater sweeps once covers have had time to stop showing, and again
// while any has a player.
func (m *motionArt) sweepLater() {
	if m.sweeping || m.later == nil {
		return
	}
	m.sweeping = true
	m.later(motionKeep, func() {
		m.sweeping = false
		m.sweep(time.Now())
		for _, cl := range m.clips {
			if cl.player != nil || cl.loading {
				m.sweepLater()
				return
			}
		}
	})
}

// stop lets go of every cover's player, as covers stop moving.
func (m *motionArt) stop() {
	if m == nil {
		return
	}
	for key, cl := range m.clips {
		if !cl.failed {
			m.drop(key)
		}
	}
}
