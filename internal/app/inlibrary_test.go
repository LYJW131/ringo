package app

import (
	"image"
	"image/draw"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// keepApp is an app signed in to a source that says which songs are in the
// library as in says, with tr showing in Now Playing.
func keepApp(t *testing.T, tr music.Track, in map[string]bool) (*app, *ui.Tester, chan func()) {
	t.Helper()
	a := newTestApp()
	ch := posts(a)
	f := fake(a)
	f.inLibrary = func(kind string, ids []string) (map[string]bool, error) { return in, nil }
	a.account, a.accountAsked = &music.Account{ID: "1"}, true
	a.pl.Cue([]music.Track{tr})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	return a, tt, ch
}

// rect is where the element labeled s shows, failing the test if it doesn't.
func rect(t *testing.T, tt *ui.Tester, s string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		t.Fatalf("no %s; the window shows %q", s, tt.Texts())
	}
	return r
}

// Beside the star in Now Playing, a song not known to be in the library
// holds an empty place until its source says; a plus then adds it, and
// turns into a check, which a click leaves as it is. The star and the dots
// stay where they are throughout.
func TestKeepButton(t *testing.T) {
	added := make(chan bool, 4)
	a, tt, ch := keepApp(t, song("7", "宝船", "DOUDOU"), map[string]bool{})
	fake(a).addToLibrary = func(kind, id string, on bool) error { added <- on; return nil }

	if _, ok := tt.Find("Add to Library"); ok {
		t.Fatal("a plus before the source said")
	}
	star, more := rect(t, tt, "Favorite"), rect(t, tt, "More")
	runPost(t, ch)
	tt.Advance(time.Second)
	tt.Frame()
	plus := rect(t, tt, "Add to Library")
	if rect(t, tt, "Favorite") != star || rect(t, tt, "More") != more {
		t.Error("the star or the dots moved as the plus came")
	}
	if left, right := plus.X-(star.X+star.W), more.X-(plus.X+plus.W); left != right || left < 0 {
		t.Errorf("the plus %v from the star and %v from the dots", left, right)
	}

	if err := tt.Click("Add to Library"); err != nil {
		t.Fatal(err)
	}
	if on := wait(t, added, "the song added"); !on {
		t.Fatal("asked to delete it")
	}
	runPost(t, ch)
	tt.Advance(time.Second)
	tt.Frame()
	if rect(t, tt, "Added to Library") != plus {
		t.Error("the check isn't where the plus was")
	}
	if rect(t, tt, "Favorite") != star || rect(t, tt, "More") != more {
		t.Error("the star or the dots moved as the song was added")
	}
	tt.Click("Added to Library")
	tt.Frame()
	select {
	case on := <-added:
		t.Errorf("the check asked the source again, to add: %v", on)
	case <-time.After(100 * time.Millisecond):
	}
}

// A song uploaded to the library shows no plus, and leaves no place for one.
func TestKeepButtonNotForUploads(t *testing.T) {
	tr := song("7", "宝船", "DOUDOU")
	_, tt, _ := keepApp(t, tr, map[string]bool{"7": true})
	star := rect(t, tt, "Favorite")

	up := song("i.8", "Demo", "DOUDOU")
	up.Uploaded, up.Saved, up.SavedKnown = true, true, true
	_, tt, _ = keepApp(t, up, nil)
	if _, ok := tt.Find("Added to Library"); ok {
		t.Error("an upload shows the check")
	}
	if got := rect(t, tt, "Favorite"); got.X <= star.X {
		t.Errorf("the star at %v with an upload, as at %v with a place for the plus", got.X, star.X)
	}
}

// A song's menu offers to delete it from the library once its source said
// it's there, and to add it until then.
func TestSongMenuFollowsLibrary(t *testing.T) {
	_, tt, ch := keepApp(t, song("7", "宝船", "DOUDOU"), map[string]bool{"7": true})
	tt.Click("More")
	if menu := tt.Menu(); !contains(menu, "Add to Library") || contains(menu, "Delete From Library") {
		t.Errorf("before the source said, the menu shows %q", menu)
	}
	tt.CloseMenu()
	runPost(t, ch)
	tt.Frame()
	tt.Click("More")
	if menu := tt.Menu(); contains(menu, "Add to Library") || !contains(menu, "Delete From Library") {
		t.Errorf("in the library, the menu shows %q", menu)
	}
	tt.CloseMenu()
}

