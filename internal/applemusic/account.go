package applemusic

import (
	"cmp"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Accounts. A listener signs in, on music.apple.com in a window the app
// shows or with a media-user-token they copied out of their browser, as many
// times as they have accounts, and switches between them here. Accounts the
// listener's wrapper holds can be switched to as well, without signing in
// (wrapperaccount.go).
//
// Whoever is current is the token in userTokenKey, with their storefront and
// profile kept beside it, as when there was one account: everything that
// asks Apple reads that. Each account signed in to here is kept in
// accountsKey with its token, and what the list of them shows; which one is
// current is in currentKey, as "own:<id>" or "wrapper:<the wrapper's name
// for it>".

// ownAccount is an account signed in to here.
type ownAccount struct {
	ID    string `json:"id"`
	Token string `json:"token"`
	// Social is the ID of the account's Apple Music profile, the one thing
	// Apple's answers say that tells two accounts apart: /v1/me/account
	// answers the same ID for any. "" for an account without a profile.
	Social string `json:"social,omitempty"`
	// What the list shows of it, as it was when last current.
	Name       string `json:"name,omitempty"`
	ImageURL   string `json:"imageUrl,omitempty"`
	Storefront string `json:"storefront,omitempty"`
	// Used is when it was last switched to, in ms.
	Used int64 `json:"used,omitempty"`
}

const (
	ownPrefix     = "own:"
	wrapperPrefix = "wrapper:"
)

func cookieValue(cookie, name string) string {
	for _, part := range strings.Split(cookie, ";") {
		at := strings.Index(part, "=")
		if at > 0 && strings.TrimSpace(part[:at]) == name {
			v := strings.TrimSpace(part[at+1:])
			if d, err := url.PathUnescape(v); err == nil {
				return d
			}
			return v
		}
	}
	return ""
}

var (
	cookiePrefix = regexp.MustCompile(`(?i)^cookie:\s*`)
	tokenCookie  = regexp.MustCompile(`(^|[\s;])media-user-token=`)
	quoted       = regexp.MustCompile(`^"(.*)"$`)
	notAToken    = regexp.MustCompile(`[\s;,]`)
)

// tokenIn is the token in what the listener pasted. The window can't sign
// in to an account that signs in with a passkey or a security key, as it has
// no WebAuthn: such a listener copies the media-user-token out of their own
// browser's cookies for music.apple.com, as the token alone or the cookies
// as copied.
func tokenIn(given string) string {
	given = strings.TrimSpace(given)
	cookie := cookiePrefix.ReplaceAllString(given, "")
	if tokenCookie.MatchString(cookie) {
		given = cookieValue(cookie, "media-user-token")
	}
	return strings.TrimSpace(quoted.ReplaceAllString(given, "$1"))
}

func (s *Source) ownAccounts() []ownAccount {
	var list []ownAccount
	s.storage.get(accountsKey, &list)
	return list
}

func (s *Source) current() string { return s.storage.getString(currentKey) }

// currentOwn is the account signed in to here that's current, if one is.
func (s *Source) currentOwn() (ownAccount, bool) {
	id, ok := strings.CutPrefix(s.current(), ownPrefix)
	if !ok {
		return ownAccount{}, false
	}
	for _, a := range s.ownAccounts() {
		if a.ID == id {
			return a, true
		}
	}
	return ownAccount{}, false
}

// become makes token the one used, for the account named current, and
// forgets what was kept for whoever was before. sf is the account's
// storefront, if it's known. Under acctMu.
func (s *Source) become(current, token, sf string) {
	s.storage.set(userTokenKey, token)
	s.storage.remove(storefrontKey, profileKey)
	if sf != "" {
		s.storage.set(storefrontKey, sf)
	}
	if current == "" {
		s.storage.remove(currentKey)
	} else {
		s.storage.set(currentKey, current)
	}
	s.forgetLibrary()
}

// signOut leaves nobody current. Under acctMu.
func (s *Source) signOut() {
	s.storage.remove(userTokenKey, storefrontKey, profileKey, currentKey)
	s.forgetLibrary()
}

// becomeOwn makes the account signed in to here current. Under acctMu.
func (s *Source) becomeOwn(a ownAccount, sf string) {
	if sf == "" {
		sf = a.Storefront
	}
	s.become(ownPrefix+a.ID, a.Token, sf)
	s.touchOwn(a.ID)
}

func (s *Source) touchOwn(id string) {
	list := s.ownAccounts()
	for i := range list {
		if list[i].ID == id {
			list[i].Used = s.now()
		}
	}
	s.storage.set(accountsKey, list)
}

// who is what Apple says of the account a token signs in to.
type who struct {
	profile
	storefront string
	social     string
}

// whoIs asks Apple who token signs in to, without making it current. A
// token Apple refuses is ErrSignIn.
func (s *Source) whoIs(token string) (who, error) {
	o := reqOpts{token: token}
	var (
		wg sync.WaitGroup
		w  who
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.profile, w.social, _ = s.fetchProfile(o)
	}()
	b, err := s.api("/v1/me/storefront", nil, o)
	wg.Wait()
	if err != nil {
		return who{}, err
	}
	if len(b.Data) == 0 || b.Data[0] == nil || b.Data[0].ID == "" {
		return who{}, fail("signIn", i18n.T("Apple Music didn't accept the sign-in. Sign in again."))
	}
	w.storefront = b.Data[0].ID
	return w, nil
}

// addAccount signs in with token, as an account of its own, or as the one
// signed in to here before that has the same profile, or the same token,
// whose token it replaces. Then it's current.
func (s *Source) addAccount(token string) (*music.Account, error) {
	s.migrate()
	if token == "" || notAToken.MatchString(token) {
		return nil, fail("setup", i18n.T("That isn't a media-user-token: paste the value of the media-user-token cookie"))
	}
	w, err := s.whoIs(token)
	if errors.Is(err, music.ErrSignIn) {
		return nil, fail("setup", i18n.T("Apple Music didn't accept this Media User Token. Copy a new one and try again."))
	}
	if err != nil {
		return nil, err
	}
	s.acctMu.Lock()
	list := s.ownAccounts()
	at := slices.IndexFunc(list, func(a ownAccount) bool {
		return a.Token == token || w.social != "" && a.Social == w.social
	})
	if at < 0 {
		list = append(list, ownAccount{ID: uuid()})
		at = len(list) - 1
	}
	a := &list[at]
	a.Token, a.Social, a.Storefront = token, w.social, w.storefront
	a.Name, a.ImageURL = w.Name, w.ImageURL
	s.storage.set(accountsKey, list)
	s.becomeOwn(*a, w.storefront)
	s.storage.set(profileKey, w.profile)
	s.acctMu.Unlock()
	return s.account()
}

// useAccount switches to the account of id. Apple, or the wrapper, is asked
// first, so that one that fails leaves whoever was current.
func (s *Source) useAccount(id string) (*music.Account, error) {
	s.migrate()
	if pick, ok := strings.CutPrefix(id, wrapperPrefix); ok {
		if err := s.useWrapper(pick); err != nil {
			return nil, err
		}
		return s.account()
	}
	own, ok := strings.CutPrefix(id, ownPrefix)
	i := slices.IndexFunc(s.ownAccounts(), func(a ownAccount) bool { return a.ID == own })
	if !ok || i < 0 {
		return nil, fail("unavailable", i18n.T("This account isn't here anymore"))
	}
	a := s.ownAccounts()[i]
	if s.current() == id && s.userToken() == a.Token {
		return s.account()
	}
	w, err := s.whoIs(a.Token)
	if errors.Is(err, music.ErrSignIn) {
		return nil, fail("signIn", i18n.T("Apple Music didn't accept this account's sign-in. Add it again."))
	}
	if err != nil {
		return nil, err
	}
	s.acctMu.Lock()
	s.becomeOwn(a, w.storefront)
	s.storage.set(profileKey, w.profile)
	s.acctMu.Unlock()
	return s.account()
}

// removeAccount forgets an account signed in to here; the current one is signed out of.
func (s *Source) removeAccount(id string) error {
	s.migrate()
	if strings.HasPrefix(id, wrapperPrefix) {
		return fail("unsupported", i18n.T("An account the wrapper holds can't be removed here"))
	}
	if s.current() == id {
		return s.leave()
	}
	own := strings.TrimPrefix(id, ownPrefix)
	s.acctMu.Lock()
	defer s.acctMu.Unlock()
	list := slices.DeleteFunc(s.ownAccounts(), func(a ownAccount) bool { return a.ID == own })
	s.storage.set(accountsKey, list)
	return nil
}

// leave signs out of the current account, forgetting it, and goes on as the
// account signed in to here that was used last, if there's another. Not out
// of an account of the wrapper's, which the listener leaves by switching to
// another: there's nothing of it here to sign out of.
func (s *Source) leave() error {
	s.migrate()
	s.acctMu.Lock()
	defer s.acctMu.Unlock()
	cur := s.current()
	if strings.HasPrefix(cur, wrapperPrefix) && s.signedIn() {
		return fail("unsupported", i18n.T("An account the wrapper holds can't be signed out of: switch to another account instead"))
	}
	own := strings.TrimPrefix(cur, ownPrefix)
	list := slices.DeleteFunc(s.ownAccounts(), func(a ownAccount) bool { return cur != "" && a.ID == own })
	s.storage.set(accountsKey, list)
	s.storage.remove(wrapperAcctKey)
	if len(list) == 0 {
		s.signOut()
		return nil
	}
	last := slices.MaxFunc(list, func(a, b ownAccount) int { return cmp.Compare(a.Used, b.Used) })
	s.becomeOwn(last, "")
	return nil
}

func (s *Source) signIn() (*music.Account, error) {
	if s.webSignIn == nil {
		return nil, fail("unsupported", i18n.T("This computer can't show the page to sign in on"))
	}
	cookie, err := s.webSignIn(WebSignIn{
		Name:      "Apple Music",
		URL:       s.web + "/us/browse",
		CookiesOf: s.web,
		Until:     []string{"media-user-token"},
		UserAgent: userAgent,
	})
	if err != nil {
		return nil, err
	}
	token := cookieValue(cookie, "media-user-token")
	if token == "" {
		return nil, errors.New(i18n.T("Apple Music didn't sign you in"))
	}
	return s.addAccount(token)
}

// accounts lists the accounts signed in to here, in the order they were
// added, then those the wrapper offers, the current one among them.
func (s *Source) accounts(fresh bool) ([]music.AccountChoice, error) {
	s.migrate()
	cur := s.current()
	var out []music.AccountChoice
	for _, a := range s.ownAccounts() {
		name := a.Name
		if name == "" {
			name = fallbackName(a.Storefront)
		}
		out = append(out, music.AccountChoice{
			ID: ownPrefix + a.ID, Name: name, ImageURL: a.ImageURL, Region: a.Storefront,
			Current: cur == ownPrefix+a.ID && s.signedIn(), Used: usedAt(a.Used),
		})
	}
	held, err := s.wrapperChoices(fresh)
	return append(out, held...), err
}

func usedAt(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// fallbackName is what an account without a profile goes by.
func fallbackName(sf string) string {
	if sf == "" {
		return "Apple Music"
	}
	return "Apple Music (" + strings.ToUpper(sf) + ")"
}

// migrate turns how accounts were kept when there was one into the list,
// once: a token from the window, or from the Media User Token setting, is
// an account signed in to here, and the account the Wrapper Account setting
// named is an account of the wrapper's, current if it was signed in with.
func (s *Source) migrate() {
	s.migrated.Do(func() {
		s.acctMu.Lock()
		defer s.acctMu.Unlock()
		var list []ownAccount
		if s.storage.get(accountsKey, &list) {
			return
		}
		inUse := s.userToken()
		kept := s.keptFromWrapper()
		named := strings.TrimSpace(s.Setting("wrapperAccount"))
		given := tokenIn(s.Setting("mediaUserToken"))
		if given != "" && notAToken.MatchString(given) {
			given = ""
		}
		current := ""
		switch {
		case inUse != "" && kept != nil && kept.Token == inUse:
			// The account of the wrapper's signed in with, as it was named.
			pick := named
			if _, after, ok := strings.Cut(kept.Of, "\n"); ok && after != "" {
				pick = after
			}
			current = wrapperPrefix + pick
			again := *kept
			again.Of = s.wrapperOf(pick)
			s.storage.set(wrapperAcctKey, again)
		case inUse != "":
			// From the window, or the setting.
			var p profile
			s.storage.get(profileKey, &p)
			a := ownAccount{ID: uuid(), Token: inUse, Name: p.Name, ImageURL: p.ImageURL,
				Storefront: s.storage.getString(storefrontKey), Used: s.now()}
			list = append(list, a)
			current = ownPrefix + a.ID
		case named != "" && given == "" && kept == nil:
			// Named, and not yet signed in with, nor signed out of.
			current = wrapperPrefix + named
		}
		if given != "" && given != inUse {
			list = append(list, ownAccount{ID: uuid(), Token: given})
		}
		if list == nil {
			list = []ownAccount{}
		}
		s.storage.set(accountsKey, list)
		if current != "" {
			s.storage.set(currentKey, current)
		}
		s.storage.remove(givenTokenKey)
		s.settings.remove("mediaUserToken", "wrapperAccount")
	})
}

// Who's signed in, as the web player shows them: the name and picture of
// their Apple Music profile, or, without one, the picture of their account.
// Kept for a day, as the app asks for the account often.

type profile struct {
	Name     string `json:"name,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	At       int64  `json:"at"`
}

const profileKept = 24 * time.Hour

func (s *Source) profile() profile {
	var kept profile
	if s.storage.get(profileKey, &kept) && s.now()-kept.At < profileKept.Milliseconds() {
		return kept
	}
	found, social, ok := s.fetchProfile(reqOpts{})
	// Apple not answering isn't who they are: try again next time.
	if ok {
		s.storage.set(profileKey, found)
		s.noteSocial(social)
	}
	return found
}

// noteSocial keeps the profile's ID of the current account, which may have
// made one since it was added.
func (s *Source) noteSocial(social string) {
	if social == "" {
		return
	}
	s.acctMu.Lock()
	defer s.acctMu.Unlock()
	a, ok := s.currentOwn()
	if !ok || a.Social == social {
		return
	}
	list := s.ownAccounts()
	for i := range list {
		if list[i].ID == a.ID {
			list[i].Social = social
		}
	}
	s.storage.set(accountsKey, list)
}

// fetchProfile asks Apple for the profile, answering its ID too, and
// whether Apple answered.
func (s *Source) fetchProfile(o reqOpts) (found profile, social string, ok bool) {
	var (
		wg         sync.WaitGroup
		mine, own  *body
		errS, errO error
	)
	ask := func(path string, into **body, e *error) {
		defer wg.Done()
		*into, *e = s.api("/v1/me"+path, nil, o)
	}
	wg.Add(2)
	go ask("/social-profile", &mine, &errS)
	go ask("/account", &own, &errO)
	wg.Wait()
	found = profile{At: s.now()}
	var picture *artwork
	if mine != nil && len(mine.Data) > 0 && mine.Data[0] != nil {
		a := mine.Data[0].attrs()
		found.Name = a.Name
		picture = a.Artwork
		social = mine.Data[0].ID
	}
	if picture == nil && own != nil && len(own.Data) > 0 && own.Data[0] != nil {
		picture = own.Data[0].attrs().AvatarArtwork
	}
	found.ImageURL = strings.Replace(art(picture), "{size}x{size}", "300x300", 1)
	return found, social, errS == nil || errO == nil
}

func (s *Source) account() (*music.Account, error) {
	s.migrate()
	// A wrapper not answering is said as the listener switches to its account;
	// until then, whoever is signed in stays.
	s.useWrapperAccount(false)
	if !s.signedIn() {
		return nil, nil
	}
	var (
		wg    sync.WaitGroup
		p     profile
		sf    string
		sfErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		p = s.profile()
	}()
	sf, sfErr = s.storefront()
	wg.Wait()
	if sfErr != nil {
		return nil, sfErr
	}
	name := p.Name
	if name == "" {
		name = fallbackName(sf)
	}
	cur := s.current()
	s.snapshot(p, sf)
	return &music.Account{ID: cur, Name: name, ImageURL: p.ImageURL, Shared: strings.HasPrefix(cur, wrapperPrefix)}, nil
}

// snapshot keeps what the list shows of the current account, as it's now.
func (s *Source) snapshot(p profile, sf string) {
	s.acctMu.Lock()
	defer s.acctMu.Unlock()
	a, ok := s.currentOwn()
	if !ok || a.Token != s.userToken() || a.Name == p.Name && a.ImageURL == p.ImageURL && a.Storefront == sf {
		return
	}
	list := s.ownAccounts()
	for i := range list {
		if list[i].ID == a.ID {
			list[i].Name, list[i].ImageURL, list[i].Storefront = p.Name, p.ImageURL, sf
		}
	}
	s.storage.set(accountsKey, list)
}
