package applemusic

import (
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ringo/internal/music"
)

// countTrip counts the requests that go out.
type countTrip struct{ n *atomic.Int64 }

func (c countTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet {
		// Nothing here writes to the listener's account.
		return nil, http.ErrNotSupported
	}
	c.n.Add(1)
	return http.DefaultTransport.RoundTrip(r)
}

// This measures what the library's pages cost over the real network, as the
// listener's account (read only: GET and nothing else), for what the app
// fetches ahead and for a long list fetched whole. It prints only the kinds of
// page, counts and times, and is skipped without a file holding the account's
// token, whose path is the environment's APPLE_MUSIC_TOKEN_FILE.
func TestLiveLibraryPace(t *testing.T) {
	file := os.Getenv("APPLE_MUSIC_TOKEN_FILE")
	if file == "" {
		t.Skip("no APPLE_MUSIC_TOKEN_FILE")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("the token file can't be read")
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	var n atomic.Int64
	s.client = &http.Client{Transport: countTrip{&n}}
	if _, err := s.Account(); err != nil {
		t.Fatal("the account can't be asked for")
	}

	// measure runs ask and says how long it took and how many requests it made.
	measure := func(kind string, ask func() (items int, err error)) {
		before, start := n.Load(), time.Now()
		items, err := ask()
		took := time.Since(start).Milliseconds()
		if err != nil {
			t.Logf("%-16s failed after %d ms", kind, took)
			return
		}
		t.Logf("%-16s %4d items %5d ms %2d requests", kind, items, took, n.Load()-before)
	}
	count := func(b *music.Browse, err error) (int, error) {
		if err != nil {
			return 0, err
		}
		items := 0
		for _, sec := range b.Sections {
			items += len(sec.Items) + len(sec.Tracks) + len(sec.Albums) + len(sec.Artists) + len(sec.Playlists)
		}
		return items, nil
	}

	t.Log("The first part of each page, asked for as the app does ahead:")
	for _, page := range []string{
		music.PageNew, music.PageRadio, music.PageSearchLanding,
		music.PageRecentlyAdded, music.PageLibraryAlbums, music.PageLibrarySongs, music.PageLibraryArtist,
		music.PageAllPlaylists, music.PageMadeForYou,
	} {
		measure(page, func() (int, error) { return count(s.PreloadBrowse(page)) })
	}
	measure("home", func() (int, error) { return count(s.home()) })
	var listed []music.Playlist
	measure("playlists", func() (int, error) {
		var err error
		listed, err = s.playlists()
		return len(listed), err
	})
	measure("favorite songs", func() (int, error) {
		page, err := s.PreloadLiked()
		if err != nil {
			return 0, err
		}
		return len(page.Tracks), nil
	})
	if len(listed) > 0 {
		measure("a playlist", func() (int, error) {
			pl, err := s.PreloadPlaylist(listed[0].ID)
			if err != nil {
				return 0, err
			}
			return len(pl.Tracks), nil
		})
	}

	// The source's own fetching ahead would take part of the time and the
	// requests of what's measured next, which the app doesn't ask of it.
	s.stopWarming()
	time.Sleep(3 * time.Second)
	t.Log("A whole list: a part at a time, as it was, and in threes, as it is:")
	for _, page := range []string{music.PageLibrarySongs, music.PageLibraryAlbums} {
		list := libraryLists[page]
		p := s.mapped(list.params)
		var items, total int
		measure(page+", one", func() (int, error) {
			b, err := s.me(list.path, p, reqOpts{})
			for err == nil {
				items += len(b.Data)
				total = max(total, b.total())
				if b.Next == "" {
					break
				}
				b, err = s.follow(b.Next, p)
			}
			return items, err
		})
		t.Logf("%-16s the first part says %s limit, %d in all", page, p["limit"], total)
		measure(page+", three", func() (int, error) {
			b, err := s.libraryPage(page, "")
			items = 0
			for err == nil {
				got, _ := count(b, nil)
				items += got
				if b.Next == "" {
					break
				}
				_, next, _ := strings.Cut(b.Next, "|")
				b, err = s.libraryPage(page, next)
			}
			return items, err
		})
	}
}
