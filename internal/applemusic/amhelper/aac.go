package amhelper

// The web player's AAC: Apple's fragmented MP4, encrypted with CENC (AES-CTR)
// under a Widevine content key the applemusic package gets from Apple's
// license server. It hands the file's address and the key over, and the song
// is served decrypted as the ALAC is, a second to a fragment, which Ringo's
// player plays itself: no ffmpeg.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// aacPriming is Apple's AAC encoder's delay, in samples: every song starts
// with it. The playlist's lengths add up to the song's with it; Apple's
// catalog gives the song's without.
const aacPriming = 2112

// aacSong is what the caller said of a song: where its file is and its key.
type aacSong struct {
	media   string
	key     []byte
	seconds float64 // with the priming, as the playlist adds up to
}

// aacKept is how many songs the caller may have handed over and not yet
// asked for: a few, as the app asks for the next ones ahead.
const aacKept = 64

// openedMedia is a song's file, opened at Apple as the song was handed over,
// for the player's request to take: the caller learns then whether the file is
// still there, and the player's request finds its first bytes on the way.
type openedMedia struct {
	res    *http.Response
	cancel context.CancelFunc
	timer  *time.Timer
}

// openedFor is how long an opened file waits for the player's request: the
// player asks for the song right after the caller hands it over.
const openedFor = 30 * time.Second

// cencKey decrypts CENC samples: AES-128 in CTR mode, from each sample's IV.
type cencKey struct{ block cipher.Block }

func newCENCKey(key []byte) (cencKey, error) {
	block, err := aes.NewCipher(key)
	return cencKey{block}, err
}

func (k cencKey) DecryptSample(sample []byte, enc sampleEncryption) error {
	if len(enc.iv) == 0 {
		return errors.New("a sample without an IV")
	}
	iv := make([]byte, aes.BlockSize)
	copy(iv, enc.iv)
	stream := cipher.NewCTR(k.block, iv)
	if len(enc.subs) == 0 {
		stream.XORKeyStream(sample, sample)
		return nil
	}
	// Subsamples: clear bytes, then encrypted ones, the counter running on
	// across them all.
	at := 0
	for _, s := range enc.subs {
		at += int(s.BytesOfClearData)
		end := at + int(s.BytesOfProtectedData)
		if end > len(sample) {
			return fmt.Errorf("a subsample past the end of its sample (%d > %d)", end, len(sample))
		}
		stream.XORKeyStream(sample[at:end], sample[at:end])
		at = end
	}
	return nil
}

