package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

// songMenu is three dots that open the menu of tr.
func (a *app) songMenu(c *ui.Context, tr *music.Track, size float32, color ui.Color) ui.Element {
	b := iconButton(c, iconMore, size, color, T("More"))
	b.Menu(a.songItems(tr, ""))
	return b
}

// songItems builds the menu of what can be done with tr, behind its dots or
// a right click on its row, in the order of Apple Music's: adding it to the
// library or a playlist, or taking it out of pid, the playlist it is listed
// in, if set; playing it next or last; sharing it; making it a favorite; and
// going to its album's or artists' pages. What its source can't do isn't
// offered.
func (a *app) songItems(tr *music.Track, pid string) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		lib := a.trackLib(tr)
		kind := trackKind(tr)
		group := false
		if lib.can("addToLibrary") && !lib.signedOut() {
			if lib.inLibrary(kind, tr.ID, tr.Saved) {
				if m.Item(T("Delete From Library")).Chosen() {
					a.setInLibrary(lib, kind, tr.ID, false)
				}
			} else if m.Item(T("Add to Library")).Chosen() {
				a.setInLibrary(lib, kind, tr.ID, true)
			}
			group = true
		}
		if a.playlistItems(m, tr, pid) {
			group = true
		}
		if group {
			m.Separator()
		}
		if !tr.Unavailable {
			if m.Item(T("Play Next")).Chosen() {
				a.enqueue([]music.Track{*tr}, true, tr.Name)
			}
			if m.Item(T("Play Last")).Chosen() {
				a.enqueue([]music.Track{*tr}, false, tr.Name)
			}
			m.Separator()
		}
		m.Submenu(T("Share"), func(m *ui.Menu) {
			if tr.URL != "" && m.Item(T("Copy Link")).Chosen() {
				a.share(tr.URL)
			}
			if m.Item(T("Copy Title")).Chosen() {
				copyText(tr.Name)
			}
			if names := tr.ArtistNames(); names != "" && m.Item(T("Copy Title and Artists")).Chosen() {
				copyText(tr.Name + " - " + names)
			}
		})
		if lib.canLike(tr) {
			m.Separator()
			if a.isLiked(tr) {
				if m.Item(T("Undo Favorite")).Chosen() {
					a.toggleLike(tr)
				}
			} else if m.Item(T("Favorite")).Chosen() {
				a.toggleLike(tr)
			}
		}
		// Its album's and artists' pages, those the source has.
		linked := tr.Album.ID != "" && lib.can("album")
		for _, ar := range tr.Artists {
			linked = linked || ar.ID != "" && lib.can("artist")
		}
		if !linked {
			return
		}
		m.Separator()
		if tr.Album.ID != "" && lib.can("album") && m.Item(T("Show Album")).Chosen() {
			a.openPage(route{page: pageAlbum, src: lib.id, id: tr.Album.ID})
		}
		for _, ar := range tr.Artists {
			if ar.ID != "" && lib.can("artist") && m.Item(Tf("Go to Artist: %s", ar.Name)).Chosen() {
				a.openArtist(lib.id, ar.ID)
			}
		}
	}
}
