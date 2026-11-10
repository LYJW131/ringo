package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// signInDialog is signing in to a source. The source does it its own way, in
// the listener's browser or wherever, which can take minutes.
type signInDialog struct {
	open    bool
	lib     *library
	here    bool // asked in the settings window, which shows it, not the main one
	waiting bool
	gen     int // counts the ones started, to tell the last
	err     string

	// asked is when the dialog first showed this sign-in on its way, by the
	// view's clock, and last is the error of the one before. Until it has
	// gone on for signInSettle, the dialog shows what it did, as one that
	// answers at once would otherwise flash its wait, and move the dialog
	// for it, as the wait takes a line where an error may take two.
	asked time.Time
	last  string
}

const signInSettle = 250 * time.Millisecond

// loadAccount asks a source who is signed in.
func (a *app) loadAccount(lib *library) {
	if !lib.can("account") || lib.accountBusy {
		return
	}
	lib.accountBusy = true
	go func() {
		acc, err := lib.src.Account()
		a.post(func() {
			lib.accountBusy = false
			first := !lib.accountAsked
			lib.accountAsked = true
			if err != nil {
				a.problem(lib, err, T("Couldn't reach Apple Music"))
				return
			}
			was := lib.account
			lib.account = acc
			switch {
			case first && was == nil && acc != nil && lib.homeErr == "" && !lib.needsSignIn:
				// Home, as it started, was already this listener's, and loading
				// it again could only move it. Their playlists wait for them
				// (loadMine). But hearts asked about before they were known are
				// dropped as someone else's.
				lib.liked = nil
				a.loadMine(lib)
			case (was == nil) != (acc == nil):
				a.afterAccount(lib)
			}
		})
	}()
}

// afterAccount loads anew what's personal to who listens to a source.
func (a *app) afterAccount(lib *library) {
	lib.accountAsked = true
	lib.forget()
	lib.needsSignIn, lib.askedSignIn = false, false
	a.loadMine(lib)
	a.loadHome(lib)
	if a.route.src == lib.id {
		a.reload(a.route)
	}
}

// promptSignIn asks the listener to sign in to a source, showing the window it
// asks in. A source that has no accounts has nothing to ask.
func (a *app) promptSignIn(lib *library) {
	if !lib.can("signIn") {
		a.toast = Tf("%s isn't available right now", lib.name())
		return
	}
	a.showMain()
	a.signIn = signInDialog{open: true, lib: lib, gen: a.signIn.gen + 1}
}

// promptSignInInSettings asks it in the settings window, where they asked,
// instead of bringing up the main window behind it.
func (a *app) promptSignInInSettings(lib *library) {
	a.signIn = signInDialog{open: true, lib: lib, here: true, gen: a.signIn.gen + 1}
}

// startSignIn signs in to the source asked about, off the main thread: it
// waits for the listener at the source. A sign-in that ends after they closed
// the dialog still counts.
func (a *app) startSignIn() {
	d := &a.signIn
	lib := d.lib
	if lib == nil || d.waiting {
		return
	}
	d.waiting, d.err, d.last = true, "", d.err
	d.asked = time.Time{}
	gen := d.gen
	over := a.win
	if d.here && a.settingsWin != nil {
		over = a.settingsWin
	}
	if a.signInOver != nil {
		a.signInOver.Store(over)
	}
	go func() {
		acc, err := lib.src.SignIn()
		a.post(func() {
			current := a.signIn.gen == gen
			if current {
				a.signIn.waiting = false
			}
			if err != nil {
				log := err.Error()
				if current {
					a.signIn.err = log
				}
				return
			}
			if current {
				a.signIn.open = false
			}
			lib.account = acc
			a.afterAccount(lib)
			// Signed in to another source from the settings: that's the one they're
			// after. One finishing after they closed the dialog doesn't move them.
			if current && a.library != lib {
				a.switchSource(lib.id)
			}
		})
	}()
}

