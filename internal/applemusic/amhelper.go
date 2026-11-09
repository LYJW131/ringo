package applemusic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"mygo-music/internal/applemusic/amhelper"
)

// amhelper takes the encryption off Apple's audio as it comes, the AAC with
// the key the source got (playback.go) and the ALAC with a wrapper's
// (lossless.go), and serves it on this machine as a fragmented MP4 the app
// plays itself. It also keeps private playlists' covers (artwork.go). It
// runs in this process, and is still asked over HTTP on 127.0.0.1, as the
// player fetches what it serves from there.

// helperServer is a running amhelper; tests stand one in.
type helperServer interface {
	Base() string
	Close() error
}

func startHelper(cfg amhelper.Config) (helperServer, error) { return amhelper.Start(cfg) }

func (s *Source) helperConfig() amhelper.Config {
	return amhelper.Config{
		Wrapper: strings.TrimSpace(s.Setting("wrapper")),
		Token:   strings.TrimSpace(s.Setting("wrapperKey")),
		// amhelper keeps key templates and the songs it worked out in
		// cache, so that a song played before plays at once after a restart.
		Cache:  filepath.Join(s.dataDir, "lossless"),
		Covers: s.dataDir,
	}
}

// hasWrapper says whether the listener named a wrapper.
func (s *Source) hasWrapper() bool { return strings.TrimSpace(s.Setting("wrapper")) != "" }

// helperBase is amhelper's address, starting it if it isn't running. One
// runs at a time, started when first needed and again when the settings
// change.
func (s *Source) helperBase() (string, error) {
	cfg := s.helperConfig()
	s.helperMu.Lock()
	defer s.helperMu.Unlock()
	if s.helper != nil && s.helperCfg != cfg {
		s.helper.Close()
		s.helper = nil
	}
	if s.helper == nil {
		h, err := s.startHelper(cfg)
		if err != nil {
			return "", err
		}
		s.helper, s.helperCfg = h, cfg
	}
	return s.helper.Base(), nil
}

// warmHelper starts amhelper ahead of the first song.
func (s *Source) warmHelper() {
	s.spawn(func() { s.helperBase() })
}

func (s *Source) stopHelper() {
	s.helperMu.Lock()
	defer s.helperMu.Unlock()
	if s.helper != nil {
		s.helper.Close()
		s.helper = nil
	}
}

type helperAnswer struct {
	status int
	body   []byte
}

// ok says whether amhelper answered 2xx.
func (a *helperAnswer) ok() bool { return a.status/100 == 2 }

// helperError is what amhelper answers when it fails: {code, message}.
type helperError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (a *helperAnswer) err() helperError {
	var e helperError
	json.Unmarshal(a.body, &e)
	return e
}

// helperDo asks amhelper, with v as JSON if there is one.
func (s *Source) helperDo(method, u string, v any) (*helperAnswer, error) {
	var rd io.Reader
	if v != nil {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(s.ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	if v != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return &helperAnswer{res.StatusCode, b}, nil
}