// aacRegister takes {media, key, seconds} for a song, to be asked for as
// aac/<id>.m4a: the key goes in a body, not the address the app is given. It
// opens the file at Apple, and answers {code: "media"} when Apple has it no
// longer, so that the caller asks for the song afresh, and else {ok, loudness}.
func (s *server) aacRegister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Media   string  `json:"media"`
		Key     string  `json:"key"`
		Seconds float64 `json:"seconds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		reply(w, http.StatusBadRequest, errorBody{"badRequest", err.Error()})
		return
	}
	key, err := hex.DecodeString(body.Key)
	switch {
	case !catalogID(id):
		err = errors.New("not a catalog ID")
	case !appleURL(body.Media):
		err = errors.New("not one of Apple's files")
	case err != nil || len(key) != 16:
		err = errors.New("not a 128-bit key in hex")
	case body.Seconds < 0 || body.Seconds > 24*3600:
		err = errors.New("not a song's length")
	}
	if err != nil {
		reply(w, http.StatusBadRequest, errorBody{"badRequest", err.Error()})
		return
	}
	ctx, cancel := context.WithCancel(s.base())
	res, err := s.openMedia(ctx, body.Media)
	if err != nil {
		cancel()
		var status *mediaStatus
		if errors.As(err, &status) {
			reply(w, http.StatusBadGateway, errorBody{"media", err.Error()})
		} else {
			fail(w, err)
		}
		return
	}
	// Its header comes first, and says how loud it is, which the caller
	// is told: then it's read again, by the player's request, as if it
	// hadn't been.
	head, err := readHead(res.Body)
	if err != nil {
		res.Body.Close()
		cancel()
		fail(w, fmt.Errorf("media header: %w", err))
		return
	}
	loudness, _ := readLoudness(head)
	res.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(head), res.Body), res.Body}
	opened := &openedMedia{res: res, cancel: cancel}
	s.mu.Lock()
	s.aac.put(id, aacSong{media: body.Media, key: key, seconds: body.Seconds})
	if old := s.aacOpened[id]; old != nil {
		old.close()
	}
	s.aacOpened[id] = opened
	opened.timer = time.AfterFunc(openedFor, func() { s.expireOpened(id, opened) })
	s.mu.Unlock()
	reply(w, http.StatusOK, struct {
		OK       bool     `json:"ok"`
		Loudness Loudness `json:"loudness"`
	}{true, loudness})
}

// expireOpened closes a file opened for a song the app didn't ask for in
// time. The timer may fire as the app's request takes the file, too late to
// be stopped: then the file is the request's, and stays open.
func (s *server) expireOpened(id string, opened *openedMedia) {
	s.mu.Lock()
	unclaimed := s.aacOpened[id] == opened
	if unclaimed {
		delete(s.aacOpened, id)
	}
	s.mu.Unlock()
	if unclaimed {
		opened.close()
	}
}

func (o *openedMedia) close() {
	if o.timer != nil {
		o.timer.Stop()
	}
	o.res.Body.Close()
	o.cancel()
}

// mediaStatus is Apple answering for a file with other than 200.
type mediaStatus struct{ code int }

func (e *mediaStatus) Error() string { return fmt.Sprintf("media: HTTP %d", e.code) }

func (s *server) openMedia(ctx context.Context, media string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, media, nil)
	if err != nil {
		return nil, err
	}
	res, err := s.cdn.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, &mediaStatus{res.StatusCode}
	}
	return res, nil
}

// aacSongFile serves aac/<id>.m4a: the song the caller handed over, decrypted.
func (s *server) aacSongFile(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	id, ok := strings.CutSuffix(r.PathValue("file"), ".m4a")
	if !ok || !catalogID(id) {
		reply(w, http.StatusBadRequest, errorBody{"badRequest", "not a song"})
		return
	}
	s.mu.Lock()
	song, ok := s.aac.get(id)
	opened := s.aacOpened[id]
	delete(s.aacOpened, id)
	s.mu.Unlock()
	if !ok {
		reply(w, http.StatusNotFound, errorBody{"unknown", "the caller didn't hand this song over"})
		return
	}
	key, err := newCENCKey(song.key)
	if err != nil {
		fail(w, err)
		return
	}
	ctx := r.Context()
	var res *http.Response
	if opened != nil {
		opened.timer.Stop()
		defer opened.cancel()
		// The app going away ends the file opened for it.
		stop := context.AfterFunc(ctx, opened.cancel)
		defer stop()
		res = opened.res
	} else if res, err = s.openMedia(ctx, song.media); err != nil {
		fail(w, err)
		return
	}
	body := newResumingBody(ctx, s.cdn, song.media, id, res)
	defer body.Close()
	w.Header().Set("Content-Type", "audio/mp4")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	headers := time.Since(start)
	var firstAudio time.Duration
	var prime uint64
	if song.seconds > 0 {
		prime = aacPriming
	}
	err = decryptStream(bufio.NewReaderSize(body, 1<<20), w, func(int, bool) (sampleDecrypter, error) { return key, nil }, streamOptions{
		seconds: song.seconds,
		prime:   prime,
		flush: func() {
			if flusher != nil {
				flusher.Flush()
			}
			if firstAudio == 0 && time.Since(start) > headers {
				firstAudio = time.Since(start)
			}
		},
	})
	if err != nil && ctx.Err() == nil {
		log.Printf("amhelper: aac %s: %v", id, err)
		panic(http.ErrAbortHandler)
	}
	if err == nil {
		log.Printf("amhelper: aac %s: Apple's headers %dms, first audio %dms, all %dms",
			id, headers.Milliseconds(), firstAudio.Milliseconds(), time.Since(start).Milliseconds())
	}
}

// fairplayKey is a FairPlay key template as a sampleDecrypter: the whole
// sample, without senc.
type fairplayKey struct {
	t interface{ DecryptSample([]byte) }
}

func (k fairplayKey) DecryptSample(sample []byte, _ sampleEncryption) error {
	k.t.DecryptSample(sample)
	return nil
}
