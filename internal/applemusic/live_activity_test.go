package applemusic

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"ringo/internal/music"
)

// Reporting a play to the real activity service, so run only with
// APPLE_MUSIC_WRITE_TOKEN_FILE, the token of an account kept for testing:
// it makes plays, which show in its Recently Played, and changes nothing
// else. A song of an album that isn't in Recently Played yet is played, and
// paused 35 seconds in, as the web player reports it, and then of a
// playlist; each should come to the top of Recently Played. Only IDs, the
// kind of container and HTTP status codes are printed.

// statusLog keeps the status of what the activity service answers.
type statusLog struct {
	base http.RoundTripper
	mu   sync.Mutex
	got  []int
}

func (l *statusLog) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := l.base.RoundTrip(r)
	if err == nil && r.URL.String() == activityURL {
		l.mu.Lock()
		l.got = append(l.got, res.StatusCode)
		l.mu.Unlock()
	}
	return res, err
}

func (l *statusLog) take() []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.got
	l.got = nil
	return out
}

func TestLiveActivity(t *testing.T) {
	file := os.Getenv("APPLE_MUSIC_WRITE_TOKEN_FILE")
	if file == "" {
		t.Skip("APPLE_MUSIC_WRITE_TOKEN_FILE names the token of an account that may make plays")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	s.storage.set(userTokenKey, strings.TrimSpace(string(raw)))
	status := &statusLog{base: http.DefaultTransport}
	s.client = &http.Client{Transport: status}

	// recent is Recently Played's containers, the newest first, as kind:ID.
	recent := func() []string {
		t.Helper()
		b, err := s.me("/recent/played", params{"limit": "10"}, reqOpts{})
		if err != nil {
			t.Fatalf("reading Recently Played: %v", err)
		}
		var out []string
		for _, r := range b.Data {
			if r != nil {
				out = append(out, r.Type+":"+r.ID)
			}
		}
		return out
	}
	// What's there before, which is the account's, isn't printed.
	before := recent()
	known := func(kind, id string) bool {
		for _, r := range before {
			if r == kind+":"+id {
				return true
			}
		}
		return false
	}

	// The song's play, as Apple's own players say it: from the start, and
	// paused 35 seconds in.
	play := func(song music.Track) {
		t.Helper()
		s.Played(music.PlayEvent{Action: music.Started, Track: song})
		s.Played(music.PlayEvent{Action: music.Stopped, Track: song, Position: 35 * time.Second, Why: music.StopPaused})
		s.Flush(30 * time.Second)
		t.Logf("%s from %s: HTTP statuses %v", song.ID, kindName(song.Origin.Kind), status.take())
	}
	topWithin := func(want string) {
		t.Helper()
		start := time.Now()
		for {
			now := recent()
			if len(now) > 0 && now[0] == want {
				t.Logf("%s is first in Recently Played, after %v", want, time.Since(start).Round(time.Second))
				return
			}
			if time.Since(start) > settleWithin {
				t.Errorf("%s is not first in Recently Played after %v", want, settleWithin)
				return
			}
			time.Sleep(10 * time.Second)
		}
	}
	// songOf is the first song of tracks that is played from from.
	songOf := func(tracks []music.Track, from music.Origin) music.Track {
		t.Helper()
		for _, tr := range tracks {
			if isCatalogID(tr.ID) && tr.Duration > 0 {
				tr.Origin = from
				return tr
			}
		}
		t.Fatalf("no song of %s to play", from.ID)
		return music.Track{}
	}

	var album *music.AlbumPage
	for _, al := range searchFor(t, s, "lofi", music.ItemAlbum) {
		if isCatalogID(al.ID) && !known("albums", al.ID) {
			if album, err = s.Album(al.ID); err == nil && len(album.Tracks) > 0 {
				break
			}
			album = nil
		}
	}
	if album == nil {
		t.Fatal("no album that isn't in Recently Played")
	}
	from := music.Origin{Kind: music.FromAlbum, ID: album.Album.ID}
	play(songOf(album.Tracks, from))
	topWithin("albums:" + album.Album.ID)

	var list *music.Playlist
	for _, pl := range searchFor(t, s, "lofi", music.ItemPlaylist) {
		if strings.HasPrefix(pl.ID, "pl.") && !known("playlists", pl.ID) {
			if list, err = s.Playlist(pl.ID); err == nil && len(list.Tracks) > 0 && list.Version != "" {
				break
			}
			list = nil
		}
	}
	if list == nil {
		t.Fatal("no playlist, with a version, that isn't in Recently Played")
	}
	from = music.Origin{Kind: music.FromPlaylist, ID: list.ID, Version: list.Version}
	play(songOf(list.Tracks, from))
	topWithin("playlists:" + list.ID)
}

func kindName(k music.OriginKind) string {
	switch k {
	case music.FromAlbum:
		return "album (container type 3)"
	case music.FromPlaylist:
		return "playlist (container type 2)"
	}
	return "elsewhere"
}
