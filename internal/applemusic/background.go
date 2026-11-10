package applemusic

import (
	"math"
	"strings"
	"sync/atomic"
	"time"
)

// Work done while the listener isn't waiting on the source: what they keep
// in their library is fetched ahead (warmup.go), so that their playlists,
// their favorites and their albums open at once. It waits whenever the app
// is asking for something, and a second after, and goes one request at a
// time.

const quiet = time.Second

// forever is a maxAge that doesn't run out.
const forever = time.Duration(math.MaxInt64)

// asked marks a call of the app's: until done is called, the work in the
// background waits. It also starts fetching the library ahead, once it's
// quiet, and wakes the wrapper for lossless, unless wakes is false: a song
// to play asks the wrapper itself if it has to.
func (s *Source) asked(wakes bool) (done func()) {
	// The first thing asked waits for the current account, which may be the
	// wrapper's, to have its token: answered before it, the app's first pages
	// would be those of nobody signed in, and it would ask the listener to
	// sign in while the wrapper was still being asked.
	s.settle.Do(func() {
		s.migrate()
		s.useWrapperAccount(false)
	})
	s.warmLibrary(firstWarmAfter)
	if wakes {
		s.wake()
	}
	s.askMu.Lock()
	s.asking++
	s.askMu.Unlock()
	return func() {
		s.askMu.Lock()
		s.asking--
		s.lastAsked = s.clk.Now()
		s.askMu.Unlock()
	}
}

func (s *Source) appQuiet() bool {
	s.askMu.Lock()
	defer s.askMu.Unlock()
	return s.asking == 0 && s.clk.Now().Sub(s.lastAsked) >= quiet
}

// idle returns once the app has asked nothing for a second, or once job,
// fetched ahead, is wanted by the app after all, or the source closes.
func (s *Source) idle(job *keptJob) {
	for !s.appQuiet() && (job == nil || !job.urgent.Load()) && s.ctx.Err() == nil {
		s.sleep(250 * time.Millisecond)
	}
}

// What was fetched, by key. An answer stays good while its version is the
// one asked for (a playlist's lastModifiedDate), or, without one, for maxAge.
// A key fetched in the background and then asked for by the app is the same
// fetch, which stops waiting for the app from then on: it's the app's.

type keptJob struct {
	urgent  atomic.Bool
	version string
	done    chan struct{}
	value   any
	err     error
}

type keptEntry struct {
	value   any
	at      time.Time // zero while nothing was ever kept
	version string
	job     *keptJob
}

// freshness says how long an answer stays good.
type freshness struct {
	// version is what the answer has to have been fetched for; "" for any.
	version string
	maxAge  time.Duration
}

const keptMost = 200

// kept answers key, kept or fetched with load. ahead marks a fetch for the
// background, whose load calls pause between its requests to wait for the
// app; the app's own don't wait.
func kept[T any](s *Source, key string, fresh freshness, load func(pause func()) (T, error), ahead bool) (T, error) {
	var zero T
	now := s.clk.Now()
	s.keptMu.Lock()
	e, _ := s.entries.get(key)
	if e != nil && !e.at.IsZero() && now.Sub(e.at) < fresh.maxAge && (fresh.version == "" || e.version == fresh.version) {
		v := e.value
		s.keptMu.Unlock()
		return v.(T), nil
	}
	if e != nil && e.job != nil && e.job.version == fresh.version {
		job := e.job
		if !ahead {
			job.urgent.Store(true)
		}
		s.keptMu.Unlock()
		<-job.done
		if job.err != nil {
			return zero, job.err
		}
		return job.value.(T), nil
	}
	job := &keptJob{version: fresh.version, done: make(chan struct{})}
	job.urgent.Store(!ahead)
	entry := &keptEntry{job: job}
	if e != nil {
		entry.value, entry.at, entry.version = e.value, e.at, e.version
	}
	s.entries.set(key, entry)
	s.entries.trim(keptMost)
	s.keptMu.Unlock()

	value, err := load(func() {
		if !job.urgent.Load() {
			s.idle(job)
		}
	})

	s.keptMu.Lock()
	// Forgotten while it was fetched: what came is already old.
	if cur, _ := s.entries.get(key); cur == entry {
		switch {
		case err == nil:
			s.entries.set(key, &keptEntry{value: value, at: s.clk.Now(), version: fresh.version})
		case e != nil && !e.at.IsZero():
			s.entries.set(key, &keptEntry{value: e.value, at: e.at, version: e.version})
		default:
			s.entries.delete(key)
		}
	}
	job.value, job.err = value, err
	close(job.done)
	s.keptMu.Unlock()
	return value, err
}

// forget forgets every answer whose key starts with prefix, as it changed.
func (s *Source) forget(prefix string) {
	s.keptMu.Lock()
	defer s.keptMu.Unlock()
	for _, k := range s.entries.keys() {
		if strings.HasPrefix(k, prefix) {
			s.entries.delete(k)
		}
	}
}

// forgetAll forgets everything: another listener signed in, or none.
func (s *Source) forgetAll() {
	s.keptMu.Lock()
	defer s.keptMu.Unlock()
	s.entries.clear()
}
