package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"ringo/internal/i18n"
	"ringo/internal/music"
)

// Changing the language says the app's words in it from the next frame on, and
// English again once chosen.
func TestSetLanguage(t *testing.T) {
	defer i18n.SetLanguage(i18n.English)
	a := newTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	onMainThread(func() { a.setLanguage(i18n.Chinese) })
	if i18n.Language() != i18n.Chinese || a.settings.Language != i18n.Chinese {
		t.Fatalf("language %q, kept %q", i18n.Language(), a.settings.Language)
	}
	tt.Frame()
	if _, ok := tt.Find(T("Home")); !ok {
		t.Errorf("no %q in %q", T("Home"), tt.Texts())
	}

	onMainThread(func() { a.setLanguage(i18n.English) })
	tt.Frame()
	if _, ok := tt.Find("Home"); !ok {
		t.Errorf("English didn't come back: %q", tt.Texts())
	}
}

// Going back to following the system speaks its language again.
func TestFollowSystemLanguage(t *testing.T) {
	defer i18n.SetLanguage(i18n.English)
	was := systemLanguages
	defer func() { systemLanguages = was }()
	systemLanguages = func() []string { return []string{"zh-Hans-CN"} }
	a := newTestApp()
	onMainThread(func() { a.setLanguage(i18n.English) })
	if i18n.Language() != i18n.English {
		t.Fatalf("speaks %q", i18n.Language())
	}
	onMainThread(func() { a.setLanguage(i18n.System) })
	if i18n.Language() != i18n.Chinese || a.settings.Language != i18n.System {
		t.Fatalf("speaks %q, kept %q", i18n.Language(), a.settings.Language)
	}
}

// The settings' sidebar opens its pages, one at a time.
func TestSettingsSidebarOpensPages(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.settingsView, 780, 800)
	tt.Frame()
	if _, ok := tt.Find("Language"); !ok {
		t.Fatalf("General isn't the first page: %q", tt.Texts())
	}
	if err := tt.Click("Playback"); err != nil {
		t.Fatal(err)
	}
	settled(tt)
	if _, ok := tt.Find("Streaming"); !ok {
		t.Errorf("no Streaming on Playback: %q", tt.Texts())
	}
	if _, ok := tt.Find("Language"); ok {
		t.Error("General's still showing")
	}
}

// Dolby Atmos is a setting of its own beside Streaming: choosing it for
// headphones, then High Efficiency, asks the songs played next for both,
// and turning it off again for the level alone.
func TestDolbyAtmosSettingAsksForItsVersion(t *testing.T) {
	a := newTestApp()
	a.settings.Quality = "high"
	a.settingsPage = "playback"
	tt := ui.NewTester(a.settingsView, 780, 900)
	tt.Frame()
	choose := func(shown, option string) {
		t.Helper()
		if err := tt.Click(shown); err != nil {
			t.Fatal(err)
		}
		settled(tt)
		if err := tt.Click(option); err != nil {
			t.Fatal(err)
		}
		settled(tt)
	}
	choose("Off", "Headphones")
	if got := a.settings.quality(); got != music.Ask("high", music.AtmosHeadphones) {
		t.Fatalf("asks for %q", got)
	}
	choose("High Quality", "High Efficiency")
	if got := a.settings.quality(); got != music.Ask("efficient", music.AtmosHeadphones) {
		t.Fatalf("asks for %q", got)
	}
	choose("Headphones", "Off")
	if got := a.settings.quality(); got != "efficient" {
		t.Fatalf("asks for %q", got)
	}
}

// A search finds settings across the pages, and only those; one that finds
// nothing says so; and choosing a page leaves it.
func TestSettingsSearch(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.settingsView, 780, 800)
	tt.Frame()
	if err := tt.Click("Search Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Type("QUALITY")
	settled(tt)
	if _, ok := tt.Find("Streaming"); !ok {
		t.Fatalf("didn't find it: %q", tt.Texts())
	}
	if _, ok := tt.Find("Language"); ok {
		t.Error("shows what it didn't look for")
	}
	if _, ok := tt.Find("Sound Check"); ok {
		t.Error("shows a row of the same page it didn't look for")
	}

	a.settingsQuery = "zzzz"
	settled(tt)
	if _, ok := tt.Find("No settings found"); !ok {
		t.Errorf("says nothing of finding nothing: %q", tt.Texts())
	}

	// By a page's name, all of it.
	a.settingsQuery = "playback"
	settled(tt)
	if _, ok := tt.Find("Sound Check"); !ok {
		t.Errorf("the page's rows aren't there: %q", tt.Texts())
	}

	if err := tt.Click("Advanced"); err != nil {
		t.Fatal(err)
	}
	settled(tt)
	if a.settingsQuery != "" || a.settingsPage != "advanced" {
		t.Errorf("query %q on page %q", a.settingsQuery, a.settingsPage)
	}
}

// A quality kept from when there was a Standard level asks for High Quality: a
// level the cache doesn't know would keep no song.
func TestStandardQualityAsksForHigh(t *testing.T) {
	for kept, want := range map[string]string{"standard": "high", "": "high", "high": "high", "lossless": "lossless", "hires": "hires"} {
		if got := (settings{Quality: kept}).quality(); got != want {
			t.Errorf("%q asks for %q, want %q", kept, got, want)
		}
	}
}

// A control a search found works as on its page.
func TestSettingsFoundControlWorks(t *testing.T) {
	a := newTestApp()
	a.settingsQuery = "sound check"
	tt := ui.NewTester(a.settingsView, 780, 800)
	settled(tt)
	was := a.settings.Normalize
	// The row's title is labelled as its switch is: click the switch, at its end.
	top, ok1 := tt.Find("Sound Check")
	bottom, ok2 := tt.Find("Plays every song at the same volume")
	if !ok1 || !ok2 {
		t.Fatalf("not found: %q", tt.Texts())
	}
	tt.ClickAt(780-28-16-12, (top.Y+bottom.Y+bottom.H)/2)
	tt.Frame()
	if a.settings.Normalize == was {
		t.Error("the switch did nothing")
	}
}

// With MM_FRAMES set, saves the settings' Advanced page with an OTLP endpoint
// and headers filled in, light and dark, and in Chinese.
func TestTelemetrySettingsFrames(t *testing.T) {
	dir := framesDir(t)
	shot := func(name string, dark bool) {
		a := newTestApp()
		a.settings.TelemetryEndpoint = "http://127.0.0.1:4318"
		a.settings.TelemetryHeaders = "Authorization=Bearer not-a-real-token"
		a.settingsPage = "advanced"
		tt := ui.NewTester(a.settingsView, 780, 1200)
		tt.SetDark(dark)
		settled(tt)
		saveFrame(t, dir, name, tt.Image())
	}
	shot("telemetry-light.png", false)
	shot("telemetry-dark.png", true)
	defer i18n.SetLanguage(i18n.English)
	i18n.SetLanguage(i18n.Chinese)
	shot("telemetry-zh-dark.png", true)
}
