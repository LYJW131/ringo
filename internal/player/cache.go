package player

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"ringo/internal/music"
)

// Cache keeps the songs played on disk, so that they play again at once,
// and without the network: each named for its ID at the source and its
// quality, as <key>-<quality>.<type>, with what its stream was
// beside it in the same name's .json. Once they take more than the limit,
// those played longest ago go first.
type Cache struct {
	dir string

	mu    sync.Mutex
	limit int64 // in bytes; 0 keeps nothing
}

// NewCache returns a cache in dir of limit bytes.
func NewCache(dir string, limit int64) *Cache {
	return &Cache{dir: dir, limit: limit}
}

// SetLimit changes how much the cache may take, and drops what is over.
func (c *Cache) SetLimit(limit int64) {
	c.mu.Lock()
	c.limit = limit
	c.mu.Unlock()
	c.prune("")
}

// cacheKey names a song's files, whatever the source's IDs look like: they
// are the source's, and may hold anything.
func cacheKey(id, quality string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:16]) + "-" + quality
}

// songTypes are the types of song the app knows, which name their files.
var songTypes = []string{"m4a"}

// keepable reports whether a song of quality and type ext may be kept: they
// name its file, so only those known, which keep it in the cache's folder.
func keepable(quality, ext string) bool {
	return (slices.Contains(music.Qualities, quality) || music.IsAtmos(quality)) && slices.Contains(songTypes, ext)
}

// What the cache is asked for is an Ask: a level, and maybe how a song with a
// Dolby Atmos version plays. Its songs are each kept under what the stream
// was (music.Stream.Quality), and found by these rules:
//
//   - A level is served by a song kept at it or a better level, as Hi-Res
//     Lossless serves Lossless.
//   - With Dolby Atmos on, the song's Dolby Atmos version as that setting
//     plays it comes first: Apple's binaural rendering for headphones, or
//     Dolby's downmix for speakers, never the one for the other, as they
//     sound nothing alike.
//   - After it, a song kept at a level serves only if it says the song has
//     no Dolby Atmos version (NoAtmos): one kept with Dolby Atmos off may
//     have one, which the listener now wants instead.
//   - A Dolby Atmos version serves no level: it's AAC, and another mix.

// lookup returns a song kept as ask wants it, open to read, with its stream.
func (c *Cache) lookup(id, ask string) (*download, *music.Stream, bool) {
	f, st, ok := c.find(id, ask)
	if !ok {
		return nil, nil, false
	}
	// Played now: the last to go.
	now := time.Now()
	os.Chtimes(f.Name(), now, now)
	return opened(f, st.Size), st, true
}

// has reports whether a song is kept as ask wants it.
func (c *Cache) has(id, ask string) bool {
	f, _, ok := c.find(id, ask)
	if ok {
		f.Close()
	}
	return ok
}

// find opens a song kept as ask wants it, all of it there, with its stream.
func (c *Cache) find(id, ask string) (*os.File, *music.Stream, bool) {
	if c == nil {
		return nil, nil, false
	}
	level, atmos := music.SplitAsk(ask)
	if atmos != music.AtmosOff {
		if f, st, ok := c.open(id, music.AtmosQualities[atmos]); ok {
			return f, st, true
		}
	}
	at := max(slices.Index(music.Qualities, level), 0)
	for _, q := range music.Qualities[at:] {
		f, st, ok := c.open(id, q)
		if !ok {
			continue
		}
		if atmos != music.AtmosOff && !st.NoAtmos {
			f.Close()
			continue
		}
		return f, st, true
	}
	return nil, nil, false
}

// open opens the song kept under quality, all of it there, with its stream.
func (c *Cache) open(id, quality string) (*os.File, *music.Stream, bool) {
	key := cacheKey(id, quality)
	meta, err := os.ReadFile(filepath.Join(c.dir, key+".json"))
	if err != nil {
		return nil, nil, false
	}
	var st music.Stream
	if json.Unmarshal(meta, &st) != nil || !keepable(quality, st.Type) {
		return nil, nil, false
	}
	f, err := os.Open(filepath.Join(c.dir, key+"."+st.Type))
	if err != nil {
		return nil, nil, false
	}
	if info, err := f.Stat(); err != nil || info.Size() != st.Size {
		f.Close()
		return nil, nil, false
	}
	return f, &st, true
}

