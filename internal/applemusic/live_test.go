package applemusic

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

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
		n := len(s.Tracks) + len(s.Albums) + len(s.Playlists) + len(s.Stations)
		out = append(out, fmt.Sprintf("%s (%d)", s.Title, n))
	}
	return strings.Join(out, ", ")
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

	songs, err := s.Search("MyGO!!!!! 迷星叫", music.SearchTracks, 0)
	must(err)
	if len(songs.Tracks) == 0 {
		t.Fatal("no songs")
	}
	song := songs.Tracks[0]
	t.Logf("song: %s %s / %s / %s %s", song.ID, song.Name, song.Artists[0].Name, song.Album.Name, song.Album.ImageURL)
	next, err := s.Search("MyGO!!!!!", music.SearchTracks, 25)
	must(err)
	t.Logf("page 2: %d more %v", len(next.Tracks), next.More)

	albums, err := s.Search("MyGO!!!!!", music.SearchAlbums, 0)
	must(err)
	album, err := s.Album(albums.Albums[0].ID)
	must(err)
	t.Logf("album: %s, %d tracks", album.Album.Name, len(album.Tracks))
	if len(album.Tracks) == 0 {
		t.Error("an album without tracks")
	}

	artists, err := s.Search("MyGO!!!!!", music.SearchArtists, 0)
	must(err)
	artist, err := s.Artist(artists.Artists[0].ID)
	must(err)
	t.Logf("artist: %s, %d top songs, %d albums", artist.Artist.Name, len(artist.Tracks), len(artist.Albums))
	byName, err := s.Artist(song.Album.Artists[0].ID)
	must(err)
	t.Logf("artist by name: %s", byName.Artist.Name)

	lists, err := s.Search("anime", music.SearchPlaylists, 0)
	must(err)
	list, err := s.Playlist(lists.Playlists[0].ID)
	must(err)
	t.Logf("playlist: %s, %d tracks", list.Name, len(list.Tracks))

	fromLink, err := s.Search("https://music.apple.com/jp/album/x/1783644603?i=1783644604", music.SearchTracks, 0)
	must(err)
	var names []string
	for _, tr := range fromLink.Tracks {
		names = append(names, tr.Name)
	}
	t.Logf("link: %q", names)
	if len(names) != 1 {
		t.Error("the link isn't one song")
	}

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
		return a.ID + " " + a.Name, nil
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
	step("savedAlbums", func() (string, error) {
		r, err := s.SavedAlbums(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprint(len(r.Albums), " albums"), nil
	})
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
			out += fmt.Sprintf("; %s: %d tracks", l.Name, len(l.Tracks))
		}
		return out, nil
	})
	step("stations", func() (string, error) {
		st, err := s.Stations()
		if err != nil || len(st) == 0 {
			return "no station", err
		}
		next, err := s.StationNext(st[0].ID)
		return fmt.Sprint(len(st), " stations, ", len(next), " next"), err
	})
	step("home", func() (string, error) { h, err := s.Home(); return sectionsOr(h), err })
	step("recentlyAdded", func() (string, error) { h, err := s.Browse("recentlyAdded"); return sectionsOr(h), err })
	s.Played(music.Track{ID: id})
}

func sectionsOr(b *music.Browse) string {
	if b == nil {
		return ""
	}
	return sections(b)
}
