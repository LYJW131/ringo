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

// fakeAccounts stands in for Apple and amhelper for the accounts' tests.
// Apple takes the tokens in accepted, each signing in to the storefront and
// profile it says; amhelper's wrapper holds the accounts in held, giving the
// tokens in tokens.
type fakeAccounts struct {
	mu          sync.Mutex
	accepted    map[string]fakeWho
	held        []heldAccount
	tokens      map[string]string // by what names the account to the wrapper
	next        int64
	wrapperDown bool
	asked       []string
}

type fakeWho struct{ sf, social, name string }

func (f *fakeAccounts) serve(t *testing.T) func(*http.Request) *http.Response {
	return func(r *http.Request) *http.Response {
		f.mu.Lock()
		defer f.mu.Unlock()
		u := r.URL
		if u.Host == "127.0.0.1:1" {
			if f.wrapperDown && (u.Path == "/s/token" || u.Path == "/s/accounts") {
				f.asked = append(f.asked, u.Path)
				return nil
			}
			switch u.Path {
			case "/s/accounts":
				f.asked = append(f.asked, "accounts")
				return answer(200, map[string]any{"accounts": f.held})
			case "/s/token":
				named := u.Query().Get("account")
				f.asked = append(f.asked, "token "+named)
				if f.tokens[named] == "" {
					return answer(404, map[string]string{"code": "account", "message": fmt.Sprintf("the wrapper holds no account %q", named)})
				}
				instance := ""
				for _, h := range f.held {
					if h.ID == named {
						instance = h.ID
					}
				}
				return answer(200, map[string]any{"token": f.tokens[named], "region": "jp", "next": f.next, "account": named, "instance": instance})
			}
			return answer(404, "{}")
		}
		if r.Method == "HEAD" {
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
		f.asked = append(f.asked, u.Path+" "+user)
		w, ok := f.accepted[user]
		if !ok {
			return answer(401, "")
		}
		switch u.Path {
		case "/v1/me/storefront":
			return answer(200, map[string]any{"data": []any{map[string]any{"id": w.sf}}})
		case "/v1/me/social-profile":
			if w.social == "" {
				return answer(200, map[string]any{"data": []any{}})
			}
			return answer(200, map[string]any{"data": []any{map[string]any{"id": w.social, "attributes": map[string]any{"name": w.name}}}})
		case "/v1/me/account", "/v1/me/library/songs":
			return answer(200, map[string]any{"data": []any{}})
		}
		t.Errorf("unexpected fetch %s", u)
		return answer(500, "")
	}
}

// askedOf is what was asked since last time, that starts with prefix.
func (f *fakeAccounts) askedOf(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, a := range f.asked {
		if strings.HasPrefix(a, prefix) {
			out = append(out, a)
		}
	}
	f.asked = nil
	return out
}

// accountName is who an answer of Account's says, or its error.
func accountName(a *music.Account, err error) string {
	if err != nil {
		return "error: " + err.Error()
	}
	if a == nil {
		return "<none>"
	}
	return a.Name
}

// current is the ID of the current account as Accounts says, and how many it lists.
func currentChoice(t *testing.T, ts *testSource) (string, int) {
	t.Helper()
	list, _ := ts.Accounts(false)
	cur := ""
	for _, c := range list {
		if c.Current {
			if cur != "" {
				t.Errorf("two current: %+v", list)
			}
			cur = c.ID
		}
	}
	return cur, len(list)
}

// A machine that signed in with the window, or with the Media User Token
// setting, before there was a list of accounts, is still signed in after
// the update, as that account, now one of the list, without asking Apple; a
// token in the setting that wasn't the one used is listed too.
func TestOldTokenBecomesAnAccount(t *testing.T) {
	for _, c := range []struct {
		name, given string
		want        int
	}{
		{"from the setting", "Old", 1},
		{"from the window, with another in the setting", "Other", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			ts := newTestSource(t, nowhere(t))
			ts.storage.set(userTokenKey, "Old")
			ts.storage.set(givenTokenKey, c.given)
			ts.storage.set(storefrontKey, "cn")
			ts.storage.set(profileKey, profile{Name: "Alex", At: ts.now()})
			ts.setSettings(map[string]string{"mediaUserToken": c.given})

			if got := accountName(ts.Account()); got != "Alex" {
				t.Fatalf("account: %s", got)
			}
			list, err := ts.Accounts(false)
			if err != nil || len(list) != c.want {
				t.Fatalf("listed %+v, %v", list, err)
			}
			cur := list[slices.IndexFunc(list, func(c music.AccountChoice) bool { return c.Current })]
			if cur.Name != "Alex" || cur.Region != "cn" || cur.Shared || ts.userToken() != "Old" {
				t.Errorf("current %+v", cur)
			}
			if ts.Setting("mediaUserToken") != "" || ts.storage.getString(givenTokenKey) != "" {
				t.Error("the old setting is still there")
			}
		})
	}
}

