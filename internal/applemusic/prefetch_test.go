package applemusic

import (
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ringo/internal/applemusic/amhelper"
	"ringo/internal/music"
)

// recorder keeps what the source asked, by when, on the simulated clock.
type recorder struct {
	mu    sync.Mutex
	asked []asked
}

type asked struct {
	url string
	at  time.Time
}

func (r *recorder) note(u string, at time.Time) {
	r.mu.Lock()
	r.asked = append(r.asked, asked{u, at})
	r.mu.Unlock()
}

func (r *recorder) clear() {
	r.mu.Lock()
	r.asked = nil
	r.mu.Unlock()
}

func (r *recorder) all() []asked {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]asked(nil), r.asked...)
}

var toWrapperPath = regexp.MustCompile(`/s/(wake|prepare)\b`)

// toWrapper is what reached the wrapper: a song's keys, and waking it.
func (r *recorder) toWrapper() []asked {
	var out []asked
	for _, a := range r.all() {
		if toWrapperPath.MatchString(a.url) {
			out = append(out, a)
		}
	}
	return out
}

type preparedSong struct {
	id    string
	ahead bool
	at    time.Time
}

var preparePath = regexp.MustCompile(`/s/prepare/(\d+)`)

func (r *recorder) prepared() []preparedSong {
	var out []preparedSong
	for _, a := range r.all() {
		if m := preparePath.FindStringSubmatch(a.url); m != nil {
			out = append(out, preparedSong{m[1], strings.Contains(a.url, "ahead=1"), a.at})
		}
	}
	return out
}

func tracksOf(ids []string) []music.Track {
	out := make([]music.Track, len(ids))
	for i, id := range ids {
		out[i] = music.Track{ID: id, Name: id}
	}
	return out
}

func idRange(from, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = strconv.Itoa(from + i)
	}
	return out
}

// What the wrapper is asked while songs play lossless, against a stand-in
// for amhelper: nothing once the songs coming up are worked out, so that a
// wrapper on Cloudflare can sleep. Signed out: the wrapper plays anyway.
func TestPrefetchLossless(t *testing.T) {
	rec := &recorder{}
	var ts *testSource
	ts = newTestSource(t, func(r *http.Request) *http.Response {
		u := r.URL.String()
		rec.note(u, ts.clk.Now())
		switch {
		case strings.Contains(u, "/prepare/"):
			return answer(200, map[string]any{"sampleRate": 44100, "bitDepth": 16, "bandwidth": 1000000})
		case strings.Contains(u, "/wake"):
			return answer(200, map[string]any{"woke": true})
		case strings.Contains(u, "/find/"):
			return answer(200, map[string]any{})
		}
		return answer(404, "not here")
	})
	ts.setSettings(map[string]string{"wrapper": "https://wrapper.example"})
	lossless := &attributes{AudioTraits: []string{"lossless"}}
	// The listener chose Lossless before.
	ts.storage.set(qualityKey, "lossless")
	// An album the listener played before: every song is worked out, kept from then.
	album := idRange(1000, 20)
	known := map[string]keptLossless{}
	for _, id := range album {
		known[id] = keptLossless{&knownQ{Level: "lossless", SampleRate: 44100, BitDepth: 16, Bandwidth: 1000000, Loudness: &amhelper.Loudness{LKFS: -9}}, ts.now()}
		ts.noteAudio(id, lossless)
	}
	ts.storage.set(losslessKey, known)
	wrapperURLs := func() []string {
		var out []string
		for _, a := range rec.all() {
			if regexp.MustCompile(`/(wake|prepare|find)\b`).MatchString(a.url) {
				out = append(out, a.url)
			}
		}
		return out
	}

	// Browsing, the wrapper is woken ahead of the first song.
	ts.asked(true)()
	ts.clk.settle(t)
	if got := wrapperURLs(); !reflect.DeepEqual(got, []string{helperBaseURL + "/wake"}) {
		t.Errorf("browsing: %q", got)
	}

	// Playing songs worked out before, the wrapper isn't asked anything for six minutes.
	ts.clk.minutes(t, 1)
	rec.clear()
	ts.prefetch(tracksOf(album), nil)
	for _, id := range album[:2] {
		s, err := ts.Stream(id, "lossless")
		if err != nil || s.Quality != "lossless" {
			t.Fatalf("stream %+v %v", s, err)
		}
		for range 6 {
			ts.clk.minutes(t, 0.5)
			ts.asked(true)() // browsing
			ts.clk.settle(t)
		}
	}
	if got := wrapperURLs(); len(got) != 0 {
		t.Errorf("while playing: %q", got)
	}

	// Songs stopped for a while, browsing wakes it again.
	ts.clk.minutes(t, 11)
	ts.asked(true)()
	ts.clk.settle(t)
	if got := wrapperURLs(); !reflect.DeepEqual(got, []string{helperBaseURL + "/wake"}) {
		t.Errorf("after a while: %q", got)
	}

	// A playlist never played: playing one song works out the ones after it, one after the other.
	playlist := idRange(2000, 40)
	for _, id := range playlist {
		ts.noteAudio(id, lossless)
	}
	ts.clk.minutes(t, 2)
	ts.prefetch(tracksOf(playlist), nil)
	ts.clk.settle(t)
	ids := func(p []preparedSong) []string {
		var out []string
		for _, x := range p {
			out = append(out, x.id)
		}
		return out
	}
	if got := ids(rec.prepared()); !reflect.DeepEqual(got, idRange(2000, 3)) {
		t.Errorf("the page's first three: %q", got)
	}
	rec.clear()
	ts.Stream("2004", "lossless")
	ts.clk.settle(t)
	p := rec.prepared()
	want := []preparedSong{{"2004", false, ts.clk.Now()}}
	for _, id := range playlist[5:17] {
		want = append(want, preparedSong{id, true, ts.clk.Now()})
	}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("playing 2004: %+v", p)
	}

	// With the next three worked out, playing asks the wrapper nothing.
	rec.clear()
	for _, id := range []string{"2005", "2006", "2007"} {
		ts.Stream(id, "lossless")
		ts.clk.minutes(t, 3)
	}
	if got := wrapperURLs(); len(got) != 0 {
		t.Errorf("worked out already: %q", got)
	}

	// More than fifteen songs a minute wait for the minute to pass.
	rec.clear()
	from := ts.clk.Now()
	ts.Stream("2014", "lossless") // 2015, 2016 are worked out, 2017 isn't
	ts.clk.settle(t)
	ts.Stream("2026", "lossless") // and 2027 to 2038
	ts.clk.settle(t)
	ahead := func() []preparedSong {
		var out []preparedSong
		for _, x := range rec.prepared() {
			if x.ahead {
				out = append(out, x)
			}
		}
		return out
	}
	if n := len(ahead()); n != 15 {
		t.Errorf("at once: %d", n)
	}
	ts.clk.minutes(t, 0.9)
	if n := len(ahead()); n != 15 {
		t.Errorf("within the minute: %d", n)
	}
	ts.clk.minutes(t, 0.2)
	a := ahead()
	if len(a) != 22 { // 2017 to 2038
		t.Errorf("after the minute: %d", len(a))
	}
	for i, x := range a {
		if i >= 15 && x.at.Sub(a[i-15].at) < time.Minute {
			t.Errorf("%s too soon", x.id)
		}
		if x.at.Sub(from) >= 2*time.Minute {
			t.Errorf("%s too late", x.id)
		}
	}
}
