package app

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// The listener's own playlists: making them, renaming them, deleting them,
// and adding songs to them, taking songs out or moving them; and saving
// others'. What a source can't do isn't offered.

// nameDialog asks for a playlist's name: a new one's, or one's new name.
type nameDialog struct {
	open   bool
	name   string
	rename string       // the playlist renamed, or "" to make one
	add    *music.Track // the song to add to the playlist made, if any
	busy   bool         // the source is making the playlist
}

// deleteDialog asks whether to delete one of the listener's playlists.
type deleteDialog struct {
	open bool
	id   string
	name string
}

// editable returns the listener's playlist id if they may edit its songs
// and name, or nil: not one the source keeps itself.
func (a *app) editable(id string) *music.Playlist {
	for i := range a.mine {
		if a.mine[i].ID == id && !a.mine[i].Fixed {
			return &a.mine[i]
		}
	}
	return nil
}

// ownList reports whether playlist id is one of the listener's, as the
// sidebar lists them: one the source keeps for them too.
func (a *app) ownList(id string) bool {
	return slices.ContainsFunc(a.mine, func(m music.Playlist) bool { return m.ID == id })
}

// isSaved reports whether the listener saved playlist pl, as its page or
// the sidebar says, or as they since added it to their library or deleted
// it from there (setInLibrary).
func (a *app) isSaved(pl *music.Playlist) bool {
	was := pl.Saved || slices.ContainsFunc(a.saved, func(s music.Playlist) bool { return s.ID == pl.ID })
	return a.library.inLibrary(music.ItemPlaylist, pl.ID, was)
}

// loadMine loads the playlists the listener made and saved, once someone
// is signed in: signed out, there are none to ask for, and asking would only
// be told to sign in, which the listener didn't ask to.
func (a *app) loadMine(lib *library) {
	if !lib.can("playlists") || lib.can("account") && lib.account == nil {
		return
	}
	lib.mineAt = time.Now()
	go func() {
		lists, err := lib.src.Playlists()
		a.post(func() {
			if err != nil {
				a.problem(lib, err, T("Couldn't load your playlists"))
				return
			}
			lib.mine, lib.saved = nil, nil
			// The list is the source's word on which playlists are in the
			// library, over what the listener did here since, but for what's
			// still on its way: saved again elsewhere, one stays hidden no more.
			for key := range lib.kept {
				if _, id, ok := strings.Cut(key, "\x00"); ok && key == itemKey(music.ItemPlaylist, id) && !lib.busy[key] {
					delete(lib.kept, key)
				}
			}
			for _, pl := range lists {
				if pl.Mine {
					lib.mine = append(lib.mine, pl)
				} else {
					lib.saved = append(lib.saved, pl)
				}
			}
			a.keepPages(lib)
			a.warmPages(lib)
		})
	}()
}

// playlistChanged forgets what was loaded of playlist id, which its source
// changed: loaded anew if shown, and the sidebar's covers with it.
func (a *app) playlistChanged(lib *library, id string) {
	delete(lib.playlists, id)
	if a.route.page == pagePlaylist && a.route.src == lib.id && a.route.id == id {
		a.loadPlaylist(lib, id, false)
	}
	a.loadMine(lib)
}

// newPlaylist asks for the name of a playlist to make, then adds tr to it
// if set. Signed out, it asks the listener to sign in.
func (a *app) newPlaylist(tr *music.Track) {
	if a.library.signedOut() {
		a.promptSignIn(a.library)
		return
	}
	if !a.can("createPlaylist") {
		return
	}
	a.naming = nameDialog{open: true, add: tr}
}

// startRename asks for a new name for the listener's playlist pl.
func (a *app) startRename(pl *music.Playlist) {
	a.naming = nameDialog{open: true, name: pl.Name, rename: pl.ID}
}

