package applemusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// The accounts the listener's wrapper holds, to switch to without signing
// in. AMDL-Web's wrapper gives the media-user-token of each account it holds
// (GET /token) and refreshes it on its own, so a listener who logged their
// account in to the wrapper needn't sign in here too: not with the window,
// nor by copying the token out of a browser, which an account with a passkey
// would need.
//
// A wrapper-manager holds up to ten accounts, perhaps a household's or one
// for each region, each with its own library. Each ready one is offered,
// if the wrapper gives tokens at all, and named by the wrapper's id for it.
// The token is asked for as the listener switches to it, and asked again as
// the wrapper refreshes it; it's kept only while the account is current.
// The songs played lossless are the wrapper's business either way: it plays
// each with any account whose region has it.
//
// amhelper asks the wrapper, as it already holds the wrapper's address and
// key.

// fromWrapper is what the wrapper gave for the current account, kept in
// storage ("wrapperAccount").
type fromWrapper struct {
	// Of is the wrapper and the account, as wrapperOf says, when asked.
	Of    string `json:"of"`
	Token string `json:"token"`
	// Next is when to ask the wrapper again, in ms.
	Next int64 `json:"next"`
	// What the list shows of it while the wrapper doesn't answer.
	Name   string `json:"name,omitempty"`
	Region string `json:"region,omitempty"`
}

// wrapperAccount is what names the current account, if it's the wrapper's, or "".
func (s *Source) wrapperAccount() string {
	pick, _ := strings.CutPrefix(s.current(), wrapperPrefix)
	if !strings.HasPrefix(s.current(), wrapperPrefix) {
		return ""
	}
	return pick
}

// wrapperOf is the wrapper and an account of it, as one, to tell when either changes.
func (s *Source) wrapperOf(pick string) string {
	return strings.TrimSpace(s.Setting("wrapper")) + "\n" + pick
}

func (s *Source) wrapperAndAccount() string { return s.wrapperOf(s.wrapperAccount()) }

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

// askWrapper asks the wrapper for the token of the account pick names. What
// it gives is named by the wrapper's id for the account, which a region code
// kept from before isn't.
func (s *Source) askWrapper(pick string) (*fromWrapper, error) {
	if !s.hasWrapper() {
		return nil, fail("setup", i18n.T("Set the Wrapper Address in the settings to use the wrapper's accounts"))
	}
	base, err := s.helperBase()
	if err != nil {
		return nil, err
	}
	res, err := s.helperDo("GET", base+"/token?account="+url.QueryEscape(pick), nil)
	if err != nil {
		return nil, err
	}
	var b struct {
		Token    string `json:"token"`
		Next     int64  `json:"next"`
		Region   string `json:"region"`
		Account  string `json:"account"`
		Instance string `json:"instance"`
		Code     string `json:"code"`
		Message  string `json:"message"`
	}
	json.Unmarshal(res.body, &b)
	if res.status == 404 && b.Code == "account" {
		return nil, fail("setup", i18n.T("The wrapper doesn't hold this account anymore, or it isn't ready")+" ("+b.Message+")")
	}
	if !res.ok() || b.Token == "" {
		why := b.Message
		if why == "" {
			why = fmt.Sprint(res.status)
		}
		return nil, errors.New("amhelper: " + why)
	}
	if b.Instance != "" {
		pick = b.Instance
	}
	return &fromWrapper{Of: s.wrapperOf(pick), Token: b.Token, Next: s.askAt(b.Next),
		Name: b.Account, Region: strings.ToLower(b.Region)}, nil
}

