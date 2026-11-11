package app

import (
	"time"

	"ringo/internal/music"
)

// moving is what a cover is of, to play it moving where Apple has it so:
// a song's album, an album or a playlist, by an Item's kind and its ID, or
// url, the video itself, where the page that showed it already knew.
type moving struct {
	kind, id string
	url      string
}

// movingVideo is a cover whose video is known, or none.
func movingVideo(url string) moving { return moving{url: url} }

// songMoving is the cover of a song's album.
func songMoving(tr *music.Track) moving {
	if tr == nil {
		return moving{}
	}
	return moving{kind: music.ItemSong, id: tr.ID, url: tr.Album.VideoURL}
}

func albumMoving(al *music.Album) moving {
	if al == nil {
		return moving{}
	}
	return moving{kind: music.ItemAlbum, id: al.ID, url: al.VideoURL}
}

func playlistMoving(pl *music.Playlist) moving {
	if pl == nil {
		return moving{}
	}
	return moving{kind: music.ItemPlaylist, id: pl.ID, url: pl.VideoURL}
}

// itemMoving is the square cover of an item, whose card shows it.
func itemMoving(it *music.Item) moving {
	switch it.Kind {
	case music.ItemAlbum, music.ItemPlaylist:
		return moving{kind: it.Kind, id: it.ID, url: it.VideoURL}
	case music.ItemSong, music.ItemVideo:
		if it.Track != nil && !it.Track.Video {
			return songMoving(it.Track)
		}
	}
	return moving{}
}

// motionKnown is what the source said of which covers move, for those
// that showed.
type motionKnown struct {
	// urls are the videos by movingKey: "" for a cover that doesn't move,
	// or is being asked about.
	urls map[string]string
	// want are the covers that showed this frame and weren't known, by
	// kind, to ask about as it ends.
	want map[string][]string
}

func movingKey(kind, id string) string { return kind + "|" + id }

// movingURL is the video of mv's cover, or "" while it isn't known to
// have one: the source is asked about it as the frame ends, once.
func (a *app) movingURL(mv moving) string {
	if mv.url != "" || mv.id == "" {
		return mv.url
	}
	lib := a.library
	if lib == nil {
		return ""
	}
	if _, ok := lib.src.(music.Mover); !ok {
		return ""
	}
	if lib.moves == nil {
		lib.moves = &motionKnown{urls: map[string]string{}, want: map[string][]string{}}
	}
	known := lib.moves
	key := movingKey(mv.kind, mv.id)
	if url, ok := known.urls[key]; ok {
		return url
	}
	// Asked: it doesn't come up again until the source answers.
	known.urls[key] = ""
	known.want[mv.kind] = append(known.want[mv.kind], mv.id)
	return ""
}

// askMoves asks the source about the covers this frame showed, as it ends.
func (a *app) askMoves() {
	lib := a.library
	if lib == nil || lib.moves == nil || len(lib.moves.want) == 0 {
		return
	}
	known := lib.moves
	mover, ok := lib.src.(music.Mover)
	if !ok {
		return
	}
	want := known.want
	known.want = map[string][]string{}
	for kind, ids := range want {
		go func() {
			found, err := mover.Motion(kind, ids)
			a.post(func() {
				// Forgotten meanwhile: the storefront may be another's.
				if lib.moves != known {
					return
				}
				for id, url := range found {
					known.urls[movingKey(kind, id)] = url
				}
				if err != nil {
					// The network, maybe: those it didn't answer are asked
					// about again as they show, after a while.
					time.AfterFunc(motionRetry, func() {
						a.post(func() {
							for _, id := range ids {
								if known.urls[movingKey(kind, id)] == "" {
									delete(known.urls, movingKey(kind, id))
								}
							}
						})
					})
				}
			})
		}()
	}
}
