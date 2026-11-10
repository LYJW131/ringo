package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// fakeUpdater swaps the update plugin's preferences for ones in memory,
// counting the checks asked for, until the returned function puts them
// back.
func fakeUpdater(checks, downloads *bool, checked *int) func() {
	check, getChecks, setChecks := checkForUpdates, updateChecks, setUpdateChecks
	getDownloads, setDownloads, last := updateDownloads, setUpdateDownloads, lastUpdateCheck
	checkForUpdates = func() { *checked++ }
	updateChecks = func() bool { return *checks }
	setUpdateChecks = func(on bool) { *checks = on }
	updateDownloads = func() bool { return *downloads }
	setUpdateDownloads = func(on bool) { *downloads = on }
	lastUpdateCheck = func() time.Time { return time.Time{} }
	return func() {
		checkForUpdates, updateChecks, setUpdateChecks = check, getChecks, setChecks
		updateDownloads, setUpdateDownloads, lastUpdateCheck = getDownloads, setDownloads, last
	}
}

// Turning off checking for updates turns it off in the updater, and
// installing updates without asking can't be turned on without it.
func TestTurnOffUpdateChecks(t *testing.T) {
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	a := newTestApp()
	a.updates = true
	a.settingsPage = "general"
	tt := ui.NewTester(a.settingsView, 760, 2000)
	tt.Frame()
	// The row's title is labelled as its switch is: click the switch, at
	// the row's end.
	toggle := func(title, desc string) {
		t.Helper()
		top, ok1 := tt.Find(title)
		bottom, ok2 := tt.Find(desc)
		if !ok1 || !ok2 {
			t.Fatalf("no row %s", title)
		}
		tt.ClickAt(760-28-16-12, (top.Y+bottom.Y+bottom.H)/2)
		tt.Frame()
	}
	toggle("Check for Updates Automatically", "Check for a new version once a day")
	if checks {
		t.Fatal("checks for updates still")
	}
	toggle("Download and Install Automatically", "Download new versions in the background, to take effect the next time the app opens")
	if downloads {
		t.Fatal("turned on installing updates without checking for them")
	}
	// It does turn on while the updater checks.
	checks = true
	tt.Frame()
	toggle("Download and Install Automatically", "Download new versions in the background, to take effect the next time the app opens")
	if !downloads {
		t.Fatal("didn't turn on installing updates")
	}
}

// Check for Updates in the settings checks for updates as the user asked.
func TestCheckForUpdatesFromSettings(t *testing.T) {
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	a := newTestApp()
	a.updates = true
	a.settingsPage = "general"
	tt := ui.NewTester(a.settingsView, 760, 2000)
	tt.Frame()
	if err := tt.Click("Check for Updates"); err != nil {
		t.Fatal(err)
	}
	if checked != 1 {
		t.Fatalf("checked %d times", checked)
	}
}
