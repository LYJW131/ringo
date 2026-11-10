package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
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

// Delete From Library on a saved playlist's menu in the sidebar goes the way
// its page's does, through the library: it called SavePlaylist, and its page
// AddToLibrary. It leaves the sidebar at once, and its page no longer reads
// saved.
func TestUnsavePlaylistFromSidebar(t *testing.T) {
	calls := make(chan saveCall, 1)
	a := editTestApp()
	fake(a).addToLibrary = func(kind, id string, on bool) error {
		if kind == music.ItemPlaylist {
			calls <- saveCall{id, on}
		}
		return nil
	}
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
	if got := wait(t, calls, "the deleting"); got != (saveCall{"20", false}) {
		t.Errorf("asked the source for %+v, want playlist 20 deleted from the library", got)
	}
	if _, ok := tt.Find("飙升榜"); ok || a.isSaved(a.playlists["20"]) {
		t.Errorf("playlist 20 still reads saved: %+v", a.saved)
	}
}
