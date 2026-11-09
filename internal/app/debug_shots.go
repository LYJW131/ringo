package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"mygo-music/internal/music"
)

// With MM_SHOTS set to a directory, the app walks through its pages and
// saves what its window shows of each there, for checking the design against
// what the source really answers. It plays a song without sound, to see the
// player at work, and leaves the listener's settings as they were.
func init() {
	dir := os.Getenv("MM_SHOTS")
	if dir == "" {
		return
	}
	shotHook = func(a *app) {
		a.settings.path = ""
		go func() {
			n := 0
			shot := func(name string) {
				n++
				png, err := a.win.CapturePage()
				if err == nil {
					os.WriteFile(filepath.Join(dir, fmt.Sprintf("%02d-%s.png", n, name)), png, 0o644)
				}
			}
			step := func(fn func()) { mygo.RunOnMain(func() { fn(); a.redraw() }) }
			// A page is asked for, then shown once its pictures have had
			// time to come.
			page := func(name string, r route, wait time.Duration) {
				step(func() { a.open(r) })
				time.Sleep(wait)
				shot(name)
			}
			browse := func(id string) route { return route{page: pageBrowse, id: id} }
			mygo.RunOnMain(func() { a.win.Show(); a.win.Focus() })
			time.Sleep(6 * time.Second)
			shot("home")
			page("new", browse(music.PageNew), 6*time.Second)
			page("radio", browse(music.PageRadio), 6*time.Second)
			page("search-landing", route{page: pageSearch}, 5*time.Second)
			page("search", route{page: pageSearch, query: os.Getenv("MM_SHOTS_SEARCH")}, 6*time.Second)
			if id := os.Getenv("MM_SHOTS_ARTIST"); id != "" {
				page("artist", route{page: pageArtist, id: id}, 6*time.Second)
			}
			if id := os.Getenv("MM_SHOTS_PLAYLIST"); id != "" {
				page("playlist", route{page: pagePlaylist, id: id}, 6*time.Second)
			}
			if !a.library.signedOut() {
				for _, p := range []string{music.PageRecentlyAdded, music.PageLibraryArtist, music.PageLibraryAlbums, music.PageLibrarySongs, music.PageMadeForYou, music.PageAllPlaylists} {
					page(p[len("library:"):], browse(p), 6*time.Second)
				}
			}
			if id := os.Getenv("MM_SHOTS_ALBUM"); id != "" {
				page("album", route{page: pageAlbum, id: id}, 6*time.Second)
				step(func() {
					a.volume = 0
					a.pl.SetVolume(0)
					if d := a.library.albums[id]; d != nil && d.album != nil && len(d.album.Tracks) > 0 {
						a.pl.PlayQueue(d.album.Tracks, 0)
					}
				})
				time.Sleep(8 * time.Second)
				shot("playing")
				if a.pl.Current() != nil {
					step(func() { a.nowPlaying = true })
					time.Sleep(4 * time.Second)
					shot("now-playing")
					step(func() { a.npPane = paneQueue })
					time.Sleep(2 * time.Second)
					shot("queue")
					step(func() { a.pl.TogglePlay() })
				}
			}
			mygo.RunOnMain(mygo.App.Quit)
		}()
	}
}