// download fetches the song of the stream st, asked for as ask, into a
// file, to keep once it has all come in; or into memory, if the cache would
// not keep it.
func (c *Cache) download(id, ask string, st *music.Stream) *download {
	part := c.part(st)
	if part == nil {
		return startDownload(st.URL, st.Size, nil, nil)
	}
	return startDownload(st.URL, st.Size, part, func(path string, n int64) (string, bool) {
		return c.keep(id, ask, st, path, n)
	})
}

// part makes a file for a song of the stream st to come into, or returns
// nil if the cache would not keep it. Each has a name of its own: the same
// song played again while it still comes in has another.
func (c *Cache) part(st *music.Stream) *os.File {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	limit := c.limit
	c.mu.Unlock()
	// A song of more than a quarter of it would push out too many.
	if limit <= 0 || st.Size > limit/4 {
		return nil
	}
	if os.MkdirAll(c.dir, 0o700) != nil {
		return nil
	}
	f, err := os.CreateTemp(c.dir, "*.part")
	if err != nil {
		return nil
	}
	return f
}

// keep keeps the song of the stream st, n bytes, from the file part, which
// is closed, and drops what is over the limit then. It's kept under the
// quality the source says it is, or else the level asked for. It returns
// where the song is: kept, or still at part.
func (c *Cache) keep(id, ask string, st *music.Stream, part string, n int64) (string, bool) {
	c.mu.Lock()
	limit := c.limit
	c.mu.Unlock()
	if n > limit/4 {
		return part, false
	}
	kept := *st
	if kept.Type == "" {
		kept.Type = "m4a"
	}
	kept.Size = n
	// The URL is of no use once it's here, and a source may sign it.
	kept.URL = ""
	quality, _ := music.SplitAsk(ask)
	if keepable(kept.Quality, kept.Type) {
		quality = kept.Quality
	}
	if !keepable(quality, kept.Type) {
		return part, false
	}
	key := cacheKey(id, quality)
	meta, _ := json.Marshal(kept)
	path := filepath.Join(c.dir, key+"."+kept.Type)
	if os.Rename(part, path) != nil {
		return part, false
	}
	os.WriteFile(filepath.Join(c.dir, key+".json"), meta, 0o600)
	c.prune(path)
	return path, true
}

// Size returns how many bytes the songs kept take.
func (c *Cache) Size() int64 {
	var n int64
	for _, f := range c.songs() {
		n += f.size
	}
	return n
}

// Clear drops every song kept.
func (c *Cache) Clear() {
	for _, f := range c.songs() {
		c.remove(f.path)
	}
}

type cachedSong struct {
	path string
	size int64
	used time.Time
}

func (c *Cache) songs() []cachedSong {
	entries, _ := os.ReadDir(c.dir)
	var out []cachedSong
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext == "" || !slices.Contains(songTypes, ext[1:]) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, cachedSong{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
	}
	return out
}

func (c *Cache) remove(path string) {
	os.Remove(path)
	os.Remove(strings.TrimSuffix(path, filepath.Ext(path)) + ".json")
}

// prune drops the songs played longest ago until the rest fit the limit,
// but keep, the song just kept.
func (c *Cache) prune(keep string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropParts()
	songs := c.songs()
	slices.SortFunc(songs, func(a, b cachedSong) int { return a.used.Compare(b.used) })
	var total int64
	for _, f := range songs {
		total += f.size
	}
	for _, f := range songs {
		if total <= c.limit {
			return
		}
		if f.path != keep {
			c.remove(f.path)
			total -= f.size
		}
	}
}

// dropParts removes the files of songs that never all came in, left by an
// app that quit as they came: one coming in is written to every moment.
func (c *Cache) dropParts() {
	parts, _ := filepath.Glob(filepath.Join(c.dir, "*.part"))
	for _, path := range parts {
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > time.Hour {
			os.Remove(path)
		}
	}
}
