package app

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/i18n"
	"ringo/internal/music"
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
	if want := (route{page: pageBrowse, id: "user:1"}); a.route != want || a.accountOpen {
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

// Sign in to X in the account row opens the dialog, whose Sign In calls the source,
// and the account shows; Sign Out goes back.
func TestSignInAndOut(t *testing.T) {
	asked := make(chan struct{}, 1)
	signedOut := make(chan struct{}, 1)
	a := newTestApp()
	ch := posts(a)
	f := fake(a)
	f.signIn = func() (*music.Account, error) {
		asked <- struct{}{}
		return &music.Account{ID: "1", Name: "小明"}, nil
	}
	f.signOut = func() error { signedOut <- struct{}{}; return nil }
	a.homeLoading = true // for the dialog's Sign In to be the one that shows
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Sign in to Apple Music"); err != nil {
		t.Fatal(err)
	}
	settled(tt)
	if !a.signIn.open {
		t.Fatal("no dialog")
	}
	if err := tt.Click("Sign In"); err != nil {
		t.Fatal(err)
	}
	wait(t, asked, "signing in")
	until(t, ch, func() bool { return a.account != nil })
	if a.signIn.open || a.signIn.waiting {
		t.Errorf("the dialog stayed: %+v", a.signIn)
	}
	settled(tt)
	if _, ok := tt.Find("小明"); !ok {
		t.Fatalf("no account in %q", tt.Texts())
	}
	if _, ok := tt.Find("Sign in to Apple Music"); ok {
		t.Error("still offers to sign in")
	}

	if err := tt.Click("小明"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Sign Out"); err != nil {
		t.Fatal(err)
	}
	wait(t, signedOut, "signing out")
	settled(tt)
	if a.account != nil {
		t.Error("still signed in")
	}
	if _, ok := tt.Find("Sign in to Apple Music"); !ok {
		t.Errorf("doesn't offer to sign in again: %q", tt.Texts())
	}
}

// Sign In in the settings asks in the settings window. It used to open the
// dialog in the main window, behind the settings, where it went unseen.
func TestSignInFromSettingsAsksThere(t *testing.T) {
	a := newTestApp()
	a.settingsPage = "account"
	main := ui.NewTester(a.view, 1200, 800)
	settings := ui.NewTester(a.settingsView, 780, 1000)
	main.Frame()
	settings.Frame()
	if err := settings.Click("Sign In"); err != nil {
		t.Fatal(err)
	}
	settled(settings)
	settled(main)
	if _, ok := settings.Find("Cancel"); !ok {
		t.Errorf("no dialog in the settings: %q", settings.Texts())
	}
	if _, ok := main.Find("Cancel"); ok {
		t.Error("the dialog opened in the main window")
	}
}

// A sign-in that fails says why in its dialog, ready to try again.
func TestSignInFails(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	fake(a).signIn = func() (*music.Account, error) { return nil, errors.New("the code expired") }
	a.promptSignIn(a.library)
	a.startSignIn()
	if !a.signIn.waiting {
		t.Fatal("not waiting")
	}
	runPost(t, ch)
	if !a.signIn.open || a.signIn.waiting || a.signIn.err != "the code expired" || a.account != nil {
		t.Errorf("the dialog is %+v, account %+v", a.signIn, a.account)
	}
}

