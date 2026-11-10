package applemusic

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"ringo/internal/music"
)

// With APPLE_MUSIC_TOKEN_FILE and APPLE_MUSIC_WRITE_TOKEN_FILE, two
// accounts are added and switched between, reading only: nothing is asked
// of the second account but what the first is asked. With
// APPLE_MUSIC_WRAPPER too, the wrapper's accounts are listed and one is
// switched to. What's printed says nothing of whose they are: accounts go
// by their place in the list, their region, and whether they're current.
func TestLiveAccounts(t *testing.T) {
	first, second := os.Getenv("APPLE_MUSIC_TOKEN_FILE"), os.Getenv("APPLE_MUSIC_WRITE_TOKEN_FILE")
	if first == "" || second == "" {
		t.Skip("APPLE_MUSIC_TOKEN_FILE and APPLE_MUSIC_WRITE_TOKEN_FILE name files holding two accounts' media-user-tokens")
	}
	s := New(Options{DataDir: t.TempDir()})
	defer s.Close()
	if w := os.Getenv("APPLE_MUSIC_WRAPPER"); w != "" {
		s.settings.set("wrapper", w)
	}
	add := func(file string) *music.Account {
		t.Helper()
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		a, err := s.AddAccount(strings.TrimSpace(string(raw)))
		if err != nil || a == nil {
			t.Fatalf("adding: %s", failed(err))
		}
		return a
	}
	// librarySongs is the IDs of the first page of the library's songs,
	// which are the account's own: two accounts share none.
	librarySongs := func() []string {
		t.Helper()
		b, err := s.me("/library/songs", params{"limit": "25"}, reqOpts{})
		if err != nil {
			t.Fatalf("the library: %s", failed(err))
		}
		var ids []string
		for _, r := range b.Data {
			if r != nil {
				ids = append(ids, r.ID)
			}
		}
		return ids
	}
	own := func() []music.AccountChoice {
		t.Helper()
		list, _ := s.Accounts(false)
		return slices.DeleteFunc(list, func(c music.AccountChoice) bool { return c.Shared })
	}

	a1 := add(first)
	songs1 := librarySongs()
	a2 := add(second)
	list := own()
	if len(list) != 2 || list[0].Region == list[1].Region || a1.ID == a2.ID {
		t.Fatalf("%d accounts signed in to here, regions differ: %v", len(list), len(list) == 2 && list[0].Region != list[1].Region)
	}
	for i, c := range list {
		t.Logf("account %d: %s, current %v", i+1, c.Region, c.Current)
	}

	for round := range 2 {
		for i, c := range list {
			a, err := s.UseAccount(c.ID)
			if err != nil || a == nil || a.ID != c.ID {
				t.Fatalf("switching to %d: %s", i+1, failed(err))
			}
			sf, err := s.storefront()
			if err != nil || sf != c.Region {
				t.Errorf("switched to %d: storefront %s, want %s", i+1, sf, c.Region)
			}
			if acc, _ := s.Account(); acc == nil || acc.ID != c.ID {
				t.Errorf("switched to %d: Account says another", i+1)
			}
			songs := librarySongs()
			shared := slices.ContainsFunc(songs, func(id string) bool { return slices.Contains(songs1, id) })
			if c.ID == a1.ID && !slices.Equal(songs, songs1) {
				t.Errorf("round %d, account 1: its library changed", round+1)
			}
			if c.ID != a1.ID && shared {
				t.Errorf("round %d, account %d: the library is account 1's", round+1, i+1)
			}
			t.Logf("round %d: account %d current, %s, %d library songs", round+1, i+1, sf, len(songs))
		}
	}

	if s.hasWrapper() {
		all, err := s.Accounts(true)
		if err != nil {
			t.Fatalf("the wrapper's accounts: %s", failed(err))
		}
		var held []music.AccountChoice
		for _, c := range all {
			if c.Shared {
				held = append(held, c)
			}
		}
		var regions []string
		for _, c := range held {
			regions = append(regions, c.Region)
		}
		t.Logf("the wrapper lends %d: %v", len(held), regions)
		if len(held) == 0 {
			t.Fatal("the wrapper lends none")
		}
		a, err := s.UseAccount(held[0].ID)
		if err != nil || a == nil || !a.Shared {
			t.Fatalf("switching to the wrapper's first: %s", failed(err))
		}
		list, _ := s.Accounts(false)
		at := slices.IndexFunc(list, func(c music.AccountChoice) bool { return c.Current })
		if at < 0 || !list[at].Shared || list[at].ID != held[0].ID {
			t.Errorf("the wrapper's account isn't current")
		}
		if err := s.SignOut(); err == nil {
			t.Error("signed out of the wrapper's account")
		}
		t.Logf("the wrapper's account 1 current, %s, %d library songs", must(s.storefront()), len(librarySongs()))
	}

	// Back, and the accounts added taken away again.
	if _, err := s.UseAccount(a1.ID); err != nil {
		t.Fatalf("switching back: %s", failed(err))
	}
	if err := s.RemoveAccount(a2.ID); err != nil {
		t.Fatalf("removing: %s", failed(err))
	}
	if err := s.SignOut(); err != nil {
		t.Fatalf("signing out: %s", failed(err))
	}
	if acc, _ := s.Account(); acc != nil || len(own()) != 0 {
		t.Errorf("still signed in, %d accounts", len(own()))
	}
}

// failed says what kind of error err is, and no more: what Apple or the
// wrapper said of it may name the accounts.
func failed(err error) string {
	for _, k := range []error{music.ErrSignIn, music.ErrSetup, music.ErrUnavailable, music.ErrUnsupported} {
		if errors.Is(err, k) {
			return k.Error()
		}
	}
	if err == nil {
		return "no answer"
	}
	return "another error"
}
