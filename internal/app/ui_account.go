package app

import (
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
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
func (a *app) loadAccount() {
	lib := a.library
	if lib.accountBusy {
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
				a.problem(err, T("Couldn't reach Apple Music"))
				return
			}
			was := lib.account
			lib.account = acc
			a.adoptPages()
			a.loadAccounts(false)
			switch {
			case first && was == nil && acc != nil && lib.homeErr == "" && !lib.needsSignIn:
				// Home, as it started, was already this listener's, and loading
				// it again could only move it. Their playlists wait for them
				// (loadMine). But hearts asked about before they were known are
				// dropped as someone else's.
				lib.liked = nil
				a.loadMine()
			case (was == nil) != (acc == nil):
				a.afterAccount()
			}
			a.warmPages()
		})
	}()
}

// afterAccount loads anew what's personal to who listens to a source.
func (a *app) afterAccount() {
	lib := a.library
	lib.accountAsked = true
	lib.forget()
	a.doubtPlayer()
	a.resetPages()
	lib.needsSignIn, lib.askedSignIn = false, false
	lib.choicesAt = time.Time{}
	a.loadAccounts(false)
	a.loadMine()
	a.loadHome()
	a.reload(a.route)
	a.warmPages()
}

// promptSignIn asks the listener to sign in, showing the window it asks in.
func (a *app) promptSignIn() {
	a.showMain()
	a.signIn = signInDialog{open: true, lib: a.library, gen: a.signIn.gen + 1}
}

// promptSignInInSettings asks it in the settings window, where they asked,
// instead of bringing up the main window behind it.
func (a *app) promptSignInInSettings() {
	a.signIn = signInDialog{open: true, lib: a.library, here: true, gen: a.signIn.gen + 1}
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
			a.afterAccount()
		})
	}()
}

// signOut signs out of a source. One that goes on as another account the
// listener signed in to says so as it's asked who's signed in after.
func (a *app) signOut() {
	lib := a.library
	lib.account = nil
	a.afterAccount()
	go func() {
		err := lib.src.SignOut()
		a.post(func() {
			if err != nil {
				a.actionFailed(err, T("Couldn't sign out"))
			}
			lib.choicesAt = time.Time{}
			a.loadAccount()
		})
	}()
}

// switcher is the source as a music.Switcher, if it lets the listener switch accounts.
func (l *library) switcher() music.Switcher {
	sw, _ := l.src.(music.Switcher)
	return sw
}

// choicesKept is how long the accounts listed are shown before they're
// asked for again, as the settings or the account menu show.
const choicesKept = time.Minute

// loadAccounts asks the source for the accounts to switch to, again only
// after a while unless fresh, which also asks whoever lends them anew.
func (a *app) loadAccounts(fresh bool) {
	lib := a.library
	sw := lib.switcher()
	if sw == nil || !fresh && !lib.choicesAt.IsZero() && time.Since(lib.choicesAt) < choicesKept {
		return
	}
	// Asked again once the answer on its way is in, as it may be from before what changed.
	if lib.choicesBusy {
		lib.choicesAgain = true
		return
	}
	lib.choicesBusy = true
	go func() {
		list, err := sw.Accounts(fresh)
		a.post(func() {
			lib.choicesBusy, lib.choicesAt = false, time.Now()
			lib.choices, lib.choicesErr = list, ""
			if err != nil {
				lib.choicesErr = errText(err)
			}
			if lib.choicesAgain {
				lib.choicesAgain, lib.choicesAt = false, time.Time{}
				a.loadAccounts(false)
			}
		})
	}()
}

// errText is what the listener is told of err: what the source said of it, if anything.
func errText(err error) string {
	if r := music.Reason(err); r != "" {
		return r
	}
	return err.Error()
}

