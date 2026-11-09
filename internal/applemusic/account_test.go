package applemusic

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"mygo-music/internal/music"
)

// Signing in as an account the wrapper holds, against a stand-in for
// amhelper and for Apple: the account named in the settings is the one
// signed in with, its token is asked for again as the wrapper refreshes it or
// Apple refuses it, and naming another account, or none, signs out of the
// old one. The steps go on from each other, as the plugin's did.
func TestWrapperAccount(t *testing.T) {
	var mu sync.Mutex
	// The wrapper's accounts, by what names them, and their tokens; Apple
	// accepts the tokens in accepted.
	tokens := map[string]string{"a@example.com": "tokA1", "b@example.com": "tokB1"}
	var next int64
	wrapperDown := false
	accepted := map[string]bool{"tokA1": true, "tokA2": true, "tokB1": true}
	var asked []string
	ts := newTestSource(t, func(r *http.Request) *http.Response {
		mu.Lock()
		defer mu.Unlock()
		u := r.URL
		if u.Host == "127.0.0.1:1" && u.Path == "/s/token" {
			named := u.Query().Get("account")
			asked = append(asked, "token "+named)
			if wrapperDown {
				return nil
			}
			if tokens[named] == "" {
				return answer(404, map[string]string{"code": "account", "message": fmt.Sprintf("the wrapper holds no account %q; it holds a@example.com (US)", named)})
			}
			return answer(200, map[string]any{"token": tokens[named], "region": "us", "next": next, "account": named})
		}
		if u.Host == "127.0.0.1:1" || r.Method == "HEAD" {
			return answer(404, "{}")
		}
		// The web player, for a fresh developer token after a 401.
		if u.Host == "music.apple.com" {
			if u.Path == "/" || u.Path == "" {
				return answer(200, `<script src="/assets/index-1.js">`)
			}
			return answer(200, "eyJhbGciOiJFUzI1NiJ9.e30.sig")
		}
		user := r.Header.Get("media-user-token")
		asked = append(asked, u.Path+" "+user)
		if !accepted[user] {
			return answer(401, "")
		}
		switch u.Path {
		case "/v1/me/storefront":
			sf := "us"
			if strings.HasPrefix(user, "tokB") {
				sf = "jp"
			}
			return answer(200, map[string]any{"data": []any{map[string]any{"id": sf}}})
		case "/v1/me/social-profile":
			return answer(200, map[string]any{"data": []any{map[string]any{"attributes": map[string]any{"name": user}}}})
		case "/v1/me/account", "/v1/me/library/songs":
			return answer(200, map[string]any{"data": []any{}})
		}
		t.Errorf("unexpected fetch %s", u)
		return answer(500, "")
	})
	ts.setSettings(map[string]string{"wrapper": "http://wrapper.example"})
	reset := func() {
		mu.Lock()
		asked = nil
		mu.Unlock()
		ts.retryWrapper()
	}
	tokenAsks := func() []string {
		mu.Lock()
		defer mu.Unlock()
		var out []string
		for _, a := range asked {
			if strings.HasPrefix(a, "token") {
				out = append(out, a)
			}
		}
		return out
	}
	name := func(a *music.Account, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if a == nil {
			return "<none>"
		}
		return a.ID + " " + a.Name
	}

	// The account named is signed in with, and the wrapper asked once.
	reset()
	ts.setSettings(map[string]string{"wrapperAccount": "a@example.com"})
	mu.Lock()
	next = ts.clk.Now().Unix() + 1800
	mu.Unlock()
	if got := name(ts.Account()); got != "us tokA1" {
		t.Errorf("account: %s", got)
	}
	if ts.userToken() != "tokA1" {
		t.Errorf("token %q", ts.userToken())
	}
	ts.Account()
	if got := tokenAsks(); !slices.Equal(got, []string{"token a@example.com"}) {
		t.Errorf("asked %q", got)
	}

	// Naming another account signs out of the first and in as it, forgetting what was kept.
	reset()
	ts.setSettings(map[string]string{"wrapperAccount": "b@example.com"})
	if got := name(ts.Account()); got != "jp tokB1" {
		t.Errorf("account: %s", got)
	}

	// Naming none signs out, and a wrapper account named nowhere isn't signed in with.
	reset()
	ts.setSettings(map[string]string{"wrapperAccount": ""})
	if got := name(ts.Account()); got != "<none>" {
		t.Errorf("account: %s", got)
	}
	if ts.storage.getString(userTokenKey) != "" || ts.keptFromWrapper() != nil {
		t.Error("still signed in")
	}
	ts.setSettings(map[string]string{"wrapperAccount": "c@example.com"})
	if got := name(ts.Account()); got != "<none>" {
		t.Errorf("account: %s", got)
	}
	if _, err := ts.SignIn(); !errors.Is(err, music.ErrSetup) || !strings.Contains(err.Error(), "c@example.com") {
		t.Errorf("signing in as an account the wrapper doesn't hold: %v", err)
	}

	// Signed out, the account stays signed out until signing in.
	reset()
	ts.setSettings(map[string]string{"wrapperAccount": "a@example.com"})
	if got := name(ts.Account()); got != "us tokA1" {
		t.Errorf("account: %s", got)
	}
	ts.SignOut()
	if got := name(ts.Account()); got != "<none>" {
		t.Errorf("after signing out: %s", got)
	}
	if got := name(ts.SignIn()); got != "us tokA1" {
		t.Errorf("signing in: %s", got)
	}

	// A token given in the settings goes first.
	reset()
	ts.setSettings(map[string]string{"mediaUserToken": "tokB1"})
	if got := name(ts.Account()); !strings.HasPrefix(got, "jp ") {
		t.Errorf("account: %s", got)
	}
	if ts.userToken() != "tokB1" || ts.keptFromWrapper() != nil {
		t.Error("the given token isn't what's signed in with")
	}
	ts.setSettings(map[string]string{"mediaUserToken": ""})
	ts.storage.remove(profileKey)
	if got := name(ts.Account()); got != "us tokA1" {
		t.Errorf("account: %s", got)
	}

	// A token Apple refuses is asked for again, and the request made again with the new one.
	reset()
	mu.Lock()
	delete(accepted, "tokA1")
	tokens["a@example.com"] = "tokA2"
	mu.Unlock()
	if _, err := ts.me("/library/songs", nil, reqOpts{}); err != nil {
		t.Fatal(err)
	}
	if ts.userToken() != "tokA2" {
		t.Errorf("token %q", ts.userToken())
	}
	mu.Lock()
	last := slices.Clone(asked[len(asked)-2:])
	mu.Unlock()
	if !slices.Equal(last, []string{"token a@example.com", "/v1/me/library/songs tokA2"}) {
		t.Errorf("asked %q", last)
	}
	// The wrapper giving the one refused again: log it in to the wrapper again.
	mu.Lock()
	delete(accepted, "tokA2")
	mu.Unlock()
	if _, err := ts.me("/library/songs", nil, reqOpts{}); !errors.Is(err, music.ErrSetup) || !strings.Contains(err.Error(), "wrapper") {
		t.Errorf("refused again: %v", err)
	}
	mu.Lock()
	accepted["tokA2"] = true
	mu.Unlock()

	// Once the wrapper has refreshed the token, it's asked again, in the background.
	reset()
	k := ts.keptFromWrapper()
	k.Next = ts.now() - 1
	ts.storage.set(wrapperAcctKey, k)
	mu.Lock()
	tokens["a@example.com"] = "tokA3"
	accepted["tokA3"] = true
	mu.Unlock()
	if _, err := ts.me("/library/songs", nil, reqOpts{}); err != nil {
		t.Fatal(err)
	}
	ts.clk.settle(t)
	if ts.userToken() != "tokA3" {
		t.Errorf("token %q", ts.userToken())
	}
	if k := ts.keptFromWrapper(); k.Next <= ts.now()+50_000 {
		t.Errorf("asked again at %d, now %d", k.Next, ts.now())
	}

	// A wrapper not answering isn't asked again within a minute, but signing in asks it.
	reset()
	ts.setSettings(map[string]string{"wrapperAccount": "b@example.com"})
	mu.Lock()
	wrapperDown = true
	mu.Unlock()
	if got := name(ts.Account()); got != "<none>" {
		t.Errorf("account: %s", got)
	}
	ts.clk.advance(t, 10*time.Second)
	if got := name(ts.Account()); got != "<none>" {
		t.Errorf("account: %s", got)
	}
	if got := tokenAsks(); !slices.Equal(got, []string{"token b@example.com"}) {
		t.Errorf("asked %q", got)
	}
	mu.Lock()
	wrapperDown = false
	mu.Unlock()
	if got := name(ts.SignIn()); !strings.HasPrefix(got, "jp ") {
		t.Errorf("signing in: %s", got)
	}
}
