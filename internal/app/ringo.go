package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// accountGroups are the account page of the settings: who's signed in, and
// the accounts to switch to, those signed in to here and those the wrapper
// lends alike, with the ways to add one.
func (a *app) accountGroups(c *ui.Context, p palette) []settingsGroup {
	groups := []settingsGroup{
		{"", func(row settingsRow, block settingsBlock) {
			block(T("Apple Account")+" "+T("Sign In")+" "+T("Sign Out"), func() { a.accountCard(c, p) })
		}},
	}
	lib := a.library
	if lib.switcher() == nil {
		return groups
	}
	return append(groups, settingsGroup{T("Accounts"), func(row settingsRow, block settingsBlock) {
		if lib.choices == nil && lib.choicesBusy {
			block(T("Loading…"), func() { a.accountNote(c, p, T("Loading…"), false) })
		}
		for _, ch := range lib.choices {
			text := ch.Name + " " + ch.Region
			if ch.Shared {
				text += " " + T("Wrapper")
			}
			block(text, func() { a.choiceRow(c, p, lib, ch) })
		}
		// The wrapper's accounts are left out while it doesn't answer, said
		// quietly, as the rest are there all the same.
		if lib.choicesErr != "" && strings.TrimSpace(lib.src.Setting("wrapper")) != "" {
			text := T("The wrapper can't be reached, so its accounts aren't listed.")
			block(text+" "+T("Try Again"), func() { a.accountNote(c, p, text, true) })
		}
		if lib.switchErr != "" {
			block(lib.switchErr, func() {
				ui.Text(c, lib.switchErr).FontSize(12).TextColor(p.accent).LineHeight(1.4).Padding(10, 16)
			})
		}
		block(T("Add Account…")+" "+T("Sign In on Apple's Page")+" "+T("Use a Media User Token")+" "+T("Media User Token"), func() {
			a.addAccountRows(c, p, lib)
		})
	}})
}

// accountNote is a line of the account list that says what it waits on, or
// what kept some of it out, with a way to ask again.
func (a *app) accountNote(c *ui.Context, p palette, text string, retry bool) {
	ui.Row(c.Key("account-note")).Padding(10, 16).Gap(12).AlignItems(ui.Center).MinHeight(40).Children(func() {
		ui.Text(c, text).FontSize(12).TextColor(p.muted).LineHeight(1.4).Grow(1).Shrink(1).MinWidth(0)
		if retry && ui.Button(c.Key("retry"), T("Try Again")).Shrink(0).Disabled(a.library.choicesBusy).Clicked() {
			a.loadAccounts(a.library, true)
		}
	})
}

// choiceRow is an account of the list: its picture, name and where it is,
// whether the wrapper lends it, and a check on the current one. Choosing it
// switches to it. One signed in to here, other than the current one, can be
// removed from its menu, out of the way of a click on the row.
func (a *app) choiceRow(c *ui.Context, p palette, lib *library, ch music.AccountChoice) {
	ui.Row(c.Key("choice:" + ch.ID)).AlignItems(ui.Center).PaddingEnd(12).Children(func() {
		b := ui.ButtonBase(c.Key("use")).Grow(1).MinWidth(0).MinHeight(52).Padding(10, 8, 10, 16).Gap(12).
			AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(ch.Name)
		b.Children(func() {
			a.choiceAvatar(c, p, ch, 30, false)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, ch.Name).FontSize(13.5).FontWeight(500).SingleLine()
				if ch.Region != "" || ch.Shared {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						if ch.Region != "" {
							regionBadge(c, p, strings.ToUpper(ch.Region))
						}
						if ch.Shared {
							regionBadge(c, p, T("Wrapper"))
						}
					})
				}
			})
			switch {
			case lib.switching == ch.ID:
				ui.Text(c, T("Switching…")).FontSize(12).TextColor(p.muted).Shrink(0)
			case ch.Current:
				ui.Icon(c, iconCheck).Size(15, 15).TextColor(p.accent).Shrink(0)
			}
		})
		if b.Clicked() && !ch.Current {
			a.useAccount(lib, ch.ID)
		}
		if ch.Shared || ch.Current {
			ui.Box(c).Size(28, 28).Shrink(0)
			return
		}
		m := ui.ButtonBase(c.Key("more")).Size(28, 28).Shrink(0).Radius(14).Center().FocusRing(false).Label(T("More")).TextColor(p.muted)
		if m.Hovered() {
			m.Background(p.hover)
		}
		m.Children(func() { ui.Icon(c, iconMore).Size(14, 14) })
		m.Menu(func(menu *ui.Menu) {
			if menu.Item(T("Remove Account")).Chosen() {
				a.removeAccount(lib, ch.ID)
			}
		})
	})
}

