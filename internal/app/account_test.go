package app

import (
	"fmt"
	"image"
	"image/draw"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Clicking the account row drops its menu, which takes the focus so the
// arrows walk its items, and Escape or a second click puts it away.
func TestAccountMenu(t *testing.T) {
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "小明"}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()

	if err := tt.Click("小明"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.accountOpen || !tt.HasText("Sign Out") {
		t.Fatalf("menu not open: %v", a.accountOpen)
	}
	if !tt.Focused("Settings…") {
		t.Fatal("the first item has no focus")
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if !tt.Focused("Sign Out") {
		t.Fatal("Down did not move to signing out")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.accountOpen {
		t.Fatal("Escape left the menu open")
	}

	tt.Click("小明")
	tt.Frame()
	tt.Click("小明")
	tt.Frame()
	if a.accountOpen {
		t.Fatal("a second click left the menu open")
	}
}

// My Page in the account menu opens the listener's own page at the source,
// and loads it anew each time, as what they follow changes on other pages.
func TestAccountMyPage(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	a.account = &music.Account{ID: "1", Name: "小明", Page: "user:1"}
	asked := make(chan string, 4)
	fake(a).browse = func(page string) (*music.Browse, error) {
		asked <- page
		return &music.Browse{Title: "小明", ImageURL: "https://example.com/a.jpg", Description: "3 关注 · 5 粉丝\n你好",
			Sections: []music.Section{{Title: "创建的歌单", Empty: "暂无公开歌单"}}}, nil
	}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()

	tt.Click("小明")
	tt.Frame()
	if !tt.Focused("Profile") {
		t.Fatal("My Page isn't first in the menu")
	}
	if err := tt.Click("Profile"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if want := (route{page: pageBrowse, id: "user:1", src: "fake"}); a.route != want || a.accountOpen {
		t.Fatalf("route %+v, menu open %v", a.route, a.accountOpen)
	}
	if got := wait(t, asked, "the page"); got != "user:1" {
		t.Fatalf("asked for %q", got)
	}
	runPost(t, ch)
	tt.Frame()

	a.open(route{page: pageHome})
	tt.Frame()
	tt.Click("小明")
	tt.Frame()
	tt.Click("Profile")
	wait(t, asked, "the page again")
}

// With MM_FRAMES set, saves the foot of the sidebar with the account menu
// open, light and dark.
func TestAccountMenuFrames(t *testing.T) {
	dir := os.Getenv("MM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.account = &music.Account{ID: "1", Name: "小明"}
		tt := ui.NewTester(a.view, 1000, 600)
		tt.SetDark(dark)
		tt.Frame()
		tt.Click("小明")
		tt.Advance(time.Second)
		tt.Frame()
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 1000
		crop := image.Rect(0, int(400*s), int((sidebarWidth+20)*s), img.Bounds().Dy())
		out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
		draw.Draw(out, out.Bounds(), img, crop.Min, draw.Src)
		name := "account-light.png"
		if dark {
			name = "account-dark.png"
		}
		saveFrame(t, dir, name, out)
	}
}

// withAccounts gives the app of newTestApp accounts to switch to: Alex and
// Sam, signed in to here, and two the wrapper lends, of which current is
// the one signed in with. Made up: none is anyone's.
func withAccounts(a *app, current string) []music.AccountChoice {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	list := []music.AccountChoice{
		{ID: "own:1", Name: "Alex", ImageURL: "avatar:alex", Region: "us", Used: at.Add(3 * time.Hour)},
		{ID: "own:2", Name: "Sam", Region: "jp", Used: at.Add(2 * time.Hour)},
		{ID: "wrapper:a", Name: "jamie@example.com", Region: "cn", Shared: true, Used: at.Add(time.Hour)},
		{ID: "wrapper:b", Name: "Account in GB", Region: "gb", Shared: true},
	}
	for i := range list {
		if list[i].ID == current {
			list[i].Current = true
			a.account = &music.Account{ID: current, Name: list[i].Name, ImageURL: list[i].ImageURL, Shared: list[i].Shared}
		}
	}
	img := coverImage(ui.RGB(240, 150, 90), ui.RGB(170, 60, 120))
	for _, px := range []int{54, 78, 90, 120} {
		a.art.art[sized("avatar:alex", px)] = &artwork{bmp: ui.NewBitmap(img)}
	}
	a.library.choices, a.library.choicesAt = list, time.Now()
	fake(a).accounts = func(bool) ([]music.AccountChoice, error) { return list, nil }
	return list
}

// Choosing another account in the account menu switches to it and closes the
// menu, loads what's personal anew once it's there, and leaves what plays
// playing.
func TestSwitchFromAccountMenu(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	withAccounts(a, "own:1")
	a.pl.Cue([]music.Track{song("1", "One"), song("2", "Two")})
	used := make(chan string, 1)
	homes := make(chan struct{}, 4)
	f := fake(a)
	f.useAccount = func(id string) (*music.Account, error) {
		used <- id
		return &music.Account{ID: id, Name: "Sam"}, nil
	}
	f.home = func() (*music.Browse, error) {
		homes <- struct{}{}
		return &music.Browse{}, nil
	}
	tt := ui.NewTester(a.view, 1000, 700)
	tt.Frame()
	tt.Click("Alex")
	tt.Frame()
	if err := tt.Click("Sam"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	if got := wait(t, used, "the switch"); got != "own:2" || a.accountOpen {
		t.Fatalf("switched to %q, menu open %v", got, a.accountOpen)
	}
	until(t, ch, func() bool { return a.account.ID == "own:2" })
	wait(t, homes, "Home anew")
	if cur := a.pl.Current(); cur == nil || cur.ID != "1" || len(a.pl.Queue()) != 2 {
		t.Errorf("the queue went: %+v", a.pl.Queue())
	}
}

// An account the wrapper lends has no Sign Out, in the menu or the
// settings; one signed in to here has.
func TestWrapperAccountHasNoSignOut(t *testing.T) {
	for current, want := range map[string]bool{"wrapper:a": false, "own:1": true} {
		a := newTestApp()
		withAccounts(a, current)
		tt := ui.NewTester(a.view, 1000, 700)
		tt.Frame()
		tt.Click(a.account.Name)
		tt.Frame()
		if _, ok := tt.Find("Sign Out"); ok != want {
			t.Errorf("%s: Sign Out in the menu %v", current, ok)
		}
		a.settingsPage = "account"
		settings := ui.NewTester(a.settingsView, 780, 900)
		settled(settings)
		if _, ok := settings.Find("Sign Out"); ok != want {
			t.Errorf("%s: Sign Out in the settings %v", current, ok)
		}
	}
}

// A row of the settings' list switches to its account; a switch that fails
// says why there, and whoever was signed in stays.
func TestSwitchInSettings(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	withAccounts(a, "own:1")
	used := make(chan string, 1)
	fake(a).useAccount = func(id string) (*music.Account, error) {
		used <- id
		return nil, music.Fail(music.ErrSetup, "The wrapper is asleep")
	}
	a.settingsPage = "account"
	tt := ui.NewTester(a.settingsView, 780, 900)
	settled(tt)
	if err := tt.Click("jamie@example.com"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, used, "the switch"); got != "wrapper:a" {
		t.Fatalf("switched to %q", got)
	}
	runPost(t, ch)
	settled(tt)
	if _, ok := tt.Find("The wrapper is asleep"); !ok || a.account.ID != "own:1" {
		t.Errorf("account %q, says %q", a.account.ID, tt.Texts())
	}
}

// Add Account… with a Media User Token adds it with what was pasted, and
// switches to it.
func TestAddAccountWithToken(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	withAccounts(a, "own:1")
	added := make(chan string, 1)
	fake(a).addAccount = func(token string) (*music.Account, error) {
		added <- token
		return &music.Account{ID: "own:3", Name: "Robin"}, nil
	}
	a.settingsPage = "account"
	tt := ui.NewTester(a.settingsView, 780, 1000)
	settled(tt)
	tt.Click("Add Account…")
	settled(tt)
	if err := tt.Click("Use a Media User Token"); err != nil {
		t.Fatalf("%v: %q", err, tt.Texts())
	}
	settled(tt)
	if err := tt.Click("Media User Token"); err != nil {
		t.Fatal(err)
	}
	tt.Type("not-a-real-token")
	tt.Key(0, ui.KeyEnter)
	if got := wait(t, added, "the adding"); got != "not-a-real-token" {
		t.Fatalf("added %q", got)
	}
	until(t, ch, func() bool { return a.account.ID == "own:3" })
	if a.adding.open || a.adding.text != "" {
		t.Errorf("still adding: %+v", a.adding)
	}
}

// An account signed in to here, not the current one, is removed from its
// row's menu; the current one and the wrapper's have none.
func TestRemoveAccount(t *testing.T) {
	a := newTestApp()
	withAccounts(a, "own:1")
	removed := make(chan string, 1)
	fake(a).removeAccount = func(id string) error { removed <- id; return nil }
	a.settingsPage = "account"
	tt := ui.NewTester(a.settingsView, 780, 900)
	settled(tt)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Remove Account"); err != nil {
		t.Fatalf("%v: %q", err, tt.Menu())
	}
	if got := wait(t, removed, "the removing"); got != "own:2" {
		t.Fatalf("removed %q", got)
	}
}

// Of many accounts the menu lists the six used last, the current one
// always, in the list's order, and says there are more; of one, none.
func TestMenuChoices(t *testing.T) {
	var all []music.AccountChoice
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := range 8 {
		all = append(all, music.AccountChoice{ID: fmt.Sprint(i), Used: at.Add(time.Duration(i) * time.Hour)})
	}
	all[0].Current = true
	shown, more := menuChoices(all)
	var ids []string
	for _, c := range shown {
		ids = append(ids, c.ID)
	}
	if want := []string{"0", "3", "4", "5", "6", "7"}; !slices.Equal(ids, want) || !more {
		t.Errorf("shown %q, more %v", ids, more)
	}
	if shown, _ := menuChoices(all[:1]); shown != nil {
		t.Errorf("one alone: %+v", shown)
	}
}

// With MM_FRAMES set, saves the settings' account page with two accounts
// signed in to here and two the wrapper lends: one of them current, then a
// wrapper's; at the narrowest; in Chinese; with Add Account… open; and
// without the wrapper answering. And the account menu, light and dark.
func TestAccountsFrames(t *testing.T) {
	dir := framesDir(t)
	settingsShot := func(name string, w, h int, dark bool, set func(a *app)) {
		a := newTestApp()
		fake(a).info.Name = "Apple Music"
		fake(a).vals["wrapper"] = "http://wrapper.example"
		withAccounts(a, "own:1")
		if set != nil {
			set(a)
		}
		a.settingsPage = "account"
		tt := ui.NewTester(a.settingsView, w, h)
		tt.SetDark(dark)
		settled(tt)
		saveFrame(t, dir, name, tt.Image())
	}
	settingsShot("accounts-dark.png", 780, 640, true, nil)
	settingsShot("accounts-light.png", 780, 640, false, nil)
	settingsShot("accounts-min-dark.png", 640, 420, true, nil)
	settingsShot("accounts-wrapper-current-dark.png", 780, 640, true, func(a *app) { withAccounts(a, "wrapper:a") })
	settingsShot("accounts-add-dark.png", 780, 640, true, func(a *app) {
		a.adding = addingAccount{open: true, token: true, text: "not-a-real-token"}
	})
	settingsShot("accounts-no-wrapper-dark.png", 780, 640, true, func(a *app) {
		a.library.choices = a.library.choices[:2]
		a.library.choicesErr = "amhelper: wrapper /status: connection refused"
	})
	func() {
		defer i18n.SetLanguage(i18n.English)
		i18n.SetLanguage(i18n.Chinese)
		settingsShot("accounts-zh-dark.png", 780, 640, true, func(a *app) {
			a.library.choices[3].Name = i18n.Tf("Account in %s", "GB")
		})
	}()

	for _, dark := range []bool{false, true} {
		a := newTestApp()
		withAccounts(a, "own:1")
		tt := ui.NewTester(a.view, 1000, 600)
		tt.SetDark(dark)
		tt.Frame()
		tt.Click("Alex")
		tt.Advance(time.Second)
		tt.Frame()
		img := tt.Image()
		s := float32(img.Bounds().Dx()) / 1000
		crop := image.Rect(0, int(260*s), int((sidebarWidth+20)*s), img.Bounds().Dy())
		out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
		draw.Draw(out, out.Bounds(), img, crop.Min, draw.Src)
		name := "account-menu-light.png"
		if dark {
			name = "account-menu-dark.png"
		}
		saveFrame(t, dir, name, out)
	}
}
