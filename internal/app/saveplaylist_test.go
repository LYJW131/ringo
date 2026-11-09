package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

type saveCall struct {
	id   string
	save bool
}

// The listener's own playlist offers no adding to their library, where it
// is already.
func TestOwnPlaylistNotSavable(t *testing.T) {
	a := editTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("Add to Library"); ok {
		t.Error("offered to add the listener's own playlist to their library")
	}
}

// Choosing Remove from Saved from a saved playlist's menu in the sidebar
// unsaves it, and its page, once loaded, no longer reads saved.
func TestUnsavePlaylistFromSidebar(t *testing.T) {
	calls := make(chan saveCall, 1)
	a := editTestApp()
	fake(a).save = func(id string, save bool) error { calls <- saveCall{id, save}; return nil }
	a.playlists["20"] = &music.Playlist{ID: "20", Name: "飙升榜", Saved: true}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("飙升榜"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete From Library"); err != nil {
		t.Fatalf("%v; the menu shows %q", err, tt.Menu())
	}
	tt.Frame()
	if got := wait(t, calls, "the unsaving"); got != (saveCall{"20", false}) {
		t.Errorf("asked the source for %+v, want playlist 20 unsaved", got)
	}
	if len(a.saved) != 0 || a.isSaved(a.playlists["20"]) {
		t.Errorf("playlist 20 still reads saved: %+v", a.saved)
	}
}
