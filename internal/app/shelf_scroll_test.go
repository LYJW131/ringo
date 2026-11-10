package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// shelfAt is a page of shelves with the pointer on the card of New Releases
// that is third in its shelf, and where that card is.
func shelfAt(t *testing.T) (*app, *ui.Tester, ui.Rect) {
	t.Helper()
	a, _ := shelfApp()
	tt := ui.NewTester(a.view, 1180, 1400)
	tt.Frame()
	r, ok := tt.Find("Better Over Time")
	if !ok {
		t.Fatal("no shelf")
	}
	tt.Move(r.X+r.W/2, r.Y+10)
	tt.Frame()
	return a, tt, r
}

// touchpad sends n scrolls of a touchpad with the pointer over the card at r.
func touchpad(tt *ui.Tester, r ui.Rect, n int, dx, dy float32) {
	for range n {
		tt.Scroll(r.X+r.W/2, r.Y+10, dx, dy)
	}
}

// moved is how far the card r was has gone, across and down.
func moved(t *testing.T, tt *ui.Tester, r ui.Rect) (dx, dy float32) {
	t.Helper()
	now, ok := tt.Find("Better Over Time")
	if !ok {
		t.Fatal("the card went")
	}
	return now.X - r.X, now.Y - r.Y
}

// Two fingers going down a page that has shelves in it scroll the page, and
// the little they go sideways as they do never moves a shelf. It had, and
// the page stuttered as the shelves swayed.
func TestTouchpadDownKeepsShelves(t *testing.T) {
	_, tt, r := shelfAt(t)
	touchpad(tt, r, 10, 2, 10)
	dx, dy := moved(t, tt, r)
	if dx != 0 {
		t.Errorf("the shelf went %v across", dx)
	}
	if dy > -50 {
		t.Errorf("the page went %v down", dy)
	}
}

// Two fingers going across a shelf scroll it, and the page stays where it
// is as they drift a little down.
func TestTouchpadAcrossScrollsShelf(t *testing.T) {
	_, tt, r := shelfAt(t)
	touchpad(tt, r, 10, 10, 2)
	dx, dy := moved(t, tt, r)
	if dx > -50 {
		t.Errorf("the shelf went %v across", dx)
	}
	if dy != 0 {
		t.Errorf("the page went %v down", dy)
	}
}

// A gesture goes the way it started until the fingers rest: one that began
// going down goes on scrolling the page as it veers across, and the next
// one can go across.
func TestTouchpadGestureKeepsItsWay(t *testing.T) {
	_, tt, r := shelfAt(t)
	touchpad(tt, r, 1, 1, 10)
	touchpad(tt, r, 10, 10, 2)
	dx, dy := moved(t, tt, r)
	if dx != 0 || dy > -20 {
		t.Fatalf("the shelf went %v across, the page %v down", dx, dy)
	}
	tt.Advance(200 * time.Millisecond)
	was, wasY := dx, dy
	touchpad(tt, r, 10, 10, 2)
	dx, dy = moved(t, tt, r)
	if dx > was-50 || dy != wasY {
		t.Errorf("the next gesture went %v across and %v down", dx-was, dy-wasY)
	}
}

// A shelf scrolled to its end that the fingers go on pushing leaves the page
// alone: it is the shelf they are on.
func TestTouchpadPastShelfEndKeepsPage(t *testing.T) {
	_, tt, r := shelfAt(t)
	touchpad(tt, r, 40, 100, 0)
	tt.Advance(200 * time.Millisecond)
	dx, dy := moved(t, tt, r)
	if dx > -100 {
		t.Fatalf("the shelf only went %v across", dx)
	}
	touchpad(tt, r, 10, 20, 4)
	if x, y := moved(t, tt, r); x != dx || y != dy {
		t.Errorf("past its end the shelf went %v across and the page %v down", x-dx, y-dy)
	}
}

// A mouse wheel that goes across, as a tilt wheel does, scrolls the shelf,
// and gives the page nothing.
func TestWheelAcrossScrollsShelf(t *testing.T) {
	_, tt, r := shelfAt(t)
	tt.Wheel(r.X+r.W/2, r.Y+10, 120, 0)
	tt.Advance(time.Second)
	tt.Frame()
	if dx, dy := moved(t, tt, r); dx > -60 || dy != 0 {
		t.Errorf("the shelf went %v across and the page %v down", dx, dy)
	}
}

