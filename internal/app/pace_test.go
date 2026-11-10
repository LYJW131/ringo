package app

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// A source that answers after a delay, as Apple does over a real network,
// and a clock that the test moves on, so that waits of seconds take no time
// and the same run gives the same numbers.

type pace struct {
	t       *testing.T
	a       *app
	tt      *ui.Tester
	ch      chan func()
	latency time.Duration

	mu         sync.Mutex
	now        time.Duration
	waiting    []*paceWait
	requests   int
	flying     int
	mostFlying int
	// ahead counts the pages and playlists asked for, which are what's
	// fetched before the listener asks.
	ahead, mostAhead int
	released         int
	returned         int
	asked            map[string]int
}

type paceWait struct {
	due time.Duration
	ch  chan struct{}
}

// answer holds a request for the latency, in the test's time.
func (p *pace) answer(what string) {
	p.mu.Lock()
	p.requests++
	p.asked[what]++
	p.flying++
	p.mostFlying = max(p.mostFlying, p.flying)
	if what != "account" && what != "home" && what != "playlists" {
		p.ahead++
		p.mostAhead = max(p.mostAhead, p.ahead)
	}
	w := &paceWait{due: p.now + p.latency, ch: make(chan struct{})}
	p.waiting = append(p.waiting, w)
	p.mu.Unlock()
	<-w.ch
	p.mu.Lock()
	p.flying--
	if what != "account" && what != "home" && what != "playlists" {
		p.ahead--
	}
	p.returned++
	p.mu.Unlock()
}

// step moves the test's time on by d: what was due is answered, what the
// answers post is run, and the window draws a frame.
func (p *pace) step(d time.Duration) {
	p.mu.Lock()
	p.now += d
	var due []*paceWait
	rest := p.waiting[:0]
	for _, w := range p.waiting {
		if w.due <= p.now {
			due = append(due, w)
		} else {
			rest = append(rest, w)
		}
	}
	p.waiting = rest
	p.released += len(due)
	p.mu.Unlock()
	for _, w := range due {
		close(w.ch)
	}
	for {
		p.mu.Lock()
		done := p.returned == p.released
		p.mu.Unlock()
		if done {
			break
		}
		time.Sleep(50 * time.Microsecond)
	}
	// What an answer posts comes a moment after the answer.
	for range 2 {
		time.Sleep(200 * time.Microsecond)
		p.drain()
	}
	p.tt.Advance(d)
	p.tt.Frame()
}

func (p *pace) drain() {
	for {
		select {
		case fn := <-p.ch:
			fn()
		default:
			return
		}
	}
}

// idle lets d pass in frames of 16 ms.
func (p *pace) idle(d time.Duration) {
	for left := d; left > 0; left -= 16 * time.Millisecond {
		p.step(16 * time.Millisecond)
	}
}

func (p *pace) clock() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.now
}

// firstOf is a page's first item, which tells the page is up.
func firstOf(page string) string { return page + " first" }

// A grid of albums, or what the source makes of a page of the sidebar.
func pacePage(page string, n int) *music.Browse {
	sec := music.Section{Style: music.ShelfGrid}
	if page == music.PageNew || page == music.PageRadio || page == music.PageSearchLanding {
		sec.Title, sec.Style = "Shelf of "+page, music.ShelfSquare
	}
	for i := range n {
		title := fmt.Sprintf("%s %d", page, i)
		if i == 0 {
			title = firstOf(page)
		}
		sec.Items = append(sec.Items, music.Item{Kind: music.ItemAlbum, ID: fmt.Sprint(page, i), Title: title})
	}
	return &music.Browse{Sections: []music.Section{sec}}
}

func paceSongs(from, n int) []music.Item {
	var items []music.Item
	for i := from; i < from+n; i++ {
		tr := song(fmt.Sprint("s", i), fmt.Sprint("Song ", i), "Someone")
		items = append(items, music.Item{Kind: music.ItemSong, ID: tr.ID, Title: tr.Name, Track: &tr})
	}
	return items
}