// addAccountRows is Add Account…, which opens the two ways to add one:
// signing in on Apple's page, or pasting a media-user-token, which an
// account with a passkey needs. Either way it's one more account of the list.
func (a *app) addAccountRows(c *ui.Context, p palette, lib *library) {
	add := &a.adding
	ui.Column(c.Key("add-account")).Children(func() {
		b := ui.ButtonBase(c.Key("add")).MinHeight(48).Padding(10, 16).Gap(12).AlignItems(ui.Center).Justify(ui.Start).
			FocusRing(false).Label(T("Add Account…")).Expanded(add.open)
		b.Children(func() {
			ui.Box(c).Size(30, 30).Radius(15).Shrink(0).Background(p.press).Center().Children(func() {
				ui.Icon(c, iconPlus).Size(15, 15).TextColor(p.accent)
			})
			ui.Text(c, T("Add Account…")).FontSize(13.5).FontWeight(500).TextColor(p.accent)
		})
		if b.Clicked() {
			*add = addingAccount{open: !add.open, text: add.text}
		}
		if !add.open {
			return
		}
		ui.Column(c).Padding(0, 16, 14, 58).Gap(10).AlignItems(ui.Start).Children(func() {
			if ui.Button(c.Key("web"), T("Sign In on Apple's Page")).Clicked() {
				add.token = false
				a.promptSignInInSettings(lib)
			}
			if ui.Button(c.Key("token"), T("Use a Media User Token")).Clicked() {
				add.token = true
			}
			if !add.token {
				return
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).AlignSelf(ui.Stretch).Children(func() {
				in := ui.TextInput(c.Key("token-value"), &add.text).Label(T("Media User Token")).Placeholder(T("Media User Token")).
					Password().FontSize(13).Grow(1).MinWidth(0)
				ready := strings.TrimSpace(add.text) != "" && !add.busy
				added := ui.PrimaryButton(c.Key("add-token"), T("Add")).Shrink(0).Disabled(!ready).Clicked()
				if ready && (added || in.Submitted()) {
					a.addAccount(lib, add.text)
				}
			})
			ui.Text(c, T("For an Apple Account that signs in with a passkey or security key, which the sign-in page can't use: sign in at music.apple.com in your browser, and paste the value of its media-user-token cookie here.")).
				FontSize(12).TextColor(p.muted).LineHeight(1.4)
			switch {
			case add.busy:
				ui.Text(c, T("Signing in…")).FontSize(12).TextColor(p.muted)
			case add.err != "":
				ui.Text(c, add.err).FontSize(12).TextColor(p.accent).LineHeight(1.4)
			}
		})
	})
}

// accountCard is who is signed in, with the way out of it or into it. An
// account the wrapper lends has no way out but switching to another.
func (a *app) accountCard(c *ui.Context, p palette) {
	lib := a.library
	if lib.account == nil && lib.accountUnknown() {
		a.loadAccount(lib)
	}
	a.loadAccounts(lib, false)
	acc := lib.account
	name, status := T("Apple Account"), T("Sign in to play music and see your library")
	if acc != nil {
		if acc.Name != "" {
			name = acc.Name
		}
		status = T("Signed in to Apple Music")
		if acc.Shared {
			status = T("Signed in through the wrapper")
		}
	}
	// What Apple Music said it wants, over the rest: it's what to fix here.
	if lib.needsSetup && lib.setupReason != "" {
		status = lib.setupReason
	}
	ui.Row(c.Key("account-card")).Padding(14, 16).Gap(12).AlignItems(ui.Center).MinHeight(68).Children(func() {
		if acc != nil && acc.ImageURL != "" {
			a.avatar(c, acc.ImageURL, 40, p.press)
		} else {
			ui.Box(c).Size(40, 40).Radius(20).Shrink(0).Background(p.press).Center().Children(func() {
				ui.Icon(c, iconUser).Size(20, 20).TextColor(p.muted)
			})
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, name).FontSize(14).FontWeight(600)
			ui.Text(c, status).FontSize(12).TextColor(p.muted).LineHeight(1.4)
		})
		switch {
		case acc != nil && !acc.Shared && lib.can("signOut"):
			if ui.Button(c.Key("sign-out"), T("Sign Out")).Shrink(0).Clicked() {
				a.signOut(lib)
			}
		case acc == nil && lib.can("signIn"):
			if ui.PrimaryButton(c.Key("sign-in"), T("Sign In")).Shrink(0).Clicked() {
				a.promptSignInInSettings(lib)
			}
		}
	})
}

