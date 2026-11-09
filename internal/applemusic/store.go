package applemusic

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// A JSON file of the source's own in its data folder: storage.json, what it
// keeps of the listener (tokens, songs worked out, covers), by the keys the
// plugin kept them under, and settings.json, what the listener set. Read
// once, when first wanted, and written whole on every change, only by its
// owner, who may read it again: 0600, as tokens are in it.
type jsonFile struct {
	path string

	mu     sync.Mutex
	loaded bool
	data   map[string]json.RawMessage
}

func (f *jsonFile) load() {
	if f.loaded {
		return
	}
	f.loaded = true
	f.data = map[string]json.RawMessage{}
	b, err := os.ReadFile(f.path)
	if err != nil {
		return
	}
	if err := json.Unmarshal(b, &f.data); err != nil {
		log.Printf("Apple Music: %s: %v", filepath.Base(f.path), err)
		f.data = map[string]json.RawMessage{}
	}
}

// get reads key into v, answering whether it was there.
func (f *jsonFile) get(key string, v any) bool {
	f.mu.Lock()
	f.load()
	raw, ok := f.data[key]
	f.mu.Unlock()
	if !ok {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

func (f *jsonFile) getString(key string) string {
	var s string
	f.get(key, &s)
	return s
}

func (f *jsonFile) set(key string, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		log.Printf("Apple Music: keeping %s: %v", key, err)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.load()
	f.data[key] = raw
	f.save()
}

func (f *jsonFile) remove(keys ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.load()
	changed := false
	for _, k := range keys {
		if _, ok := f.data[k]; ok {
			delete(f.data, k)
			changed = true
		}
	}
	if changed {
		f.save()
	}
}

// save writes the file whole, through a temporary one, so that a crash
// halfway leaves the old one. Under f.mu.
func (f *jsonFile) save() {
	b, err := json.Marshal(f.data)
	if err != nil {
		return
	}
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("Apple Music: %v", err)
		return
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(f.path)+"-*")
	if err != nil {
		log.Printf("Apple Music: %v", err)
		return
	}
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o600)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), f.path)
	}
	if err != nil {
		os.Remove(tmp.Name())
		log.Printf("Apple Music: writing %s: %v", filepath.Base(f.path), err)
	}
}