// Tab brings a card that is out of view into it, whole and no further than
// the page's edge; a press on one half out of view leaves it where it is,
// as a click on it would else miss it.
func TestShelfFocusComesIntoView(t *testing.T) {
	_, tt, r := shelfAt(t)
	last, ok := tt.Find("Mosaic")
	if !ok || last.X < 1180-pagePad {
		t.Fatalf("the last card is at %+v, in view already", last)
	}
	tt.Press(r.X+r.W/2, r.Y+10)
	tt.Release(5, 5)
	for range 12 {
		if tt.Focused("Mosaic") {
			break
		}
		tt.Key(0, ui.KeyTab)
	}
	if !tt.Focused("Mosaic") {
		t.Fatal("Tab never came to the last card")
	}
	tt.Advance(time.Second)
	tt.Frame()
	last, _ = tt.Find("Mosaic")
	if last.X+last.W > 1180-pagePad+1 {
		t.Errorf("the card is at %v to %v, out of view", last.X, last.X+last.W)
	}

	// Scrolled until the first card is half out of view, a press on it.
	_, tt, r = shelfAt(t)
	touchpad(tt, r, 10, 10, 0)
	first, _ := tt.Find("Young Manhood and Other Donatable Organs")
	tt.Press(first.X+first.W-10, first.Y+10)
	tt.Advance(time.Second)
	tt.Frame()
	if now, _ := tt.Find("Young Manhood and Other Donatable Organs"); now != first {
		t.Errorf("a press moved the card from %+v to %+v", first, now)
	}
}

// What the shelf scrolled out of view takes no click, even where it lies
// over the sidebar.
func TestShelfClippedCardTakesNoClick(t *testing.T) {
	a, tt, r := shelfAt(t)
	touchpad(tt, r, 10, 10, 0)
	first, _ := tt.Find("Young Manhood and Other Donatable Organs")
	// What shows of it is what is not under the sidebar.
	if first.X != sidebarWidth || first.W > 120 {
		t.Fatalf("the first card shows as %+v, not half out of view", first)
	}
	tt.ClickAt(sidebarWidth-10, first.Y+first.H/2)
	if a.route.page == pageAlbum {
		t.Errorf("the card out of view opened %+v", a.route)
	}
}

// How touchpad gestures are told apart, and a wheel's notches: ones the
// Tester cannot send, as momentum and Shift held.
func TestGestureWays(t *testing.T) {
	at := time.Now()
	scroll := func(after time.Duration, dx, dy float32, momentum bool) ui.InputEvent {
		at = at.Add(after)
		return ui.InputEvent{Kind: ui.InputScroll, DX: dx, DY: dy, Precise: true, Momentum: momentum, Time: at}
	}
	var g touchGesture
	if way, _ := g.feed(scroll(0, 1, 1, false)); way != wayUnknown {
		t.Error("told the way of a gesture before it had gone far")
	}
	if way, dx := g.feed(scroll(10*time.Millisecond, 4, 1, false)); way != wayAcross || dx != 5 {
		t.Errorf("went %v, %v across: not all it had gone so far", way, dx)
	}
	// It goes on with its momentum past a pause, and until the fingers come
	// down again.
	if way, _ := g.feed(scroll(400*time.Millisecond, 3, 3, true)); way != wayAcross {
		t.Error("momentum after a pause began another gesture")
	}
	if way, _ := g.feed(scroll(10*time.Millisecond, 0, 30, false)); way != wayDown {
		t.Error("the fingers coming down on momentum did not begin another gesture")
	}
	if way, _ := g.feed(scroll(300*time.Millisecond, 20, 3, false)); way != wayAcross {
		t.Error("a gesture after a pause was not its own")
	}
	if way, _ := g.feed(scroll(300*time.Millisecond, 10, 8, false)); way != wayDown {
		t.Error("a gesture across and down as much went across")
	}

	notch := func(dx, dy float32, mods ui.Modifiers) float32 {
		d, _ := wheelAcross(ui.InputEvent{Kind: ui.InputScroll, DX: dx, DY: dy, Mods: mods})
		return d
	}
	if notch(0, 40, 0) != 0 || notch(0, 40, ui.Shift) != 40 || notch(30, 0, 0) != 30 {
		t.Error("a wheel's notch went the wrong way")
	}
}