// newPace is a signed-out app, whose source answers each request after
// latency, that starts as the listener signs in.
func newPace(t *testing.T, latency time.Duration) *pace {
	a := newTestApp()
	p := &pace{t: t, a: a, ch: posts(a), latency: latency, asked: map[string]int{}}
	f := fake(a)
	f.account = func() (*music.Account, error) {
		p.answer("account")
		return &music.Account{ID: "1", Name: "Alex"}, nil
	}
	f.home = func() (*music.Browse, error) {
		p.answer("home")
		return pacePage(music.PageNew, 12), nil
	}
	f.playlists = func() ([]music.Playlist, error) {
		p.answer("playlists")
		var lists []music.Playlist
		for i := range 20 {
			lists = append(lists, music.Playlist{ID: fmt.Sprint("p", i), Name: fmt.Sprint("Mix ", i), Mine: true})
		}
		return lists, nil
	}
	f.playlist = func(id string) (*music.Playlist, error) {
		p.answer("playlist")
		return &music.Playlist{ID: id, Name: "Mix " + strings.TrimPrefix(id, "p"), Mine: true, Tracks: []music.Track{
			song(id+"a", "First of "+id, "Someone"), song(id+"b", "Second of "+id, "Someone"),
		}}, nil
	}
	f.liked = func(offset int) (*music.Page, error) {
		p.answer("liked")
		return &music.Page{Tracks: []music.Track{song("l0", "First liked", "Someone"), song("l1", "Second liked", "Someone")}}, nil
	}
	f.browse = func(page string) (*music.Browse, error) {
		p.answer("browse")
		// A list of the library is itself however it's sorted.
		page, _ = music.ParseLibraryPage(page)
		if page == music.PageLibrarySongs {
			return &music.Browse{Sections: []music.Section{{Style: music.ShelfList, Items: paceSongs(0, 30)}}}, nil
		}
		return pacePage(page, 24), nil
	}
	lib := a.library
	lib.started, lib.accountAsked, lib.account = false, false, nil
	p.tt = ui.NewTester(a.view, 1200, 1000)
	p.tt.Frame()
	return p
}

// sidebarPages are the entries of the sidebar, and what shows once each is up.
var sidebarPages = []struct {
	label, marker string
}{
	{"New", firstOf(music.PageNew)},
	{"Radio", firstOf(music.PageRadio)},
	{"Recently Added", firstOf(music.PageRecentlyAdded)},
	{"Artists", firstOf(music.PageLibraryArtist)},
	{"Albums", firstOf(music.PageLibraryAlbums)},
	{"Songs", "Song 0"},
	{"Made for You", firstOf(music.PageMadeForYou)},
	{"All Playlists", firstOf(music.PageAllPlaylists)},
	{"Favorite Songs", "First liked"},
	{"Mix 0", "First of p0"},
	{"Mix 3", "First of p3"},
}

// opening clicks entry, and says how long it takes in the test's time, and
// in frames, for its content to show.
func (p *pace) opening(label, marker string) (time.Duration, int) {
	p.t.Helper()
	if err := p.tt.Click(label); err != nil {
		p.t.Fatalf("%s: %v", label, err)
	}
	p.tt.Frame()
	start, frames := p.clock(), 0
	for {
		if _, ok := p.tt.Find(marker); ok {
			return p.clock() - start, frames
		}
		if p.clock()-start > 5*time.Second {
			p.t.Fatalf("%s showed nothing in five seconds", label)
		}
		p.step(16 * time.Millisecond)
		frames++
	}
}

// Every page of the sidebar is up as it is clicked, once the listener has
// been signed in for a few seconds: the source is asked for them before.
func TestSidebarPagesWaitForNothing(t *testing.T) {
	p := newPace(t, 400*time.Millisecond)
	p.a.startLibrary()
	p.idle(4 * time.Second)
	var total time.Duration
	for _, e := range sidebarPages {
		d, frames := p.opening(e.label, e.marker)
		t.Logf("%-15s %4d ms, %d frames", e.label, d.Milliseconds(), frames)
		total += d
		if frames > 0 {
			t.Errorf("%s: its content wasn't up as it was clicked, but %d ms after", e.label, d.Milliseconds())
		}
		p.idle(300 * time.Millisecond)
	}
	t.Logf("together %d ms; %d requests, at most %d of the pages at once", total.Milliseconds(), p.requests, p.mostAhead)
	if p.mostAhead > warmAtOnce {
		t.Errorf("%d pages were asked for at once, more than %d", p.mostAhead, warmAtOnce)
	}
	if p.requests > 30 {
		t.Errorf("%d requests to have the sidebar up", p.requests)
	}
}
