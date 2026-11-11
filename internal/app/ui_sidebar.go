package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// The sidebar is Music's: the search field, the pages of Apple Music, then
// the listener's library and their playlists, which show once they are
// signed in, as Music has no library to show before.

// sidebarItem is an entry of the sidebar.
type sidebarItem struct {
	icon  *ui.SVG
	label string
	route route
}

// storeItems are the pages of Apple Music the sidebar opens.
func (a *app) storeItems() []sidebarItem {
	return []sidebarItem{
		{iconHome, T("Home"), route{page: pageHome}},
		{iconNew, T("New"), route{page: pageBrowse, id: music.PageNew}},
		{iconRadio, T("Radio"), route{page: pageBrowse, id: music.PageRadio}},
	}
}

// libraryItems are the pages of the listener's library.
func (a *app) libraryItems() []sidebarItem {
	return []sidebarItem{
		{iconClock, T("Recently Added"), route{page: pageBrowse, id: music.PageRecentlyAdded}},
		{iconMic, T("Artists"), route{page: pageBrowse, id: music.PageLibraryArtist}},
		{iconAlbumStack, T("Albums"), route{page: pageBrowse, id: a.libraryPage(music.PageLibraryAlbums)}},
		{iconNote, T("Songs"), route{page: pageBrowse, id: a.libraryPage(music.PageLibrarySongs)}},
		{iconForYou, T("Made for You"), route{page: pageBrowse, id: music.PageMadeForYou}},
	}
}

// hasLibrary reports whether the listener has a library to show: once they
// are signed in.
func (a *app) hasLibrary() bool {
	lib := a.library
	return lib.account != nil
}

func (a *app) sidebar(c *ui.Context) {
	// Under the window buttons where they sit over the sidebar, as the
	// traffic lights do; elsewhere, as close to the top as to the sides.
	top := float32(14)
	if c.TitleBar().Left > 0 {
		top = topBar(c) + 4
	}
	// Where nothing in it takes the press, the sidebar moves the window, as
	// the title bar it reaches up into would.
	ui.Column(c).Fill().Padding(top, 14, 14, 14).Gap(4).DragWindow().Children(func() {
		a.searchField(c)

		nav := ui.Scroll(c.Key("nav")).TrackScroll(&a.navScroll).Grow(1).Margin(0, -6).Padding(0, 6).Gap(2)
		a.bars.quiet(c, nav, "nav", barPlace{y: a.navScroll.Y, last: -1})
		a.navFade(nav)
		nav.Children(func() {
			for _, n := range a.storeItems() {
				a.navItem(c, n.icon, n.label, n.route)
			}
			if !a.hasLibrary() {
				return
			}
			if items := a.libraryItems(); len(items) > 0 {
				a.sidebarGroup(c.Key("library"), T("Library"), &a.libraryCollapsed, nil)
				if !a.libraryCollapsed {
					for _, n := range items {
						a.navItem(c, n.icon, n.label, n.route)
					}
				}
			}
			a.sidebarPlaylists(c)
		})

		a.accountRow(c)
	})
}

// navFadeHeight is how far the sidebar's list fades out at an edge it is
// scrolled away from.
const navFadeHeight = 12

// navFade fades nav, the sidebar's list, out at its edges where there is
// more of it beyond, rather than cut a row in half there.
func (a *app) navFade(nav ui.Element) {
	st := &a.navScroll
	h := nav.Bounds().H
	if h <= 2*navFadeHeight || st.MaxY <= 0.5 {
		return
	}
	black := ui.RGB(0, 0, 0)
	edge := navFadeHeight / h
	top, bottom := black, black
	if st.Y > 0.5 {
		top = ui.Transparent
	}
	if st.Y < st.MaxY-0.5 {
		bottom = ui.Transparent
	}
	nav.MaskLinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
		{Color: top, At: 0}, {Color: black, At: edge}, {Color: black, At: 1 - edge}, {Color: bottom, At: 1},
	}})
}

