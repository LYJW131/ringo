package applemusic

import (
	"net/http"
	"slices"
	"sync"
	"time"

	"mygo-music/internal/music"
)

// Getting ready before the listener picks a song, so it starts at once.

// Apple's play servers are a TLS handshake away, which from far off costs a
// good part of a second. While the listener browses, the connections are kept
// open, so that the song they pick next starts sooner.
const warmEvery = 40 * time.Second

// wrapperTier is what the app last asked for (music.Ask), and whether songs
// may play through the wrapper at that: at a level other than High Quality,
// or with Dolby Atmos on, and with a wrapper to ask.
func (s *Source) wrapperTier() (string, bool) {
	ask := s.wantedQuality()
	level, atmos := music.SplitAsk(ask)
	return ask, (viaWrapper(level) || atmos != music.AtmosOff) && s.hasWrapper()
}

// throughWrapper says whether songs may play through the wrapper (wrapperTier).
func (s *Source) throughWrapper() bool {
	_, ok := s.wrapperTier()
	return ok
}

// While the listener browses, the wrapper is woken ahead of the song they'll
// pick (lossless.go). While songs play it's left alone: the songs coming up
// are worked out ahead, after which nothing asks it anything, and a wrapper
// on Cloudflare can go to sleep, which costs nothing, instead of being kept
// up for as long as the app is open. A song played within playingFor counts
// as playing: few are longer.
const playingFor = 10 * time.Minute

// wake wakes the wrapper while the listener is about, when songs play through it and none is playing.
func (s *Source) wake() {
	if !s.throughWrapper() {
		return
	}
	s.pfMu.Lock()
	idle := s.clk.Now().Sub(s.playedAt) > playingFor
	s.pfMu.Unlock()
	if idle {
		s.wakeWrapper()
	}
}

// So the songs coming up are worked out as one plays: the app doesn't say
// what it plays next, but it's most likely what follows the song on the page
// it was played from, an album or a playlist. When one of the next soon isn't
// worked out, the next comingUp are, one after the other, which takes the
// wrapper half a minute or so; then it's left alone for most of an album, and
// goes to sleep. Asking for the next one each song would keep it up.
const (
	soon     = 3
	comingUp = 12
)

// songList is the songs of a page opened, in their order. aside marks a
// list the app loaded by itself, as home: a song on a page the listener
// opened stays on that one, which is more likely what they play it from.
// Home's Recently Played has the album playing on it, by then.
type songList struct {
	ids   []string
	aside bool
}

const listedMost = 5000

// Lists the app goes on with as it asks for more: a station's next songs, a
// search's next page. The app adds them to the end of what it has, but those
// it has already, and so are they here.
const (
	runsMost   = 50
	runLongest = 500
)

// run is a list the songs go on, from the start or after the songs before.
type run struct {
	key   string
	start bool
}

func playable(tracks []music.Track) []string {
	ids := []string{}
	for _, t := range tracks {
		if !t.Unavailable && isCatalogID(t.ID) {
			ids = append(ids, t.ID)
		}
	}
	return ids
}

// note notes all as the list its songs are on. Under s.pfMu.
func (s *Source) note(all []string, weakly bool) {
	list := &songList{all, weakly}
	for _, id := range all {
		if was, ok := s.lists.get(id); weakly && ok && !was.aside {
			continue
		}
		s.lists.set(id, list)
	}
	s.lists.trim(listedMost)
}

// shown notes songs the app loaded by itself, home's, as a list it may play
// from, so that playing one works out the ones after it, but works none out
// now: the app loads home again every hour, songs playing or not. A song
// noted from a page the listener opened stays on that page's list.
func (s *Source) shown(tracks []music.Track) {
	s.pfMu.Lock()
	defer s.pfMu.Unlock()
	s.note(playable(tracks), true)
}

