package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
)

// appleMusicSettings is the settings' page of the source: who's signed in,
// and what it asks of the listener.
func (a *app) appleMusicSettings(c *ui.Context, row settingsRow, block settingsBlock, p palette) {
	lib := a.library
	info := lib.src.Info()

	who := T("Not signed in")
	switch {
	case lib.needsSetup:
		who = setupNotice(lib, info)
	case lib.account != nil:
		who = Tf("Signed in as %s", lib.account.Name)
	}
	row(T("Apple Account"), who, func() {
		switch {
		case lib.account == nil && lib.accountUnknown():
			a.loadAccount(lib)
		case lib.account != nil && lib.can("signOut"):
			if ui.Button(c.Key("sign-out"), T("Sign Out")).Clicked() {
				a.signOut(lib)
			}
		case lib.account == nil && lib.can("signIn"):
			if ui.PrimaryButton(c.Key("sign-in"), T("Sign In")).Clicked() {
				a.promptSignInInSettings(lib)
			}
		}
	})

	for _, st := range info.Settings {
		if st.Key == historySetting {
			// The source reads "off" for it: here it's a switch.
			row(T("Use Listening History"), T("Songs you play here show in Recently Played and count toward Replay and recommendations, as in Apple's own apps"), func() {
				on := !strings.EqualFold(strings.TrimSpace(lib.src.Setting(historySetting)), "off")
				if ui.Switch(c.Key("history"), &on).Label(T("Use Listening History")).Changed() {
					value := ""
					if !on {
						value = "off"
					}
					a.keepSetting(lib, historySetting, value)
				}
			})
			continue
		}
		block(st.Label+" "+st.Help, func() {
			ui.Column(c.Key(st.Key)).Padding(12, 16).Children(func() { a.sourceSetting(c, lib, st, p) })
		})
	}
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
