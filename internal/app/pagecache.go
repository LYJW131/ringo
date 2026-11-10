package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// What's kept on this computer of what the listener's pages showed, so that
// the next start shows them at once, before the source has answered, and
// replaces them as it does.
//
// Where: a file in the app's user data directory (pagesDir), one for each
// source, which holds the pages of the one account last signed in to. Signing
// in to another account, or out, deletes it; the file says whose it is by a
// hash of the account's ID, and one that isn't the signed-in account's is
// dropped, with what it showed, as the source says who that is.
//
// What: Home, the first part of each page of the sidebar, the first part of
// the favorite songs, the listener's playlists, and up to pagesPlaylists
// playlists of at most pagesTracks songs. Not the pictures, which the cover
// cache keeps, but where to find them, and no token: a source's answers don't
// hold one, and the file holds nothing else of the account than the hash.
// What's more than pagesMaxBytes is left out, the playlists first.
//
// When: pagesDebounce after the last thing that was loaded, and as the app
// quits. What's read is shown as if old, so it's asked for anew.

const (
	pagesVersion   = 1
	pagesDebounce  = 3 * time.Second
	pagesMaxBytes  = 4 << 20
	pagesPlaylists = 12
	pagesTracks    = 300
	pagesRecent    = 50
)

// pageFile is what's in the file.
type pageFile struct {
	Version int
	// Lang is the language the source's titles were in.
	Lang string
	// Account is who it was loaded for, as accountKey has it.
	Account string

	Home      *music.Browse              `json:",omitempty"`
	Recent    []music.Track              `json:",omitempty"`
	Mine      []music.Playlist           `json:",omitempty"`
	Saved     []music.Playlist           `json:",omitempty"`
	Browses   map[string]*music.Browse   `json:",omitempty"`
	Liked     []music.Track              `json:",omitempty"`
	LikedMore bool                       `json:",omitempty"`
	Playlists map[string]*music.Playlist `json:",omitempty"`
}

// pageStore is the file of a source's pages, and the state of writing it.
type pageStore struct {
	path string
	// account is who's signed in, which what's kept is for, or "" while that
	// isn't known: nothing is written till then.
	account string
	pending bool // a write is waiting for its time

	// mu orders the writes, which are on goroutines, seq being the last
	// asked for and done the last that landed.
	mu        sync.Mutex
	seq, done int
}

// accountKey is how the file names whose it is: a hash, so that it doesn't
// hold the account's ID.
func accountKey(lib *library, acc *music.Account) string {
	if acc == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(lib.id + "\x00" + acc.ID))
	return hex.EncodeToString(sum[:8])
}

// pagesPath is the file of a source's pages.
func pagesPath(dir, source string) string {
	sum := sha256.Sum256([]byte(source))
	return filepath.Join(dir, hex.EncodeToString(sum[:6])+".json")
}

// stagePages reads what was kept last time, as the source starts, and shows
// it as the pages it was of, until the source says who's signed in (adoptPages).
func (a *app) stagePages(lib *library) {
	if a.pagesDir == "" || !lib.can("account") || lib.pages != nil {
		return
	}
	lib.pages = &pageStore{path: pagesPath(a.pagesDir, lib.id)}
	data, err := os.ReadFile(lib.pages.path)
	if err != nil {
		return
	}
	var f pageFile
	if err := json.Unmarshal(data, &f); err != nil || f.Version != pagesVersion || f.Lang != i18n.Language() || f.Account == "" {
		trashPages(lib.pages.path)
		return
	}
	lib.staged = &f
	lib.home, lib.recent, lib.homeKept = f.Home, f.Recent, f.Home != nil || len(f.Recent) > 0
	lib.mine, lib.saved = f.Mine, f.Saved
	for page, b := range f.Browses {
		if b != nil {
			lib.browses[page] = &browseData{page: b, next: paged{loaded: true, more: b.Next != ""}}
		}
	}
	for id, pl := range f.Playlists {
		if pl != nil {
			lib.playlists[id] = pl
		}
	}
	if len(f.Liked) > 0 {
		a.setLiked(lib, &music.Page{Tracks: f.Liked, More: f.LikedMore}, time.Time{})
	}
}

// adoptPages is told who's signed in. What was staged for them stays; what
// was for another is dropped, with what it showed.
func (a *app) adoptPages(lib *library) {
	st := lib.pages
	if st == nil {
		return
	}
	st.account = accountKey(lib, lib.account)
	f := lib.staged
	lib.staged = nil
	if f == nil {
		a.keepPages(lib)
		return
	}
	if st.account == f.Account {
		a.keepPages(lib)
		return
	}
	a.dropStaged(lib)
	if r := a.route; r.src == lib.id && lib.account != nil {
		a.reload(r)
	}
}