// submitName makes the playlist named, or renames one: at once in the app,
// as its source may refuse, when renaming.
func (a *app) submitName() {
	d := &a.naming
	name := strings.TrimSpace(d.name)
	lib := a.library
	if name == "" || d.busy {
		return
	}
	if d.rename != "" {
		id := d.rename
		d.open = false
		pl := a.editable(id)
		if pl == nil || pl.Name == name {
			return
		}
		old := pl.Name
		lib.setPlaylistName(id, name)
		go func() {
			err := lib.src.RenamePlaylist(id, name)
			a.post(func() {
				if err != nil {
					lib.setPlaylistName(id, old)
					a.actionFailed(lib, err, T("Couldn't rename the playlist. Try again later."))
					return
				}
				// The sidebar's list asked for before the change may have come
				// back meanwhile: asked again, the source has the new name.
				a.loadMine(lib)
			})
		}()
		return
	}
	d.busy = true
	add := d.add
	go func() {
		pl, err := answered(lib.src.CreatePlaylist(name))
		a.post(func() {
			a.naming.busy = false
			if err != nil {
				a.actionFailed(lib, err, T("Couldn't create the playlist. Try again later."))
				return
			}
			a.naming.open = false
			pl.Mine = true
			lib.mine = slices.Insert(slices.Clone(lib.mine), 0, *pl)
			pl.Tracks = nil
			lib.playlists[pl.ID] = pl
			a.open(route{page: pagePlaylist, src: lib.id, id: pl.ID})
			if add != nil {
				a.addTo(lib, pl.ID, add)
			}
		})
	}()
}

// confirmDelete asks whether to delete the listener's playlist pl.
func (a *app) confirmDelete(pl *music.Playlist) {
	a.deleting = deleteDialog{open: true, id: pl.ID, name: pl.Name}
}

// deleteChosen deletes the playlist the dialog asked about: at once from
// the sidebar, leaving its page, and then with its source, which may refuse:
// it comes back then.
func (a *app) deleteChosen() {
	id := a.deleting.id
	lib := a.library
	a.deleting.open = false
	i := slices.IndexFunc(lib.mine, func(pl music.Playlist) bool { return pl.ID == id })
	if i < 0 {
		return
	}
	pl := lib.mine[i]
	lib.mine = slices.Delete(slices.Clone(lib.mine), i, i+1)
	undoCards := lib.dropPlaylistCards(id)
	// Leave no way back to its page, and leave it for the page before.
	gone := func(r route) bool { return r.page == pagePlaylist && r.src == lib.id && r.id == id }
	a.history = slices.DeleteFunc(a.history, gone)
	a.forward = slices.DeleteFunc(a.forward, gone)
	if gone(a.route) {
		r := route{page: pageHome, src: lib.id}
		if n := len(a.history); n > 0 {
			r, a.history = a.history[n-1], a.history[:n-1]
		}
		a.show(r)
	}
	go func() {
		err := lib.src.DeletePlaylist(id)
		a.post(func() {
			if err != nil {
				lib.mine = slices.Insert(slices.Clone(lib.mine), min(i, len(lib.mine)), pl)
				undoCards()
				a.actionFailed(lib, err, T("Couldn't delete the playlist. Try again later."))
				return
			}
			delete(lib.playlists, id)
			a.toast = T("Playlist deleted")
			// As after a rename: the list may have come back from before.
			a.loadMine(lib)
		})
	}()
}

// setPlaylistName names the listener's playlist id wherever the app shows
// it: the sidebar, its page, and its cards, as All Playlists has.
func (l *library) setPlaylistName(id, name string) {
	for i := range l.mine {
		if l.mine[i].ID == id {
			l.mine[i].Name = name
		}
	}
	if pl := l.playlists[id]; pl != nil {
		pl.Name = name
	}
	for _, sec := range l.playlistSections(id) {
		for i := range sec.Items {
			if it := &sec.Items[i]; isPlaylistCard(it, id) {
				it.Title = name
				if it.Playlist != nil {
					it.Playlist.Name = name
				}
			}
		}
	}
}

// isPlaylistCard reports whether it is a card of playlist id.
func isPlaylistCard(it *music.Item, id string) bool {
	return it.Kind == music.ItemPlaylist && itemID(it, it.Playlist != nil, func() string { return it.Playlist.ID }) == id
}

// playlistSections are the sections of the pages loaded with a card of
// playlist id.
func (l *library) playlistSections(id string) []*music.Section {
	var out []*music.Section
	for _, d := range l.browses {
		if d.page == nil {
			continue
		}
		for i := range d.page.Sections {
			sec := &d.page.Sections[i]
			if slices.ContainsFunc(sec.Items, func(it music.Item) bool { return isPlaylistCard(&it, id) }) {
				out = append(out, sec)
			}
		}
	}
	return out
}