// useAccount switches to another account, off the main thread, as the
// wrapper may be asked for its token: the row shows it's on its way, and
// everything personal loads anew once it's there. What plays goes on.
func (a *app) useAccount(id string) {
	lib := a.library
	sw := lib.switcher()
	if sw == nil || lib.switching != "" {
		return
	}
	lib.switching, lib.switchErr = id, ""
	go func() {
		acc, err := sw.UseAccount(id)
		a.post(func() {
			lib.switching = ""
			if err != nil {
				lib.switchErr = errText(err)
				a.toast = Tf("Couldn't switch accounts: %s", lib.switchErr)
				return
			}
			lib.account = acc
			lib.needsSetup, lib.setupReason = false, ""
			a.afterAccount()
		})
	}()
}

// addAccount signs in with a media-user-token the listener pasted, as an
// account to switch to, and switches to it.
func (a *app) addAccount(token string) {
	lib := a.library
	sw := lib.switcher()
	add := &a.adding
	if sw == nil || add.busy {
		return
	}
	add.busy, add.err = true, ""
	go func() {
		acc, err := sw.AddAccount(token)
		a.post(func() {
			add.busy = false
			if err != nil {
				add.err = errText(err)
				return
			}
			*add = addingAccount{}
			lib.account = acc
			lib.needsSetup, lib.setupReason = false, ""
			a.afterAccount()
		})
	}()
}

// removeAccount forgets an account the listener signed in to, other than
// the current one, whose way out is signing out.
func (a *app) removeAccount(id string) {
	lib := a.library
	sw := lib.switcher()
	if sw == nil {
		return
	}
	lib.choices = slices.DeleteFunc(slices.Clone(lib.choices), func(c music.AccountChoice) bool { return c.ID == id })
	go func() {
		err := sw.RemoveAccount(id)
		a.post(func() {
			if err != nil {
				a.toast = Tf("Couldn't remove the account: %s", errText(err))
			}
			a.loadAccounts(false)
		})
	}()
	lib.choicesAt = time.Time{}
}

// addingAccount is the Add Account… of the settings, opened: the ways to
// add one, and the token being pasted.
type addingAccount struct {
	open, token bool
	text        string
	busy        bool
	err         string
}

// menuChoices are the accounts the account menu lists: none for one alone,
// and of many, the most recently used, as they're listed. more says some were left out.
func menuChoices(all []music.AccountChoice) (shown []music.AccountChoice, more bool) {
	if len(all) < 2 {
		return nil, false
	}
	if len(all) <= menuAccounts {
		return all, false
	}
	recent := slices.Clone(all)
	slices.SortStableFunc(recent, func(x, y music.AccountChoice) int {
		if x.Current != y.Current {
			if x.Current {
				return -1
			}
			return 1
		}
		return y.Used.Compare(x.Used)
	})
	keep := map[string]bool{}
	for _, c := range recent[:menuAccounts] {
		keep[c.ID] = true
	}
	for _, c := range all {
		if keep[c.ID] {
			shown = append(shown, c)
		}
	}
	return shown, true
}

// menuAccounts is how many accounts the account menu lists at most.
const menuAccounts = 6

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
// with a dropdown of the settings and signing out; or a way to sign in.
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
		label := Tf("Sign in to %s", lib.name())
		b.Label(label).Children(func() {
			ui.Box(c).Size(26, 26).Radius(13).Background(p.press).Center().Children(func() {
				ui.Icon(c, iconUser).Size(14, 14).TextColor(p.muted)
			})
			ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(p.muted).SingleLine().Shrink(1).MinWidth(0)
		})
		if b.Clicked() {
			a.promptSignIn()
		}
		return
	}
	acc := lib.account
	b.Label(acc.Name).Expanded(a.accountOpen).Children(func() {
		a.choiceAvatar(c, p, a.accountChoice(acc), 26, true)
		ui.Text(c, acc.Name).FontSize(13).FontWeight(600).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ui.Icon(c, iconUpDown).Size(13, 13).TextColor(p.faint)
	})
	if b.Clicked() {
		a.accountOpen = !a.accountOpen
	}
	a.accountMenu(c, b)
}

