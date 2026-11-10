package app

import (
	"ringo/internal/music"
)

// Whether a song is in the listener's library. A page of the library says
// so of its songs, and so does an album's or a playlist's of Apple Music;
// of the rest, those shown are asked about, a frame's at a time, once each,
// as hearts are (ui_like.go). The answers go where adding and deleting from
// the app go (setInLibrary), so the one says what the other did.
//
// It decides what a song's button is, as in Apple Music, where a song is
// added before it's a favorite: the plus that adds it while it isn't in the
// library, and its star once it is (likeButton).

// keptSong is a song or a music video to ask about.
type keptSong struct{ kind, id string }

// songInLibrary reports whether tr is in the listener's library, and
// whether that's known: as the listener made it in the app, or as its
// source answered, or else as the track itself says, if it was asked.
func (a *app) songInLibrary(tr *music.Track) (in, known bool) {
	lib := a.library
	key := itemKey(trackKind(tr), tr.ID)
	if on, ok := lib.kept[key]; ok {
		return on, true
	}
	if tr.SavedKnown && !lib.doubted[key] {
		return tr.Saved, true
	}
	return false, false
}

// canKeep reports whether tr can be added to the library or deleted from
// there: not signed out, and not an upload, which deleted would be gone for
// good, with no catalog to add it back from.
func (l *library) canKeep(tr *music.Track) bool {
	return !l.signedOut() && !tr.Uploaded
}

// wantInLibrary notes that whether tr is in the library may show, to ask
// its source about it if that isn't known.
func (a *app) wantInLibrary(tr *music.Track) {
	lib := a.library
	if !lib.canKeep(tr) {
		return
	}
	if _, known := a.songInLibrary(tr); known || lib.askedKept[itemKey(trackKind(tr), tr.ID)] {
		return
	}
	lib.wantKept = append(lib.wantKept, keptSong{trackKind(tr), tr.ID})
}

// askInLibrary asks the source about the songs this frame showed, as it
// ends. Until who's signed in is known, there's no one to ask for.
func (a *app) askInLibrary() {
	lib := a.library
	want := lib.wantKept
	lib.wantKept = nil
	if len(want) == 0 || lib.accountUnknown() {
		return
	}
	type was struct{ on, had bool }
	before := map[string]was{}
	byKind := map[string][]string{}
	for _, w := range want {
		key := itemKey(w.kind, w.id)
		if lib.askedKept[key] {
			continue
		}
		// Asked: it doesn't come up again, whatever it answers.
		lib.askedKept[key] = true
		on, had := lib.kept[key]
		before[key] = was{on, had}
		byKind[w.kind] = append(byKind[w.kind], w.id)
	}
	if len(byKind) == 0 {
		return
	}
	who := lib.listener()
	go func() {
		answers := map[string]map[string]bool{}
		for kind, ids := range byKind {
			in, err := lib.src.InLibrary(kind, ids)
			if err != nil {
				// Not known, and not to be: their stars show (addsFirst).
				a.post(func() {
					if lib.listener() != who {
						return
					}
					for kind, ids := range byKind {
						for _, id := range ids {
							lib.unsaid[itemKey(kind, id)] = true
						}
					}
				})
				return
			}
			answers[kind] = in
		}
		a.post(func() {
			if lib.listener() != who {
				return
			}
			for kind, in := range answers {
				for _, id := range byKind[kind] {
					key := itemKey(kind, id)
					// Forgotten meanwhile, as the library was, it's asked
					// anew; and what the listener did meanwhile stands.
					on, had := lib.kept[key]
					if !lib.askedKept[key] || lib.busy[key] || (was{on, had}) != before[key] {
						continue
					}
					lib.kept[key] = in[id]
				}
			}
		})
	}()
}

// reaskInLibrary asks again whether tr is in the library, unless it's known
// to be: liking a song may have added it.
func (a *app) reaskInLibrary(tr music.Track) {
	lib := a.library
	if in, _ := a.songInLibrary(&tr); in || !lib.canKeep(&tr) {
		return
	}
	delete(lib.askedKept, itemKey(trackKind(&tr), tr.ID))
	lib.wantKept = append(lib.wantKept, keptSong{trackKind(&tr), tr.ID})
}

// doubtPlayer has the songs the player holds asked about anew, as another
// listener signed in: what they said of the library was the one before's.
func (a *app) doubtPlayer() {
	if a.pl == nil {
		return
	}
	tracks := a.pl.Queue()
	if tr := a.pl.Current(); tr != nil {
		tracks = append([]music.Track{*tr}, tracks...)
	}
	for i := range tracks {
		a.library.doubted[itemKey(trackKind(&tracks[i]), tracks[i].ID)] = true
	}
}

// addsFirst reports whether tr's button is the plus that adds it to the
// library (add), not yet its star, and whether which it is is known: it
// isn't while its source hasn't said whether the song is in the library. A
// favorite has its star whatever the library says. So has a song that can't
// be added: signed out, where the star asks to sign in, an upload, which is
// in the library, one the catalog no longer has, and one its source failed
// to answer of.
func (a *app) addsFirst(tr *music.Track, liked bool) (add, known bool) {
	lib := a.library
	if liked || !lib.canKeep(tr) || tr.Unavailable {
		return false, true
	}
	a.wantInLibrary(tr)
	in, known := a.songInLibrary(tr)
	if !known && lib.unsaid[itemKey(trackKind(tr), tr.ID)] {
		return false, true
	}
	return known && !in, known
}
