package video

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Cache keeps the MP4s fetched on disk, so that a page opened again plays
// its cover without fetching it again, up to a limit past which those used
// longest ago go. A file's time is when it was last used.
type Cache struct {
	dir   string
	limit int64
	mu    sync.Mutex
}

// NewCache keeps up to limit bytes in dir.
func NewCache(dir string, limit int64) *Cache { return &Cache{dir: dir, limit: limit} }

func (c *Cache) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:16])+".mp4")
}

// Get is what's kept for key.
func (c *Cache) Get(key string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	p := c.path(key)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	now := time.Now()
	os.Chtimes(p, now, now)
	return b, true
}

// Put keeps b for key, then trims the cache to its limit.
func (c *Cache) Put(key string, b []byte) {
	if c == nil {
		return
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return
	}
	p := c.path(key)
	tmp := p + ".part"
	if os.WriteFile(tmp, b, 0o600) != nil || os.Rename(tmp, p) != nil {
		os.Remove(tmp)
		return
	}
	c.trim()
}

// trim removes the files used longest ago while the cache is over its
// limit.
func (c *Cache) trim() {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		size int64
		used time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, file{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].used.Before(files[j].used) })
	for _, f := range files {
		if total <= c.limit {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
