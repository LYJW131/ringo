package app

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// The listener's pins: what they pinned of their library, which the sidebar
// lists at its top, pinned and unpinned from menus and moved by dragging, at
// once in the app and then with the source, which may refuse: they go back
// then.

// pinKind is the kind a pin of kind is, songs and videos one.
func pinKind(kind string) string {
	if kind == music.ItemVideo {
		return music.ItemSong
	}
	return kind
}

// pinnedAt is where what kind and id name is among the pins, or -1.
func (l *library) pinnedAt(kind, id string) int {
	return slices.IndexFunc(l.pins, func(it music.Item) bool {
		return pinKind(it.Kind) == pinKind(kind) && (it.ID == id || it.Pin != nil && it.Pin.ID == id)
	})
}

// pinLabels are the items that pin and unpin a kind, in Music's words.
var pinLabels = map[string][2]string{
	music.ItemSong:     {N("Pin Song"), N("Unpin Song")},
	music.ItemVideo:    {N("Pin Music Video"), N("Unpin Music Video")},
	music.ItemAlbum:    {N("Pin Album"), N("Unpin Album")},
	music.ItemPlaylist: {N("Pin Playlist"), N("Unpin Playlist")},
	music.ItemArtist:   {N("Pin Artist"), N("Unpin Artist")},
}

// pinItem is the item of a menu that pins what kind and id name, or unpins
// it if it's pinned, for what's in the listener's library.
func (a *app) pinItem(m *ui.Menu, kind, id string) {
	labels, ok := pinLabels[kind]
	if !ok || id == "" {
		return
	}
	on := a.library.pinnedAt(kind, id) < 0
	label := labels[1]
	if on {
		label = labels[0]
	}
	if m.Item(T(label)).Chosen() {
		a.setPinned(kind, id, on)
	}
}

// setPinned pins what kind and id name, or unpins it. An unpinned one goes
// from the sidebar at once; a pinned one shows as the source lists it.
func (a *app) setPinned(kind, id string, on bool) {
	lib := a.library
	before := lib.pins
	if !on {
		if i := lib.pinnedAt(kind, id); i >= 0 {
			lib.pins = slices.Delete(slices.Clone(before), i, i+1)
		}
	}
	go func() {
		err := lib.src.Pin(kind, id, on)
		a.post(func() {
			if err != nil {
				if !on {
					lib.pins = before
				}
				a.actionFailed(err, T("Couldn't change your pins. Try again later."))
			}
			a.loadPins()
		})
	}()
}

// movePin moves the pin at from to before the one at to, the number of
// pins for the end, as the sidebar's list says of a drag.
func (a *app) movePin(from, to int) {
	lib := a.library
	before := lib.pins
	if from < 0 || from >= len(before) || to > from && to-1 == from || to == from {
		return
	}
	it := before[from]
	if it.Pin == nil {
		return
	}
	moved := slices.Delete(slices.Clone(before), from, from+1)
	if to > from {
		to--
	}
	moved = slices.Insert(moved, to, it)
	after := ""
	if to > 0 && moved[to-1].Pin != nil {
		after = moved[to-1].Pin.Position
	}
	lib.pins = moved
	go func() {
		err := lib.src.MovePin(it.Pin.ID, after)
		a.post(func() {
			if err != nil {
				lib.pins = before
				a.actionFailed(err, T("Couldn't change your pins. Try again later."))
			}
			// Its place has a new mark, which the next move after it needs.
			a.loadPins()
		})
	}()
}

// isLibraryArtist reports whether id is an artist's of the library.
func isLibraryArtist(id string) bool { return strings.HasPrefix(id, "r.") }
