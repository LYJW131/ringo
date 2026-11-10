package applemusic

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
	mygomp4 "github.com/egoist/mygo/audio/mp4"

	_ "mygo-music/internal/heaac"
	"mygo-music/internal/music"
)

// Against the real Apple Music, so not run by default: LIVE=1 signed out
// (search, albums, artists, playlists and charts), and with
// APPLE_MUSIC_TOKEN_FILE, a file holding a media-user-token, signed in
// (account, library, lyrics, and one song played whole through amhelper).
// The token is never printed.

func sections(b *music.Browse) string {
	var out []string
	for _, s := range b.Sections {
		n := len(s.Tracks) + len(s.Albums) + len(s.Playlists) + len(s.Stations) + len(s.Items)
		out = append(out, fmt.Sprintf("%s (%d)", s.Title, n))
	}
	return strings.Join(out, ", ")
}

// searchFor is what of kind Apple Music's search finds for term, as its
// page lists it, and fails the test if it finds none.
func searchFor(t *testing.T, s *Source, term, kind string) []music.Item {
	t.Helper()
	b, err := s.Browse(music.PageSearch + term)
	if err != nil {
		t.Fatal(err)
	}
	var out []music.Item
	for _, sec := range b.Sections {
		for _, it := range sec.Items {
			if it.Kind == kind {
				out = append(out, it)
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no %s found for %q", kind, term)
	}
	return out
}

func TestLiveSignedOut(t *testing.T) {
	if os.Getenv("LIVE") != "1" {
		t.Skip("LIVE=1 asks the real Apple Music")
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	songs := searchFor(t, s, "MyGO!!!!! 迷星叫", music.ItemSong)
	song := *songs[0].Track
	t.Logf("song: %s %s / %s / %s %s", song.ID, song.Name, song.Artists[0].Name, song.Album.Name, song.Album.ImageURL)

	albums := searchFor(t, s, "MyGO!!!!!", music.ItemAlbum)
	album, err := s.Album(albums[0].ID)
	must(err)
	t.Logf("album: %s, %d tracks", album.Album.Name, len(album.Tracks))
	if len(album.Tracks) == 0 {
		t.Error("an album without tracks")
	}

	artists := searchFor(t, s, "MyGO!!!!!", music.ItemArtist)
	artist, err := s.Artist(artists[0].ID)
	must(err)
	t.Logf("artist: %s, %d top songs, %d albums", artist.Artist.Name, len(artist.Tracks), len(artist.Albums))
	byName, err := s.Artist(song.Album.Artists[0].ID)
	must(err)
	t.Logf("artist by name: %s", byName.Artist.Name)

	lists := searchFor(t, s, "anime", music.ItemPlaylist)
	list, err := s.Playlist(lists[0].ID)
	must(err)
	t.Logf("playlist: %s, %d tracks", list.Name, len(list.Tracks))

	suggest, err := s.Suggest("mygo")
	must(err)
	t.Logf("suggest: %q", suggest)
	home, err := s.Home()
	must(err)
	t.Logf("home: %s", sections(home))
	if len(home.Sections) == 0 {
		t.Error("an empty home")
	}
	charts, err := s.Browse("charts")
	must(err)
	t.Logf("charts: %s", sections(charts))
	if len(charts.Sections) == 0 {
		t.Error("no charts")
	}
}

func TestLiveSignedIn(t *testing.T) {
	file := os.Getenv("APPLE_MUSIC_TOKEN_FILE")
	if file == "" {
		t.Skip("APPLE_MUSIC_TOKEN_FILE names a file holding a media-user-token")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	id := os.Getenv("APPLE_MUSIC_SONG")
	if id == "" {
		id = "1783644604"
	}
	step := func(name string, fn func() (string, error)) {
		t0 := time.Now()
		r, err := fn()
		if err != nil {
			t.Errorf("FAIL %s: %v", name, err)
			return
		}
		t.Logf("ok   %s (%d ms): %s", name, time.Since(t0).Milliseconds(), r)
	}
	step("account", func() (string, error) {
		a, err := s.Account()
		if err != nil || a == nil {
			return "", fmt.Errorf("signed out: %v", err)
		}
		// Who it is stays out of the test's output.
		return fmt.Sprintf("signed in, with a name: %v", a.Name != ""), nil
	})
	play := func() (string, error) {
		st, err := s.Stream(id, "high")
		if err != nil {
			return "", err
		}
		res, err := http.Get(st.URL)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return "", err
		}
		if res.StatusCode != 200 || len(b) < 100_000 || !strings.Contains(string(b[:64]), "ftyp") {
			return "", fmt.Errorf("HTTP %d, %d bytes", res.StatusCode, len(b))
		}
		return fmt.Sprintf("%s, %.1f MB", st.Type, float64(len(b))/1e6), nil
	}
	step("stream (amhelper)", play)
	step("stream again (key kept)", play)
	step("lyrics", func() (string, error) {
		l, err := s.Lyrics(id)
		if err != nil || l == nil {
			return "none", err
		}
		translated := 0
		for _, x := range l.Lines {
			if x.Translation != "" {
				translated++
			}
		}
		return fmt.Sprintf("%d lines, words: %v, translation: %d", len(l.Lines), l.Lines[0].Words != nil, translated), nil
	})
	step("recent", func() (string, error) { r, err := s.Recent(); return fmt.Sprint(len(r), " tracks"), err })
	step("liked", func() (string, error) {
		r, err := s.Liked(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d tracks, more %v", len(r.Tracks), r.More), nil
	})
	step("isLiked", func() (string, error) { r, err := s.IsLiked([]string{id}); return fmt.Sprint(r), err })
	step("playlists", func() (string, error) {
		p, err := s.Playlists()
		if err != nil {
			return "", err
		}
		out := fmt.Sprint(len(p), " playlists")
		if len(p) > 0 {
			l, err := s.Playlist(p[0].ID)
			if err != nil {
				return "", err
			}
			out += fmt.Sprintf("; the first: %d tracks", len(l.Tracks))
		}
		return out, nil
	})
	step("home", func() (string, error) { h, err := s.Home(); return sectionsOr(h), err })
	step("recentlyAdded", func() (string, error) { h, err := s.Browse(music.PageRecentlyAdded); return sectionsOr(h), err })
	// Nothing is reported as played: the test leaves the account's listening
	// history as it found it.
}

// Asked of the library's first five songs and five found by a search that
// the library doesn't have (as each one's own resource says), all at once,
// InLibrary says the first are in it and the others aren't. Only counts are
// logged: what's in the library would say whose it is.
func TestLiveInLibrary(t *testing.T) {
	file := os.Getenv("APPLE_MUSIC_TOKEN_FILE")
	if file == "" {
		t.Skip("APPLE_MUSIC_TOKEN_FILE names a file holding a media-user-token")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))

	lib, err := s.me("/library/songs", params{"limit": "25"}, reqOpts{})
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]bool{}
	var ids []string
	for _, r := range lib.Data {
		if id := songID(r); isCatalogID(id) && len(ids) < 5 {
			in[id] = true
			ids = append(ids, id)
		}
	}
	var out []string
	found := searchFor(t, s, "lullaby music box", music.ItemSong)
	for _, it := range found {
		tr := it.Track
		if len(out) == 5 {
			break
		}
		if !isCatalogID(tr.ID) || in[tr.ID] {
			continue
		}
		if own, _, err := s.personal("songs", tr.ID); err != nil || own != nil {
			continue
		}
		out = append(out, tr.ID)
	}
	if len(ids) < 5 || len(out) < 5 {
		t.Fatalf("%d songs of the library and %d not in it to ask about, of %d found", len(ids), len(out), len(found))
	}
	start := time.Now()
	got, err := s.InLibrary(music.ItemSong, append(slices.Clone(ids), out...))
	if err != nil {
		t.Fatal(err)
	}
	right := 0
	for _, id := range ids {
		if got[id] {
			right++
		}
	}
	for _, id := range out {
		if !got[id] {
			right++
		}
	}
	t.Logf("%d of 10 right, in %d ms", right, time.Since(start).Milliseconds())
	if right != 10 {
		t.Errorf("%d of 10 right", right)
	}
}

// sectionsOr is a signed-in page's shape, without its titles, which can
// name the listener.
func sectionsOr(b *music.Browse) string {
	if b == nil {
		return ""
	}
	return shape(b)
}

// With APPLE_MUSIC_WRAPPER too, the address of a wrapper that holds an
// account, a song asked for at Lossless and at Hi-Res Lossless comes as ALAC
// through it, rather than falling back to AAC: Lossless within 48 kHz, and
// Hi-Res Lossless at no lower a rate than that.
func TestLiveLossless(t *testing.T) {
	file, wrapper := os.Getenv("APPLE_MUSIC_TOKEN_FILE"), os.Getenv("APPLE_MUSIC_WRAPPER")
	if file == "" || wrapper == "" {
		t.Skip("APPLE_MUSIC_TOKEN_FILE names a file holding a media-user-token, and APPLE_MUSIC_WRAPPER a wrapper")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	s.settings.set("wrapper", wrapper)
	id := os.Getenv("APPLE_MUSIC_SONG")
	if id == "" {
		id = "1783644604"
	}
	streams := map[string]*music.Stream{}
	for _, level := range []string{"lossless", "hires"} {
		t0 := time.Now()
		st, err := s.Stream(id, level)
		if err != nil {
			t.Fatal(err)
		}
		if st.Quality != level {
			t.Fatalf("the song asked for at %s came as %q", level, st.Quality)
		}
		streams[level] = st
		res, err := http.Get(st.URL)
		if err != nil {
			t.Fatal(err)
		}
		// The start of it is enough to tell it's there: a whole song is tens of megabytes.
		b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 || len(b) < 100_000 || !strings.Contains(string(b[:64]), "ftyp") {
			t.Fatalf("%s: HTTP %d, %d bytes", level, res.StatusCode, len(b))
		}
		t.Logf("%s in %d ms: %d Hz, %d bit (served as %s Hz, %s bit)", level, time.Since(t0).Milliseconds(), st.SampleRate, st.BitDepth,
			res.Header.Get("X-Sample-Rate"), res.Header.Get("X-Bit-Depth"))
	}
	if r := streams["lossless"].SampleRate; r <= 0 || r > 48000 {
		t.Errorf("Lossless came at %d Hz, want up to 48000", r)
	}
	if hi, lo := streams["hires"].SampleRate, streams["lossless"].SampleRate; hi < lo {
		t.Errorf("Hi-Res Lossless came at %d Hz, below Lossless's %d", hi, lo)
	}
}

// With APPLE_MUSIC_WRAPPER too, High Efficiency and a song's Dolby Atmos
// version, as each of its stereo versions, come through the wrapper as what
// they are, and the player's decoder plays three seconds of each, not
// silent. A song without a Dolby Atmos version, asked for with it on, plays
// at the level and says so.
func TestLiveQualities(t *testing.T) {
	file, wrapper := os.Getenv("APPLE_MUSIC_TOKEN_FILE"), os.Getenv("APPLE_MUSIC_WRAPPER")
	if file == "" || wrapper == "" {
		t.Skip("APPLE_MUSIC_TOKEN_FILE names a file holding a media-user-token, and APPLE_MUSIC_WRAPPER a wrapper")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	s.settings.set("wrapper", wrapper)
	atmos, plain := os.Getenv("APPLE_MUSIC_ATMOS_SONG"), os.Getenv("APPLE_MUSIC_SONG")
	if atmos == "" {
		atmos = "1472563571"
	}
	if plain == "" {
		plain = "1783644604"
	}
	for _, c := range []struct {
		id, ask, want string
		rate          int // what the player decodes at
	}{
		{plain, "efficient", "efficient", 0},
		{atmos, music.Ask("high", music.AtmosHeadphones), "atmos-binaural", 48000},
		{atmos, music.Ask("hires", music.AtmosSpeakers), "atmos-downmix", 48000},
	} {
		t0 := time.Now()
		st, err := s.Stream(c.id, c.ask)
		if err != nil {
			t.Fatalf("%s as %s: %v", c.id, c.ask, err)
		}
		if st.Quality != c.want || st.NoAtmos {
			t.Fatalf("%s as %s came as %q (no Dolby Atmos %v)", c.id, c.ask, st.Quality, st.NoAtmos)
		}
		res, err := http.Get(st.URL)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("%s: HTTP %d, %v", c.want, res.StatusCode, err)
		}
		entry, channels, asc := sampleEntry(t, b)
		d, err := mygomp4.NewDecoder(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: the player's decoder: %v", c.want, err)
		}
		pcm := make([]float32, 4096)
		peak, frames := float32(0), 0
		for frames < 3*d.SampleRate() {
			n, err := d.Read(pcm)
			for _, v := range pcm[:n] {
				peak = max(peak, v, -v)
			}
			frames += n / 2
			if err != nil {
				break
			}
		}
		t.Logf("%s as %s in %d ms: %s, %s %d ch, ASC %x, %d kbps; decoded %.1f s at %d Hz, peak %.2f", c.id, c.ask, time.Since(t0).Milliseconds(),
			st.Quality, entry, channels, asc, st.BitRate/1000, float64(frames)/float64(d.SampleRate()), d.SampleRate(), peak)
		if entry != "mp4a" || channels != 2 || st.BitRate <= 0 {
			t.Errorf("%s: %s of %d channels at %d bit/s, want stereo AAC", c.want, entry, channels, st.BitRate)
		}
		if frames < 3*d.SampleRate() || peak == 0 || (c.rate > 0 && d.SampleRate() != c.rate) {
			t.Errorf("%s: %d frames at %d Hz, peak %v", c.want, frames, d.SampleRate(), peak)
		}
	}
	st, err := s.Stream("1555813962", music.Ask("lossless", music.AtmosHeadphones))
	if err != nil {
		t.Fatal(err)
	}
	if st.Quality != "lossless" || !st.NoAtmos {
		t.Errorf("a song without Dolby Atmos, asked for with it on, came as %q, no Dolby Atmos %v", st.Quality, st.NoAtmos)
	}
}

// sampleEntry is what an MP4's first track's sample entry says: its type,
// channels and, of AAC, its AudioSpecificConfig.
func sampleEntry(t *testing.T, file []byte) (string, int, []byte) {
	t.Helper()
	// Only the header: the file is cut short.
	r := bytes.NewReader(file)
	var moov *mp4.MoovBox
	for pos := uint64(0); moov == nil; {
		box, err := mp4.DecodeBox(pos, r)
		if err != nil {
			t.Fatalf("not an MP4: %v", err)
		}
		pos += box.Size()
		moov, _ = box.(*mp4.MoovBox)
	}
	entry := moov.Traks[0].Mdia.Minf.Stbl.Stsd.Children[0]
	ae, ok := entry.(*mp4.AudioSampleEntryBox)
	if !ok {
		return entry.Type(), 0, nil
	}
	var asc []byte
	if ae.Esds != nil && ae.Esds.DecConfigDescriptor.DecSpecificInfo != nil {
		asc = ae.Esds.DecConfigDescriptor.DecSpecificInfo.DecConfig
	}
	return entry.Type(), int(ae.ChannelCount), asc
}
