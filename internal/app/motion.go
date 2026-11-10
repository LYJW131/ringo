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
// over and over where their still shows, as Apple's apps play them: an
// album's or a playlist's at the head of its page, an artist's across the
// head of theirs, and Home's picks on their cards three to four. The still
// shows first, and the video fades in over it once it has a frame.

// motionSize is the biggest variant fetched for a place, in pixels. Past
// these, the next sizes up took about twice the CPU to decode for no
// difference at the size they show (.work/motion/decision.md): a card at
// 436×582 takes some 6% of a core, a page's head at 486² as much, an
// artist's at 768×432 some 8%.
type motionSize struct{ w, h int }

var (
	motionCard   = motionSize{436, 582}
	motionHead   = motionSize{486, 486}
	motionArtist = motionSize{768, 432}
)

// motionPlayers is how many covers decode at once: five cards took 20–40%
// of a core between them on an M4 Pro, so four keep it near a third at
// most, and a screen of Home shows three or four cards. Others show their
// still.
const motionPlayers = 4

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
// the cache if it's there, and opens it. The cache keeps it by the
// variant, which places of other sizes may pick too; the master playlist
// that says which is a few kilobytes.
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
	return video.Open(data)
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

// motionOver plays the video at url over the still that el shows, w×h,
// its corners rounded by radius, once it has a frame, and only while el
// shows. Nothing is fetched or decoded while covers don't move.
func (a *app) motionOver(c *ui.Context, el ui.Element, url string, w, h float32, size motionSize, radius float32, focus ...float32) {
	var p *ui.VideoPlayer
	if url != "" && a.motionOn(c) {
		// Sharp on a Retina display: twice the place, as far as size goes.
		p = a.motionArt().player(c.Now(), url, int(w*2), int(h*2), size, el.InView())
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
		if !m.room(now) {
			return nil
		}
		cl = &motionClip{loading: true}
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
// longest ago if as many as can decode at once have one, and none is
// free while all of those show.
func (m *motionArt) room(now time.Time) bool {
	busy := 0
	var oldest string
	for key, cl := range m.clips {
		if cl.player == nil && !cl.loading {
			continue
		}
		busy++
		if now.Sub(cl.seen) >= motionStale && (oldest == "" || cl.seen.Before(m.clips[oldest].seen)) {
			oldest = key
		}
	}
	if busy < motionPlayers {
		return true
	}
	if oldest == "" {
		return false
	}
	m.drop(oldest)
	return true
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
