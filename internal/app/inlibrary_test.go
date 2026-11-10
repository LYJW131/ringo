package app

import (
	"errors"
	"image"
	"image/draw"
	"math"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
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

// A song not known to be in the library holds an empty place in Now Playing
// until its source says. One that isn't there has a plus, not a star: a
// song is added before it's a favorite. The plus adds it, and its star
// takes the plus's place, which then makes it a favorite. The dots beside
// stay where they are throughout.
func TestPlusBeforeStar(t *testing.T) {
	added := make(chan bool, 4)
	liked := make(chan bool, 4)
	a, tt, ch := keepApp(t, stray("7", "宝船", "DOUDOU"), map[string]bool{})
	fake(a).addToLibrary = func(kind, id string, on bool) error { added <- on; return nil }
	fake(a).like = func(id string, like bool) error { liked <- like; return nil }

	more := rect(t, tt, "More")
	for _, label := range []string{"Add to Library", "Favorite"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("%s before the source said", label)
		}
	}
	runPost(t, ch)
	tt.Advance(time.Second)
	tt.Frame()
	plus := rect(t, tt, "Add to Library")
	if _, ok := tt.Find("Favorite"); ok {
		t.Fatal("a star on a song that isn't in the library")
	}
	if rect(t, tt, "More") != more {
		t.Error("the dots moved as the plus came")
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
	if _, ok := tt.Find("Add to Library"); ok {
		t.Fatal("still a plus on a song that was added")
	}
	// The star comes up on a spring, a hair off its size till it rests.
	if star := rect(t, tt, "Favorite"); math.Abs(float64(star.X-plus.X)) > 0.5 || math.Abs(float64(star.Y-plus.Y)) > 0.5 || rect(t, tt, "More") != more {
		t.Errorf("the star at %v, not where the plus was, %v, or the dots moved, to %v from %v", star, plus, rect(t, tt, "More"), more)
	}
	if err := tt.Click("Favorite"); err != nil {
		t.Fatal(err)
	}
	if on := wait(t, liked, "the favorite"); !on {
		t.Error("asked to undo a favorite")
	}
}

// A song uploaded to the library has its star at once, without its source
// being asked: it's in the library, and can't be added to it.
func TestStarForUploads(t *testing.T) {
	up := stray("i.8", "Demo", "DOUDOU")
	up.Uploaded = true
	_, tt, _ := keepApp(t, up, nil)
	rect(t, tt, "Favorite")
	if _, ok := tt.Find("Add to Library"); ok {
		t.Error("an upload offered to be added")
	}
}

// A song its source fails to answer of has its star, not an empty place
// for good.
func TestStarWhereTheSourceFails(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	fake(a).inLibrary = func(string, []string) (map[string]bool, error) { return nil, errors.New("offline") }
	a.account, a.accountAsked = &music.Account{ID: "1"}, true
	a.pl.Cue([]music.Track{stray("7", "宝船", "DOUDOU")})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("Favorite"); ok {
		t.Fatal("a star before the source answered")
	}
	runPost(t, ch)
	tt.Frame()
	rect(t, tt, "Favorite")
}

// A song's menu offers to delete it from the library once its source said
// it's there, and to add it until then.
func TestSongMenuFollowsLibrary(t *testing.T) {
	_, tt, ch := keepApp(t, stray("7", "宝船", "DOUDOU"), map[string]bool{"7": true})
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
	a, tt, ch := keepApp(t, stray("7", "宝船", "DOUDOU"), map[string]bool{})
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
	rect(t, tt, "Favorite")
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
// with a song not in the library (its plus), in it (its star), not known
// yet (an empty place), and uploaded (its star); then the stage's bar and
// the wall with one in it.
func TestKeepButtonFrames(t *testing.T) {
	dir := os.Getenv("MM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	upload := stray("i.8", "宝船", "DOUDOU")
	upload.Uploaded = true
	var rows []image.Image
	for _, c := range []struct {
		tr     music.Track
		answer bool
	}{
		{stray("7", "宝船", "DOUDOU"), true},
		{stray("7", "宝船", "DOUDOU"), false},
		{stray("7", "宝船", "DOUDOU"), false},
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
