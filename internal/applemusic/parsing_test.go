package applemusic

import (
	"encoding/json"
	"net/url"
	"slices"
	"testing"
)

// What the source reads without asking Apple: TTML lyrics, cover templates
// and Apple's resources.

func TestTTML(t *testing.T) {
	ttml := `<tt xmlns="http://www.w3.org/ns/ttml" itunes:timing="Word"><head><metadata><iTunesMetadata><translations><translation type="subtitle" xml:lang="zh-Hans"><text for="L1">你好 世界</text></translation></translations></iTunesMetadata></metadata></head><body><div><p begin="1.5" end="3.000" itunes:key="L1"><span begin="1.5" end="2.0">Hello</span> <span begin="2.0" end="3.0">world</span><span ttm:role="x-bg"><span begin="2.5" end="2.9">(yeah)</span></span></p><p begin="1:02.25" end="1:04">Line &amp; two</p></div></body></tt>`
	parsed := parseTTML(ttml, "zh")
	if parsed == nil || len(parsed.Lines) != 2 {
		t.Fatalf("lines: %+v", parsed)
	}
	l := parsed.Lines[0]
	if l.Start != 1500 || l.End != 3000 || l.Translation != "你好 世界" {
		t.Errorf("first line: %+v", l)
	}
	var words []string
	for _, w := range l.Words {
		words = append(words, w.Text)
	}
	if !slices.Equal(words, []string{"Hello ", "world"}) {
		t.Errorf("words: %q", words)
	}
	if parsed.Lines[1].Start != 62250 || parsed.Lines[1].Text != "Line & two" {
		t.Errorf("second line: %+v", parsed.Lines[1])
	}
	if en := parseTTML(ttml, "en"); en.Lines[0].Translation != "" {
		t.Errorf("a translation into English: %q", en.Lines[0].Translation)
	}
	if parseTTML("<tt><body></body></tt>", "en") != nil {
		t.Error("lyrics from nothing")
	}
}

func TestCovers(t *testing.T) {
	if got := art(&artwork{URL: "https://is1-ssl.mzstatic.com/a/{w}x{h}bb.{f}"}); got != "https://is1-ssl.mzstatic.com/a/{size}x{size}bb.jpg" {
		t.Errorf("art: %s", got)
	}
	if art(nil) != "" {
		t.Error("art of nothing")
	}
	if got := coverName("https://store-030.blobstore.apple.com/sq/51/94/cc/5194CCF7-8d6a-655b-b9e2-00ef5180ed89/image?X-Amz=1"); got != "5194ccf7-8d6a-655b-b9e2-00ef5180ed89" {
		t.Errorf("coverName: %s", got)
	}
	if got := fileURL("/home/a b/cover.jpg"); got != "file:///home/a%20b/cover.jpg" {
		t.Errorf("fileURL: %s", got)
	}
	if got := fileURL(`C:\Users\a\cover.jpg`); got != "file:///C:/Users/a/cover.jpg" {
		t.Errorf("fileURL on Windows: %s", got)
	}
	u, _ := url.Parse("https://is1-ssl.mzstatic.com/image/thumb/a.jpg")
	if h := ImageHeaders(u); h["referer"] != "https://music.apple.com/" || h["user-agent"] != userAgent {
		t.Errorf("mzstatic headers: %v", h)
	}
	u, _ = url.Parse("https://example.com/a.jpg")
	if ImageHeaders(u) != nil {
		t.Error("headers for another host")
	}
}

func resourceOf(t *testing.T, s string) *resource {
	t.Helper()
	var r resource
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

func TestSongs(t *testing.T) {
	s := New(Options{DataDir: t.TempDir()})
	library := s.toTrack(resourceOf(t, `{"id":"i.abc","type":"library-songs","attributes":{"name":"迷星叫","artistName":"MyGO!!!!!","albumName":"迷跡波","durationInMillis":1000,"playParams":{"catalogId":"1783644604"},"url":"https://music.apple.com/jp/album/x/1783644603?i=1783644604"}}`))
	if library.ID != "1783644604" || library.Album.ID != "1783644603" || library.Artists[0].ID != "of:songs:1783644604:MyGO!!!!!" || library.Unavailable {
		t.Errorf("library song: %+v", library)
	}
	gone := s.toTrack(resourceOf(t, `{"id":"i.xyz","type":"library-songs","attributes":{"name":"gone"}}`))
	if !gone.Unavailable {
		t.Error("a song the catalog no longer has plays")
	}
	if n := len(s.songsOf([]*resource{{ID: "1", Type: "music-videos"}})); n != 0 {
		t.Errorf("music videos among songs: %d", n)
	}
	if id := (&Source{}).toAlbum(resourceOf(t, `{"id":"l.abc","type":"library-albums","attributes":{"playParams":{"catalogId":42}}}`)).ID; id != "42" {
		t.Errorf("album by its catalog ID: %s", id)
	}
}

// An artist's page plays their picture moving as Music on the Mac does,
// the wide one, where the fullscreen one was played, which is another
// film; its first frame is the still until it plays.
func TestArtistMotion(t *testing.T) {
	a := toArtist(resourceOf(t, `{"id": "1", "type": "artists", "attributes": {"name": "Paper Sails",
		"artwork": {"url": "https://example.test/img/photo/{w}x{h}bb.{f}"},
		"editorialArtwork": {"bannerUber": {"url": "https://example.test/img/banner/{w}x{h}{c}.{f}"}},
		"editorialVideo": {
			"motionArtistWide16x9": {"video": "https://example.test/hls/wide/master.m3u8", "previewFrame": {"url": "https://example.test/img/wideframe/{w}x{h}{c}.{f}"}},
			"motionArtistFullscreen16x9": {"video": "https://example.test/hls/full/master.m3u8", "previewFrame": {"url": "https://example.test/img/fullframe/{w}x{h}{c}.{f}"}},
			"motionArtistSquare1x1": {"video": "https://example.test/hls/square/master.m3u8"}}}}`))
	if a.VideoURL != "https://example.test/hls/wide/master.m3u8" || a.WideURL != "https://example.test/img/wideframe/{w}x{h}{c}.{f}" {
		t.Errorf("video %q over %q", a.VideoURL, a.WideURL)
	}
}

// An artist known by name is the one named to the letter, whoever a search
// lists first: LiSA and LISA are two artists, and the first found used to
// be taken.
func TestNamedArtistToTheLetter(t *testing.T) {
	artist := func(id, name string) *resource {
		r := &resource{ID: id, Type: "artists"}
		r.Attributes = &attributes{Name: name}
		return r
	}
	found := []*resource{artist("1", "LISA"), nil, artist("2", "LiSA"), artist("3", "Lisa")}
	for name, want := range map[string]string{"LiSA": "2", "LISA": "1", "lisa": "1", "Someone Else": "1"} {
		if got := named(found, name); got == nil || got.ID != want {
			t.Errorf("%q: %+v, want %s", name, got, want)
		}
	}
	if named(nil, "LiSA") != nil {
		t.Error("someone found among none")
	}
	// What the artist of a catalog song is known by, and of one that isn't.
	if got := artistOf("songs", "1538286723", "LiSA").ID; got != "of:songs:1538286723:LiSA" {
		t.Errorf("of a catalog song: %q", got)
	}
	if got := artistOf("songs", "i.abc", "LiSA").ID; got != "name:LiSA" {
		t.Errorf("of an upload: %q", got)
	}
	if typ, of, name, ok := artistOfParts("of:albums:42:A: B"); !ok || typ != "albums" || of != "42" || name != "A: B" {
		t.Errorf("parts %q %q %q %v", typ, of, name, ok)
	}
}
