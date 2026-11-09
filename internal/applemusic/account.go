package applemusic

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Signing in, on music.apple.com in a window the app shows, with a
// media-user-token the listener gives in the settings, or as an account the
// listener's wrapper holds (wrapperaccount.go). A token given goes first,
// then the wrapper's account, and the window only without either.

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

// keep keeps the token the sign-in gives, and forgets what was kept for
// whoever signed in before.
func (s *Source) keep(token string) {
	s.storage.set(userTokenKey, token)
	s.storage.remove(storefrontKey, profileKey)
	s.forgetLibrary()
}

var (
	cookiePrefix = regexp.MustCompile(`(?i)^cookie:\s*`)
	tokenCookie  = regexp.MustCompile(`(^|[\s;])media-user-token=`)
	quoted       = regexp.MustCompile(`^"(.*)"$`)
	notAToken    = regexp.MustCompile(`[\s;,]`)
)

// givenToken is the token in the settings. The window can't sign in to an
// account that signs in with a passkey or a security key, as it has no
// WebAuthn: such a listener copies the media-user-token out of their own
// browser's cookies for music.apple.com, and gives it in the settings, as
// the token alone or the cookies as copied.
func (s *Source) givenToken() string {
	given := strings.TrimSpace(s.Setting("mediaUserToken"))
	cookie := cookiePrefix.ReplaceAllString(given, "")
	if tokenCookie.MatchString(cookie) {
		given = cookieValue(cookie, "media-user-token")
	}
	return strings.TrimSpace(quoted.ReplaceAllString(given, "$1"))
}

// useGivenToken signs in with the token given in the settings as it
// changes, and out once it's taken away, unless the listener signed in
// another way since. Which one was used is kept, as the setting may have
// changed while the source wasn't running to hear of it.
func (s *Source) useGivenToken() error {
	s.acctMu.Lock()
	defer s.acctMu.Unlock()
	given := s.givenToken()
	used := s.storage.getString(givenTokenKey)
	if given == used {
		return nil
	}
	if given != "" && notAToken.MatchString(given) {
		return fail("setup", i18n.T("That isn't a media-user-token: paste the value of the media-user-token cookie"))
	}
	if given != "" {
		s.storage.set(givenTokenKey, given)
		s.keep(given)
		return nil
	}
	s.storage.remove(givenTokenKey)
	if s.userToken() == used {
		s.signOut()
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

// useWrapperAccount signs in as the account of the wrapper the settings
// name, once it's named or the wrapper changes, and out of the one named
// before. Not again after the listener signed out, until they sign in.
func (s *Source) useWrapperAccount(force bool) error {
	s.acctMu.Lock()
	defer s.acctMu.Unlock()
	kept := s.keptFromWrapper()
	wanted := ""
	if s.wrapperAccount() != "" && s.givenToken() == "" {
		wanted = s.wrapperAndAccount()
	}
	if kept != nil && kept.Of == wanted && !force {
		return nil
	}
	if kept != nil {
		s.storage.remove(wrapperAcctKey)
		if s.userToken() == kept.Token {
			s.signOut()
		}
	}
	if wanted == "" {
		return nil
	}
	if f := s.wrapperFailed; !force && f != nil && f.of == wanted && s.clk.Now().Sub(f.at) < wrapperRetryAfter {
		return nil
	}
	got, err := s.askWrapper()
	if err != nil {
		s.wrapperFailed = &wrapperFailure{wanted, s.clk.Now()}
		return err
	}
	s.wrapperFailed = nil
	s.storage.set(wrapperAcctKey, got)
	s.keep(got.Token)
	return nil
}

// retryWrapper forgets that the wrapper didn't answer, as its address or key changed.
func (s *Source) retryWrapper() {
	s.acctMu.Lock()
	s.wrapperFailed = nil
	s.acctMu.Unlock()
}

func (s *Source) signIn() (*music.Account, error) {
	if s.givenToken() == "" && s.wrapperAccount() != "" {
		if err := s.useWrapperAccount(true); err != nil {
			return nil, err
		}
		a, err := s.account()
		if errors.Is(err, music.ErrSignIn) {
			return nil, fail("setup", i18n.T("Apple Music didn't accept the token of the wrapper's account. Log it in to the wrapper again."))
		}
		return a, err
	}
	if given := s.givenToken(); given != "" {
		// Again, as after signing out the token there hasn't changed. The
		// window won't help while it's there: it wouldn't be used.
		if err := s.useGivenToken(); err != nil {
			return nil, err
		}
		s.keep(given)
		a, err := s.account()
		if errors.Is(err, music.ErrSignIn) {
			return nil, fail("setup", i18n.T("Apple Music didn't accept the Media User Token in the settings. Paste a new one, or empty it to sign in here."))
		}
		return a, err
	}
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
	s.keep(token)
	return s.account()
}

func (s *Source) signOut() {
	s.storage.remove(userTokenKey, storefrontKey, profileKey)
	s.forgetLibrary()
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
	var (
		wg          sync.WaitGroup
		social, own *body
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		social, _ = s.me("/social-profile", nil, reqOpts{})
	}()
	go func() {
		defer wg.Done()
		own, _ = s.me("/account", nil, reqOpts{})
	}()
	wg.Wait()
	found := profile{At: s.now()}
	var picture *artwork
	if social != nil && len(social.Data) > 0 && social.Data[0] != nil {
		a := social.Data[0].attrs()
		found.Name = a.Name
		picture = a.Artwork
	}
	if picture == nil && own != nil && len(own.Data) > 0 && own.Data[0] != nil {
		picture = own.Data[0].attrs().AvatarArtwork
	}
	found.ImageURL = strings.Replace(art(picture), "{size}x{size}", "300x300", 1)
	// Apple not answering isn't who they are: try again next time.
	if social != nil || own != nil {
		s.storage.set(profileKey, found)
	}
	return found
}

func (s *Source) account() (*music.Account, error) {
	// A token not taken is said as it's given; until then, whoever is
	// signed in stays. A wrapper not answering is said as the listener signs in.
	s.useGivenToken()
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
		name = "Apple Music (" + strings.ToUpper(sf) + ")"
	}
	return &music.Account{ID: sf, Name: name, ImageURL: p.ImageURL}, nil
}
