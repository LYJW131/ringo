package applemusic

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// What's fetched ahead: kept while good, shared with the app's own ask, and
// waiting while the app asks for something. On the real clock.

func TestKeptWhileGood(t *testing.T) {
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	loads := 0
	load := func(func()) (int, error) { loads++; return loads, nil }
	check := func(key string, f freshness, want int) {
		t.Helper()
		if got, _ := kept(s, key, f, load, false); got != want {
			t.Errorf("%s: %d, want %d", key, got, want)
		}
	}
	check("a", freshness{version: "1", maxAge: forever}, 1)
	check("a", freshness{version: "1", maxAge: forever}, 1)
	check("a", freshness{version: "2", maxAge: forever}, 2)
	check("b", freshness{maxAge: time.Minute}, 3)
	check("b", freshness{maxAge: time.Minute}, 3)
	check("b", freshness{maxAge: 0}, 4)
	s.forget("b")
	check("b", freshness{maxAge: time.Minute}, 5)
}

func TestFailedFetchNotKept(t *testing.T) {
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	loads := 0
	_, err := kept(s, "c", freshness{maxAge: time.Minute}, func(func()) (int, error) { loads++; return 0, errors.New("no") }, false)
	if err == nil {
		t.Fatal("no error")
	}
	if got, _ := kept(s, "c", freshness{maxAge: time.Minute}, func(func()) (int, error) { loads++; return loads, nil }, false); got != 2 {
		t.Errorf("got %d", got)
	}
}

func TestForgottenWhileFetched(t *testing.T) {
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	release := make(chan struct{})
	first := make(chan string)
	go func() {
		v, _ := kept(s, "d", freshness{maxAge: time.Minute}, func(func()) (string, error) { <-release; return "old", nil }, false)
		first <- v
	}()
	time.Sleep(20 * time.Millisecond)
	s.forget("d")
	close(release)
	if v := <-first; v != "old" {
		t.Errorf("first: %s", v)
	}
	if v, _ := kept(s, "d", freshness{maxAge: time.Minute}, func(func()) (string, error) { return "new", nil }, false); v != "new" {
		t.Errorf("after: %s", v)
	}
}

func TestBackgroundWaitsForTheApp(t *testing.T) {
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	done := s.asked(false)
	var steps atomic.Int32
	ahead := make(chan int32)
	go func() {
		v, _ := kept(s, "e", freshness{maxAge: time.Minute}, func(pause func()) (int32, error) {
			steps.Add(1)
			pause()
			return steps.Add(1), nil
		}, true)
		ahead <- v
	}()
	time.Sleep(50 * time.Millisecond)
	if steps.Load() != 1 {
		t.Fatalf("went on while the app was asking: %d", steps.Load())
	}
	// The app asks for the same, while it still asks: it's shared, and no longer waits.
	joined, _ := kept(s, "e", freshness{maxAge: time.Minute}, func(func()) (int32, error) { return -1, nil }, false)
	done()
	if joined != 2 || <-ahead != 2 {
		t.Errorf("joined: %d", joined)
	}

	// Still in the background: it waits for the app, then a second more.
	done = s.asked(false)
	var finished atomic.Bool
	later := make(chan struct{})
	go func() {
		kept(s, "f", freshness{maxAge: time.Minute}, func(pause func()) (bool, error) {
			pause()
			finished.Store(true)
			return true, nil
		}, true)
		close(later)
	}()
	time.Sleep(20 * time.Millisecond)
	done()
	time.Sleep(300 * time.Millisecond)
	if finished.Load() {
		t.Error("didn't wait for the app to be quiet")
	}
	select {
	case <-later:
	case <-time.After(3 * time.Second):
		t.Fatal("never went on")
	}
}
