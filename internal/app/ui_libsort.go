package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// The library's Albums and Songs, sorted and filtered as Music's are: beside
// the list's title, a button opens the menu of how (All Albums or Favorites
// Only; Sort Options: Title or Recently Added, Ascending or Descending), and
// a field finds what has a word. How a list sorts and filters is its page
// (music.LibraryView), which the source answers and the app keeps as any
// other page; the last way each list was shown is kept in the settings,
// what was looked for isn't.

// libraryWords are a list's words for its menu and its field, Music's own.
var libraryWords = map[string]struct{ all, find string }{
	music.PageLibraryAlbums: {N("All Albums"), N("Find in Albums")},
	music.PageLibrarySongs:  {N("All Songs"), N("Find in Songs")},
}

// librarySorts are the ways a list sorts, as Apple's library can, and the
// way round each starts when it's chosen: titles from A, and what was added
// last first.
var librarySorts = []struct {
	key, label string
	descending bool
}{
	{"", N("Title"), false},
	{music.LibrarySortAdded, N("Recently Added"), true},
}

// findPause is how long the field waits after a key before it looks.
const findPause = 300 * time.Millisecond

// libraryFind is what the field of a list holds: text as typed, seen at,
// and applied, the word the page shown looks for.
type libraryFind struct {
	base, text, seen, applied string
	at                        time.Time
}

// libraryDefault is how a list shows until the listener says how: what was
// added last first, which is what they're likeliest to have come for.
var libraryDefault = music.LibraryView{Sort: music.LibrarySortAdded, Descending: true}

// libraryPage is the page of base the sidebar opens: shown as it was last,
// or as libraryDefault.
func (a *app) libraryPage(base string) string {
	if id := a.settings.LibraryViews[base]; id != "" {
		if b, _ := music.ParseLibraryPage(id); b == base {
			return id
		}
	}
	if _, sorts := libraryWords[base]; !sorts {
		return base
	}
	return music.LibraryPage(base, libraryDefault)
}

// sameLibraryPage says two pages are of one list, however each is shown.
func sameLibraryPage(x, y string) bool {
	bx, _ := music.ParseLibraryPage(x)
	by, _ := music.ParseLibraryPage(y)
	return bx == by
}

// showLibrary shows base as v in place of the page shown, keeping how it
// sorts and filters for the next time it opens, unless that's libraryDefault.
func (a *app) showLibrary(base string, v music.LibraryView) {
	kept := v
	kept.Term = ""
	if kept != libraryDefault {
		if a.settings.LibraryViews == nil {
			a.settings.LibraryViews = map[string]string{}
		}
		a.settings.LibraryViews[base] = music.LibraryPage(base, kept)
	} else {
		delete(a.settings.LibraryViews, base)
	}
	a.settings.save()
	a.show(route{page: pageBrowse, id: music.LibraryPage(base, v)})
}

// libraryHeader heads a list that sorts: its title, then its button and its
// field, as Music's toolbar has them. It reports whether page is one.
func (a *app) libraryHeader(c *ui.Context, page *music.Browse, id string) bool {
	base, v := music.ParseLibraryPage(id)
	w, ok := libraryWords[base]
	if !ok {
		return false
	}
	title := page.Title
	if title == "" {
		title = T(pageTitles[base])
	}
	ui.Row(c).AlignItems(ui.Center).Gap(10).Margin(0, 0, 18, 0).Children(func() {
		ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		a.libraryMenu(c, base, v, w.all)
		a.libraryField(c, base, v, w.find)
	})
	return true
}

// libraryMenu is the button of a list's menu of how it shows.
func (a *app) libraryMenu(c *ui.Context, base string, v music.LibraryView, all string) {
	b := iconButton(c.Key("view"), iconFilter, 17, a.pal.muted, T("Sort Options"))
	b.Menu(func(m *ui.Menu) {
		with := func(change func(*music.LibraryView)) {
			next := v
			change(&next)
			a.showLibrary(base, next)
		}
		if m.Item(T(all)).Checked(!v.Favorites).Chosen() {
			with(func(n *music.LibraryView) { n.Favorites = false })
		}
		if m.Item(T("Favorites Only")).Checked(v.Favorites).Chosen() {
			with(func(n *music.LibraryView) { n.Favorites = true })
		}
		m.Separator()
		m.Submenu(T("Sort Options"), func(m *ui.Menu) {
			for _, s := range librarySorts {
				if m.Item(T(s.label)).Checked(v.Sort == s.key).Chosen() {
					with(func(n *music.LibraryView) { n.Sort, n.Descending = s.key, s.descending })
				}
			}
			m.Separator()
			if m.Item(T("Ascending")).Checked(!v.Descending).Chosen() {
				with(func(n *music.LibraryView) { n.Descending = false })
			}
			if m.Item(T("Descending")).Checked(v.Descending).Chosen() {
				with(func(n *music.LibraryView) { n.Descending = true })
			}
		})
	})
}

// libraryField finds what of a list has a word, as Music's field does, a
// moment after the last key: the page shown is then the list looking for it.
func (a *app) libraryField(c *ui.Context, base string, v music.LibraryView, placeholder string) {
	p := a.pal
	f := &a.libFind
	if f.base != base || v.Term != f.applied {
		// Another list, or this one opened anew from the sidebar.
		*f = libraryFind{base: base, text: v.Term, seen: v.Term, applied: v.Term}
	}
	field := ui.Row(c).Width(220).Height(30).Radius(8).AlignItems(ui.Stretch).Shrink(0)
	if c.Focused(a.libFindBox) {
		field.Background(p.bg).Shadow(0, 0, 0, 3, p.accent.Alpha(0.25)).Border(1, p.accent)
	} else {
		field.Background(p.hover).Border(1, p.hairline)
	}
	field.Children(func() {
		end := float32(9)
		if f.text != "" {
			end = 2
		}
		ui.TextInputBase(c.Key("find"), &f.text).Bind(&a.libFindBox).Grow(1).MinWidth(0).PaddingStart(29).PaddingEnd(end).
			Placeholder(T(placeholder)).FontSize(13).Label(T(placeholder))
		ui.Icon(c, iconSearch).Size(13, 13).TextColor(p.muted).Absolute().Left(9).Top(7.5).IgnoreInput(true)
		if f.text != "" {
			x := ui.ButtonBase(c.Key("clear")).Size(22, 22).Shrink(0).Center().AlignSelf(ui.Center).Margin(0, 4, 0, 0).
				KeepFocus().FocusRing(false).Label(T("Clear"))
			fill := p.faint
			if x.Hovered() {
				fill = p.muted
			}
			x.Children(func() {
				ui.Box(c).Size(14, 14).Radius(7).Center().Background(fill).Children(func() {
					ui.Icon(c, iconX).Size(8, 8).TextColor(p.bg)
				})
			})
			if x.Clicked() {
				f.text = ""
			}
		}
	})
	now := c.Now()
	if f.text != f.seen {
		f.seen, f.at = f.text, now
	}
	if strings.TrimSpace(f.text) == strings.TrimSpace(f.applied) {
		return
	}
	// Emptied, the whole list comes back at once.
	if wait := findPause - now.Sub(f.at); wait > 0 && f.text != "" {
		c.After(wait)
		return
	}
	f.applied = f.text
	v.Term = strings.TrimSpace(f.text)
	a.showLibrary(base, v)
}