// dropStaged takes back what stagePages showed, of whoever it was for.
func (a *app) dropStaged(lib *library) {
	lib.forget()
	if lib.homeKept {
		lib.home, lib.recent, lib.homeKept = nil, nil, false
	}
	lib.mine, lib.saved = nil, nil
	if lib.pages != nil {
		trashPages(lib.pages.path)
	}
}

// resetPages is for who's signed in having changed: what was kept is
// deleted, and what's loaded from now on is kept for the one who is.
func (a *app) resetPages(lib *library) {
	st := lib.pages
	if st == nil {
		return
	}
	if lib.staged != nil {
		lib.staged = nil
		a.dropStaged(lib)
	}
	trashPages(st.path)
	// A write on its way is for the account it started under.
	lib.pages = &pageStore{path: st.path, account: accountKey(lib, lib.account)}
}

// trashPages deletes a file of pages.
func trashPages(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("pages: %v", err)
	}
}

// keepPages writes what's loaded a little after the last of it, on a
// goroutine.
func (a *app) keepPages(lib *library) {
	st := lib.pages
	if st == nil || st.account == "" || st.pending {
		return
	}
	st.pending = true
	time.AfterFunc(pagesDebounce, func() {
		a.post(func() {
			st.pending = false
			if lib.pages == st {
				a.savePages(lib, false)
			}
		})
	})
}

// flushPages writes what's loaded as the app quits, before it's gone.
func (a *app) flushPages() {
	for _, lib := range a.libs {
		if lib.pages != nil && lib.pages.account != "" && lib.pages.pending {
			a.savePages(lib, true)
		}
	}
}

// savePages makes the file's content, on the main thread, where what's loaded
// is changed in place, and writes it on another unless it's to be there on
// return.
func (a *app) savePages(lib *library, wait bool) {
	st := lib.pages
	data := encodePages(lib, st.account)
	if data == nil {
		trashPages(st.path)
		return
	}
	st.mu.Lock()
	st.seq++
	seq := st.seq
	st.mu.Unlock()
	write := func() {
		st.mu.Lock()
		defer st.mu.Unlock()
		// A newer one has been written.
		if seq < st.done {
			return
		}
		st.done = seq
		if err := writePages(st.path, data); err != nil {
			log.Printf("pages: %v", err)
		}
	}
	if wait {
		write()
		return
	}
	go write()
}

// encodePages is what the library has of what's kept, for who's signed in,
// or nil if there's nothing to keep.
func encodePages(lib *library, account string) []byte {
	f := pageFile{Version: pagesVersion, Lang: i18n.Language(), Account: account, Home: lib.home, Mine: lib.mine, Saved: lib.saved}
	f.Recent = lib.recent[:min(len(lib.recent), pagesRecent)]
	for page, d := range lib.browses {
		// What's gone on past its first part isn't kept as that. Nor is what
		// was looked for in a list of the library: the word isn't written
		// down, and what has it can be most of the list.
		if _, v := music.ParseLibraryPage(page); v.Term != "" {
			continue
		}
		if d.page != nil && !d.paged && d.err == "" {
			if f.Browses == nil {
				f.Browses = map[string]*music.Browse{}
			}
			f.Browses[page] = d.page
		}
	}
	if lib.likedPaged.loaded {
		f.Liked = lib.likedList[:min(len(lib.likedList), likedPart)]
		f.LikedMore = lib.likedPaged.more || len(lib.likedList) > likedPart
	}
	lists := append(append([]music.Playlist(nil), lib.mine...), lib.saved...)
	for i := 0; i < len(lists) && len(f.Playlists) < pagesPlaylists; i++ {
		if pl := lib.playlists[lists[i].ID]; pl != nil && len(pl.Tracks) <= pagesTracks {
			if f.Playlists == nil {
				f.Playlists = map[string]*music.Playlist{}
			}
			f.Playlists[lists[i].ID] = pl
		}
	}
	data, err := json.Marshal(&f)
	if err == nil && len(data) > pagesMaxBytes && len(f.Playlists) > 0 {
		f.Playlists = nil
		data, err = json.Marshal(&f)
	}
	if err != nil {
		log.Printf("pages: %v", err)
		return nil
	}
	if len(data) > pagesMaxBytes {
		return nil
	}
	return data
}

// writePages replaces the file whole, so that one that's half written is
// never read.
func writePages(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