// dropPlaylistCards takes playlist id's cards off the pages loaded, and
// answers what puts them back.
func (l *library) dropPlaylistCards(id string) (undo func()) {
	secs := l.playlistSections(id)
	was := make([][]music.Item, len(secs))
	for i, sec := range secs {
		was[i] = sec.Items
		sec.Items = slices.DeleteFunc(slices.Clone(sec.Items), func(it music.Item) bool { return isPlaylistCard(&it, id) })
	}
	return func() {
		for i, sec := range secs {
			sec.Items = was[i]
		}
	}
}

// addTo adds tr to the listener's playlist pid; a toast says how it went.
func (a *app) addTo(lib *library, pid string, tr *music.Track) {
	id := tr.ID
	go func() {
		err := lib.src.AddToPlaylist(pid, []string{id})
		a.post(func() {
			if err != nil {
				a.actionFailed(lib, err, T("Couldn't add it to the playlist. Try again later."))
				return
			}
			a.toast = T("Added to the playlist")
			a.playlistChanged(lib, pid)
		})
	}()
}

// removeFrom takes tr out of the listener's playlist pid, at once in the
// app and then with its source, which may refuse: the song comes back then.
func (a *app) removeFrom(lib *library, pid string, tr *music.Track) {
	id := tr.ID
	pl := lib.playlists[pid]
	var before []music.Track
	if pl != nil {
		// A new list: the player may be playing the old one.
		before = pl.Tracks
		pl.Tracks = slices.DeleteFunc(slices.Clone(before), func(t music.Track) bool { return t.ID == id })
	}
	go func() {
		err := lib.src.RemoveFromPlaylist(pid, []string{id})
		a.post(func() {
			if err != nil {
				if pl != nil && lib.playlists[pid] == pl {
					pl.Tracks = before
					a.playlistRefused(lib, pid)
				}
				a.actionFailed(lib, err, T("Couldn't remove it from the playlist. Try again later."))
				return
			}
			// The songs listed are right: only the sidebar's covers may
			// have changed.
			a.loadMine(lib)
		})
	}()
}

// moveTrack moves tr up or down the listener's playlist pid, at once in the
// app and then with its source, which may refuse: it goes back then.
func (a *app) moveTrack(lib *library, pid string, tr *music.Track, by int) {
	pl := lib.playlists[pid]
	if pl == nil {
		return
	}
	from := slices.IndexFunc(pl.Tracks, func(t music.Track) bool { return t.ID == tr.ID })
	to := from + by
	if from < 0 || to < 0 || to >= len(pl.Tracks) {
		return
	}
	before := pl.Tracks
	moved := slices.Clone(before)
	moved[from], moved[to] = moved[to], moved[from]
	pl.Tracks = moved
	go func() {
		err := lib.src.MovePlaylistTrack(pid, from, to)
		a.post(func() {
			if err != nil {
				if lib.playlists[pid] == pl {
					pl.Tracks = before
					a.playlistRefused(lib, pid)
				}
				a.actionFailed(lib, err, T("Couldn't move the song. Try again later."))
			}
		})
	}()
}

// playlistRefused asks anew for playlist pid, whose source refused a change
// to its songs: it may have changed elsewhere since it loaded. It shows as
// it is until the answer comes.
func (a *app) playlistRefused(lib *library, pid string) {
	if lib.playlists[pid] != nil && !lib.freshPl[pid] {
		a.freshenPlaylist(lib, pid)
	}
}