// A machine that signed in as the account the Wrapper Account setting named
// is still signed in as it after the update, without asking the wrapper
// again while its token is good; one where the setting named it but it
// wasn't asked for yet asks for it as it starts.
func TestOldWrapperAccountStays(t *testing.T) {
	f := &fakeAccounts{accepted: map[string]fakeWho{"tokCN": {sf: "cn"}}, tokens: map[string]string{"cn": "tokCN"}}
	ts := newTestSource(t, f.serve(t))
	ts.setSettings(map[string]string{"wrapper": "http://wrapper.example", "wrapperAccount": "cn"})
	ts.storage.set(userTokenKey, "tokCN")
	ts.storage.set(wrapperAcctKey, fromWrapper{Of: "http://wrapper.example\ncn", Token: "tokCN", Next: ts.now() + time.Hour.Milliseconds()})
	ts.storage.set(storefrontKey, "cn")
	ts.storage.set(profileKey, profile{At: ts.now()})

	a, err := ts.Account()
	if got := accountName(a, err); got != "Apple Music (CN)" || !a.Shared {
		t.Fatalf("account: %s %+v", got, a)
	}
	if asked := f.askedOf(""); len(asked) != 0 {
		t.Errorf("asked %q", asked)
	}
	if err := ts.SignOut(); !errors.Is(err, music.ErrUnsupported) || ts.userToken() != "tokCN" {
		t.Errorf("signed out of the wrapper's account: %v", err)
	}
	if ts.Setting("wrapperAccount") != "" {
		t.Error("the old setting is still there")
	}

	fresh := newTestSource(t, f.serve(t))
	fresh.setSettings(map[string]string{"wrapper": "http://wrapper.example", "wrapperAccount": "cn"})
	if got := accountName(fresh.Account()); got != "Apple Music (CN)" {
		t.Fatalf("named, not yet asked: %s", got)
	}
	if asked := f.askedOf("token"); !slices.Equal(asked, []string{"token cn"}) {
		t.Errorf("asked %q", asked)
	}
}

