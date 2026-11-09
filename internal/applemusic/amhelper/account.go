package amhelper

// The media-user-token of an account the wrapper holds, for the applemusic package to
// sign in with instead of Apple's sign-in window (AMDL-Web's fork answers
// GET /token with it, and refreshes it on its own). A wrapper-manager holds
// up to ten accounts, each with its own library, and answers GET /token for
// any of them, so the listener names the one they listen as; the first that
// happens to be ready could be anyone's.

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// instance is an account the wrapper holds, as /status lists it.
type instance struct {
	ID     string `json:"id"` // "" for a wrapper-lite's one account
	Region string `json:"region"`
	Name   string `json:"name"` // the Apple ID; "" if logged in before the wrapper noted it
	Ready  bool   `json:"ready"`
}

// accounts lists what the wrapper holds: a wrapper-manager's instances, or
// a wrapper-lite's one account, which /status gives as its name and regions.
func (w *wrapper) accounts(ctx context.Context) ([]instance, error) {
	data, err := w.call(ctx, "/status", nil)
	if err != nil {
		return nil, err
	}
	var s struct {
		Instances *[]instance `json:"instances"`
		Name      string      `json:"name"`
		Regions   []string    `json:"regions"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%w /status: %w", errWrapper, err)
	}
	if s.Instances != nil {
		return *s.Instances, nil
	}
	if len(s.Regions) == 0 {
		return nil, nil
	}
	return []instance{{Region: s.Regions[0], Name: s.Name, Ready: true}}, nil
}

// instanceID is the id a wrapper-manager gives the account logged in as
// username: a UUID v5 of it in the nil namespace (manager/registry.go). It
// finds an account logged in before the wrapper noted Apple IDs.
func instanceID(username string) string {
	h := sha1.New()
	h.Write(make([]byte, 16))
	h.Write([]byte(username))
	b := h.Sum(nil)[:16]
	b[6] = b[6]&0x0f | 0x50
	b[8] = b[8]&0x3f | 0x80
	s := hex.EncodeToString(b)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

// pick finds the account the listener named: by its Apple ID or id first,
// else the first ready one of a region, as a region code names it.
func pick(accounts []instance, named string) (instance, bool) {
	named = strings.TrimSpace(named)
	id := instanceID(named)
	for _, a := range accounts {
		if a.ID != "" && (strings.EqualFold(a.ID, named) || a.ID == id) || a.Name != "" && strings.EqualFold(a.Name, named) {
			return a, true
		}
	}
	for _, a := range accounts {
		if a.Ready && strings.EqualFold(a.Region, named) {
			return a, true
		}
	}
	return instance{}, false
}

// errAccount is the wrapper not holding the account named, or it not being
// ready; its message says what the wrapper holds.
var errAccount = errors.New("account")

// tokenReply is what the caller is answered: the token, and when the wrapper
// refreshes it next, in Unix seconds.
type tokenReply struct {
	Token   string `json:"token"`
	Region  string `json:"region"`
	Next    int64  `json:"next"`
	Account string `json:"account"` // the Apple ID, or the region where the wrapper has none
}

// accountToken asks the wrapper for the media-user-token of the account named.
func (w *wrapper) accountToken(ctx context.Context, named string) (tokenReply, error) {
	accounts, err := w.accounts(ctx)
	if err != nil {
		return tokenReply{}, err
	}
	a, ok := pick(accounts, named)
	if !ok {
		return tokenReply{}, fmt.Errorf("%w: the wrapper holds no account %q; it holds %s", errAccount, named, describe(accounts))
	}
	if !a.Ready {
		return tokenReply{}, fmt.Errorf("%w: the wrapper's account %s isn't ready", errAccount, label(a))
	}
	var query url.Values
	if a.ID != "" {
		query = url.Values{"instance": {a.ID}}
	}
	data, err := w.call(ctx, "/token", query)
	var re *replyError
	if errors.As(err, &re) && re.status == http.StatusNotFound {
		return tokenReply{}, fmt.Errorf("%w: this wrapper can't give its accounts' tokens; AMDL-Web's can", errWrapper)
	}
	if err != nil {
		return tokenReply{}, err
	}
	var t struct {
		Region        string `json:"region"`
		MusicToken    string `json:"music_token"`
		NextRefreshAt int64  `json:"next_refresh_at"`
	}
	// The decode error is left out, as it may quote the token.
	if json.Unmarshal(data, &t) != nil || t.MusicToken == "" {
		return tokenReply{}, fmt.Errorf("%w /token: no music token in the answer", errWrapper)
	}
	return tokenReply{Token: t.MusicToken, Region: t.Region, Next: t.NextRefreshAt, Account: label(a)}, nil
}

func label(a instance) string {
	if a.Name != "" {
		return a.Name
	}
	return strings.ToUpper(a.Region)
}

func describe(accounts []instance) string {
	if len(accounts) == 0 {
		return "none"
	}
	var parts []string
	for _, a := range accounts {
		s := label(a)
		if a.Name != "" {
			s += " (" + strings.ToUpper(a.Region) + ")"
		}
		if !a.Ready {
			s += ", not ready"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}
