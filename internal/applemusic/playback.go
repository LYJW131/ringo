package applemusic

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"mygo-music/internal/applemusic/widevine"
	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Playing a song whole: the web player's playlist for it, its key from
// Apple's license server, and amhelper taking the encryption off as the
// audio comes in.

func (s *Source) playbackSignInAgain() error {
	return fail("signIn", i18n.T("Apple Music didn't accept the sign-in. Sign in again."))
}

// webPlayback is the web player's playlist of the song: its encrypted .mp4 and its Widevine key.
func (s *Source) webPlayback(id string) (string, error) {
	if err := s.needAccount(); err != nil {
		return "", err
	}
	var b struct {
		FailureType     flexString `json:"failureType"`
		CustomerMessage string     `json:"customerMessage"`
		SongList        []struct {
			HLSPlaylistURL string `json:"hls-playlist-url"`
			Assets         []struct {
				Flavor string `json:"flavor"`
				URL    string `json:"URL"`
			} `json:"assets"`
		} `json:"songList"`
	}
	err := s.requestJSON(s.playURL, reqOpts{
		method:     "POST",
		body:       map[string]string{"salableAdamId": id, "language": "en-US"},
		userHeader: "X-Apple-Music-User-Token",
	}, &b)
	if err != nil {
		return "", err
	}
	if code := string(b.FailureType); code != "" {
		return "", s.playbackFailed(code, b.CustomerMessage)
	}
	if len(b.SongList) == 0 {
		return "", fail("unavailable", i18n.T("Apple Music won't play this song"))
	}
	song := b.SongList[0]
	playlist := song.HLSPlaylistURL
	if playlist == "" {
		for _, a := range song.Assets {
			if a.Flavor == "28:ctrp256" {
				playlist = a.URL
				break
			}
		}
	}
	if playlist == "" {
		return "", fail("unavailable", i18n.T("Apple Music has no AAC of this song"))
	}
	return playlist, nil
}

// playbackFailed says why Apple won't play a song, from the failureType of
// its answer. 3077 is also its word for a library song the catalog no
// longer has.
func (s *Source) playbackFailed(code, message string) error {
	switch code {
	case "2002":
		return s.playbackSignInAgain()
	case "3076", "3077":
		return fail("unavailable", i18n.T("This song isn't available in your country or region"))
	}
	why := code
	if message != "" {
		why += ": " + message
	}
	return fail("unavailable", i18n.T("Apple Music won't play this song")+" ("+why+")")
}

// A song the listener uploaded is asked for by its library ID, as the web
// player asks for it. Apple answers a link to the file as uploaded, or as
// iTunes made it from theirs: AAC at 256 kbps in a plain .m4a, unencrypted.
// The link lasts a day, so it isn't kept. From one connection it comes at
// half the speed it plays at, so amhelper fetches it over several, and
// keeps it (amhelper/upload.go).

func (s *Source) uploadStream(id string) (*music.Stream, error) {
	link, err := s.uploadLink(id)
	if err != nil {
		return nil, err
	}
	base, err := s.helperBase()
	if err != nil {
		return nil, err
	}
	res, err := s.helperDo("POST", base+"/upload/"+id, map[string]string{"url": link})
	if err != nil {
		return nil, err
	}
	if !res.ok() {
		why := res.err().Message
		if why == "" {
			why = strconv.Itoa(res.status)
		}
		return nil, &helperFailed{why, false}
	}
	var b struct {
		Size int64 `json:"size"`
	}
	json.Unmarshal(res.body, &b)
	return &music.Stream{URL: base + "/upload/" + id + ".m4a", Type: "m4a", Quality: "high", BitRate: 256000, Size: b.Size}, nil
}

