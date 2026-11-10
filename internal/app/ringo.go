package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
)

// accountGroups are the account page of the settings: who's signed in, and the
// ways to sign in that the sign-in window can't do.
func (a *app) accountGroups(c *ui.Context, p palette) []settingsGroup {
	return []settingsGroup{
		{"", func(row settingsRow, block settingsBlock) {
			block(T("Apple Account")+" "+T("Sign In")+" "+T("Sign Out"), func() { a.accountCard(c, p) })
		}},
		{T("Other Ways to Sign In"), func(row settingsRow, block settingsBlock) {
			a.textSetting(c, p, block, "mediaUserToken", T("Media User Token"),
				T("For an Apple Account that signs in with a passkey or security key, which the sign-in window can't use: sign in at music.apple.com in your browser, and paste the value of its media-user-token cookie here. Empty it to sign out, or to sign in with the window again."), true)
			a.textSetting(c, p, block, "wrapperAccount", T("Wrapper Account"),
				T("To sign in as an Apple Account the wrapper holds, without the sign-in window: its Apple ID as logged in to the wrapper, or a country or region code such as jp for the wrapper's account there. When the wrapper holds several accounts, each listener names their own. Empty to sign in here. It uses the wrapper set under Playback."), false)
		}},
	}
}

// accountCard is who is signed in, with the way out of it or into it.
func (a *app) accountCard(c *ui.Context, p palette) {
	lib := a.library
	if lib.account == nil && lib.accountUnknown() {
		a.loadAccount(lib)
	}
	acc := lib.account
	name, status := T("Apple Account"), T("Sign in to play music and see your library")
	if acc != nil {
		if acc.Name != "" {
			name = acc.Name
		}
		status = T("Signed in to Apple Music")
		if strings.TrimSpace(lib.src.Setting("wrapperAccount")) != "" {
			status = T("Signed in through the wrapper")
		}
	}
	// What Apple Music said it wants, over the rest: it's what to fix here.
	if lib.needsSetup && lib.setupReason != "" {
		status = lib.setupReason
	}
	ui.Row(c.Key("account-card")).Padding(14, 16).Gap(12).AlignItems(ui.Center).MinHeight(68).Children(func() {
		if acc != nil {
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
		case acc != nil && lib.can("signOut"):
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