// playlistItems are the items of a song's menu that put it in the
// listener's playlists, or take it out of pid or move it there, the one it is
// listed in, if set. It reports whether it added any.
func (a *app) playlistItems(m *ui.Menu, tr *music.Track, pid string) bool {
	lib := a.trackLib(tr)
	if lib.signedOut() || !lib.can("addToPlaylist") && pid == "" {
		return false
	}
	mine := lib.playlists[pid]
	editing := pid != "" && lib == a.library && a.editable(pid) != nil
	if lib.can("addToPlaylist") {
		m.Submenu(T("Add to Playlist"), func(m *ui.Menu) {
			if lib.can("createPlaylist") && lib == a.library && m.Item(T("New Playlist…")).Chosen() {
				a.newPlaylist(tr)
			}
			var to []*music.Playlist
			for i := range lib.mine {
				if pl := &lib.mine[i]; pl.ID != pid && !pl.Fixed {
					to = append(to, pl)
				}
			}
			if len(to) > 0 {
				m.Separator()
			}
			for _, pl := range to {
				if m.Item(pl.Name).Chosen() {
					a.addTo(lib, pl.ID, tr)
				}
			}
		})
	}
	if !editing {
		return lib.can("addToPlaylist")
	}
	if lib.can("removeFromPlaylist") && m.Item(T("Remove from Playlist")).Chosen() {
		a.removeFrom(lib, pid, tr)
	}
	if lib.can("movePlaylistTrack") && mine != nil {
		at := slices.IndexFunc(mine.Tracks, func(t music.Track) bool { return t.ID == tr.ID })
		if at > 0 && m.Item(T("Move Up")).Chosen() {
			a.moveTrack(lib, pid, tr, -1)
		}
		if at >= 0 && at+1 < len(mine.Tracks) && m.Item(T("Move Down")).Chosen() {
			a.moveTrack(lib, pid, tr, 1)
		}
	}
	return true
}

// dialogFade is how long a dialog's backdrop takes to fade in, and out.
const dialogFade = 200 * time.Millisecond

// dialogIn brings a dialog in and takes it away: its backdrop fades in and
// out, and its panel grows to its size from a little smaller as it comes.
// MyGo plays the Enter and Exit of the backdrop, the overlay's own child, but
// not of the panel, which is new in a parent new in the same frame: the
// panel is posed smaller in its first frame and springs to its size from
// the next.
func dialogIn(c *ui.Context, backdrop, panel ui.Element) {
	backdrop.Transition(ui.ElementTransition{Duration: dialogFade, Enter: &ui.Motion{Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
	entered := ui.Local(panel, "entered", func() bool { return false })
	pose := ui.Pose{}
	if !*entered {
		pose, *entered = ui.Pose{Scale: 0.94}, true
		c.Invalidate()
	}
	panel.AnimatePose(pose, ui.Snappy)
}

// playlistNameDialog asks for the name of a playlist to make, or a new
// name for one.
func (a *app) playlistNameDialog(c *ui.Context) {
	p := a.pal
	d := &a.naming
	ui.DialogBase(c.Key("playlist-name"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(18).Gap(16).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4))
		dialogIn(c, backdrop, panel)
		title, action := T("New Playlist"), T("Create")
		if d.rename != "" {
			title, action = T("Edit Playlist"), T("Done")
		}
		if d.busy {
			action = T("Creating…")
		}
		ui.Text(c, title).FontSize(17).FontWeight(700)
		in := ui.TextInput(c.Key("name"), &d.name).Placeholder(T("Playlist title")).Label(T("Playlist title")).FontSize(14).AutoFocus()
		if in.Submitted() {
			a.submitName()
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if pillButton(c.Key("cancel"), nil, T("Cancel"), p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			ok := pillButton(c.Key("ok"), nil, action, p.accent, p.onAccent)
			if strings.TrimSpace(d.name) == "" {
				ok.Disabled(true).Opacity(0.5)
			}
			if ok.Clicked() {
				a.submitName()
			}
		})
	})
}

// deletePlaylistDialog asks whether to delete a playlist, which can't be
// undone, as Apple Music asks it: Delete From Library, with OK and Cancel.
func (a *app) deletePlaylistDialog(c *ui.Context) {
	p := a.pal
	d := &a.deleting
	ui.DialogBase(c.Key("playlist-delete"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(18).Gap(8).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4))
		dialogIn(c, backdrop, panel)
		ui.Text(c, T("Delete From Library")).FontSize(17).FontWeight(700)
		ui.Text(c, T("Are you sure you want to delete this playlist from your Library? This will delete it from all your devices.")).FontSize(13).TextColor(p.muted).LineHeight(1.45)
		ui.Row(c).Gap(10).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
			if pillButton(c.Key("cancel"), nil, T("Cancel"), p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			if pillButton(c.Key("ok"), nil, T("OK"), p.accent, p.onAccent).Clicked() {
				a.deleteChosen()
			}
		})
	})
}