// listed notes the songs as a list the app may play from, so that playing
// one works out the ones after it, but works none out now. It answers the
// songs that play, the run's new ones for a run.
func (s *Source) listed(tracks []music.Track, r *run) []string {
	ids := playable(tracks)
	s.pfMu.Lock()
	defer s.pfMu.Unlock()
	if r == nil {
		s.note(ids, false)
		return ids
	}
	var before []string
	if !r.start {
		before, _ = s.runs.get(r.key)
	}
	var fresh []string
	for _, id := range ids {
		if !slices.Contains(before, id) && !slices.Contains(fresh, id) {
			fresh = append(fresh, id)
		}
	}
	all := append(slices.Clone(before), fresh...)
	if len(all) > runLongest {
		all = all[len(all)-runLongest:]
	}
	s.runs.set(r.key, all)
	s.runs.trim(runsMost)
	s.note(all, false)
	return fresh
}

// The app asks for the lyrics of every song it plays, as it starts, but for
// the audio only of a song it doesn't keep: a song played before plays from
// its cache, and says it plays by its lyrics only.

// heard is the app asking for a song's lyrics, which it does as the song starts.
func (s *Source) heard() {
	s.pfMu.Lock()
	s.playedAt = s.clk.Now()
	s.pfMu.Unlock()
}

// playing is the app asking for a song to play.
func (s *Source) playing(id string) {
	s.heard()
	if !s.throughWrapper() {
		return
	}
	s.pfMu.Lock()
	list, ok := s.lists.get(id)
	s.pfMu.Unlock()
	if !ok {
		return
	}
	at := slices.Index(list.ids, id)
	next := slices.Clone(list.ids[at+1 : min(at+1+comingUp, len(list.ids))])
	if !slices.ContainsFunc(next[:min(soon, len(next))], func(id string) bool { return !s.ready(id) }) {
		return
	}
	// The catalog says which have ALAC, for the songs past the page's first.
	s.spawn(func() {
		s.learnAudio(next)
		s.enqueue(next)
	})
}

func (s *Source) warm() {
	if s.throughWrapper() {
		s.warmLossless()
	}
	if !s.signedIn() {
		return
	}
	s.pfMu.Lock()
	now := s.clk.Now()
	due := now.Sub(s.warmed) >= warmEvery
	if due {
		s.warmed = now
	}
	s.pfMu.Unlock()
	if !due {
		return
	}
	// Starting amhelper costs a play its first tenth of a second.
	s.warmHelper()
	for _, u := range []string{"https://play.music.apple.com/", "https://play.itunes.apple.com/", "https://aod-ssl.itunes.apple.com/"} {
		s.spawn(func() {
			req, err := http.NewRequestWithContext(s.ctx, "HEAD", u, nil)
			if err != nil {
				return
			}
			req.Header.Set("User-Agent", userAgent)
			if res, err := s.client.Do(req); err == nil {
				res.Body.Close()
			}
		})
	}
}

// Songs on the page the listener opened are worked out before they pick
// one: the first few of an album, a playlist, an artist's top songs or a
// search. It's what Apple, or the wrapper, would be asked anyway once they
// play, but not more than perMinute songs a minute, one at a time, so that
// the account isn't asking for keys faster than a listener would: more wait
// for the minute to pass.
const (
	firstFew    = 3
	perMinute   = 15
	queuedMost  = comingUp + firstFew
	foundOnPage = 20
	// For lossless, more of the page goes further, for nothing from the
	// wrapper: what the catalog says of the first foundOnPage songs' audio,
	// in one request, and their playlists from Apple's CDN, a few at a time.
	// The one picked then waits for its keys only.
	findingAtOnce = 4
)

func (s *Source) ready(id string) bool {
	if ask, ok := s.wrapperTier(); ok {
		if quality := s.playsAsKnown(id, ask); viaWrapper(quality) {
			return s.preparedOrPreparing(id, quality)
		}
	}
	return s.resolvedOrResolving(id)
}

