package app

import (
	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// What can be done with an album, a playlist, an artist or a song wherever it
// shows, on its page, in its menu or on a shelf's card, in Apple Music's
// order and words: adding it to the library, playing it next or last,
// sharing it, making it a favorite, going to its album or artist.

// itemKey names what an item of kind is, for the library's overlays.
func itemKey(kind, id string) string { return kind + "\x00" + id }

// trackKind is the kind of item a track is: a song, or a music video.
func trackKind(tr *music.Track) string {
	if tr.Video {
		return music.ItemVideo
	}
	return music.ItemSong
}

// isFavorite reports whether the listener made what kind and id name a
// favorite: as they did in the app, or else as was says, what the source
// answered.
func (l *library) isFavorite(kind, id string, was bool) bool {
	if on, ok := l.favorites[itemKey(kind, id)]; ok {
		return on
	}
	return was
}

// inLibrary reports whether what kind and id name is in the listener's
// library, as isFavorite does of favorites.
func (l *library) inLibrary(kind, id string, was bool) bool {
	if on, ok := l.kept[itemKey(kind, id)]; ok {
		return on
	}
	return was
}

// overlay sets what kind and id name on or off in marks at once, as the
// listener asked, and then asks send to make it so: going back, saying so,
// if it refuses, or saying done of it if it does. Signed out, it asks the
// listener to sign in.
func (a *app) overlay(marks func() map[string]bool, kind, id string, on bool, send func() error, done func()) {
	lib := a.library
	if lib.signedOut() {
		a.promptSignIn()
		return
	}
	key := itemKey(kind, id)
	if lib.busy[key] {
		return
	}
	m := marks()
	was, had := m[key]
	m[key], lib.busy[key] = on, true
	go func() {
		err := send()
		a.post(func() {
			delete(lib.busy, key)
			// Forgotten meanwhile, as the listener signed out: there is
			// nothing to go back to.
			now := marks()
			if err != nil {
				if now[key] == on && now != nil {
					if had {
						now[key] = was
					} else {
						delete(now, key)
					}
				}
				a.actionFailed(err, T("That didn't work. Try again later."))
				return
			}
			done()
		})
	}()
}

// setFavorite makes what kind and id name a favorite, or no longer one. A
// song's favorite is its like (toggleLike).
func (a *app) setFavorite(kind, id string, on bool) {
	a.overlay(func() map[string]bool { return a.library.favorites }, kind, id, on,
		func() error { return a.library.src.Favorite(kind, id, on) },
		func() {
			if on {
				a.toast = T("Added to Favorites")
			} else {
				a.toast = T("Removed from Favorites")
			}
		})
}

// setInLibrary adds what kind and id name to the listener's library, or
// deletes it from there. A playlist shows in the sidebar, or goes, once its
// source has it.
func (a *app) setInLibrary(kind, id string, on bool) {
	a.overlay(func() map[string]bool { return a.library.kept }, kind, id, on,
		func() error { return a.library.src.AddToLibrary(kind, id, on) },
		func() {
			if on {
				a.toast = T("Added to Library")
			} else {
				a.toast = T("Deleted From Library")
			}
			if kind == music.ItemPlaylist {
				a.loadMine()
			}
		})
}

// enqueue plays tracks, which from is where they're played from, after the
// song playing, if next, or after the rest of the queue, which go on as they
// were; what says what they are, for the toast. With nothing playing, they
// play now.
func (a *app) enqueue(tracks []music.Track, next bool, what string, from music.Origin) {
	tracks = playable(tracks)
	if len(tracks) == 0 {
		return
	}
	pl := a.pl
	if pl.Current() == nil {
		pl.PlayQueue(tracks, 0, from)
		return
	}
	if next {
		pl.PlayNext(tracks, from)
		a.toast = Tf("“%s” will play next", what)
	} else {
		pl.PlayLast(tracks, from)
		a.toast = Tf("“%s” will play last", what)
	}
}

// tracksOf gives the songs of an album or a playlist to then, loading them
// first if they haven't, and where they're played from.
func (a *app) tracksOf(kind, id string, then func([]music.Track, music.Origin)) {
	lib := a.library
	switch kind {
	case music.ItemAlbum:
		if d := lib.albums[id]; d != nil && d.album != nil {
			then(d.album.Tracks, albumOrigin(id))
			return
		}
		go func() {
			al, err := answered(lib.src.Album(id))
			a.post(func() {
				if err != nil {
					a.actionFailed(err, T("Couldn't load this album"))
					return
				}
				then(al.Tracks, albumOrigin(id))
			})
		}()
	case music.ItemPlaylist:
		if pl := lib.playlists[id]; pl != nil {
			then(pl.Tracks, playlistOrigin(pl))
			return
		}
		go func() {
			pl, err := answered(lib.src.Playlist(id))
			a.post(func() {
				if err != nil {
					a.actionFailed(err, T("Couldn't load this playlist"))
					return
				}
				then(pl.Tracks, playlistOrigin(pl))
			})
		}()
	}
}

// share puts the link to something's page on the clipboard.
func (a *app) share(url string) {
	if copyText(url) == nil {
		a.toast = T("Link Copied")
	}
}

// listItems are the items of the menu of an album or a playlist, as the
// listener's library has it or not, and their favorites; then, if more is
// set, what more its menu has, before Share and Favorite.
func (a *app) listItems(m *ui.Menu, kind, id, name, url string, saved, favorite, stays bool, more func()) {
	// What stays in the library, as an album with uploads, isn't offered
	// to be taken out.
	if !(stays && a.library.inLibrary(kind, id, saved)) {
		if a.library.inLibrary(kind, id, saved) {
			if m.Item(T("Delete From Library")).Chosen() {
				a.setInLibrary(kind, id, false)
			}
		} else if m.Item(T("Add to Library")).Chosen() {
			a.setInLibrary(kind, id, true)
		}
		m.Separator()
	}
	if m.Item(T("Play Next")).Chosen() {
		a.tracksOf(kind, id, func(tracks []music.Track, from music.Origin) { a.enqueue(tracks, true, name, from) })
	}
	if m.Item(T("Play Last")).Chosen() {
		a.tracksOf(kind, id, func(tracks []music.Track, from music.Origin) { a.enqueue(tracks, false, name, from) })
	}
	if more != nil {
		more()
	}
	a.shareAndFavorite(m, kind, id, url, favorite)
}

// shareAndFavorite are the last items of a menu: sharing the link to its
// page, and making it a favorite or not.
func (a *app) shareAndFavorite(m *ui.Menu, kind, id, url string, favorite bool) {
	if url != "" {
		m.Separator()
		m.Submenu(T("Share"), func(m *ui.Menu) {
			if m.Item(T("Copy Link")).Chosen() {
				a.share(url)
			}
		})
	}
	m.Separator()
	if a.library.isFavorite(kind, id, favorite) {
		if m.Item(T("Undo Favorite")).Chosen() {
			a.setFavorite(kind, id, false)
		}
	} else if m.Item(T("Favorite")).Chosen() {
		a.setFavorite(kind, id, true)
	}
}

// albumItems builds the menu of an album: behind its dots on its page, or a
// right click on its card.
func (a *app) albumItems(al music.Album) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		a.listItems(m, music.ItemAlbum, al.ID, al.Name, al.URL, al.Saved, al.Favorite, al.Uploaded, func() {
			first := true
			for _, ar := range al.Artists {
				if ar.ID == "" {
					continue
				}
				if first {
					m.Separator()
					first = false
				}
				if m.Item(Tf("Go to Artist: %s", ar.Name)).Chosen() {
					a.openArtist(ar.ID)
				}
			}
		})
	}
}

