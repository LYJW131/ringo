package applemusic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"mygo-music/internal/i18n"
)

// Apple's API at amp-api.music.apple.com, as the web player asks it.

// params are a request's query. Empty values are left out.
type params map[string]string

func query(p params) string {
	v := url.Values{}
	for k, x := range p {
		if x != "" {
			v.Set(k, x)
		}
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

type reqOpts struct {
	method string
	body   any
	// anonymous leaves the listener's token out.
	anonymous bool
	// userHeader is the header the listener's token goes in.
	userHeader string
}

func (s *Source) signInAgain() error {
	if s.wrapperInUse() != nil {
		return fail("setup", i18n.T("Apple Music didn't accept the token of the wrapper's account. Log it in to the wrapper again."))
	}
	return fail("signIn", i18n.T("Apple Music didn't accept the sign-in. Sign in again."))
}

// request answers Apple's JSON, or nil for an empty answer.
func (s *Source) request(u string, o reqOpts) ([]byte, error) {
	if o.method == "" {
		o.method = "GET"
	}
	if o.userHeader == "" {
		o.userHeader = "media-user-token"
	}
	if !o.anonymous {
		s.renewIfDue()
	}
	var payload []byte
	if o.body != nil {
		var err error
		if payload, err = json.Marshal(o.body); err != nil {
			return nil, err
		}
	}
	renewed := false
	freshDev := false
	for attempt := 0; ; attempt++ {
		dev, err := s.developerToken(freshDev)
		if err != nil {
			return nil, err
		}
		var rd io.Reader
		if payload != nil {
			rd = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(s.ctx, o.method, u, rd)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+dev)
		req.Header.Set("Origin", s.web)
		req.Header.Set("Referer", s.web+"/")
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		mine := ""
		if !o.anonymous {
			mine = s.userToken()
		}
		if mine != "" {
			req.Header.Set(o.userHeader, mine)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		text, err := io.ReadAll(res.Body)
		res.Body.Close()
		// A 401 can be a developer token that has run out: try once with a new one.
		if res.StatusCode == 401 && attempt == 0 {
			freshDev = true
			continue
		}
		freshDev = false
		if err != nil {
			return nil, err
		}
		if res.StatusCode == 204 || res.StatusCode == 202 {
			return nil, nil
		}
		if res.StatusCode == 401 || res.StatusCode == 403 {
			// A token from the wrapper may have been replaced by its own refresh.
			if mine != "" && !renewed && s.renewRefused(mine) {
				renewed = true
				continue
			}
			if mine != "" {
				return nil, s.signInAgain()
			}
			return nil, fmt.Errorf("Apple Music: HTTP %d", res.StatusCode)
		}
		if res.StatusCode == 404 {
			return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
		}
		if res.StatusCode >= 400 {
			var e struct {
				Errors []struct{ Detail, Title string } `json:"errors"`
			}
			why := ""
			if json.Unmarshal(text, &e) == nil && len(e.Errors) > 0 {
				why = e.Errors[0].Detail
				if why == "" {
					why = e.Errors[0].Title
				}
			}
			if why != "" {
				why = " " + why
			}
			return nil, fmt.Errorf("Apple Music: HTTP %d%s", res.StatusCode, why)
		}
		if len(bytes.TrimSpace(text)) == 0 {
			return nil, nil
		}
		return text, nil
	}
}

// requestJSON is request, read into out; an empty answer leaves out as it is.
func (s *Source) requestJSON(u string, o reqOpts, out any) error {
	b, err := s.request(u, o)
	if err != nil || b == nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// api asks amp-api for path, answering its body (empty, not nil, for no answer).
func (s *Source) api(path string, p params, o reqOpts) (*body, error) {
	var b body
	if err := s.requestJSON(s.amp+path+query(p), o, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// catalogPath is a path of Apple's catalog in the listener's storefront.
func (s *Source) catalogPath(path string) (string, error) {
	sf, err := s.storefront()
	if err != nil {
		return "", err
	}
	return "/v1/catalog/" + sf + path, nil
}

// withSongAssets adds what makes the catalog's songs come with the address
// of their lossless master playlist, which a song played lossless starts
// from (lossless.go).
func withSongAssets(p params) params {
	out := params{"extend[songs]": "extendedAssetUrls"}
	for k, v := range p {
		out[k] = v
	}
	return out
}

// catalog asks Apple's catalog, in the listener's storefront.
func (s *Source) catalog(path string, p params) (*body, error) {
	full, err := s.catalogPath(path)
	if err != nil {
		return nil, err
	}
	return s.api(full, s.withLanguage(withSongAssets(p)), reqOpts{})
}

// catalogInto is catalog, read into out, for the answers that aren't a page of resources.
func (s *Source) catalogInto(path string, p params, out any) error {
	full, err := s.catalogPath(path)
	if err != nil {
		return err
	}
	return s.requestJSON(s.amp+full+query(s.withLanguage(withSongAssets(p))), reqOpts{}, out)
}

// me asks for the listener's own: their library, ratings and recommendations.
func (s *Source) me(path string, p params, o reqOpts) (*body, error) {
	if err := s.needAccount(); err != nil {
		return nil, err
	}
	return s.api("/v1/me"+path, p, o)
}

// first is the one resource an answer is about.
func first(b *body) (*resource, error) {
	if b == nil || len(b.Data) == 0 || b.Data[0] == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	return b.Data[0], nil
}

// allPages is every page of a list, up to most items, calling pause between pages.
func (s *Source) allPages(path string, p params, most int, pause func()) ([]*resource, error) {
	if strings.HasPrefix(path, "/v1/me") {
		if err := s.needAccount(); err != nil {
			return nil, err
		}
	}
	var out []*resource
	next := path + query(p)
	for next != "" && len(out) < most {
		if len(out) > 0 && pause != nil {
			pause()
		}
		var b body
		if err := s.requestJSON(s.amp+next, reqOpts{}, &b); err != nil {
			return nil, err
		}
		out = append(out, b.Data...)
		next = b.Next
		if next != "" && !strings.HasPrefix(next, "/") {
			if u, err := url.Parse(next); err == nil {
				next = u.RequestURI()
			}
		}
	}
	return out, nil
}

// storefront is the account's, once signed in; else the listener's
// region's (lang.go). Catalog requests made as the listener have to be in
// theirs.
func (s *Source) storefront() (string, error) {
	if !s.signedIn() {
		return s.regionStorefront()
	}
	if sf := s.storage.getString(storefrontKey); sf != "" {
		return sf, nil
	}
	b, err := s.api("/v1/me/storefront", nil, reqOpts{})
	if err != nil {
		return "", err
	}
	if len(b.Data) == 0 || b.Data[0] == nil || b.Data[0].ID == "" {
		return "", s.signInAgain()
	}
	s.storage.set(storefrontKey, b.Data[0].ID)
	return b.Data[0].ID, nil
}

// The web player's way of asking: in the listener's language, on the web,
// and with what's related given once, by type and ID, which the answer's
// pointers lead to (body.get).
func (s *Source) mapped(p params) params {
	out := s.withLanguage(p)
	out["platform"] = "web"
	out["format[resources]"] = "map"
	return out
}

// editorial asks for the editors' pages: path is under /v1/editorial/{sf}.
func (s *Source) editorial(path string, p params) (*body, error) {
	sf, err := s.storefront()
	if err != nil {
		return nil, err
	}
	return s.api("/v1/editorial/"+sf+path, s.mapped(p), reqOpts{})
}

// continued is where next leads, asked as base was: Apple's next has its
// offset and little else, so what base asks for that next doesn't say is
// added. The offset is next's own; it isn't always the count so far.
func continued(next string, base params) string {
	u, err := url.Parse(next)
	if err != nil {
		return next
	}
	q := u.Query()
	for k, v := range base {
		if v != "" && !q.Has(k) {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.RequestURI()
}

// follow asks for the next part of a list, at next, as base was asked.
func (s *Source) follow(next string, base params) (*body, error) {
	if !strings.HasPrefix(next, "/") {
		if u, err := url.Parse(next); err == nil {
			next = u.RequestURI()
		}
	}
	if strings.HasPrefix(next, "/v1/me") {
		if err := s.needAccount(); err != nil {
			return nil, err
		}
	}
	var b body
	if err := s.requestJSON(s.amp+continued(next, base), reqOpts{}, &b); err != nil {
		return nil, err
	}
	return &b, nil
}
