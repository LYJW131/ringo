package applemusic

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// catalogSong is a song's attributes as the catalog lists them: Hi-Res with
// the master playlist a request is given, or lossless without one.
func catalogSong(id string, hiRes, master bool) *attributes {
	traits := []string{"lossless"}
	if hiRes {
		traits = append(traits, "hi-res-lossless")
	}
	b := map[string]any{"audioTraits": traits}
	if master {
		b["extendedAssetUrls"] = map[string]any{"enhancedHls": "https://aod.itunes.apple.com/" + id + ".m3u8"}
	}
	raw, _ := json.Marshal(b)
	a := &attributes{}
	json.Unmarshal(raw, a)
	return a
}

// helperQueries are the queries of what reached amhelper under path, such as
// /prepare/, in the order asked.
func (r *recorder) helperQueries(path string) []url.Values {
	var out []url.Values
	for _, a := range r.all() {
		if u, err := url.Parse(a.url); err == nil && strings.Contains(u.Path, path) {
			out = append(out, u.Query())
		}
	}
	return out
}

// amhelperAnswers is amhelper working every song out as 16-bit/44.1 kHz.
func amhelperAnswers(rec *recorder, ts **testSource) func(*http.Request) *http.Response {
	return func(r *http.Request) *http.Response {
		rec.note(r.URL.String(), (*ts).clk.Now())
		switch {
		case strings.Contains(r.URL.Path, "/prepare/"):
			return answer(200, map[string]any{"sampleRate": 44100, "bitDepth": 16, "bandwidth": 1000000})
		case strings.Contains(r.URL.Path, "/find/"):
			return answer(200, map[string]any{})
		}
		return answer(404, "not here")
	}
}

// A song found at Hi-Res Lossless is asked about again at Lossless, with the
// rate capped at 48 kHz and without the wrapper's own playlist, and its
// stream says the level it was asked at, even when the song has no more than
// 44.1 kHz. What was found at one level once played at the other, past 48 kHz.
func TestLosslessLevelSwitchAsksAgain(t *testing.T) {
	rec := &recorder{}
	var ts *testSource
	ts = newTestSource(t, amhelperAnswers(rec, &ts))
	ts.setSettings(map[string]string{"wrapper": "https://wrapper.example"})
	ts.noteAudio("5000", catalogSong("5000", true, true))

	st, err := ts.Stream("5000", "hires")
	if err != nil || st.Quality != "hires" {
		t.Fatalf("Hi-Res Lossless: %+v %v", st, err)
	}
	asked := rec.helperQueries("/prepare/5000")
	if len(asked) != 1 || asked[0].Get("device") != "1" || asked[0].Get("master") == "" || asked[0].Has("maxRate") {
		t.Fatalf("Hi-Res Lossless asked %v", asked)
	}

	st, err = ts.Stream("5000", "lossless")
	if err != nil || st.Quality != "lossless" {
		t.Fatalf("Lossless: %+v %v", st, err)
	}
	asked = rec.helperQueries("/prepare/5000")
	if len(asked) != 2 {
		t.Fatalf("asked %d times, want again at the other level", len(asked))
	}
	if q := asked[1]; q.Get("maxRate") != "48000" || q.Has("device") || q.Get("master") == "" {
		t.Errorf("Lossless asked %v", q)
	}
	// And the stream itself is told the cap, as amhelper plays what it worked out.
	if u, _ := url.Parse(st.URL); u.Query().Get("maxRate") != "48000" || u.Query().Has("device") {
		t.Errorf("Lossless plays %s", st.URL)
	}

	// Asked again at the same level, nothing more is asked.
	if _, err := ts.Stream("5000", "lossless"); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.helperQueries("/prepare/5000")); n != 2 {
		t.Errorf("asked %d times", n)
	}
}

// At Lossless everything asked of amhelper, for a song about to play, one to
// come and one read from the page, is capped at 48 kHz and leaves out the
// wrapper's own playlist; at Hi-Res Lossless it asks for that and no cap.
func TestLosslessAsksForNoMoreThan48kHz(t *testing.T) {
	for _, level := range []string{"lossless", "hires"} {
		rec := &recorder{}
		var ts *testSource
		ts = newTestSource(t, amhelperAnswers(rec, &ts))
		ts.setSettings(map[string]string{"wrapper": "https://wrapper.example"})
		ts.storage.set(qualityKey, level)
		page := idRange(6000, 8)
		for _, id := range page {
			ts.noteAudio(id, catalogSong(id, true, true))
		}
		// One without a master playlist: the wrapper's own is all there is to ask.
		ts.noteAudio("6100", catalogSong("6100", true, false))
		ts.prefetch(tracksOf(append(page, "6100")), nil)
		ts.clk.settle(t)
		if _, err := ts.Stream("6100", level); err != nil {
			t.Fatal(err)
		}
		ts.clk.settle(t)

		prepared, found := rec.helperQueries("/prepare/"), rec.helperQueries("/find/")
		if len(prepared) < 4 || len(found) == 0 {
			t.Fatalf("%s: %d prepared, %d found", level, len(prepared), len(found))
		}
		for _, q := range append(prepared, found...) {
			if capped := q.Get("maxRate") == "48000"; capped != (level == "lossless") {
				t.Errorf("%s asked %v", level, q)
			}
			if q.Has("device") && level == "lossless" {
				t.Errorf("%s asked for the wrapper's own playlist: %v", level, q)
			}
			if q.Get("master") != "" && (q.Get("device") == "1") != (level == "hires") {
				t.Errorf("%s asked %v", level, q)
			}
		}
	}
}
