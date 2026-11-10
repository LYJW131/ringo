package amhelper

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A manager holding three accounts, one logged in before it noted Apple IDs.
func fakeManager(t *testing.T, asked *[]string) *wrapper {
	t.Helper()
	old := instanceID("old@example.com")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/status":
			_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"regions":["us","jp"],"ready":true,"instances":[
				{"id":"` + instanceID("a@example.com") + `","region":"us","name":"a@example.com","ready":true},
				{"id":"` + instanceID("b@example.com") + `","region":"jp","name":"B@example.com","ready":false},
				{"id":"` + old + `","region":"jp","name":"","ready":true}]}}`))
		case "/token":
			*asked = append(*asked, r.URL.RawQuery)
			_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"region":"us","music_token":"tok-` + r.URL.Query().Get("instance") + `","refreshed_at":1,"next_refresh_at":1800}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	w, err := newWrapper(srv.URL, "k", &store{})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestTokenOfTheAccountNamed(t *testing.T) {
	var asked []string
	w := fakeManager(t, &asked)
	for named, want := range map[string]string{
		"a@example.com":             instanceID("a@example.com"),
		" A@Example.com ":           instanceID("a@example.com"),
		instanceID("a@example.com"): instanceID("a@example.com"),
		"old@example.com":           instanceID("old@example.com"), // no name: found by its id
		"JP":                        instanceID("old@example.com"), // b, the first in jp, isn't ready
		strings.ToUpper(instanceID("a@example.com")): instanceID("a@example.com"),
	} {
		got, err := w.accountToken(t.Context(), named)
		if err != nil {
			t.Fatalf("%q: %v", named, err)
		}
		if got.Token != "tok-"+want || got.Next != 1800 {
			t.Fatalf("%q: got %+v, want the token of %s", named, got, want)
		}
	}
	if got, _ := w.accountToken(t.Context(), "a@example.com"); got.Account != "a@example.com" {
		t.Fatalf("account %q", got.Account)
	}
}

func TestTokenOfAnAccountNotThere(t *testing.T) {
	var asked []string
	w := fakeManager(t, &asked)
	_, err := w.accountToken(t.Context(), "c@example.com")
	if !errors.Is(err, errAccount) || !strings.Contains(err.Error(), "a@example.com (US); B@example.com (JP), not ready; JP") {
		t.Fatalf("got %v", err)
	}
	if _, err := w.accountToken(t.Context(), "b@example.com"); !errors.Is(err, errAccount) || !strings.Contains(err.Error(), "isn't ready") {
		t.Fatalf("got %v", err)
	}
	// Nor, without one named, whichever account is first.
	if _, err := w.accountToken(t.Context(), ""); !errors.Is(err, errAccount) {
		t.Fatalf("got %v", err)
	}
	if len(asked) != 0 {
		t.Fatalf("/token asked %v", asked)
	}
}

func TestTokenOfAWrapperLite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"regions":["gb"],"name":""}}`))
		case "/token":
			if r.URL.RawQuery != "" {
				t.Errorf("asked /token?%s of a wrapper-lite", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"region":"gb","music_token":"lite","next_refresh_at":5}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	w, _ := newWrapper(srv.URL, "", &store{})
	got, err := w.accountToken(t.Context(), "gb")
	if err != nil || got.Token != "lite" || got.Account != "GB" {
		t.Fatalf("got %+v, %v", got, err)
	}
	// Upstream's wrapper-lite has no /token.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"regions":["gb"]}}`))
			return
		}
		http.NotFound(w, r)
	})
	if _, err := w.accountToken(t.Context(), "gb"); !errors.Is(err, errWrapper) || !strings.Contains(err.Error(), "can't give") {
		t.Fatalf("got %v", err)
	}
}

// The accounts offered are the ready ones; whether the wrapper gives tokens
// is asked once, and a wrapper that doesn't offers none.
func TestAccountsOffered(t *testing.T) {
	var asked []string
	w := fakeManager(t, &asked)
	for range 2 {
		got, err := w.offered(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Name != "a@example.com" || got[1].ID != instanceID("old@example.com") {
			t.Fatalf("offered %+v", got)
		}
	}
	if len(asked) != 1 {
		t.Fatalf("/token asked %d times", len(asked))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"regions":["gb"]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	lite, _ := newWrapper(srv.URL, "", &store{})
	if got, err := lite.offered(t.Context()); err != nil || len(got) != 0 {
		t.Fatalf("a wrapper without /token offered %+v, %v", got, err)
	}
}
