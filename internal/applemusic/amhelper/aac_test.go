package amhelper

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/mp4"
)

var (
	testKey = bytes.Repeat([]byte{0x3c}, 16)
	testIV  = []byte{1, 2, 3, 4, 5, 6, 7, 8}
)

// webPlayerLike is an MP4 laid out as the web player's AAC: one track of
// AAC at 44.1 kHz, CENC with 8-byte IVs in each fragment's senc, fragments
// of 10 seconds, and no length in the header.
func webPlayerLike(t *testing.T) ([]byte, [][]byte) {
	t.Helper()
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(44100, "audio", "und")
	trak := init.Moov.Trak
	if err := trak.SetAACDescriptor(aac.AAClc, 44100); err != nil {
		t.Fatal(err)
	}
	kid, _ := mp4.NewUUIDFromString("00112233-4455-6677-8899-aabbccddeeff")
	ipd, err := mp4.InitProtect(init, testKey, testIV, "cenc", kid, nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := init.Encode(&buf); err != nil {
		t.Fatal(err)
	}
	var samples [][]byte
	var decodeTime uint64
	for n := range 3 {
		frag, err := mp4.CreateFragment(uint32(n+1), trak.Tkhd.TrackID)
		if err != nil {
			t.Fatal(err)
		}
		for i := range 430 { // 10 s of 1024-sample packets
			data := bytes.Repeat([]byte{byte(i), byte(n), 0x21}, 100+i%60)
			samples = append(samples, bytes.Clone(data))
			frag.AddFullSample(mp4.FullSample{
				Sample:     mp4.NewSample(mp4.SyncSampleFlags, 1024, uint32(len(data)), 0),
				DecodeTime: decodeTime,
				Data:       data,
			})
			decodeTime += 1024
		}
		if _, err := mp4.EncryptFragment(frag, testKey, testIV, ipd); err != nil {
			t.Fatal(err)
		}
		if err := frag.Encode(&buf); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes(), samples
}

func TestDecryptStreamCENC(t *testing.T) {
	in, plain := webPlayerLike(t)
	if bytes.Contains(in, plain[5]) {
		t.Fatal("the test's file isn't encrypted")
	}
	key, err := newCENCKey(testKey)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	seconds := float64(len(plain)*1024) / 44100
	err = decryptStream(bytes.NewReader(in), &out, func(int, bool) (sampleDecrypter, error) { return key, nil },
		streamOptions{seconds: seconds, prime: aacPriming, flush: func() {}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := mp4.DecodeFile(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	trak := f.Init.Moov.Trak
	if entry := trak.Mdia.Minf.Stbl.Stsd.Children[0]; entry.Type() != "mp4a" {
		t.Errorf("sample entry %s, want mp4a", entry.Type())
	}
	if f.Init.Moov.IsEncrypted(trak.Tkhd.TrackID) {
		t.Error("the header still says the track is encrypted")
	}
	if got, want := trak.Mdia.Mdhd.Duration, uint64(len(plain)*1024); got != want {
		t.Errorf("track length %d, want %d", got, want)
	}
	if trak.Edts == nil || len(trak.Edts.Elst) != 1 || trak.Edts.Elst[0].Entries[0].MediaTime != aacPriming {
		t.Fatalf("no edit list skipping the priming: %+v", trak.Edts)
	}
	// The edit list counts in the movie's timescale.
	movie := uint64(f.Init.Moov.Mvhd.Timescale)
	if got, want := trak.Edts.Elst[0].Entries[0].SegmentDuration, uint64(len(plain)*1024-aacPriming)*movie/44100; got != want {
		t.Errorf("edit of %d, want %d", got, want)
	}
	i := 0
	for _, seg := range f.Segments {
		for _, frag := range seg.Fragments {
			if frag.Moof.Traf.Senc != nil {
				t.Fatal("a fragment kept its senc")
			}
			ss, err := frag.GetFullSamples(f.Init.Moov.Mvex.Trex)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range ss {
				if !bytes.Equal(s.Data, plain[i]) {
					t.Fatalf("sample %d isn't decrypted", i)
				}
				i++
			}
		}
	}
	if i != len(plain) {
		t.Fatalf("%d samples, want %d", i, len(plain))
	}
}

// Subsamples leave their clear bytes alone, the counter running on across
// the encrypted ones, as mp4ff encrypts them.
func TestCENCKeySubsamples(t *testing.T) {
	plain := make([]byte, 300)
	for i := range plain {
		plain[i] = byte(i * 7)
	}
	subs := []mp4.SubSamplePattern{{BytesOfClearData: 10, BytesOfProtectedData: 100}, {BytesOfClearData: 40, BytesOfProtectedData: 150}}
	iv := append(bytes.Clone(testIV), make([]byte, 8)...)
	enc := bytes.Clone(plain)
	if err := mp4.CryptSampleCenc(enc, testKey, iv, subs); err != nil {
		t.Fatal(err)
	}
	key, _ := newCENCKey(testKey)
	if err := key.DecryptSample(enc, sampleEncryption{iv: testIV, subs: subs}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(enc, plain) {
		t.Fatal("not decrypted")
	}
	if err := key.DecryptSample(make([]byte, 100), sampleEncryption{iv: testIV, subs: subs}); err == nil {
		t.Fatal("subsamples past the sample's end were taken")
	}
}

// fakeCDN answers every request with files[url], or 404.
type fakeCDN map[string][]byte

func (c fakeCDN) RoundTrip(r *http.Request) (*http.Response, error) {
	b, ok := c[r.URL.String()]
	if !ok {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}
	return &http.Response{StatusCode: 200, ContentLength: int64(len(b)), Body: io.NopCloser(bytes.NewReader(b)), Request: r, Header: http.Header{}}, nil
}

func TestAACHandOverAndServe(t *testing.T) {
	in, plain := webPlayerLike(t)
	const media = "https://aod-ssl.itunes.apple.com/itunes-assets/x/song.mp4"
	s := &server{cdn: &http.Client{Transport: fakeCDN{media: in}}, aac: newLRU[aacSong](aacKept), aacOpened: map[string]*openedMedia{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /aac/{id}", s.aacRegister)
	mux.HandleFunc("GET /aac/{file}", s.aacSongFile)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	hand := func(id, media, key string) (int, errorBody) {
		body, _ := json.Marshal(map[string]any{"media": media, "key": key, "seconds": 30.0})
		res, err := http.Post(srv.URL+"/aac/"+id, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var e errorBody
		_ = json.NewDecoder(res.Body).Decode(&e)
		return res.StatusCode, e
	}
	key := hex.EncodeToString(testKey)
	if code, e := hand("123", "https://example.com/song.mp4", key); code != 400 {
		t.Errorf("a file not Apple's: %d %v", code, e)
	}
	if code, e := hand("123", media, "abcd"); code != 400 {
		t.Errorf("a short key: %d %v", code, e)
	}
	if code, e := hand("123", media+"?moved", key); code != 502 || e.Code != "media" {
		t.Errorf("a file Apple moved: %d %v, want 502 media", code, e)
	}
	if res, _ := http.Get(srv.URL + "/aac/123.m4a"); res.StatusCode != 404 {
		t.Errorf("a song not handed over: %d", res.StatusCode)
	}
	if code, e := hand("123", media, key); code != 200 {
		t.Fatalf("hand over: %d %v", code, e)
	}
	if len(s.aacOpened) != 1 {
		t.Fatal("the file wasn't opened as the song was handed over")
	}
	for range 2 { // the second time without the opened file
		res, err := http.Get(srv.URL + "/aac/123.m4a")
		if err != nil {
			t.Fatal(err)
		}
		out, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || res.Header.Get("Content-Type") != "audio/mp4" {
			t.Fatalf("song: %d %s", res.StatusCode, out)
		}
		f, err := mp4.DecodeFile(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		ss, _ := f.Segments[0].Fragments[0].GetFullSamples(f.Init.Moov.Mvex.Trex)
		if !bytes.Equal(ss[0].Data, plain[0]) {
			t.Fatal("the song isn't decrypted")
		}
	}
}

// trackedBody notes whether it was closed.
type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestExpiryLeavesAClaimedFileOpen(t *testing.T) {
	s := &server{aacOpened: map[string]*openedMedia{}}
	opened := func() (*openedMedia, *trackedBody) {
		body := &trackedBody{Reader: strings.NewReader("")}
		return &openedMedia{res: &http.Response{Body: body}, cancel: func() {}}, body
	}

	// Not asked for in time: closed.
	o, body := opened()
	s.aacOpened["1"] = o
	s.expireOpened("1", o)
	if !body.closed || s.aacOpened["1"] != nil {
		t.Fatal("a file nobody asked for was left open")
	}

	// The timer firing just after the app's request took the file.
	o, body = opened()
	s.aacOpened["1"] = o
	delete(s.aacOpened, "1") // as aacSongFile does
	s.expireOpened("1", o)
	if body.closed {
		t.Fatal("a file the app was playing was closed under it")
	}

	// Handed over again since: the newer file stays.
	o, body = opened()
	newer, newerBody := opened()
	s.aacOpened["1"] = newer
	s.expireOpened("1", o)
	if body.closed || newerBody.closed || s.aacOpened["1"] != newer {
		t.Fatal("expiring an old file touched the newer one")
	}
}
