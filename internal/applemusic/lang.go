package applemusic

import (
	"errors"
	"regexp"
	"strings"

	"ringo/internal/i18n"
	"ringo/internal/music"
)

// Where the catalog is and what language it speaks are two things to Apple:
// the storefront in the path picks what there is, and l picks the words,
// among those the storefront has. Signed out, the storefront is the
// listener's region, if Apple has one there; signed in, their account's.

// storefrontInfo is what Apple says of a storefront: whether there is one,
// and its languages.
type storefrontInfo struct {
	exists bool
	def    string
	tags   []string
}

var countryCode = regexp.MustCompile(`^[a-z]{2}$`)

// storefrontOf asks Apple of sf once a run: it doesn't change.
func (s *Source) storefrontOf(sf string) (*storefrontInfo, error) {
	s.sfMu.Lock()
	if info := s.sfInfo[sf]; info != nil {
		s.sfMu.Unlock()
		return info, nil
	}
	s.sfMu.Unlock()
	info := &storefrontInfo{}
	b, err := s.api("/v1/storefronts/"+sf, nil, reqOpts{anonymous: true})
	switch {
	case errors.Is(err, music.ErrUnavailable):
		// None there: remembered as such.
	case err != nil:
		return nil, err
	case len(b.Data) > 0 && b.Data[0] != nil:
		a := b.Data[0].attrs()
		info.exists, info.def, info.tags = true, a.DefaultLanguageTag, a.SupportedLanguageTags
	}
	s.sfMu.Lock()
	s.sfInfo[sf] = info
	s.sfMu.Unlock()
	return info, nil
}

// regionStorefront is the storefront of the region the app said the
// listener is in, when Apple has one there; else the US.
func (s *Source) regionStorefront() (string, error) {
	r := strings.ToLower(strings.TrimSpace(s.region))
	if r == "" || r == "us" || !countryCode.MatchString(r) {
		return "us", nil
	}
	info, err := s.storefrontOf(r)
	if err != nil {
		return "", err
	}
	if !info.exists {
		return "us", nil
	}
	return r, nil
}

// pickLanguage is the tag to ask in, for the app's language: Simplified
// Chinese where the storefront has it, English where it has some, and else
// what the storefront speaks first.
func pickLanguage(app, def string, tags []string) string {
	want := "en"
	if app == i18n.Chinese {
		want = "zh-Hans"
		for _, t := range tags {
			if t == "zh-Hans-CN" {
				return t
			}
		}
	}
	if strings.HasPrefix(def, want) {
		return def
	}
	for _, t := range tags {
		if strings.HasPrefix(t, want) {
			return t
		}
	}
	return def
}

// language is the l the catalog is asked with, for the storefront and the
// app's language as they are now; "" when the storefront's languages
// couldn't be found, which leaves it to Apple.
func (s *Source) language() string {
	sf, err := s.storefront()
	if err != nil {
		return ""
	}
	info, err := s.storefrontOf(sf)
	if err != nil || !info.exists {
		return ""
	}
	return pickLanguage(i18n.Language(), info.def, info.tags)
}

// withLanguage is p and the l to ask in.
func (s *Source) withLanguage(p params) params {
	out := params{}
	for k, v := range p {
		out[k] = v
	}
	if l := s.language(); l != "" {
		out["l"] = l
	}
	return out
}

// subscriber says the listener subscribes to Apple Music, which picks the
// editors' pages they're shown. Signed out, no one does. Asked once a
// sign-in; without an answer, a listener signed in is taken to, since
// songs play here only for those who do.
func (s *Source) subscriber() bool {
	if !s.signedIn() {
		return false
	}
	s.subMu.Lock()
	known := s.subscribed
	s.subMu.Unlock()
	if known != nil {
		return *known
	}
	var b struct {
		Meta struct {
			Subscription *struct {
				Active bool `json:"active"`
			} `json:"subscription"`
		} `json:"meta"`
	}
	active := true
	if err := s.requestJSON(s.amp+"/v1/me/account"+query(params{"meta": "subscription"}), reqOpts{}, &b); err == nil && b.Meta.Subscription != nil {
		active = b.Meta.Subscription.Active
	} else if err != nil {
		return true
	}
	s.subMu.Lock()
	s.subscribed = &active
	s.subMu.Unlock()
	return active
}