// playlistItemsOf builds the menu of a playlist, with renaming and deleting it
// for one of the listener's own.
func (a *app) playlistItemsOf(pl music.Playlist) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		mine := a.editable(pl.ID)
		saved := pl.Saved || a.isSaved(&pl)
		if pl.Mine || pl.Fixed || a.ownList(pl.ID) {
			// Their own is in their library already, for good.
			if m.Item(T("Play Next")).Chosen() {
				a.tracksOf(music.ItemPlaylist, pl.ID, func(tracks []music.Track, from music.Origin) { a.enqueue(tracks, true, pl.Name, from) })
			}
			if m.Item(T("Play Last")).Chosen() {
				a.tracksOf(music.ItemPlaylist, pl.ID, func(tracks []music.Track, from music.Origin) { a.enqueue(tracks, false, pl.Name, from) })
			}
			a.shareAndFavorite(m, music.ItemPlaylist, pl.ID, pl.URL, pl.Favorite)
			if mine != nil {
				m.Separator()
				a.playlistMenu(m, mine)
			}
			return
		}
		a.listItems(m, music.ItemPlaylist, pl.ID, pl.Name, pl.URL, saved, pl.Favorite, false, nil)
	}
}

// artistItems builds the menu of an artist: sharing their page and making
// them a favorite.
func (a *app) artistItems(ar music.Artist) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		a.shareAndFavorite(m, music.ItemArtist, ar.ID, ar.URL, ar.Favorite)
	}
}

// favoriteButton is the star that makes what kind and id name a favorite,
// filled once it is, as Apple Music's pages have by their Play buttons.
func (a *app) favoriteButton(c *ui.Context, kind, id string, was bool, size float32, color ui.Color) ui.Element {
	on := a.library.isFavorite(kind, id, was)
	icon, label := iconStar, T("Favorite")
	if on {
		icon, color, label = iconStarFill, a.pal.accent, T("Undo Favorite")
	}
	b := iconButton(c, icon, size, color, label)
	if b.Clicked() {
		a.setFavorite(kind, id, !on)
	}
	return b
}
