package amhelper

// amhelper end to end against fakeapple, which stands in for Apple's CDN and
// a wrapper: what comes out is a fragmented MP4 whose samples are, one for
// one, the same audio unencrypted. No account, no network.

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/mp4"

	"ringo/internal/applemusic/amhelper/fakeapple"
)

// samplesOf parses a fragmented MP4 and returns its header, its box layout
// and every sample in order.
func samplesOf(t *testing.T, name string, file []byte) (*mp4.File, [][]byte) {
	t.Helper()
	f, err := mp4.DecodeFile(bytes.NewReader(file))
	if err != nil {
		t.Fatalf("%s: not an MP4: %v", name, err)
	}
	if f.Ftyp == nil || f.Moov == nil || !f.IsFragmented() || len(f.Segments) == 0 {
		t.Fatalf("%s: want ftyp, moov and moof/mdat fragments", name)
	}
	var out [][]byte
	for _, seg := range f.Segments {
		for _, frag := range seg.Fragments {
			if frag.Moof == nil || frag.Mdat == nil {
				t.Fatalf("%s: a fragment without moof or mdat", name)
			}
			ss, err := frag.GetFullSamples(f.Moov.Mvex.Trex)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, s := range ss {
				out = append(out, bytes.Clone(s.Data))
			}
		}
	}
	return f, out
}

func get(t *testing.T, url string) []byte {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d %v %.200s", url, res.StatusCode, err, b)
	}
	return b
}

func sameSamples(t *testing.T, name string, got, want [][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d samples, want %d", name, len(got), len(want))
	}
	for i := range got {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("%s: sample %d differs from the unencrypted song's", name, i)
		}
	}
}

func TestFakeAppleEndToEnd(t *testing.T) {
	opts := fakeapple.Options{Seconds: 20}
	handler, err := fakeapple.NewHandler(opts)
	if err != nil {
		t.Fatal(err)
	}
	apple := httptest.NewServer(handler)
	defer apple.Close()

	s, err := Start(Config{Wrapper: apple.URL, Apple: apple.URL, Cache: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !strings.HasPrefix(s.Base(), "http://127.0.0.1:") {
		t.Fatalf("Base() = %q", s.Base())
	}

	t.Run("aac", func(t *testing.T) {
		// What the caller hands over: the song's file on Apple's CDN (the
		// stand-in answers for it), its key and its length.
		body, _ := json.Marshal(map[string]any{
			"media":   "https://aod-ssl.itunes.apple.com/cdn/1/aac.mp4",
			"key":     hex.EncodeToString(fakeapple.AACKey()),
			"seconds": opts.AACSeconds(),
		})
		res, err := http.Post(s.Base()+"/aac/1440000001", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		reply, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("hand over: %d %s", res.StatusCode, reply)
		}
		got, samples := samplesOf(t, "aac/1440000001.m4a", get(t, s.Base()+"/aac/1440000001.m4a"))
		_, want := samplesOf(t, "aac-plain.mp4", get(t, apple.URL+"/cdn/1/aac-plain.mp4"))
		sameSamples(t, "aac", samples, want)
		if len(want) != opts.AACPackets() {
			t.Fatalf("%d samples, want %d", len(want), opts.AACPackets())
		}
		trak := got.Moov.Trak
		if got.Moov.IsEncrypted(trak.Tkhd.TrackID) {
			t.Fatal("the header still says the track is encrypted")
		}
		if d, w := trak.Mdia.Mdhd.Duration, uint64(opts.AACPackets()*1024); d != w {
			t.Fatalf("track length %d, want %d", d, w)
		}
		if trak.Edts == nil || trak.Edts.Elst[0].Entries[0].MediaTime != aacPriming {
			t.Fatal("no edit list skipping the priming")
		}
	})

	t.Run("alac", func(t *testing.T) {
		// Through the stand-in wrapper: its playlist, its key template, and
		// FairPlay's white-box decrypting Temari's published sample.
		got, samples := samplesOf(t, "song/1440000002.m4a", get(t, s.Base()+"/song/1440000002.m4a"))
		_, want := samplesOf(t, "plain.m4a", get(t, apple.URL+"/cdn/1/plain.m4a"))
		sameSamples(t, "alac", samples, want)
		if len(want) != opts.ALACPackets() {
			t.Fatalf("%d samples, want %d", len(want), opts.ALACPackets())
		}
		if e := got.Moov.Trak.Mdia.Minf.Stbl.Stsd.Children[0].Type(); e != "alac" {
			t.Fatalf("sample entry %s, want alac", e)
		}
	})
}

// Close ends the listener and the requests in flight.
func TestServerClose(t *testing.T) {
	s, err := Start(Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Without a wrapper there is AAC and covers, and a way to say so.
	res, err := http.Get(s.Base() + "/status")
	if err != nil {
		t.Fatal(err)
	}
	var e errorBody
	_ = json.NewDecoder(res.Body).Decode(&e)
	res.Body.Close()
	if res.StatusCode != http.StatusBadGateway || e.Code != "wrapper" {
		t.Fatalf("status without a wrapper: %d %+v", res.StatusCode, e)
	}
	// No secret, no answer.
	hostPort := strings.SplitN(strings.TrimPrefix(s.Base(), "http://"), "/", 2)[0]
	res, err = http.Get("http://" + hostPort + "/status")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("a path without the secret: %d", res.StatusCode)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := http.Get(s.Base() + "/status"); err == nil {
		t.Fatal("still listening after Close")
	}
}
