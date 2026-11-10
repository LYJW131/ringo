package applemusic

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// The source as the app calls it, without asking Apple anything: what it
// says signed out, and the token given in the settings.

func nowhere(t *testing.T) func(*http.Request) *http.Response {
	return func(r *http.Request) *http.Response {
		if r.Method != "HEAD" {
			t.Errorf("asked Apple %s %s", r.Method, r.URL)
		}
		return answer(404, "")
	}
}

func TestSignedOut(t *testing.T) {
	ts := newTestSource(t, nowhere(t))
	if a, err := ts.Account(); a != nil || err != nil {
		t.Errorf("account %+v %v", a, err)
	}
	for name, call := range map[string]func() error{
		"playlists":     func() error { _, err := ts.Playlists(); return err },
		"liked":         func() error { _, err := ts.Liked(0); return err },
		"savedAlbums":   func() error { _, err := ts.SavedAlbums(0); return err },
		"recentlyAdded": func() error { _, err := ts.Browse("recentlyAdded"); return err },
		"stream":        func() error { _, err := ts.Stream("1783644604", ""); return err },
	} {
		if err := call(); !errors.Is(err, music.ErrSignIn) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if st, err := ts.Stations(); len(st) != 0 || err != nil {
		t.Errorf("stations %v %v", st, err)
	}
	if l, err := ts.Lyrics("1783644604"); l != nil || err != nil {
		t.Errorf("lyrics %v %v", l, err)
	}
	if _, err := ts.Stream("i.abc", ""); !errors.Is(err, music.ErrSignIn) {
		t.Errorf("an upload: %v", err)
	}
	if err := ts.FollowArtist("1", true); !errors.Is(err, music.ErrUnsupported) {
		t.Errorf("follow: %v", err)
	}
}

// A token is taken as pasted: alone, quoted, or among the cookies copied
// from a browser, with or without the header's name.
func TestTokenIn(t *testing.T) {
	for pasted, want := range map[string]string{
		"Cookie: geo=US; media-user-token=AbC+/12=; itspod=55": "AbC+/12=",
		"cookie:media-user-token=Spaceless":                    "Spaceless",
		` "XyZ" `:                                              "XyZ",
		"geo=US; itspod=55":                                    "geo=US; itspod=55",
	} {
		if got := tokenIn(pasted); got != want {
			t.Errorf("%q: %q", pasted, got)
		}
	}
}

// What isn't a token is said before Apple is asked, and whoever is signed in stays.
func TestAddingWhatIsntAToken(t *testing.T) {
	ts := newTestSource(t, nowhere(t))
	ts.storage.set(accountsKey, []ownAccount{{ID: "a", Token: "Kept"}})
	ts.storage.set(currentKey, ownPrefix+"a")
	ts.storage.set(userTokenKey, "Kept")
	for _, pasted := range []string{"geo=US; itspod=55", ""} {
		if _, err := ts.AddAccount(pasted); !errors.Is(err, music.ErrSetup) || ts.userToken() != "Kept" {
			t.Errorf("%q: %v, %q", pasted, err, ts.userToken())
		}
	}
	// What's kept is the source's own: 0600.
	if fi, err := os.Stat(filepath.Join(ts.dataDir, "storage.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("storage.json: %v %v", fi.Mode(), err)
	}
}

// The real amhelper starts as the settings say, again when they change, and stops with the source.
func TestHelperFollowsSettings(t *testing.T) {
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	base, err := s.helperBase()
	if err != nil || !strings.HasPrefix(base, "http://127.0.0.1:") {
		t.Fatalf("base %q %v", base, err)
	}
	res, err := http.Get(base + "/status")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("status without a wrapper: %d", res.StatusCode)
	}
	if again, _ := s.helperBase(); again != base {
		t.Error("started again with the same settings")
	}
	s.settings.set("wrapper", "http://127.0.0.1:9")
	if again, _ := s.helperBase(); again == base {
		t.Error("not started again for a new wrapper")
	}
	if _, err := http.Get(base + "/status"); err == nil {
		t.Error("the old one still answers")
	}
}

func TestSpeaksTheListenersLanguage(t *testing.T) {
	ts := newTestSource(t, nowhere(t))
	i18n.SetLanguage(i18n.Chinese)
	defer i18n.SetLanguage(i18n.English)
	if _, err := ts.Stream("pl.abc", ""); err == nil || !strings.Contains(err.Error(), "Apple Music 无法播放这首歌") {
		t.Errorf("error %v", err)
	}
}