// signOut signs out of a source.
func (a *app) signOut(lib *library) {
	lib.account = nil
	a.afterAccount(lib)
	go func() {
		if err := lib.src.SignOut(); err != nil {
			a.post(func() { a.actionFailed(lib, err, T("Couldn't sign out")) })
		}
	}()
}

// signInDialog says which source the listener signs in to, while it waits for
// them there, in the window it was asked in: settings says whether that's
// the settings window.
func (a *app) signInDialog(c *ui.Context, p palette, settings bool) {
	d := &a.signIn
	if d.here != settings {
		return
	}
	ui.DialogBase(c.Key("sign-in"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		// What it holds changes its height, and it follows at once: a spring
		// moved it, and its text with it, by fractions of a pixel for as long
		// as half a second after a sign-in failed.
		panel.Width(360).Padding(24).Radius(18).Gap(10).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Colors: true})
		dialogIn(c, backdrop, panel)
		name := ""
		if d.lib != nil {
			name = d.lib.name()
		}
		ui.Text(c, Tf("Sign in to %s", name)).FontSize(17).FontWeight(700)
		waiting := d.waiting
		if waiting {
			if d.asked.IsZero() {
				d.asked = c.Now()
			}
			if left := signInSettle - c.Now().Sub(d.asked); left > 0 {
				waiting = false
				c.After(left)
			}
		}
		text := T("You'll sign in on Apple's own page, in a window that opens.")
		switch {
		case d.err != "":
			text = d.err
		case waiting && a.signInWin != nil:
			text = T("Continue in the window that opened.")
		case waiting:
			text = T("Waiting for you to finish signing in…")
		case d.last != "":
			text = d.last
		}
		ui.Text(c, text).FontSize(13).TextColor(p.muted).LineHeight(1.45)
		ui.Row(c).Gap(10).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
			if pillButton(c.Key("cancel"), nil, T("Cancel"), p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
				// Its page goes too. Put away otherwise, the dialog leaves it to
				// finish signing in on.
				if d.lib != nil {
					a.closeSignInPage()
				}
			}
			if !waiting && pillButton(c.Key("ok"), nil, T("Sign In"), p.accent, p.onAccent).Clicked() {
				a.startSignIn()
			}
		})
	})
}

// accountRow sits at the foot of the sidebar: who is signed in to the source,
// with a dropdown of the settings and signing out; or a way to sign in, if it
// has accounts; or else the way to the settings.
func (a *app) accountRow(c *ui.Context) {
	p := a.pal
	lib := a.library
	// As tall as what takes its place, and no way to sign in to whoever may be.
	if lib.account == nil && lib.accountUnknown() {
		s := a.pageShimmer(c)
		ui.Row(c.Key("account-unknown")).Height(40).PaddingX(8).Gap(10).AlignItems(ui.Center).Children(func() {
			s.bone(c, 13).Size(26, 26)
			s.text(c, 13, 1.3).Width(96)
		})
		return
	}
	b := ui.ButtonBase(c.Key("account")).Height(40).PaddingX(8).Gap(10).Radius(10).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || a.accountOpen && lib.account != nil {
		b.Background(p.hover)
	}
	if lib.account == nil {
		a.accountOpen = false
		if lib.can("signIn") {
			label := Tf("Sign in to %s", lib.name())
			b.Label(label).Children(func() {
				ui.Box(c).Size(26, 26).Radius(13).Background(p.press).Center().Children(func() {
					ui.Icon(c, iconUser).Size(14, 14).TextColor(p.muted)
				})
				ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.muted).SingleLine().Shrink(1).MinWidth(0)
			})
			if b.Clicked() {
				a.promptSignIn(lib)
			}
			return
		}
		b.Label(T("Settings")).Children(func() {
			ui.Box(c).Size(26, 26).Center().Children(func() {
				ui.Icon(c, iconSettings).Size(15, 15).TextColor(p.muted)
			})
			ui.Text(c, T("Settings")).FontSize(13).FontWeight(500).TextColor(p.muted)
		})
		if b.Clicked() {
			a.openSettings()
		}
		return
	}
	acc := lib.account
	b.Label(acc.Name).Expanded(a.accountOpen).Children(func() {
		a.avatar(c, acc.ImageURL, 26, p.press)
		ui.Text(c, acc.Name).FontSize(13).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ui.Icon(c, iconUpDown).Size(13, 13).TextColor(p.faint)
	})
	if b.Clicked() {
		a.accountOpen = !a.accountOpen
	}
	a.accountMenu(c, b)
}