// What the source answers of a song after the listener added it doesn't
// undo the add.
func TestKeepClickBeatsAnswer(t *testing.T) {
	added := make(chan bool, 1)
	a, tt, ch := keepApp(t, song("7", "宝船", "DOUDOU"), map[string]bool{})
	fake(a).addToLibrary = func(kind, id string, on bool) error { added <- on; return nil }
	a.askInLibrary()
	// Asked at the end of the first frame, so it's on its way: the listener
	// adds it from the menu, which offers it until the source says.
	tt.Click("More")
	if err := tt.ChooseMenuItem("Add to Library"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	wait(t, added, "the song added")
	runPost(t, ch) // the answer, from before the add
	runPost(t, ch) // the add
	if in, known := a.songInLibrary(a.pl.Current()); !in || !known {
		t.Errorf("in the library: %v, known: %v", in, known)
	}
}

// Once another listener signs in, the song playing is asked about anew:
// what it said of the library was the last one's.
func TestKeepAskedAnewForTheNextListener(t *testing.T) {
	tr := song("7", "宝船", "DOUDOU")
	tr.Saved, tr.SavedKnown = true, true
	a, tt, _ := keepApp(t, tr, map[string]bool{})
	rect(t, tt, "Added to Library")
	a.account = &music.Account{ID: "2"}
	a.afterAccount(a.library)
	if _, known := a.songInLibrary(a.pl.Current()); known {
		t.Error("the last listener's library still counts")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// With MM_FRAMES set, saves the row under the cover in Now Playing, dark,
// with a song not in the library, in it, not known yet, and uploaded; then
// the stage's bar and the wall with one in it.
func TestKeepButtonFrames(t *testing.T) {
	dir := os.Getenv("MM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	upload := song("i.8", "宝船", "DOUDOU")
	upload.Uploaded, upload.Saved, upload.SavedKnown = true, true, true
	var rows []image.Image
	for _, c := range []struct {
		tr     music.Track
		answer bool
	}{
		{song("7", "宝船", "DOUDOU"), true},
		{song("7", "宝船", "DOUDOU"), false},
		{song("7", "宝船", "DOUDOU"), false},
		{upload, false},
	} {
		in := map[string]bool{}
		if len(rows) == 1 {
			in["7"] = true
		}
		_, tt, ch := keepApp(t, c.tr, in)
		tt.SetDark(true)
		tt.SetScale(2)
		if c.answer || len(rows) == 1 {
			runPost(t, ch)
		}
		tt.Advance(time.Second)
		tt.Frame()
		more := rect(t, tt, "More")
		rows = append(rows, crop(tt, ui.Rect{X: more.X - 420, Y: more.Y - 16, W: 460, H: more.H + 32}))
	}
	saveFrame(t, dir, "np-row.png", stack(rows))

	in := song("7", "宝船", "DOUDOU")
	in.Saved, in.SavedKnown = true, true
	for _, pane := range []struct {
		pane int
		name string
	}{{paneStage, "stage-bar.png"}, {paneWall, "wall.png"}} {
		a, tt, _ := keepApp(t, in, nil)
		a.npPane = pane.pane
		tt.SetDark(true)
		tt.SetScale(2)
		tt.Advance(time.Second)
		tt.Frame()
		more := rect(t, tt, "More")
		saveFrame(t, dir, pane.name, crop(tt, ui.Rect{X: more.X - 300, Y: more.Y - 24, W: 360, H: more.H + 48}))
	}
}

// crop is r of what tt shows, in its pixels.
func crop(tt *ui.Tester, r ui.Rect) image.Image {
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1200
	out := image.NewRGBA(image.Rect(0, 0, int(r.W*scale), int(r.H*scale)))
	draw.Draw(out, out.Bounds(), img, image.Pt(int(r.X*scale), int(r.Y*scale)), draw.Src)
	return out
}

// stack lays images one above the other.
func stack(imgs []image.Image) image.Image {
	w, h := 0, 0
	for _, img := range imgs {
		w, h = max(w, img.Bounds().Dx()), h+img.Bounds().Dy()
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	y := 0
	for _, img := range imgs {
		draw.Draw(out, img.Bounds().Add(image.Pt(0, y)), img, img.Bounds().Min, draw.Src)
		y += img.Bounds().Dy()
	}
	return out
}