// Trying again after a sign-in failed at once leaves the dialog as it was: it
// showed its wait for a frame, which takes a line where the error took two, and
// the dialog sprang after it and back, its text moving by fractions of a pixel
// for half a second. One that takes a while says it's waiting.
func TestSignInFailingAtOnceStaysPut(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	msg := "Paste your session's cookie in Settings > Plugins > Example, which says how to find it"
	slow := make(chan struct{})
	var hold chan struct{}
	fake(a).signIn = func() (*music.Account, error) {
		if hold != nil {
			<-hold
		}
		return nil, errors.New(msg)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.signInDialog(c, a.pal, false) }, 800, 600)
	a.promptSignIn(a.library)
	settled(tt)
	if err := tt.Click(T("Sign In")); err != nil {
		t.Fatal(err)
	}
	runPost(t, ch)
	settled(tt)
	before, ok := tt.Find(msg)
	if !ok {
		t.Fatalf("doesn't say why: %q", tt.Texts())
	}

	if err := tt.Click(T("Sign In")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	runPost(t, ch)
	for range 30 {
		if r, ok := tt.Find(msg); !ok || r != before {
			t.Fatalf("the error went from %v to %v (shown %v)", before, r, ok)
		}
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}

	hold = slow
	if err := tt.Click(T("Sign In")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(signInSettle)
	tt.Frame()
	if _, ok := tt.Find(T("Waiting for you to finish signing in…")); !ok {
		t.Errorf("doesn't say it's waiting: %q", tt.Texts())
	}
	close(slow)
	runPost(t, ch)
}

// A sign-in that ends after the dialog was put away still counts, with what's the
// listener's loaded anew.
func TestSignInEndsAfterDialog(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	f := fake(a)
	f.signIn = func() (*music.Account, error) { return &music.Account{ID: "1", Name: "小明"}, nil }
	f.playlists = func() ([]music.Playlist, error) { return []music.Playlist{{ID: "10", Name: "通勤", Mine: true}}, nil }
	a.promptSignIn(a.library)
	a.startSignIn()
	a.signIn.open = false
	until(t, ch, func() bool { return a.account != nil && len(a.mine) > 0 })
	if a.signIn.open {
		t.Error("the dialog came back")
	}
}

// Who's signed in is asked of the source once it starts: signed in already, it's
// known without the dialog, and one who isn't asks nothing of the listener.
func TestAccountAskedOnStart(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	fake(a).account = func() (*music.Account, error) { return &music.Account{ID: "1", Name: "小明"}, nil }
	a.library.started = false
	a.startLibrary(a.library)
	until(t, ch, func() bool { return a.account != nil })
	if a.signIn.open {
		t.Error("asked the listener to sign in")
	}
}

// Who's signed in, said for the first time, changes nothing listed as the source
// started: Home loaded twice at each launch, its picks changing under the listener.
func TestFirstAccountLoadsNothingAgain(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	f := fake(a)
	f.account = func() (*music.Account, error) { return &music.Account{ID: "1", Name: "小明"}, nil }
	asked, answer := make(chan struct{}, 4), make(chan struct{})
	f.home = func() (*music.Browse, error) {
		asked <- struct{}{}
		<-answer
		return &music.Browse{Sections: []music.Section{{Title: "Picks"}}}, nil
	}
	a.library.started, a.library.accountAsked = false, false
	a.startLibrary(a.library)
	wait(t, asked, "Home")
	until(t, ch, func() bool { return a.account != nil })
	close(answer)
	until(t, ch, func() bool { return a.home != nil })
	select {
	case <-asked:
		t.Error("Home was asked for again")
	case <-time.After(50 * time.Millisecond):
	}
}

// What the source refused as it started, not knowing the listener yet, it's asked
// again once it says who they are.
func TestFirstAccountLoadsAgainWhatFailed(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	f := fake(a)
	answer := make(chan struct{})
	f.account = func() (*music.Account, error) {
		<-answer
		return &music.Account{ID: "1", Name: "小明"}, nil
	}
	f.home = func() (*music.Browse, error) { return nil, errors.New("not yet") }
	a.library.started, a.library.accountAsked = false, false
	a.startLibrary(a.library)
	until(t, ch, func() bool { return a.homeErr != "" })
	f.home = func() (*music.Browse, error) {
		return &music.Browse{Sections: []music.Section{{Title: "Picks"}}}, nil
	}
	close(answer)
	until(t, ch, func() bool { return a.home != nil })
}

// A heart asked about before the source said who's signed in is asked about again
// after: its answer is dropped, which left it empty for good.
func TestLikeAskedBeforeAccountAskedAgain(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	f := fake(a)
	accountGo, likedGo := make(chan struct{}), make(chan struct{})
	f.account = func() (*music.Account, error) {
		<-accountGo
		return &music.Account{ID: "1", Name: "小明"}, nil
	}
	f.isLiked = func(ids []string) (map[string]bool, error) {
		<-likedGo
		return map[string]bool{"1": true}, nil
	}
	a.library.started, a.library.accountAsked = false, false
	a.startLibrary(a.library)
	tr := song("1", "Song")
	a.wantLike(&tr)
	a.askLikes()
	close(accountGo)
	until(t, ch, func() bool { return a.account != nil })
	close(likedGo)
	a.wantLike(&tr)
	a.askLikes()
	until(t, ch, func() bool { return a.isLiked(&tr) })
}

// Started signed out, the app asks for no playlists and opens no sign-in
// dialog by itself: it used to ask for them as it started, and open the
// dialog when told to sign in. Signed in, it asks for them.
func TestSignedOutStartAsksNothing(t *testing.T) {
	asked := make(chan struct{}, 4)
	a := newTestApp()
	ch := posts(a)
	lib := a.library
	lib.started, lib.accountAsked = false, false
	fake(a).playlists = func() ([]music.Playlist, error) {
		asked <- struct{}{}
		return nil, music.ErrSignIn
	}
	tt := ui.NewTester(a.view, 1200, 800)
	a.startLibrary(lib)
	until(t, ch, func() bool { return lib.accountAsked && !lib.homeLoading })
	settled(tt)
	if a.signIn.open || len(asked) > 0 {
		t.Fatalf("signed out: dialog open %v, playlists asked %d times", a.signIn.open, len(asked))
	}

	b := newTestApp()
	ch = posts(b)
	b.library.started, b.library.accountAsked = false, false
	fake(b).account = func() (*music.Account, error) { return &music.Account{ID: "1", Name: "Alex"}, nil }
	fake(b).playlists = func() ([]music.Playlist, error) {
		asked <- struct{}{}
		return nil, nil
	}
	b.startLibrary(b.library)
	until(t, ch, func() bool { return len(asked) > 0 })
}

// The account's row says nothing of who is signed in until the source has
// said: it said "sign in" to someone signed in, moving what's around it.
func TestAccountRowWaitsForTheAccount(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	answer := make(chan struct{})
	fake(a).account = func() (*music.Account, error) {
		<-answer
		return &music.Account{ID: "1", Name: "小明"}, nil
	}
	a.started, a.accountAsked = false, false
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	check := func() {
		t.Helper()
		if tt.HasText("Sign in to Apple Music") {
			t.Error("the account's row says what isn't known yet")
		}
	}
	check()
	a.startLibrary(a.library)
	tt.Frame()
	check()
	close(answer)
	until(t, ch, func() bool { return a.account != nil })
	tt.Frame()
	if !tt.HasText("小明") {
		t.Error("who's signed in doesn't show")
	}
}