func (a *app) navItem(c *ui.Context, icon *ui.SVG, label string, r route) {
	p := a.pal
	// A list of the library is itself however it's sorted.
	selected := !a.nowPlaying && a.route.page == r.page && (a.route.id == r.id || r.page == pageBrowse && sameLibraryPage(a.route.id, r.id))
	b := ui.ButtonBase(c.Key(label)).Height(32).PaddingX(10).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	bg := ui.Transparent
	switch {
	case selected:
		bg = p.press
	case b.Hovered():
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Icon(c, icon).Size(17, 17).TextColor(p.accent)
		weight := 500
		if selected {
			weight = 600
		}
		ui.Text(c, label).FontSize(13.5).FontWeight(weight).TextColor(p.text).SingleLine().Shrink(1).MinWidth(0)
	})
	if b.Clicked() {
		a.open(r)
	}
}

// sidebarGroup heads a group of the sidebar, as Music's do: its title opens
// and closes it, and add, if set, is a button beside it that makes one more.
func (a *app) sidebarGroup(c *ui.Context, title string, collapsed *bool, add func()) {
	p := a.pal
	ui.Row(c).AlignItems(ui.Center).Margin(16, 4, 4, 6).Children(func() {
		b := ui.ButtonBase(c.Key("toggle")).Grow(1).Height(24).PaddingX(4).Gap(6).Radius(6).
			AlignItems(ui.Center).Justify(ui.Start).Role(ui.RoleDisclosure).Label(title)
		if b.Clicked() {
			*collapsed = !*collapsed
		}
		b.Expanded(!*collapsed)
		hovered := b.Hovered()
		if hovered {
			b.Background(p.hover)
		}
		b.Children(func() {
			ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(p.faint).Grow(1)
			// The arrow shows under the pointer or closed, as Music's does.
			if hovered || *collapsed {
				arrow := iconDown
				if *collapsed {
					arrow = iconForward
				}
				ui.Icon(c, arrow).Size(12, 12).TextColor(p.faint)
			}
		})
		if add != nil && iconButton(c.Key("new"), iconPlus, 13, p.faint, T("New Playlist")).Size(26, 26).Radius(6).Clicked() {
			add()
		}
	})
}

// sidebarPlaylists is the group of the listener's playlists: all of them on
// a page, their favorite songs, then each of those they made, with a button
// to make one and a menu to rename them, and those they saved, with a menu
// to unsave them.
func (a *app) sidebarPlaylists(c *ui.Context) {
	a.sidebarGroup(c.Key("playlists"), T("Playlists"), &a.mineCollapsed, func() { a.newPlaylist(nil) })
	if a.mineCollapsed {
		return
	}
	a.navItem(c, iconGrid, T("All Playlists"), route{page: pageBrowse, id: music.PageAllPlaylists})
	a.navItem(c, iconStar, T("Favorite Songs"), route{page: pageLiked})
	a.playlistRows(c.Key("mine"), a.mine, true)
	a.playlistRows(c.Key("saved"), a.saved, false)
}

// playlistRows are the sidebar's rows of lists, the listener's own if mine.
func (a *app) playlistRows(c *ui.Context, lists []music.Playlist, mine bool) {
	p := a.pal
	for i := range lists {
		pl := &lists[i]
		if !mine && !a.isSaved(pl) {
			// Deleted from the library, and on its way out.
			continue
		}
		selected := a.route.page == pagePlaylist && a.route.id == pl.ID && !a.nowPlaying
		// Its name starts where the pages' names above do: 8+22+7 is
		// navItem's 10+17+10.
		b := ui.ButtonBase(c.Key(pl.ID)).Height(32).PaddingX(8).Gap(7).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pl.Name)
		bg := ui.Transparent
		switch {
		case selected:
			bg = p.press
		case b.Hovered():
			bg = p.hover
		}
		// As the pages' rows above it fade theirs.
		b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: hoverFade}))
		b.Children(func() {
			a.cover(c, pl.ImageURL, playlistMoving(pl), 80, 22, 4)
			ui.Text(c, pl.Name).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
		})
		switch {
		case mine && pl.Fixed:
		case mine:
			b.ContextMenu(func(m *ui.Menu) { a.playlistMenu(m, pl) })
		case !mine:
			b.ContextMenu(func(m *ui.Menu) {
				if m.Item(T("Delete From Library")).Chosen() {
					a.setInLibrary(music.ItemPlaylist, pl.ID, false)
				}
			})
		}
		if b.Clicked() {
			a.open(route{page: pagePlaylist, id: pl.ID})
		}
	}
}
