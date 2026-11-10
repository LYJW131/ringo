package app

import "mygo-music/internal/music"

// Where what's played is played from, which the player passes on to the
// source, to say it to Apple as Apple Music's own players do: an album or a
// playlist played shows in Recently Played. A song picked from a list that
// isn't a thing of its own, such as search results, Home's shelves or the
// liked songs, is played from nothing.

func albumOrigin(id string) music.Origin {
	return music.Origin{Kind: music.FromAlbum, ID: id}
}

func artistOrigin(id string) music.Origin {
	return music.Origin{Kind: music.FromArtist, ID: id}
}

// playlistOrigin is where pl's songs are played from, with what Apple
// wants of it that its page came with.
func playlistOrigin(pl *music.Playlist) music.Origin {
	return music.Origin{Kind: music.FromPlaylist, ID: pl.ID, CatalogID: pl.CatalogID, Version: pl.Version}
}

// pageOrigin is where the songs the page shown lists are played from: its
// album, its playlist or its artist, and for any other page, nothing.
func (a *app) pageOrigin() music.Origin {
	switch id := a.route.id; a.route.page {
	case pageAlbum:
		return albumOrigin(id)
	case pageArtist:
		return artistOrigin(id)
	case pagePlaylist:
		if pl := a.library.playlists[id]; pl != nil {
			return playlistOrigin(pl)
		}
		return music.Origin{Kind: music.FromPlaylist, ID: id}
	}
	return music.Origin{}
}