func (s *Source) prefetch(tracks []music.Track, r *run) {
	all := s.listed(tracks, r)
	wrapped := s.throughWrapper()
	if !wrapped && !s.signedIn() {
		return
	}
	if wrapped {
		page := all[:min(foundOnPage, len(all))]
		s.spawn(func() {
			s.learnAudio(page)
			s.prepareFirst(all)
			s.findAll(page)
		})
		return
	}
	s.prepareFirst(all)
}

// A station's songs come ten at a time, and the app asks for the next ten
// as the last but one plays, then plays them. So they're all worked out as
// they come, in one go: the wrapper is asked nothing more until the next
// ten, half an hour on, and sleeps between. The app won't ask for the first
// of them ahead, as it does a page's next song, so worked out as it plays,
// it would wait for a wrapper woken from sleep.

// A station asked again after a while is most likely played anew, and the
// app's queue is then only what it gives.
const stationAnew = 2 * time.Hour

// stationSongs is the app given the station's next songs, which it plays after those it has.
func (s *Source) stationSongs(station string, tracks []music.Track) {
	s.pfMu.Lock()
	now := s.clk.Now()
	last, seen := s.stationAt.get(station)
	start := !seen || now.Sub(last) > stationAnew
	s.stationAt.set(station, now)
	s.stationAt.trim(runsMost)
	s.pfMu.Unlock()
	fresh := s.listed(tracks, &run{key: "station:" + station, start: start})
	if s.throughWrapper() {
		s.spawn(func() {
			s.learnAudio(fresh)
			s.enqueue(fresh)
		})
		return
	}
	if s.signedIn() {
		s.prepareFirst(fresh)
	}
}

func (s *Source) findAll(ids []string) {
	for i := 0; i < len(ids); i += findingAtOnce {
		var wg sync.WaitGroup
		for _, id := range ids[i:min(i+findingAtOnce, len(ids))] {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.find(id)
			}()
		}
		wg.Wait()
	}
}

func (s *Source) prepareFirst(all []string) {
	s.enqueue(all[:min(firstFew, len(all))])
}

func (s *Source) enqueue(songs []string) {
	var ids []string
	for _, id := range songs {
		if !s.ready(id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	s.pfMu.Lock()
	defer s.pfMu.Unlock()
	// The songs asked for last come first; what's left of older ones waits behind.
	queue := slices.Clone(ids)
	for _, id := range s.queue {
		if !slices.Contains(ids, id) {
			queue = append(queue, id)
		}
	}
	s.queue = queue[:min(queuedMost, len(queue))]
	if !s.pumping {
		s.pumping = true
		s.spawn(s.pump)
	}
}

func (s *Source) pump() {
	for {
		s.pfMu.Lock()
		if len(s.queue) == 0 || s.ctx.Err() != nil {
			s.pumping = false
			s.pfMu.Unlock()
			return
		}
		id := s.queue[0]
		s.queue = s.queue[1:]
		s.pfMu.Unlock()
		if s.ready(id) {
			continue
		}
		for {
			s.pfMu.Lock()
			now := s.clk.Now()
			for len(s.startedAt) > 0 && now.Sub(s.startedAt[0]) >= time.Minute {
				s.startedAt = s.startedAt[1:]
			}
			if len(s.startedAt) < perMinute {
				s.pfMu.Unlock()
				break
			}
			wait := s.startedAt[0].Add(time.Minute).Sub(now)
			s.pfMu.Unlock()
			s.sleep(wait)
			if s.ctx.Err() != nil {
				break
			}
		}
		if s.ready(id) {
			continue
		}
		s.pfMu.Lock()
		s.startedAt = append(s.startedAt, s.clk.Now())
		s.pfMu.Unlock()
		quality := "high"
		if ask, ok := s.wrapperTier(); ok {
			quality = s.playsAsKnown(id, ask)
		}
		if viaWrapper(quality) {
			s.prepare(id, quality, true)
		} else if s.signedIn() {
			s.resolve(id)
		}
	}
}
