package amhelper

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreKeepsKeys(t *testing.T) {
	s := &store{dir: t.TempDir()}
	if s.key("skd://a") != nil {
		t.Fatal("a key never kept was found")
	}
	s.putKey("skd://a", []byte(`{"x":1}`))
	if got := s.key("skd://a"); !bytes.Equal(got, []byte(`{"x":1}`)) {
		t.Fatalf("got %q", got)
	}
	s.dropKey("skd://a")
	if s.key("skd://a") != nil {
		t.Fatal("a dropped key was found")
	}
	if (&store{}).key("skd://a") != nil {
		t.Fatal("a store without a folder found a key")
	}
}

func TestStoreKeepsTheKeysUsedLast(t *testing.T) {
	s := &store{dir: t.TempDir()}
	old := time.Now().Add(-time.Hour)
	for i := range keysKept {
		uri := "skd://" + string(rune('a'+i%26)) + time.Duration(i).String()
		s.putKey(uri, []byte("{}"))
		_ = os.Chtimes(s.keyPath(uri), old, old)
	}
	s.putKey("skd://first", []byte("{}"))
	_ = os.Chtimes(s.keyPath("skd://first"), old.Add(-time.Hour), old.Add(-time.Hour))
	s.key("skd://first") // used: now the newest
	s.putKey("skd://new", []byte("{}"))
	entries, _ := os.ReadDir(filepath.Join(s.dir, "keys"))
	if len(entries) != keysKept {
		t.Errorf("%d keys kept, want %d", len(entries), keysKept)
	}
	if s.key("skd://first") == nil || s.key("skd://new") == nil {
		t.Error("a key just used was dropped")
	}
}

func TestStoreKeepsTracks(t *testing.T) {
	s := &store{dir: t.TempDir()}
	now := time.Now()
	all := map[string]cachedTrack{
		"1/0/0/false": {track{MediaURL: "https://a.apple.com/x.mp4", Keys: []string{prefetchKey, "skd://c23"}, SampleRate: 48000, BitDepth: 24, Bandwidth: 1}, now},
		"2/0/0/false": {track{MediaURL: "https://a.apple.com/y.mp4", Keys: []string{prefetchKey}}, now.Add(-trackTTL - time.Minute)},
	}
	s.saveTracks(func() map[string]cachedTrack { return all })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(s.dir, "tracks.json")); err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	got := s.tracks()
	if len(got) != 1 {
		t.Fatalf("%d tracks read back, want the one not too old", len(got))
	}
	c := got["1/0/0/false"]
	if c.t.MediaURL != "https://a.apple.com/x.mp4" || len(c.t.Keys) != 2 || c.t.SampleRate != 48000 || !c.at.Equal(now.Truncate(0)) && c.at.Sub(now).Abs() > time.Millisecond {
		t.Errorf("read back %+v", c)
	}
}
