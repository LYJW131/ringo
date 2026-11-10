package applemusic

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mygo-music/internal/music"
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

// songWith is a song as the catalog lists it, with these audio traits and a
// master playlist.
func songWith(id string, traits ...string) *attributes {
	raw, _ := json.Marshal(map[string]any{"audioTraits": traits,
		"extendedAssetUrls": map[string]any{"enhancedHls": "https://aod.itunes.apple.com/" + id + ".m3u8"}})
	a := &attributes{}
	json.Unmarshal(raw, a)
	return a
}

// With Dolby Atmos on, a song with a Dolby Atmos version plays as the stereo
// version of it the setting asks for, from the catalog's playlist, whatever
// the level; one without plays at the level, saying so for the cache; one
// whose version the wrapper can't find falls back to the level too. High
// Efficiency is HE-AAC, through the wrapper all the same.
func TestDolbyAtmosAndHighEfficiencyPlayThroughTheWrapper(t *testing.T) {
	rec := &recorder{}
	var ts *testSource
	ts = newTestSource(t, func(r *http.Request) *http.Response {
		rec.note(r.URL.String(), ts.clk.Now())
		kind := r.URL.Query().Get("kind")
		switch {
		case strings.Contains(r.URL.Path, "/prepare/7002") && kind == "binaural":
			return answer(404, map[string]any{"code": "noAudio", "message": "the song has no such audio"})
		case strings.Contains(r.URL.Path, "/prepare/") && kind != "":
			rate := map[string]int{"binaural": 256000, "downmix": 256000, "he": 64000}[kind]
			return answer(200, map[string]any{"codec": "aac", "bandwidth": rate + 7000, "bitRate": rate})
		case strings.Contains(r.URL.Path, "/prepare/"):
			return answer(200, map[string]any{"codec": "alac", "sampleRate": 96000, "bitDepth": 24, "bandwidth": 3000000})
		}
		return answer(404, "not here")
	})
	ts.setSettings(map[string]string{"wrapper": "https://wrapper.example"})
	ts.noteAudio("7000", songWith("7000", "atmos", "hi-res-lossless", "lossless", "spatial"))
	ts.noteAudio("7001", songWith("7001", "hi-res-lossless", "lossless"))
	ts.noteAudio("7002", songWith("7002", "atmos", "hi-res-lossless", "lossless"))

	for _, c := range []struct {
		id, ask string
		quality string
		kind    string
		bitRate int
		noAtmos bool
	}{
		{"7000", music.Ask("hires", music.AtmosHeadphones), "atmos-binaural", "binaural", 256000, false},
		{"7000", music.Ask("high", music.AtmosSpeakers), "atmos-downmix", "downmix", 256000, false},
		{"7000", "hires", "hires", "", 3000000, false},
		{"7001", music.Ask("hires", music.AtmosHeadphones), "hires", "", 3000000, true},
		{"7002", music.Ask("hires", music.AtmosHeadphones), "hires", "", 3000000, false},
		{"7001", "efficient", "efficient", "he", 64000, false},
	} {
		st, err := ts.Stream(c.id, c.ask)
		if err != nil {
			t.Fatalf("%s as %s: %v", c.id, c.ask, err)
		}
		u, _ := url.Parse(st.URL)
		if st.Quality != c.quality || u.Query().Get("kind") != c.kind || st.BitRate != c.bitRate || st.NoAtmos != c.noAtmos {
			t.Errorf("%s as %s: %+v, want %s (kind %q) at %d, no Dolby Atmos %v", c.id, c.ask, st, c.quality, c.kind, c.bitRate, c.noAtmos)
		}
		if music.IsAtmos(c.quality) && (u.Query().Get("master") == "" || u.Query().Has("device")) {
			t.Errorf("%s as %s plays %s, want the catalog's playlist", c.id, c.ask, st.URL)
		}
	}
}

// With Dolby Atmos on and Streaming at High Quality, the songs of a page are
// worked out ahead through the wrapper only where they have a Dolby Atmos
// version, as the stereo version the setting asks for; the rest play as the
// web player's AAC, which the wrapper isn't asked about.
func TestDolbyAtmosWorksOutAheadOnlyItsSongs(t *testing.T) {
	rec := &recorder{}
	var ts *testSource
	ts = newTestSource(t, amhelperAnswers(rec, &ts))
	ts.setSettings(map[string]string{"wrapper": "https://wrapper.example"})
	ts.storage.set(qualityKey, music.Ask("high", music.AtmosSpeakers))
	ts.noteAudio("8000", songWith("8000", "atmos", "lossless"))
	ts.noteAudio("8001", songWith("8001", "lossless"))
	ts.noteAudio("8002", songWith("8002", "lossless"))
	ts.prefetch(tracksOf([]string{"8000", "8001", "8002"}), nil)
	ts.clk.settle(t)
	asked := rec.helperQueries("/prepare/")
	if len(asked) != 1 || asked[0].Get("kind") != "downmix" {
		t.Fatalf("asked amhelper %v, want the one song's downmix", asked)
	}
	for _, id := range []string{"8001", "8002"} {
		if found := rec.helperQueries("/find/" + id); len(found) != 0 {
			t.Errorf("read the playlists of %s, which plays as the web player's AAC", id)
		}
	}
}