// textSetting is a setting the listener types, in a row of its own: its name,
// the field the width of the card, and what it's for. A token or a key is
// hidden as it's typed. It's kept as it's submitted, or as the focus leaves it.
func (a *app) textSetting(c *ui.Context, p palette, block settingsBlock, key, label, desc string, secret bool) {
	block(label+" "+desc, func() {
		lib := a.library
		text := a.settingText[key]
		if text == nil {
			v := lib.src.Setting(key)
			text = &v
			a.settingText[key] = text
		}
		ui.Column(c.Key(key)).Padding(12, 16).Gap(6).Children(func() {
			ui.Text(c, label).FontSize(13.5).FontWeight(500)
			in := ui.TextInput(c.Key("value"), text).Label(label).FontSize(13)
			if secret {
				in.Password()
			}
			if in.Submitted() || !in.Focused() && *text != lib.src.Setting(key) {
				a.keepSetting(lib, key, *text)
			}
			ui.Text(c, desc).FontSize(12).TextColor(p.muted).LineHeight(1.4)
		})
	})
}

// keepSetting gives a source a setting, off the main thread, and loads anew
// what it may now be able to show.
func (a *app) keepSetting(lib *library, key, value string) {
	if lib.src.Setting(key) == value {
		return
	}
	if key == historySetting && strings.EqualFold(strings.TrimSpace(value), "off") {
		// What plays is told to be over while the source still hears of it.
		a.pl.StopTelling()
	}
	go func() {
		err := lib.src.SetSetting(key, value)
		a.post(func() {
			if err != nil {
				a.toast = Tf("Couldn't keep the setting: %s", err.Error())
				return
			}
			lib.needsSetup, lib.askedSetup, lib.setupReason = false, false, ""
			a.libraryChanged(lib)
			a.loadAccount(lib)
		})
	}()
}

// historySetting is the source's setting that keeps what's played out of the
// account's listening history when it says off.
const historySetting = "listeningHistory"

// iconRingo is the app's mark on the icons' grid: an apple cut across, whose
// core shows as a star of five seeds' chambers.
var iconRingo = strokeIcon(ringoMark(12, 12, 9.2, 1.9, 6.3, 1.9))

// ringoMark draws the mark around (cx, cy): the apple's round edge at r, and
// five chambers from in to out along their spokes, each as wide as twice
// half at its middle, the first pointing up.
func ringoMark(cx, cy, r, in, out, half float64) string {
	n := func(v float64) string {
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s"/>`, n(cx), n(cy), n(r))
	for k := range 5 {
		t := (-90 + 72*float64(k)) * math.Pi / 180
		ux, uy := math.Cos(t), math.Sin(t)
		mid := (in + out) / 2
		// Each side bows out through a control point beside the spoke's middle.
		fmt.Fprintf(&b, `<path d="M%s %sQ%s %s %s %sQ%s %s %s %sZ"/>`,
			n(cx+ux*in), n(cy+uy*in),
			n(cx+ux*mid-uy*half*2), n(cy+uy*mid+ux*half*2), n(cx+ux*out), n(cy+uy*out),
			n(cx+ux*mid+uy*half*2), n(cy+uy*mid-ux*half*2), n(cx+ux*in), n(cy+uy*in))
	}
	return b.String()
}