// uploadLink is the link Apple answers for the upload id, valid for a day.
func (s *Source) uploadLink(id string) (string, error) {
	if err := s.needAccount(); err != nil {
		return "", err
	}
	var b struct {
		FailureType     flexString `json:"failureType"`
		CustomerMessage string     `json:"customerMessage"`
		SongList        []struct {
			Assets []struct {
				URL      string `json:"URL"`
				Metadata struct {
					FileExtension string `json:"fileExtension"`
				} `json:"metadata"`
			} `json:"assets"`
		} `json:"songList"`
	}
	err := s.requestJSON(s.playURL, reqOpts{
		method:     "POST",
		body:       map[string]any{"universalLibraryId": id, "isLibrary": true, "language": "en-US"},
		userHeader: "X-Apple-Music-User-Token",
	}, &b)
	if err != nil {
		return "", err
	}
	if code := string(b.FailureType); code != "" {
		return "", s.playbackFailed(code, b.CustomerMessage)
	}
	// Only the unencrypted .m4a: a song bought or matched comes as an .m4p,
	// whose key this doesn't ask for.
	for _, song := range b.SongList {
		for _, a := range song.Assets {
			if a.Metadata.FileExtension == "m4a" && blobstore.MatchString(a.URL) {
				return a.URL, nil
			}
		}
	}
	return "", fail("unavailable", i18n.T("Apple Music won't play this song"))
}

var m3u8Attribute = regexp.MustCompile(`([A-Z0-9-]+)=("[^"]*"|[^,]*)`)

func m3u8Attributes(line string) map[string]string {
	out := map[string]string{}
	for _, m := range m3u8Attribute.FindAllStringSubmatch(line, -1) {
		out[m[1]] = strings.TrimSuffix(strings.TrimPrefix(m[2], `"`), `"`)
	}
	return out
}

type media struct {
	// media is the encrypted fragmented MP4.
	media string
	// keyURI is the key's URI, which the license request names.
	keyURI string
	kid    []byte
	// seconds is how long it plays, the encoder's priming with it: the segments' lengths added up.
	seconds float64
}

func (s *Source) mediaOf(playlistURL string) (*media, error) {
	text, err := s.fetchText(playlistURL)
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(playlistURL)
	var keyURI, mapURI string
	seconds := 0.0
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "#EXTINF:"):
			seconds += leadingFloat(line[8:])
		case strings.HasPrefix(line, "#EXT-X-KEY:"):
			a := m3u8Attributes(line[11:])
			isWidevine := strings.Contains(strings.ToLower(a["KEYFORMAT"]), widevineSystem)
			if a["URI"] != "" && (isWidevine || keyURI == "") {
				keyURI = a["URI"]
			}
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			a := m3u8Attributes(line[11:])
			if a["URI"] != "" && base != nil {
				if u, err := base.Parse(a["URI"]); err == nil {
					mapURI = u.String()
				}
			}
		}
	}
	if mapURI == "" || keyURI == "" {
		return nil, fail("unavailable", i18n.T("Apple Music has no AAC of this song"))
	}
	kid, _ := base64.StdEncoding.DecodeString(keyURI[strings.Index(keyURI, ",")+1:])
	return &media{media: mapURI, keyURI: keyURI, kid: kid, seconds: math.Round(seconds*1e5) / 1e5}, nil
}

