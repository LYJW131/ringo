package amhelper

// What amhelper keeps on disk, in the folder Config.Cache names, so that a song
// played before plays at once after the app starts again, without asking the
// wrapper anything:
//
//	keys/<sha256 of the key's URI>.json   a key's template, as the wrapper gave it
//	tracks.json                           how songs were worked out
//
// Neither is trusted blindly. A template, which the wrapper gives anew each
// time, different but as good, has shown no sign of going stale; but a stale
// one wouldn't say so, it would decrypt to noise. So the first samples each
// key decrypts are checked (alac.go), and a key that fails is dropped and
// asked for again before any of it reaches the app. A song's file that's
// gone from Apple's servers is worked out again.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	tracksKept = 2000
	// keysKept: a template for each song kept, most of them one key each,
	// so that a song worked out is played without the wrapper, which may be
	// asleep. About 60 KB each, 120 MB at most.
	keysKept = tracksKept
	// trackTTL: Apple keeps a song's file where it is, but not forever.
	trackTTL = 7 * 24 * time.Hour
)

type store struct {
	dir string // "" keeps nothing

	mu    sync.Mutex
	dirty bool
}

func (s *store) keyPath(uri string) string {
	sum := sha256.Sum256([]byte(uri))
	return filepath.Join(s.dir, "keys", hex.EncodeToString(sum[:16])+".json")
}

// key returns the template kept for uri, as the wrapper gave it.
func (s *store) key(uri string) []byte {
	if s.dir == "" {
		return nil
	}
	path := s.keyPath(uri)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now) // used: kept the longest
	return raw
}

func (s *store) putKey(uri string, raw []byte) {
	if s.dir == "" {
		return
	}
	dir := filepath.Join(s.dir, "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	if err := writeFile(s.keyPath(uri), raw); err != nil {
		log.Printf("amhelper: keeping a key: %v", err)
		return
	}
	// The ones used last are kept.
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= keysKept {
		return
	}
	type file struct {
		name string
		at   time.Time
	}
	files := make([]file, 0, len(entries))
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			files = append(files, file{e.Name(), info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.After(files[j].at) })
	for _, f := range files[min(keysKept, len(files)):] {
		_ = os.Remove(filepath.Join(dir, f.name))
	}
}

func (s *store) dropKey(uri string) {
	if s.dir != "" {
		_ = os.Remove(s.keyPath(uri))
	}
}

// keptTrack is a track as tracks.json keeps it.
type keptTrack struct {
	MediaURL   string    `json:"media"`
	Keys       []string  `json:"keys"`
	SampleRate int       `json:"rate"`
	BitDepth   int       `json:"bits"`
	Bandwidth  int       `json:"bandwidth"`
	Seconds    float64   `json:"seconds,omitempty"`
	At         time.Time `json:"at"`
}

// tracks reads tracks.json, leaving out what's too old.
func (s *store) tracks() map[string]cachedTrack {
	out := map[string]cachedTrack{}
	if s.dir == "" {
		return out
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, "tracks.json"))
	if err != nil {
		return out
	}
	var kept map[string]keptTrack
	if json.Unmarshal(raw, &kept) != nil {
		return out
	}
	for k, t := range kept {
		if time.Since(t.At) < trackTTL && t.MediaURL != "" && len(t.Keys) > 0 {
			out[k] = cachedTrack{track{MediaURL: t.MediaURL, Keys: t.Keys, SampleRate: t.SampleRate, BitDepth: t.BitDepth, Bandwidth: t.Bandwidth, Seconds: t.Seconds}, t.At}
		}
	}
	return out
}

// saveTracks writes tracks a few seconds after they change, once for all
// the changes of those seconds.
func (s *store) saveTracks(all func() map[string]cachedTrack) {
	if s.dir == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dirty {
		return
	}
	s.dirty = true
	time.AfterFunc(3*time.Second, func() {
		s.mu.Lock()
		s.dirty = false
		s.mu.Unlock()
		kept := map[string]keptTrack{}
		for k, c := range all() {
			kept[k] = keptTrack{c.t.MediaURL, c.t.Keys, c.t.SampleRate, c.t.BitDepth, c.t.Bandwidth, c.t.Seconds, c.at}
		}
		raw, err := json.Marshal(kept)
		if err == nil {
			err = writeFile(filepath.Join(s.dir, "tracks.json"), raw)
		}
		if err != nil {
			log.Printf("amhelper: keeping the songs worked out: %v", err)
		}
	})
}

// writeFile writes path whole or not at all.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
