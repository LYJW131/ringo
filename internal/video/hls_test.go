package video

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// A master playlist as Apple's are, cut short: H.264 and HEVC at several
// sizes, some at more than one bandwidth.
const master = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=893943,CODECS="avc1.64001f",RESOLUTION=486x486,URI="iframe_486.m3u8"
#EXT-X-STREAM-INF:AVERAGE-BANDWIDTH=2067878,BANDWIDTH=2120834,VIDEO-RANGE=SDR,CODECS="avc1.64001f",FRAME-RATE=30.000,RESOLUTION=768x768
v768.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=318785,CODECS="hvc1.2.20000000.L123.B0",FRAME-RATE=30.000,RESOLUTION=360x360
h360.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=265024,CODECS="avc1.64001f",FRAME-RATE=30.000,RESOLUTION=360x360
v360.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1156729,CODECS="avc1.64001f",FRAME-RATE=30.000,RESOLUTION=486x486
v486b.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=796401,CODECS="avc1.64001f",FRAME-RATE=30.000,RESOLUTION=486x486
v486a.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=4052768,CODECS="avc1.640020",FRAME-RATE=30.000,RESOLUTION=1080x1080
https://cdn.example.test/v1080.m3u8
`

// The variant shown is the smallest H.264 one that covers the place, at
// its least bandwidth, never one over the cap: the biggest under it when
// none covers the place.
func TestPick(t *testing.T) {
	vs, err := ParseMaster("https://example.test/a/master.m3u8", []byte(master))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 6 || vs[0].URI != "https://example.test/a/v768.m3u8" || vs[5].URI != "https://cdn.example.test/v1080.m3u8" {
		t.Fatalf("variants %+v", vs)
	}
	for _, c := range []struct {
		w, h, maxW, maxH int
		want             string
	}{
		{300, 300, 486, 486, "v360.m3u8"},
		{400, 400, 486, 486, "v486a.m3u8"},
		{600, 600, 486, 486, "v486a.m3u8"},
		{600, 600, 2000, 2000, "v768.m3u8"},
		{2000, 2000, 4000, 4000, "v1080.m3u8"},
		{100, 100, 200, 200, ""},
	} {
		v, ok := Pick(vs, c.w, c.h, c.maxW, c.maxH)
		got := filepath.Base(v.URI)
		if !ok {
			got = ""
		}
		if got != c.want {
			t.Errorf("%dx%d under %dx%d: %q, want %q", c.w, c.h, c.maxW, c.maxH, got, c.want)
		}
	}
}

// A media playlist of byte ranges of one file is that file, its map and
// segments in it, fetched once; one of files of their own is all of them,
// in order.
func TestMediaFiles(t *testing.T) {
	one := `#EXTM3U
#EXT-X-TARGETDURATION:5
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MAP:URI="P1_video_768x768_-.mp4",BYTERANGE="895@0"
#EXTINF:4.80000,
#EXT-X-BYTERANGE:1035045@895
P1_video_768x768_-.mp4
#EXTINF:4.80000,
#EXT-X-BYTERANGE:1030000@1035940
P1_video_768x768_-.mp4
#EXT-X-ENDLIST
`
	got, err := mediaFiles("https://example.test/a/v768.m3u8", []byte(one))
	if err != nil || !slices.Equal(got, []string{"https://example.test/a/P1_video_768x768_-.mp4"}) {
		t.Errorf("one file: %q, %v", got, err)
	}
	many := "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:2,\nseg1.m4s\n#EXTINF:2,\nseg2.m4s\n"
	got, err = mediaFiles("https://example.test/a/v.m3u8", []byte(many))
	if err != nil || !slices.Equal(got, []string{"https://example.test/a/init.mp4", "https://example.test/a/seg1.m4s", "https://example.test/a/seg2.m4s"}) {
		t.Errorf("files of their own: %q, %v", got, err)
	}
	if _, err := mediaFiles("https://example.test/a/v.m3u8", []byte("#EXTM3U\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"skd://x\"\n#EXTINF:2,\nseg1.m4s\n")); err == nil {
		t.Error("an encrypted stream fetched")
	}
}

// Past its limit the cache lets go of what was used longest ago.
func TestCacheTrims(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 250)
	c.Put("a", make([]byte, 100))
	c.Put("b", make([]byte, 100))
	old := time.Now().Add(-time.Hour)
	os.Chtimes(c.path("a"), old, old)
	os.Chtimes(c.path("b"), old.Add(time.Minute), old.Add(time.Minute))
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a kept")
	}
	c.Put("c", make([]byte, 100))
	_, a := c.Get("a")
	_, b := c.Get("b")
	_, cc := c.Get("c")
	if !a || b || !cc {
		t.Errorf("kept a %v, b %v, c %v", a, b, cc)
	}
}