var floatPrefix = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?`)

// leadingFloat is JavaScript's parseFloat: the number the text starts with, or 0.
func leadingFloat(s string) float64 {
	f, err := strconv.ParseFloat(floatPrefix.FindString(strings.TrimSpace(s)), 64)
	if err != nil {
		return 0
	}
	return f
}

// cdm is what makes a Widevine license request and reads the keys from its
// answer; tests stand one in.
type cdm interface {
	challenge() []byte
	keys(license []byte) ([]widevine.Key, error)
}

type widevineCDM struct{ *widevine.Session }

func (w widevineCDM) challenge() []byte { return w.Challenge }
func (w widevineCDM) keys(license []byte) ([]widevine.Key, error) {
	return w.Keys(license)
}

func newWidevine(kid []byte) (cdm, error) {
	s, err := widevine.NewSession(kid)
	if err != nil {
		return nil, err
	}
	return widevineCDM{s}, nil
}

// contentKey is the song's content key, in hex, from Apple's license server.
func (s *Source) contentKey(id, keyURI string, kid []byte) (string, error) {
	session, err := s.newCDM(kid)
	if err != nil {
		return "", err
	}
	var b struct {
		FailureType flexString `json:"failureType"`
		License     string     `json:"license"`
		Status      int        `json:"status"`
	}
	err = s.requestJSON(s.licenseURL, reqOpts{
		method: "POST",
		body: map[string]any{
			"challenge":      base64.StdEncoding.EncodeToString(session.challenge()),
			"uri":            keyURI,
			"key-system":     "com.widevine.alpha",
			"adamId":         id,
			"isLibrary":      false,
			"user-initiated": true,
		},
		userHeader: "X-Apple-Music-User-Token",
	}, &b)
	if err != nil {
		return "", err
	}
	if code := string(b.FailureType); code != "" {
		if code == "2002" {
			return "", s.playbackSignInAgain()
		}
		return "", fail("unavailable", i18n.T("Apple Music won't play this song")+" ("+code+")")
	}
	if b.License == "" {
		return "", fmt.Errorf("%s (status %d)", i18n.T("Apple Music sent no license"), b.Status)
	}
	license, err := base64.StdEncoding.DecodeString(b.License)
	if err != nil {
		return "", err
	}
	keys, err := session.keys(license)
	if err != nil {
		return "", err
	}
	var content []widevine.Key
	for _, k := range keys {
		if k.Type == 2 {
			content = append(content, k)
		}
	}
	for _, k := range content {
		if len(kid) >= 16 && bytes.Equal(k.KID, kid[len(kid)-16:]) {
			return hex.EncodeToString(k.Key), nil
		}
	}
	if len(content) == 0 {
		return "", errors.New(i18n.T("Apple Music sent no key for this song"))
	}
	return hex.EncodeToString(content[0].Key), nil
}

// What it takes to play a song, once worked out: where its encrypted audio
// is, its key and its length. None changes, so they're kept, and a song
// played before starts without asking Apple anything. A few thousand of them
// are a few hundred kilobytes.

const keysKept = 3000

type keptSong struct {
	M string `json:"m"` // media
	K string `json:"k"` // key
	// S is seconds, which songs kept by the plugin's 0.2.6 and before lack.
	S float64 `json:"s,omitempty"`
}

type resolved struct {
	media   string
	key     string
	seconds float64
	// kept says it was kept from before rather than asked for now.
	kept bool
}

type resolveCall struct {
	done chan struct{}
	song *resolved
	err  error
}

// keptSongs is what's kept, read once. Under s.songsMu.
func (s *Source) keptSongs() *omap[keptSong] {
	if s.songs == nil {
		s.songs = newOmap[keptSong]()
		s.storage.get(resolvedKey, s.songs)
	}
	return s.songs
}

func (s *Source) rememberSong(id, media, key string, seconds float64) {
	s.songsMu.Lock()
	defer s.songsMu.Unlock()
	all := s.keptSongs()
	all.set(id, keptSong{media, key, seconds})
	all.trim(keysKept)
	s.storage.set(resolvedKey, all)
}

func (s *Source) forgetSong(id string) {
	s.songsMu.Lock()
	defer s.songsMu.Unlock()
	all := s.keptSongs()
	if all.has(id) {
		all.delete(id)
		s.storage.set(resolvedKey, all)
	}
}

// resolvedOrResolving says whether the song is worked out already, or being worked out.
func (s *Source) resolvedOrResolving(id string) bool {
	s.songsMu.Lock()
	defer s.songsMu.Unlock()
	song, ok := s.keptSongs().get(id)
	return (ok && song.S != 0) || s.resolving[id] != nil
}

// resolve is the song's audio, key and length: kept, or from webPlayback,
// its playlist and a license. A song kept without its length has its
// playlist read again, and keeps its key if the audio is where it was. Asked
// for twice at once, as the app asks ahead, it's worked out once.
func (s *Source) resolve(id string) (*resolved, error) {
	s.songsMu.Lock()
	song, ok := s.keptSongs().get(id)
	if ok && song.S != 0 {
		s.songsMu.Unlock()
		return &resolved{song.M, song.K, song.S, true}, nil
	}
	if c := s.resolving[id]; c != nil {
		s.songsMu.Unlock()
		<-c.done
		return c.song, c.err
	}
	c := &resolveCall{done: make(chan struct{})}
	s.resolving[id] = c
	s.songsMu.Unlock()

	c.song, c.err = func() (*resolved, error) {
		playlist, err := s.webPlayback(id)
		if err != nil {
			return nil, err
		}
		m, err := s.mediaOf(playlist)
		if err != nil {
			return nil, err
		}
		key := song.K
		if !ok || song.M != m.media {
			if key, err = s.contentKey(id, m.keyURI, m.kid); err != nil {
				return nil, err
			}
		}
		s.rememberSong(id, m.media, key, m.seconds)
		return &resolved{m.media, key, m.seconds, false}, nil
	}()
	s.songsMu.Lock()
	delete(s.resolving, id)
	s.songsMu.Unlock()
	close(c.done)
	return c.song, c.err
}

// amhelper takes the encryption off as the audio comes in from Apple, and
// serves it on this machine as a fragmented MP4 that says its length and
// skips the encoder's priming, which the app plays itself as it arrives. The
// AAC is copied as it is. The song is handed over first, with its key, in a
// request of its own: the address the app is given names the song only.
// amhelper opens the file at Apple as it's handed over, and says so if Apple
// has it no longer.

// helperFailed is amhelper failing on a song. stale says only that Apple no
// longer has the file, which is worth asking for the song afresh.
type helperFailed struct {
	message string
	stale   bool
}

func (e *helperFailed) Error() string { return "amhelper: " + e.message }

func (s *Source) viaHelper(base, id string, song *resolved) (string, error) {
	res, err := s.helperDo("POST", base+"/aac/"+id, map[string]any{"media": song.media, "key": song.key, "seconds": song.seconds})
	if err != nil {
		return "", err
	}
	if !res.ok() {
		e := res.err()
		why := e.Message
		if why == "" {
			why = strconv.Itoa(res.status)
		}
		return "", &helperFailed{why, e.Code == "media"}
	}
	return base + "/aac/" + id + ".m4a", nil
}

// wantedQuality is what the app asked for last (music.Ask), kept: what the
// songs played next, and those worked out ahead, will want.
func (s *Source) wantedQuality() string {
	s.qualityMu.Lock()
	defer s.qualityMu.Unlock()
	if s.quality == nil {
		q := s.storage.getString(qualityKey)
		s.quality = &q
	}
	return *s.quality
}

func (s *Source) stream(id, quality string) (*music.Stream, error) {
	if isLibrarySong(id) {
		// At every quality: there is no lossless of an upload to ask for.
		return s.uploadStream(id)
	}
	if !isCatalogID(id) {
		return nil, fail("unavailable", i18n.T("Apple Music won't play this song"))
	}
	if quality != s.wantedQuality() {
		s.qualityMu.Lock()
		s.quality = &quality
		s.qualityMu.Unlock()
		s.storage.set(qualityKey, quality)
	}
	level, atmos := music.SplitAsk(quality)
	// With Dolby Atmos on, whether the song has a version of it, which a
	// song without says, for the cache (music.Stream.NoAtmos).
	var a *audioInfo
	if atmos != music.AtmosOff {
		a, _ = s.audioOf(id)
	}
	st, err := s.streamAs(id, quality, level, a)
	if st != nil && a != nil && !a.atmos {
		st.NoAtmos = true
	}
	return st, err
}

// streamAs is the song as asked: through the wrapper, unless it plays as
// the web player's AAC.
func (s *Source) streamAs(id, ask, level string, a *audioInfo) (*music.Stream, error) {
	if viaWrapper(playsAs(a, ask)) || viaWrapper(level) {
		st, err := s.wrapperStream(id, ask, a)
		if err != nil {
			return nil, err
		}
		if st != nil {
			return st, nil
		}
	}
	if err := s.needAccount(); err != nil {
		return nil, err
	}
	song, err := s.resolve(id)
	if err != nil {
		return nil, err
	}
	st, err := s.play(id, song)
	if err == nil {
		return st, nil
	}
	var hf *helperFailed
	if !song.kept || coded(err) || (errors.As(err, &hf) && !hf.stale) {
		return nil, err
	}
	// What was kept no longer opens, as Apple may have moved the audio: ask again.
	s.forgetSong(id)
	if song, err = s.resolve(id); err != nil {
		return nil, err
	}
	return s.play(id, song)
}

func (s *Source) play(id string, song *resolved) (*music.Stream, error) {
	base, err := s.helperBase()
	if err != nil {
		return nil, err
	}
	u, err := s.viaHelper(base, id, song)
	if err != nil {
		return nil, err
	}
	return &music.Stream{URL: u, Type: "m4a", Quality: "high", BitRate: 256000}, nil
}
