package applemusic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"ringo/internal/applemusic/amhelper"
)

// The tests' stand-ins for what the source reaches: Apple and amhelper
// answer from a function in this process (nothing leaves it), and time is
// simulated, so that waits of minutes take none.

// fakeClock is time that moves only when a test moves it. It counts the
// source's goroutines that aren't waiting on it, so a test can let them all
// come to rest before it looks.
type fakeClock struct {
	mu       sync.Mutex
	cond     *sync.Cond
	now      time.Time
	running  int
	sleepers []*sleeper
}

type sleeper struct {
	at    time.Time
	ch    chan struct{}
	f     func()
	fired bool
}

func newFakeClock(at time.Time) *fakeClock {
	c := &fakeClock{now: at}
	c.cond = sync.NewCond(&c.mu)
	return c
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) busy(delta int) {
	c.mu.Lock()
	c.running += delta
	c.cond.Broadcast()
	c.mu.Unlock()
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	c.mu.Lock()
	s := &sleeper{at: c.now.Add(d), ch: make(chan struct{})}
	c.sleepers = append(c.sleepers, s)
	c.running--
	c.cond.Broadcast()
	c.mu.Unlock()
	select {
	case <-s.ch:
	case <-ctx.Done():
		c.mu.Lock()
		if !s.fired {
			c.remove(s)
			c.running++
		}
		c.mu.Unlock()
	}
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := &sleeper{at: c.now.Add(d), f: f}
	c.sleepers = append(c.sleepers, s)
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		if s.fired {
			return false
		}
		c.remove(s)
		return true
	}
}

func (c *fakeClock) remove(s *sleeper) {
	if i := slices.Index(c.sleepers, s); i >= 0 {
		c.sleepers = slices.Delete(c.sleepers, i, i+1)
	}
}

// settle waits until every goroutine of the source waits on the clock or has ended.
func (c *fakeClock) settle(t testing.TB) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	done := make(chan struct{})
	defer close(done)
	go func() {
		// Wakes the wait below now and then, to see the deadline.
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				c.mu.Lock()
				c.cond.Broadcast()
				c.mu.Unlock()
			}
		}
	}()
	c.mu.Lock()
	defer c.mu.Unlock()
	for c.running > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("the source's goroutines didn't come to rest: %d running", c.running)
		}
		c.cond.Wait()
	}
}

// advance moves time on by d, waking what waits on it in order, and letting
// it run before going further.
func (c *fakeClock) advance(t testing.TB, d time.Duration) {
	t.Helper()
	c.mu.Lock()
	target := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.settle(t)
		c.mu.Lock()
		var next *sleeper
		for _, s := range c.sleepers {
			if !s.at.After(target) && (next == nil || s.at.Before(next.at)) {
				next = s
			}
		}
		if next == nil {
			c.now = target
			c.mu.Unlock()
			c.settle(t)
			return
		}
		if next.at.After(c.now) {
			c.now = next.at
		}
		c.remove(next)
		next.fired = true
		c.running++
		if next.f != nil {
			go func() {
				defer c.busy(-1)
				next.f()
			}()
		} else {
			close(next.ch)
		}
		c.mu.Unlock()
	}
}

// minutes advances by n minutes, ten seconds at a time, as the plugin's tests did.
func (c *fakeClock) minutes(t testing.TB, n float64) {
	t.Helper()
	for left := time.Duration(n * float64(time.Minute)); left > 0; left -= 10 * time.Second {
		c.advance(t, min(left, 10*time.Second))
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// answer is an HTTP answer of status with body, JSON unless it's a string.
func answer(status int, body any) *http.Response {
	var b []byte
	switch v := body.(type) {
	case nil:
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		b, _ = json.Marshal(v)
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(b)))}
}

// fakeHelper is amhelper as tests see it: it answers at helperBase, through
// the same stand-in transport as Apple.
type fakeHelper struct{}

const helperBaseURL = "http://127.0.0.1:1/s"

func (fakeHelper) Base() string { return helperBaseURL }
func (fakeHelper) Close() error { return nil }

// devJWT is a developer token that runs out at exp.
func devJWT(exp time.Time) string {
	claims, _ := json.Marshal(map[string]int64{"exp": exp.Unix()})
	return "eyJhbGciOiJFUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(claims) + ".sig"
}

type testSource struct {
	*Source
	clk     *fakeClock
	started []amhelper.Config
	mu      sync.Mutex
}

// newTestSource is a source that reaches only serve, at simulated time from
// 2026-11-08 04:00 UTC, with a developer token good for a year.
func newTestSource(t *testing.T, serve func(*http.Request) *http.Response) *testSource {
	t.Helper()
	clk := newFakeClock(time.Date(2026, 11, 8, 4, 0, 0, 0, time.UTC))
	s := New(Options{DataDir: t.TempDir()})
	ts := &testSource{Source: s, clk: clk}
	s.clk = clk
	s.client = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		res := serve(r)
		if res == nil {
			return nil, io.ErrUnexpectedEOF
		}
		res.Request = r
		return res, nil
	})}
	s.startHelper = func(cfg amhelper.Config) (helperServer, error) {
		ts.mu.Lock()
		ts.started = append(ts.started, cfg)
		ts.mu.Unlock()
		return fakeHelper{}, nil
	}
	s.storage.set(devTokenKey, devJWT(clk.Now().AddDate(1, 0, 0)))
	t.Cleanup(func() {
		s.Close()
	})
	return ts
}

// set puts settings in place without the side effects of SetSetting.
func (ts *testSource) setSettings(kv map[string]string) {
	for k, v := range kv {
		ts.settings.set(k, v)
	}
}

func readBody(r *http.Request) map[string]any {
	if r.Body == nil {
		return nil
	}
	b, _ := io.ReadAll(r.Body)
	var v map[string]any
	json.Unmarshal(b, &v)
	return v
}