// Accounts are added with a token and switched between, each switch
// forgetting what was kept for the one before; signing in to one that's
// there gives it the new token; one Apple refuses, or a wrapper that
// doesn't answer, leaves whoever was current; and signing out goes on as
// the account used last.
func TestSwitchingAccounts(t *testing.T) {
	f := &fakeAccounts{
		accepted: map[string]fakeWho{
			"tokA": {sf: "us", social: "pA", name: "Alex"}, "tokA2": {sf: "us", social: "pA", name: "Alex"},
			"tokB": {sf: "jp"}, "tokW": {sf: "jp"},
		},
		held:   []heldAccount{{ID: "i1", Region: "jp", Name: "w@example.com"}, {ID: "i2", Region: "us"}},
		tokens: map[string]string{"i1": "tokW"},
	}
	ts := newTestSource(t, f.serve(t))
	ts.setSettings(map[string]string{"wrapper": "http://wrapper.example"})
	tick := func() { ts.clk.advance(t, time.Second) }

	if got := accountName(ts.AddAccount("tokA")); got != "Alex" {
		t.Fatalf("added: %s", got)
	}
	idA, n := currentChoice(t, ts)
	if n != 3 {
		t.Errorf("listed %d", n)
	}
	tick()
	if got := accountName(ts.AddAccount("media-user-token=tokB")); got != "Apple Music (JP)" {
		t.Fatalf("added: %s", got)
	}
	idB, _ := currentChoice(t, ts)
	if idB == idA {
		t.Fatal("the second account is the first")
	}

	// Signed in again with the same profile: the same account, with the new token.
	tick()
	ts.AddAccount("tokA2")
	if cur, n := currentChoice(t, ts); cur != idA || n != 4 || ts.userToken() != "tokA2" {
		t.Errorf("current %s of %d, token %q", cur, n, ts.userToken())
	}

	tick()
	a, err := ts.UseAccount(idB)
	if accountName(a, err); a.ID != idB || ts.storage.getString(storefrontKey) != "jp" {
		t.Errorf("switched to %+v, storefront %q", a, ts.storage.getString(storefrontKey))
	}

	// The wrapper's account, asked for as it's switched to.
	tick()
	f.askedOf("")
	a, err = ts.UseAccount("wrapper:i1")
	if got := accountName(a, err); got != "Apple Music (JP)" || !a.Shared || ts.userToken() != "tokW" {
		t.Fatalf("the wrapper's: %s %+v", got, a)
	}
	if asked := f.askedOf("token"); !slices.Equal(asked, []string{"token i1"}) {
		t.Errorf("asked %q", asked)
	}
	if err := ts.RemoveAccount("wrapper:i1"); err == nil {
		t.Error("removed the wrapper's account")
	}

	// Failing to switch leaves the current one.
	ts.storage.set(accountsKey, append(ts.ownAccounts(), ownAccount{ID: "gone", Token: "tokGone"}))
	if _, err := ts.UseAccount("own:gone"); !errors.Is(err, music.ErrSignIn) || ts.userToken() != "tokW" {
		t.Errorf("a refused token: %v, %q", err, ts.userToken())
	}
	ts.RemoveAccount("own:gone")
	f.mu.Lock()
	f.wrapperDown = true
	f.mu.Unlock()
	ts.UseAccount(idB)
	if _, err := ts.UseAccount("wrapper:i1"); err == nil || ts.userToken() != "tokB" {
		t.Errorf("a wrapper not answering: %v, %q", err, ts.userToken())
	}
	// Nor does listing hide the accounts signed in to here.
	list, err := ts.Accounts(true)
	if err == nil || len(list) != 2 {
		t.Errorf("listed %d without the wrapper, %v", len(list), err)
	}

	// Signing out of B forgets it and goes on as A.
	tick()
	if err := ts.SignOut(); err != nil {
		t.Fatal(err)
	}
	if cur, n := currentChoice(t, ts); cur != idA || n != 1 || ts.userToken() != "tokA2" {
		t.Errorf("after signing out: %s of %d", cur, n)
	}
	ts.SignOut()
	if got := accountName(ts.Account()); got != "<none>" {
		t.Errorf("signed out of the last: %s", got)
	}
}

// The wrapper's account's token is asked for again as the wrapper refreshes
// it or Apple refuses it.
func TestWrapperAccountToken(t *testing.T) {
	f := &fakeAccounts{
		accepted: map[string]fakeWho{"tokA1": {sf: "us"}, "tokA2": {sf: "us"}},
		tokens:   map[string]string{"a@example.com": "tokA1"},
	}
	ts := newTestSource(t, f.serve(t))
	ts.setSettings(map[string]string{"wrapper": "http://wrapper.example"})
	f.next = ts.clk.Now().Unix() + 1800
	if _, err := ts.UseAccount("wrapper:a@example.com"); err != nil {
		t.Fatal(err)
	}
	ts.Account()
	if got := f.askedOf("token"); !slices.Equal(got, []string{"token a@example.com"}) {
		t.Errorf("asked %q", got)
	}

	// A token Apple refuses is asked for again, and the request made again with the new one.
	f.mu.Lock()
	delete(f.accepted, "tokA1")
	f.tokens["a@example.com"] = "tokA2"
	f.mu.Unlock()
	if _, err := ts.me("/library/songs", nil, reqOpts{}); err != nil {
		t.Fatal(err)
	}
	if ts.userToken() != "tokA2" {
		t.Errorf("token %q", ts.userToken())
	}
	if got := f.askedOf(""); !slices.Equal(got[len(got)-2:], []string{"token a@example.com", "/v1/me/library/songs tokA2"}) {
		t.Errorf("asked %q", got)
	}
	// The wrapper giving the one refused again: log it in to the wrapper again.
	f.mu.Lock()
	delete(f.accepted, "tokA2")
	f.mu.Unlock()
	if _, err := ts.me("/library/songs", nil, reqOpts{}); !errors.Is(err, music.ErrSetup) || !strings.Contains(err.Error(), "wrapper") {
		t.Errorf("refused again: %v", err)
	}
	f.mu.Lock()
	f.accepted["tokA2"] = fakeWho{sf: "us"}
	f.mu.Unlock()

	// Once the wrapper has refreshed the token, it's asked again, in the background.
	k := ts.keptFromWrapper()
	k.Next = ts.now() - 1
	ts.storage.set(wrapperAcctKey, k)
	f.mu.Lock()
	f.tokens["a@example.com"] = "tokA3"
	f.accepted["tokA3"] = fakeWho{sf: "us"}
	f.mu.Unlock()
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
}
