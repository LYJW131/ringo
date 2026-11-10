package applemusic

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"mygo-music/internal/i18n"
)

// Apple's addresses, and who the source says it is to them: Safari, as the
// web player at music.apple.com would be.
const (
	userAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15"
	webBase        = "https://music.apple.com"
	ampBase        = "https://amp-api.music.apple.com"
	playbackURL    = "https://play.music.apple.com/WebObjects/MZPlay.woa/wa/webPlayback"
	licenseURL     = "https://play.itunes.apple.com/WebObjects/MZPlay.woa/wa/acquireWebPlaybackLicense"
	activityURL    = "https://universal-activity-service.itunes.apple.com/play"
	widevineSystem = "edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"
	devTokenLeeway = time.Hour
)

// The keys of storage.json, the plugin's host.storage keys.
const (
	devTokenKey    = "devToken"
	userTokenKey   = "mediaUserToken"
	storefrontKey  = "storefront"
	profileKey     = "profile"
	givenTokenKey  = "givenToken" // read only to forget it, as it's from before the list
	wrapperAcctKey = "wrapperAccount"
	accountsKey    = "accounts"
	currentKey     = "currentAccount"
	wrapperUsedKey = "wrapperUsed"
	resolvedKey    = "resolved"
	coversKey      = "covers"
	qualityKey     = "quality"
	losslessKey    = "lossless"
	buildKey       = "webBuild"
)

// Tokens. The developer token is the web player's own, read out of its
// script. The media-user-token is the listener's, from signing in on
// music.apple.com.

var (
	scriptPath = regexp.MustCompile(`/assets/index[~-][^"'\s]+\.js`)
	jwtPattern = regexp.MustCompile(`eyJ[\w-]{10,}\.[\w-]+\.[\w-]+`)
	// The web player's own version, which sits beside its token in the script.
	buildPattern = regexp.MustCompile(`"(\d+\.\d+\.\d+-external)"`)
)

// defaultBuild is the web player's version as it was on 2026-11-10, for
// before the script has been read, or when it didn't show one.
const defaultBuild = "2644.9.0-external"

// webBuild is the web player's version, as the plays reported say it.
func (s *Source) webBuild() string {
	if b := s.storage.getString(buildKey); b != "" {
		return b
	}
	return defaultBuild
}

func jwtExpiry(jwt string) time.Time {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp float64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return time.Time{}
	}
	return time.UnixMilli(int64(claims.Exp * 1000))
}

func (s *Source) developerToken(fresh bool) (string, error) {
	// One at a time: requests that find it run out wait for the one reading it.
	s.devMu.Lock()
	defer s.devMu.Unlock()
	if !fresh {
		if s.devToken == "" {
			s.devToken = s.storage.getString(devTokenKey)
		}
		if s.devToken != "" && jwtExpiry(s.devToken).After(s.clk.Now().Add(devTokenLeeway)) {
			return s.devToken, nil
		}
	}
	page, err := s.fetchText(s.web)
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	for _, path := range scriptPath.FindAllString(page, -1) {
		if seen[path] {
			continue
		}
		seen[path] = true
		js, err := s.fetchText(s.web + path)
		if err != nil {
			return "", err
		}
		if found := jwtPattern.FindString(js); found != "" {
			s.devToken = found
			s.storage.set(devTokenKey, found)
			if m := buildPattern.FindStringSubmatch(js); m != nil {
				s.storage.set(buildKey, m[1])
			}
			return found, nil
		}
	}
	return "", errors.New(i18n.T("Couldn't read the Apple Music web player's token"))
}

// fetchText GETs u as Safari, for its text.
func (s *Source) fetchText(u string) (string, error) {
	req, err := http.NewRequestWithContext(s.ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return string(b), err
}

func (s *Source) userToken() string { return s.storage.getString(userTokenKey) }

func (s *Source) signedIn() bool { return s.userToken() != "" }

func (s *Source) needAccount() error {
	if !s.signedIn() {
		return fail("signIn", "")
	}
	return nil
}
