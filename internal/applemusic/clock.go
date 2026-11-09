package applemusic

import (
	"context"
	"time"
)

// clock is the time the source goes by. Tests put a simulated one in its
// place, which runs the waits of minutes the source does (a minute's limit on
// songs worked out ahead, the wrapper left to sleep) in no time.
type clock interface {
	Now() time.Time
	// Sleep waits d, or until ctx ends.
	Sleep(ctx context.Context, d time.Duration)
	// AfterFunc calls f in a goroutine of its own after d; stop keeps it from it.
	AfterFunc(d time.Duration, f func()) (stop func() bool)
	// busy counts the source's goroutines in and out, for a simulated clock
	// to tell when they've all come to rest.
	busy(delta int)
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) Sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

func (realClock) AfterFunc(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }

func (realClock) busy(int) {}

// ms is t as JavaScript's Date.now(), which is what's kept on disk.
func ms(t time.Time) int64 { return t.UnixMilli() }

func (s *Source) now() int64 { return ms(s.clk.Now()) }

// spawn runs f in the background, where Close waits for it.
func (s *Source) spawn(f func()) {
	s.bgMu.Lock()
	if s.closed {
		s.bgMu.Unlock()
		return
	}
	s.bg.Add(1)
	s.clk.busy(1)
	s.bgMu.Unlock()
	go func() {
		defer s.bg.Done()
		defer s.clk.busy(-1)
		f()
	}()
}

// sleep waits d on the source's clock, cut short by Close.
func (s *Source) sleep(d time.Duration) { s.clk.Sleep(s.ctx, d) }
