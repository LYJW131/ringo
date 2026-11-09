package applemusic

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"mygo-music/internal/applemusic/widevine"
	"mygo-music/internal/music"
)

// Playing a song as AAC, against a stand-in for Apple and for amhelper: the
// song is handed over to amhelper with its key and length, and the app is
// given an address that names the song only.

type fakeCDM struct{}

func (fakeCDM) challenge() []byte                   { return []byte("challenge") }
func (fakeCDM) keys([]byte) ([]widevine.Key, error) { return nil, nil }
func newFakeCDM([]byte) (cdm, error)                { return fakeCDM{}, nil }

func TestStream(t *testing.T) {
	const (
		mediaURL    = "https://aod-ssl.itunes.apple.com/itunes-assets/a/mzaf_1.rmhq.aac.wa.mp4"
		playlistURL = "https://aod-ssl.itunes.apple.com/itunes-assets/a/mzaf_1.rphq.aac.wa.m3u8"
	)
	keyURI := "data:text/plain;base64," + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 16))
	playlist := strings.Join([]string{
		"#EXTM3U",
		`#EXT-X-KEY:METHOD=SAMPLE-AES-CTR,KEYFORMAT="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed",URI="` + keyURI + `"`,
		`#EXT-X-MAP:URI="mzaf_1.rmhq.aac.wa.mp4",BYTERANGE="1006@0"`,
		"#EXTINF:9.98458,",
		"#EXT-X-BYTERANGE:342504@1006",
		"mzaf_1.rmhq.aac.wa.mp4",
		"#EXTINF:3.5,",
		"#EXT-X-BYTERANGE:1000@343510",
		"mzaf_1.rmhq.aac.wa.mp4",
	}, "\n")

	type ask struct {
		url  string
		body map[string]any
	}
	var (
		mu           sync.Mutex
		asked        []ask
		helperAnswer = func() *http.Response { return answer(200, `{"ok":true}`) }
	)
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		u := r.URL.String()
		mu.Lock()
		asked = append(asked, ask{u, readBody(r)})
		answerHelper := helperAnswer
		mu.Unlock()
		switch {
		case strings.HasPrefix(u, helperBaseURL+"/aac/"):
			return answerHelper()
		case strings.Contains(u, "webPlayback"):
			return answer(200, map[string]any{"songList": []any{map[string]any{"assets": []any{map[string]any{"flavor": "28:ctrp256", "URL": playlistURL}}}}})
		case u == playlistURL:
			return answer(200, playlist)
		}
		return nil // unexpected
	})
	ts.newCDM = newFakeCDM
	ts.storage.set(userTokenKey, "user")
	// What was kept, read once, as the source starts.
	kept := newOmap[keptSong]()
	kept.set("100", keptSong{mediaURL, strings.Repeat("00", 16), 200.5})
	kept.set("101", keptSong{M: mediaURL, K: strings.Repeat("11", 16)})
	kept.set("102", keptSong{mediaURL, strings.Repeat("22", 16), 100})
	kept.set("103", keptSong{mediaURL + "?old", strings.Repeat("33", 16), 100})
	ts.storage.set(resolvedKey, kept)
	reset := func() {
		ts.clk.settle(t)
		mu.Lock()
		asked = nil
		mu.Unlock()
	}
	keptNow := func(id string) (keptSong, bool) {
		all := newOmap[keptSong]()
		ts.storage.get(resolvedKey, all)
		return all.get(id)
	}

	// A song kept with its length is handed over to amhelper, and the app given its address.
	reset()
	s, err := ts.Stream("100", "high")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*s, music.Stream{URL: helperBaseURL + "/aac/100.m4a", Type: "m4a", Quality: "high", BitRate: 256000}) {
		t.Errorf("stream %+v", s)
	}
	want := []ask{{helperBaseURL + "/aac/100", map[string]any{"media": mediaURL, "key": strings.Repeat("00", 16), "seconds": 200.5}}}
	if !reflect.DeepEqual(asked, want) {
		t.Errorf("asked %+v", asked)
	}
	if strings.Contains(s.URL, strings.Repeat("00", 16)) {
		t.Error("the key is in the address")
	}
	if len(ts.started) != 1 {
		t.Errorf("amhelper started %d times", len(ts.started))
	}

	// A song kept without its length has its playlist read again, and keeps its key.
	reset()
	if _, err := ts.Stream("101", "high"); err != nil {
		t.Fatal(err)
	}
	var urls []string
	for _, a := range asked {
		urls = append(urls, strings.Split(a.url, "?")[0])
	}
	if !reflect.DeepEqual(urls, []string{playbackURL, playlistURL, helperBaseURL + "/aac/101"}) {
		t.Errorf("a license was asked for: %q", urls)
	}
	if !reflect.DeepEqual(asked[2].body, map[string]any{"media": mediaURL, "key": strings.Repeat("11", 16), "seconds": 13.48458}) {
		t.Errorf("handed over %v", asked[2].body)
	}
	if k, _ := keptNow("101"); k != (keptSong{mediaURL, strings.Repeat("11", 16), 13.48458}) {
		t.Errorf("kept %+v", k)
	}

	// amhelper failing on a song is an error, not a song asked for afresh.
	mu.Lock()
	helperAnswer = func() *http.Response { return answer(502, map[string]string{"code": "failed", "message": "boom"}) }
	mu.Unlock()
	reset()
	if _, err := ts.Stream("102", "high"); err == nil || err.Error() != "amhelper: boom" {
		t.Errorf("error %v", err)
	}
	if len(asked) != 1 {
		t.Errorf("the song was asked for afresh: %+v", asked)
	}

	// A kept song whose file Apple no longer has is asked for afresh.
	mu.Lock()
	helperAnswer = func() *http.Response {
		return answer(502, map[string]string{"code": "media", "message": "media: HTTP 404"})
	}
	mu.Unlock()
	reset()
	// The stand-in has no license server: the request for one ends it.
	if _, err := ts.Stream("103", "high"); err == nil || !strings.Contains(err.Error(), "acquireWebPlaybackLicense") {
		t.Errorf("error %v", err)
	}
	afresh := false
	for _, a := range asked {
		afresh = afresh || strings.Contains(a.url, "webPlayback")
	}
	if !afresh {
		t.Error("not asked for afresh")
	}
	if _, ok := keptNow("103"); ok {
		t.Error("still kept")
	}
}
