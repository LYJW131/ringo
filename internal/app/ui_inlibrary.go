package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// Whether a song is in the listener's library. A page of the library says
// so of its songs, and so does an album's or a playlist's of Apple Music;
// of the rest, those shown are asked about, a frame's at a time, once each,
// as hearts are (ui_like.go). The answers go where adding and deleting from
// the app go (setInLibrary), so the one says what the other did.

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
func (a *app) reaskInLibrary(lib *library, tr music.Track) {
	if in, _ := a.songInLibrary(&tr); in || !lib.canKeep(&tr) {
		return
	}
	delete(lib.askedKept, itemKey(trackKind(&tr), tr.ID))
	lib.wantKept = append(lib.wantKept, keptSong{trackKind(&tr), tr.ID})
}

// doubtPlayer has the songs the player holds asked about anew, as another
// listener signed in: what they said of the library was the one before's.
func (a *app) doubtPlayer(lib *library) {
	if a.pl == nil {
		return
	}
	tracks := a.pl.Queue()
	if tr := a.pl.Current(); tr != nil {
		tracks = append([]music.Track{*tr}, tracks...)
	}
	for i := range tracks {
		lib.doubted[itemKey(trackKind(&tracks[i]), tracks[i].ID)] = true
	}
}

// keepButton adds tr to the library, beside its star: a plus, which turns
// into a fainter check once it's in, which does nothing, as deleting it is
// for its menu, away from a stray click. Until its source says whether it's
// in, it holds its place empty, so the star and the dots beside it stay
// where they are as its icon fades in. A song that can't be added, as one
// the catalog no longer has, shows none.
func (a *app) keepButton(c *ui.Context, tr *music.Track, size float32, color ui.Color) ui.Element {
	lib := a.library
	if !lib.canKeep(tr) || tr.Unavailable {
		return ui.Element{}
	}
	a.wantInLibrary(tr)
	in, known := a.songInLibrary(tr)
	kind := trackKind(tr)
	// A song of its own, so that another's icon doesn't turn into its.
	wrap := ui.Box(c.Key(kind+":"+tr.ID)).Size(size+16, size+16).Center().Shrink(0)
	shown := wrap.AnimateTo("shown", oneIf(known), ui.Animation{Duration: 220 * time.Millisecond, NonMotion: true})
	turn := wrap.AnimateTo("in", oneIf(in), ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond, Bounce: 0.2}})
	if !known {
		return wrap
	}
	icons := func() {
		ui.Box(c).Size(size, size).Children(func() {
			for _, icon := range []struct {
				svg   *ui.SVG
				color ui.Color
				shown float32
			}{{iconPlus, color, 1 - turn}, {iconCheck, color.Alpha(0.6), turn}} {
				if icon.shown <= 0.01 {
					continue
				}
				s := 0.6 + 0.4*icon.shown
				ui.Icon(c, icon.svg).Absolute().Top(0).Left(0).Size(size, size).Scale(s, s).
					TextColor(icon.color).Opacity(min(icon.shown, 1) * shown)
			}
		})
	}
	wrap.Children(func() {
		if in {
			ui.Box(c).Size(size+16, size+16).Center().Label(T("Added to Library")).Children(icons)
			return
		}
		b := roundButton(c, size, color, T("Add to Library"))
		b.Children(icons)
		if b.Clicked() {
			a.setInLibrary(lib, kind, tr.ID, true)
		}
	})
	return wrap
}

// oneIf is 1 for true, for what animates between two states.
func oneIf(on bool) float32 {
	if on {
		return 1
	}
	return 0
}