// accountChoice is acc as the accounts to switch to list it, which know
// where it is: an account without a picture shows that in its place, as it
// does in the menu, not an empty circle. Not listed, it is a figure.
func (a *app) accountChoice(acc *music.Account) music.AccountChoice {
	for _, ch := range a.library.choices {
		if ch.Current || ch.ID == acc.ID {
			ch.ImageURL = acc.ImageURL
			return ch
		}
	}
	return music.AccountChoice{ID: acc.ID, Name: acc.Name, ImageURL: acc.ImageURL}
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
		popIn(c, panel, 0)
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
				a.openMyPage()
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
		lib := a.library
		// Listed as the account loads; here only asked again once it's been a while.
		if !lib.choicesAt.IsZero() {
			a.loadAccounts(false)
		}
		if shown, more := menuChoices(lib.choices); len(shown) > 0 {
			ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
			for _, ch := range shown {
				if a.accountItem(c, ch, lib.switching == ch.ID).Clicked() {
					a.accountOpen = false
					if !ch.Current {
						a.useAccount(ch.ID)
					}
				}
			}
			if more && a.menuItem(c, iconMore, T("More Accounts…"), "").Clicked() {
				a.accountOpen = false
				a.openAccountSettings()
			}
		}
		// An account lent by a wrapper has nothing to sign out of: the way out
		// of it is to switch to another.
		if acc := lib.account; acc == nil || !acc.Shared {
			ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
			if a.menuItem(c, iconSignOut, T("Sign Out"), "").Clicked() {
				a.accountOpen = false
				a.signOut()
			}
		}
	})
}

// accountItem is an account to switch to in the account menu: its picture,
// or where it is for one without, its name, and a check on the current one.
func (a *app) accountItem(c *ui.Context, ch music.AccountChoice, busy bool) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c.Key("account:" + ch.ID)).Role(ui.RoleMenuItem).Label(ch.Name).Height(30).PaddingX(8).Gap(9).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.press)
	}
	b.Children(func() {
		a.choiceAvatar(c, p, ch, 18, true)
		ui.Text(c, ch.Name).FontSize(13).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		switch {
		case busy:
			ui.Text(c, "…").FontSize(13).TextColor(p.faint)
		case ch.Current:
			ui.Icon(c, iconCheck).Size(13, 13).TextColor(p.accent)
		}
	})
	return b
}

// choiceAvatar is an account's picture, or, for one without, a circle with
// the letters of where it is if letters, as in a menu with no other place
// to say it, or else a figure.
func (a *app) choiceAvatar(c *ui.Context, p palette, ch music.AccountChoice, size float32, letters bool) {
	if ch.ImageURL != "" {
		a.avatar(c, ch.ImageURL, size, p.press)
		return
	}
	ui.Box(c).Size(size, size).Radius(size / 2).Shrink(0).Background(p.press).Center().Children(func() {
		if !letters || ch.Region == "" {
			ui.Icon(c, iconUser).Size(size*0.5, size*0.5).TextColor(p.muted)
			return
		}
		ui.Text(c, strings.ToUpper(ch.Region)).FontSize(size * 0.4).FontWeight(700).TextColor(p.muted)
	})
}

// regionBadge is the storefront or region of an account, as its code, in a small outline.
func regionBadge(c *ui.Context, p palette, text string) {
	ui.Box(c).Height(16).PaddingX(5).Radius(4).Border(1, p.hairline).Center().Shrink(0).Children(func() {
		ui.Text(c, text).FontSize(10).FontWeight(600).TextColor(p.muted).SingleLine()
	})
}

// openMyPage opens the listener's own page, loaded anew each time: they
// follow and save on other pages, and it keeps up. What it had shows meanwhile.
func (a *app) openMyPage() {
	r := route{page: pageBrowse, id: a.library.account.Page}
	a.open(r)
	a.loadBrowse(r.id, true)
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
// as the source can, in Apple Music's words: Edit, for its name, and Delete
// From Library, which deletes it.
func (a *app) playlistMenu(m *ui.Menu, pl *music.Playlist) {
	if m.Item(T("Edit…")).Chosen() {
		a.startRename(pl)
	}
	m.Separator()
	if m.Item(T("Delete From Library…")).Chosen() {
		a.confirmDelete(pl)
	}
}
