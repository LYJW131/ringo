package applemusic

import (
	"log"
	"time"
)

// What's fetched of the listener's library before they open it: their
// favorites, albums, stations and what they added lately, and the playlists
// they changed last. A few seconds after the source is first asked, and
// again each time the app lists their playlists, which is when it comes back
// to the front. One thing at a time, each waiting for the app to be quiet
// (background.go); what's still good isn't fetched again, so a playlist is
// fetched again only once it has changed.

const (
	firstWarmAfter = 5 * time.Second
	warmPlaylists  = 20
	warmEveryRun   = time.Minute
	warmBetween    = 300 * time.Millisecond
)

// warmLibrary fetches the library ahead, after, unless it was a minute ago.
func (s *Source) warmLibrary(after time.Duration) {
	if !s.signedIn() {
		return
	}
	s.warmMu.Lock()
	defer s.warmMu.Unlock()
	if s.warmTimer != nil || s.warmRunning || s.clk.Now().Sub(s.lastWarm) < warmEveryRun || s.ctx.Err() != nil {
		return
	}
	t := &warmTimer{}
	s.warmTimer = t
	t.stop = s.clk.AfterFunc(after, func() {
		s.warmMu.Lock()
		if s.warmTimer != t {
			s.warmMu.Unlock()
			return
		}
		s.warmTimer = nil
		s.warmRunning = true
		s.lastWarm = s.clk.Now()
		gen := s.warmGen
		s.warmMu.Unlock()
		s.spawn(func() { s.runWarmup(gen) })
	})
}

// warmTimer is the fetching ahead waiting to start.
type warmTimer struct{ stop func() bool }

// stopWarming stops what's fetched ahead: the listener signed in or out.
func (s *Source) stopWarming() {
	s.warmMu.Lock()
	defer s.warmMu.Unlock()
	s.warmGen++
	if s.warmTimer != nil {
		s.warmTimer.stop()
	}
	s.warmTimer = nil
	s.lastWarm = time.Time{}
}

func (s *Source) runWarmup(gen int) {
	defer func() {
		s.warmMu.Lock()
		s.warmRunning = false
		s.warmMu.Unlock()
	}()
	steps := []func() error{
		func() error { _, err := s.libraryPlaylists(true); return err },
		func() error { _, err := s.liked(0, true); return err },
		func() error { _, err := s.savedAlbums(0, true); return err },
		func() error { _, err := s.stations(true); return err },
		func() error { _, err := s.recentlyAdded(true); return err },
	}
	for i := 0; i < len(steps); i++ {
		s.idle(nil)
		s.warmMu.Lock()
		stale := gen != s.warmGen
		s.warmMu.Unlock()
		if stale || !s.signedIn() || s.ctx.Err() != nil {
			return
		}
		if err := steps[i](); err != nil {
			log.Printf("Apple Music: fetching ahead: %v", err)
		}
		// The playlists, once the list says which changed last.
		if i == 0 {
			ids := s.recentlyChanged()
			for _, id := range ids[:min(warmPlaylists, len(ids))] {
				steps = append(steps, func() error { _, err := s.libraryPlaylist(id, true); return err })
			}
		}
		s.sleep(warmBetween)
	}
}
