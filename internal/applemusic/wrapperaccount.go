package applemusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"mygo-music/internal/i18n"
)

// Signing in as an account the listener's wrapper holds. AMDL-Web's wrapper
// gives the media-user-token of each account it holds (GET /token) and
// refreshes it on its own, so a listener who logged their account in to the
// wrapper needn't sign in here too: not with the window, nor by copying the
// token out of a browser, which an account with a passkey would need.
//
// A wrapper-manager holds up to ten accounts, perhaps a household's or one
// for each region, each with its own library. Which one /token answers
// without being told is whichever happens to come first, so the listener
// names theirs in the Wrapper Account setting: its Apple ID, or a region
// where the wrapper holds one. The songs played lossless are the wrapper's
// business either way: it plays each with any account whose region has it.
//
// amhelper asks the wrapper, as it already holds the wrapper's address and
// key.

// fromWrapper is what the wrapper gave, kept in storage ("wrapperAccount").
type fromWrapper struct {
	// Of is the wrapper and the account, as the settings named them when asked.
	Of    string `json:"of"`
	Token string `json:"token"`
	// Next is when to ask the wrapper again, in ms.
	Next int64 `json:"next"`
}

// wrapperAccount is the account named in the settings, or "" for none.
func (s *Source) wrapperAccount() string { return strings.TrimSpace(s.Setting("wrapperAccount")) }

// wrapperAndAccount is the wrapper and the account named, as one, to tell when either changes.
func (s *Source) wrapperAndAccount() string {
	return strings.TrimSpace(s.Setting("wrapper")) + "\n" + s.wrapperAccount()
}

// Asked again a little after the wrapper refreshes the token, though not
// within a minute, nor later than an hour: a wrapper on Cloudflare that was
// asleep answers with a time long past. After a failure, in five minutes.
const (
	renewSoonest = time.Minute
	renewLatest  = time.Hour
	renewRetry   = 5 * time.Minute
)

func (s *Source) askAt(nextRefresh int64) int64 {
	now := s.now()
	wait := nextRefresh*1000 + 5000 - now
	if wait == 0 {
		wait = renewLatest.Milliseconds()
	}
	return now + min(renewLatest.Milliseconds(), max(renewSoonest.Milliseconds(), wait))
}

// notHeldError is the wrapper not holding the account named, or it not being ready.
type notHeldError struct{ error }

func (e notHeldError) Unwrap() error { return e.error }

func notHeld(err error) bool {
	var e notHeldError
	return errors.As(err, &e)
}

// askWrapper asks the wrapper for the token of the account named in the settings.
func (s *Source) askWrapper() (*fromWrapper, error) {
	if !s.hasWrapper() {
		return nil, fail("setup", i18n.T("Set the Wrapper Address too, to sign in with an account the wrapper holds"))
	}
	of := s.wrapperAndAccount()
	base, err := s.helperBase()
	if err != nil {
		return nil, err
	}
	res, err := s.helperDo("GET", base+"/token?account="+url.QueryEscape(s.wrapperAccount()), nil)
	if err != nil {
		return nil, err
	}
	var b struct {
		Token   string `json:"token"`
		Next    int64  `json:"next"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	json.Unmarshal(res.body, &b)
	if res.status == 404 && b.Code == "account" {
		return nil, notHeldError{fail("setup", i18n.T("Check the Wrapper Account in the settings")+" ("+b.Message+")")}
	}
	if !res.ok() || b.Token == "" {
		why := b.Message
		if why == "" {
			why = fmt.Sprint(res.status)
		}
		return nil, errors.New("amhelper: " + why)
	}
	return &fromWrapper{Of: of, Token: b.Token, Next: s.askAt(b.Next)}, nil
}

func (s *Source) keptFromWrapper() *fromWrapper {
	var k fromWrapper
	if !s.storage.get(wrapperAcctKey, &k) {
		return nil
	}
	return &k
}

// wrapperInUse is what the wrapper gave, while the listener is signed in with it.
func (s *Source) wrapperInUse() *fromWrapper {
	k := s.keptFromWrapper()
	if k != nil && k.Token == s.userToken() {
		return k
	}
	return nil
}

type renewCall struct {
	done  chan struct{}
	token string
	err   error
}

// renew asks the wrapper again for the account signed in with, and takes
// what it gives: the same account, so nothing kept for it is forgotten. Not
// after the listener signed out, or in another way, or named another
// account, which account.go sees to. Asked twice at once, it asks once.
func (s *Source) renew() (string, error) {
	s.renewMu.Lock()
	if c := s.renewing; c != nil {
		s.renewMu.Unlock()
		<-c.done
		return c.token, c.err
	}
	c := &renewCall{done: make(chan struct{})}
	s.renewing = c
	s.renewMu.Unlock()
	c.token, c.err = s.doRenew()
	s.renewMu.Lock()
	s.renewing = nil
	s.renewMu.Unlock()
	close(c.done)
	return c.token, c.err
}

func (s *Source) doRenew() (string, error) {
	was := s.wrapperInUse()
	if was == nil || was.Of != s.wrapperAndAccount() {
		return "", nil
	}
	got, err := s.askWrapper()
	if err != nil {
		if k := s.wrapperInUse(); k != nil && k.Token == was.Token {
			again := *was
			again.Next = s.now() + renewRetry.Milliseconds()
			s.storage.set(wrapperAcctKey, again)
		}
		return "", err
	}
	if k := s.wrapperInUse(); k == nil || k.Token != was.Token {
		return "", nil
	}
	s.storage.set(wrapperAcctKey, got)
	s.storage.set(userTokenKey, got.Token)
	return got.Token, nil
}

// renewIfDue asks the wrapper again, in the background, once the token it gave is due.
func (s *Source) renewIfDue() {
	k := s.wrapperInUse()
	if k == nil || s.now() < k.Next {
		return
	}
	s.renewMu.Lock()
	busy := s.renewing != nil
	s.renewMu.Unlock()
	if !busy {
		s.spawn(func() { s.renew() })
	}
}

// renewRefused says, after Apple refused the token, whether the wrapper
// gives another, which is then used, as its own refresh may have replaced the
// one refused.
func (s *Source) renewRefused(refused string) bool {
	if k := s.wrapperInUse(); k == nil || k.Token != refused {
		return false
	}
	got, err := s.renew()
	return err == nil && got != "" && got != refused
}