// accountMenu drops up from the account row, as wide as it, in the colors
// of the volume popover: the listener's own page, if the source has one, and
// the settings, then signing out.
func (a *app) accountMenu(c *ui.Context, row ui.Element) {
	pop := popoverColors(a.pal.dark)
	ui.PopoverBase(c.Key("account-menu"), row, &a.accountOpen, func(panel ui.Element) {
		panel.AttachTo(row, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).Padding(5).Gap(1).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label(T("Account"))
		if w := row.Bounds().W; w > 0 {
			panel.Width(w)
		}
		shortcut := "Ctrl+,"
		if runtime.GOOS == "darwin" {
			shortcut = "⌘,"
		}
		first := true
		if acc := a.library.account; acc != nil && acc.Page != "" {
			first = false
			if a.menuItem(c, iconUser, T("Profile"), "").AutoFocus().Clicked() {
				a.accountOpen = false
				a.openMyPage(a.library)
			}
		}
		settings := a.menuItem(c, iconSettings, T("Settings…"), shortcut)
		if first {
			settings.AutoFocus()
		}
		if settings.Clicked() {
			a.accountOpen = false
			a.openSettings()
		}
		ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
		if a.menuItem(c, iconSignOut, T("Sign Out"), "").Clicked() {
			a.accountOpen = false
			a.signOut(a.library)
		}
	})
}

// openMyPage opens the listener's own page at lib, loaded anew each time: they
// follow and save on other pages, and it keeps up. What it had shows meanwhile.
func (a *app) openMyPage(lib *library) {
	r := route{page: pageBrowse, id: lib.account.Page, src: lib.id}
	a.open(r)
	a.loadBrowse(lib, r.id, true)
}

// menuItem is an item of a dropdown: an icon, its label, and the shortcut
// that does the same, if any.
func (a *app) menuItem(c *ui.Context, icon *ui.SVG, label, shortcut string) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c.Key(label)).Role(ui.RoleMenuItem).Label(label).Height(30).PaddingX(8).Gap(9).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.press)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(p.muted)
		ui.Text(c, label).FontSize(13).Grow(1)
		if shortcut != "" {
			ui.Text(c, shortcut).FontSize(12).TextColor(p.faint)
		}
	})
	return b
}

// avatar shows a listener's picture, round, over empty while it loads.
func (a *app) avatar(c *ui.Context, url string, size float32, empty ui.Color) {
	bmp := bitmap(a.art.get(sized(url, int(size*3)), false))
	box := ui.Box(c).Size(size, size).Radius(size / 2).Shrink(0)
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(empty)
	}
	box.Children(func() { artImage(c, bmp, size/2, in) })
}

// playlistMenu has what can be done with a playlist of the listener's, as far
// as the source can.
func (a *app) playlistMenu(m *ui.Menu, pl *music.Playlist) {
	if a.can("renamePlaylist") && m.Item(T("Rename…")).Chosen() {
		a.startRename(pl)
	}
	if a.can("deletePlaylist") {
		if a.can("renamePlaylist") {
			m.Separator()
		}
		if m.Item(T("Delete Playlist…")).Chosen() {
			a.confirmDelete(pl)
		}
	}
}