// pickOf is the account a fromWrapper is for.
func (k *fromWrapper) pickOf() string {
	_, pick, _ := strings.Cut(k.Of, "\n")
	return pick
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

// A wrapper that didn't answer isn't asked again for a minute, as the app
// asks for the account often and a wrapper not there takes long to fail.
const wrapperRetryAfter = time.Minute

type wrapperFailure struct {
	of string
	at time.Time
}

// useWrapperAccount gets the token of the current account, if it's the
// wrapper's and none was got for it from this wrapper: as the source starts
// for the first time since the Wrapper Account setting named it, or after
// the wrapper's address changed. A wrapper that's gone leaves nobody signed
// in until it's back.
func (s *Source) useWrapperAccount(force bool) error {
	s.acctMu.Lock()
	defer s.acctMu.Unlock()
	pick := s.wrapperAccount()
	if pick == "" {
		return nil
	}
	wanted := s.wrapperAndAccount()
	kept := s.keptFromWrapper()
	if kept != nil && kept.Of == wanted && s.userToken() == kept.Token && !force {
		return nil
	}
	if kept != nil && kept.Of != wanted && s.userToken() == kept.Token {
		s.storage.remove(wrapperAcctKey, userTokenKey, storefrontKey, profileKey)
		s.forgetLibrary()
	}
	if !s.hasWrapper() {
		return nil
	}
	if f := s.wrapperFailed; !force && f != nil && f.of == wanted && s.clk.Now().Sub(f.at) < wrapperRetryAfter {
		return nil
	}
	got, err := s.askWrapper(pick)
	if err != nil {
		s.wrapperFailed = &wrapperFailure{wanted, s.clk.Now()}
		return err
	}
	s.wrapperFailed = nil
	s.storage.set(wrapperAcctKey, got)
	s.become(wrapperPrefix+got.pickOf(), got.Token, "")
	s.touchWrapper(got.pickOf())
	return nil
}

// useWrapper switches to the wrapper's account that pick names, once the
// wrapper gave its token and Apple took it.
func (s *Source) useWrapper(pick string) error {
	got, err := s.askWrapper(pick)
	if err != nil {
		return err
	}
	w, err := s.whoIs(got.Token)
	if errors.Is(err, music.ErrSignIn) {
		return fail("setup", i18n.T("Apple Music didn't accept the token of the wrapper's account. Log it in to the wrapper again."))
	}
	if err != nil {
		return err
	}
	s.acctMu.Lock()
	defer s.acctMu.Unlock()
	s.wrapperFailed = nil
	s.storage.set(wrapperAcctKey, got)
	s.become(wrapperPrefix+got.pickOf(), got.Token, w.storefront)
	s.storage.set(profileKey, w.profile)
	s.touchWrapper(got.pickOf())
	return nil
}

func (s *Source) touchWrapper(pick string) {
	used := map[string]int64{}
	s.storage.get(wrapperUsedKey, &used)
	used[pick] = s.now()
	s.storage.set(wrapperUsedKey, used)
}

// retryWrapper forgets what the wrapper answered, as its address or key changed.
func (s *Source) retryWrapper() {
	s.acctMu.Lock()
	s.wrapperFailed = nil
	s.acctMu.Unlock()
	s.heldMu.Lock()
	s.held = nil
	s.heldMu.Unlock()
}

// heldList is the wrapper's accounts as last listed.
type heldList struct {
	of       string
	at       time.Time
	accounts []heldAccount
	err      error
}

type heldAccount struct {
	ID     string `json:"id"`
	Region string `json:"region"`
	Name   string `json:"name"`
}

// pick is what names the account to the wrapper: its id, or for a
// wrapper-lite's one account, which has none, its region.
func (h heldAccount) pick() string {
	if h.ID != "" {
		return h.ID
	}
	return h.Region
}

// Listed again after a minute, as the settings and the account menu ask
// whenever they show.
const heldListed = time.Minute

// wrapperChoices are the wrapper's accounts to switch to, the current one
// among them even while the wrapper doesn't answer. Without a wrapper,
// none, and no error.
func (s *Source) wrapperChoices(fresh bool) ([]music.AccountChoice, error) {
	pick := s.wrapperAccount()
	if !s.hasWrapper() {
		return nil, nil
	}
	of := s.wrapperOf("")
	s.heldMu.Lock()
	h := s.held
	s.heldMu.Unlock()
	if fresh || h == nil || h.of != of || s.clk.Now().Sub(h.at) >= heldListed {
		h = &heldList{of: of, at: s.clk.Now()}
		h.accounts, h.err = s.listHeld()
		s.heldMu.Lock()
		s.held = h
		s.heldMu.Unlock()
	}
	used := map[string]int64{}
	s.storage.get(wrapperUsedKey, &used)
	at := -1
	if pick != "" && s.signedIn() {
		at = pickHeld(h.accounts, pick)
	}
	var out []music.AccountChoice
	found := at >= 0
	for i, a := range h.accounts {
		current := i == at
		out = append(out, music.AccountChoice{
			ID: wrapperPrefix + a.pick(), Name: heldName(a.Name, a.Region), Region: a.Region,
			Shared: true, Current: current, Used: usedAt(used[a.pick()]),
		})
	}
	if k := s.wrapperInUse(); !found && k != nil && pick != "" {
		out = append(out, music.AccountChoice{
			ID: wrapperPrefix + pick, Name: heldName(k.Name, k.Region), Region: k.Region,
			Shared: true, Current: true, Used: usedAt(used[pick]),
		})
	}
	return out, h.err
}

// pickHeld is the index of the account pick names, as amhelper finds it: by
// its id or Apple ID, else the first of a region, as a region code kept from
// the Wrapper Account setting names it; -1 for none.
func pickHeld(accounts []heldAccount, pick string) int {
	if i := slices.IndexFunc(accounts, func(h heldAccount) bool {
		return strings.EqualFold(h.pick(), pick) || h.Name != "" && strings.EqualFold(h.Name, pick)
	}); i >= 0 {
		return i
	}
	return slices.IndexFunc(accounts, func(h heldAccount) bool { return strings.EqualFold(h.Region, pick) })
}

// heldName is what a wrapper's account goes by: its Apple ID, which the
// wrapper may not have noted, else where it is.
func heldName(name, region string) string {
	if name != "" && !strings.EqualFold(name, region) {
		return name
	}
	return i18n.Tf("Account in %s", strings.ToUpper(region))
}

// listHeld asks amhelper for the wrapper's accounts to switch to.
func (s *Source) listHeld() ([]heldAccount, error) {
	base, err := s.helperBase()
	if err != nil {
		return nil, err
	}
	res, err := s.helperDo("GET", base+"/accounts", nil)
	if err != nil {
		return nil, err
	}
	if !res.ok() {
		why := res.err().Message
		if why == "" {
			why = fmt.Sprint(res.status)
		}
		return nil, errors.New("amhelper: " + why)
	}
	var b struct {
		Accounts []heldAccount `json:"accounts"`
	}
	if err := json.Unmarshal(res.body, &b); err != nil {
		return nil, err
	}
	return b.Accounts, nil
}

type renewCall struct {
	done  chan struct{}
	token string
	err   error
}

// renew asks the wrapper again for the account signed in with, and takes
// what it gives: the same account, so nothing kept for it is forgotten. Not
// after the listener switched to another account, which account.go sees to.
// Asked twice at once, it asks once.
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
	got, err := s.askWrapper(s.wrapperAccount())
	if err != nil {
		if k := s.wrapperInUse(); k != nil && k.Token == was.Token {
			again := *was
			again.Next = s.now() + renewRetry.Milliseconds()
			s.storage.set(wrapperAcctKey, again)
		}
		return "", err
	}
	s.acctMu.Lock()
	defer s.acctMu.Unlock()
	if k := s.wrapperInUse(); k == nil || k.Token != was.Token || s.wrapperAndAccount() != was.Of {
		return "", nil
	}
	s.storage.set(wrapperAcctKey, got)
	s.storage.set(userTokenKey, got.Token)
	// A region code kept from the Wrapper Account setting is now the
	// wrapper's id for the account it found.
	s.storage.set(currentKey, wrapperPrefix+got.pickOf())
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
