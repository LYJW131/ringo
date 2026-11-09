package applemusic

import (
	"encoding/json"
	"errors"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"mygo-music/internal/music"
)

// Lossless: Apple's ALAC, up to 24-bit/192 kHz, when the app's quality is
// Lossless and the listener has a wrapper. The wrapper, a wrapper-manager
// holding their Apple Music account, gives the playlist and the FairPlay key
// templates; amhelper fetches the audio from Apple, decrypts it and serves it
// on this machine as a fragmented MP4 the app plays as it comes. Without a
// wrapper, or for a song it can't play, the song plays as AAC.

// Which songs are lossless, and where their master playlist is, as the
// catalog says: noted from every page of songs, which come with both, so that
// a song played doesn't wait to be looked up. Following amdl, a song that
// isn't lossless plays as AAC without asking the wrapper anything; a lossless
// one starts from the catalog's playlist, so the wrapper is asked for its keys
// only; and a Hi-Res one from the wrapper's own, as the catalog's sometimes
// leaves out the 88.2, 96 and 192 kHz ALAC.

type audioInfo struct {
	lossless bool
	hiRes    bool
	master   string
}

const audioMost = 5000

// noteAudio notes what a catalog song says of its audio.
func (s *Source) noteAudio(id string, a *attributes) {
	if a.AudioTraits == nil {
		return
	}
	info := audioInfo{
		lossless: slices.Contains(a.AudioTraits, "lossless"),
		hiRes:    slices.Contains(a.AudioTraits, "hi-res-lossless"),
	}
	if a.ExtendedAssetUrls != nil {
		info.master = a.ExtendedAssetUrls.EnhancedHls
	}
	s.llMu.Lock()
	s.audio.set(id, info)
	s.audio.trim(audioMost)
	s.llMu.Unlock()
}

func (s *Source) audioKnown(id string) (audioInfo, bool) {
	s.llMu.Lock()
	defer s.llMu.Unlock()
	return s.audio.get(id)
}

// learnAudio learns what the catalog says of these songs' audio, in one
// request for up to 300: songs a page lists from the library come without
// it, and asking for one as it's played costs it most of a second.
func (s *Source) learnAudio(ids []string) error {
	var missing []string
	seen := map[string]bool{}
	for _, id := range ids {
		if _, ok := s.audioKnown(id); !ok && !seen[id] {
			seen[id] = true
			missing = append(missing, id)
		}
	}
	for i := 0; i < len(missing); i += 300 {
		b, err := s.catalog("/songs", params{"ids": strings.Join(missing[i:min(i+300, len(missing))], ","), "fields[songs]": "audioTraits,extendedAssetUrls"})
		if err != nil {
			return err
		}
		for _, song := range b.Data {
			if song != nil && song.Attributes != nil {
				s.noteAudio(song.ID, song.Attributes)
			}
		}
	}
	return nil
}

func (s *Source) audioOf(id string) (*audioInfo, error) {
	if _, ok := s.audioKnown(id); !ok {
		b, err := s.catalog("/songs/"+id, params{"fields[songs]": "audioTraits,extendedAssetUrls"})
		if err != nil {
			return nil, err
		}
		song, err := first(b)
		if err != nil {
			return nil, err
		}
		if song.Attributes != nil {
			s.noteAudio(id, song.Attributes)
		}
	}
	if a, ok := s.audioKnown(id); ok {
		return &a, nil
	}
	return nil, nil
}

// playlistQuery is how amhelper is to find the song's playlist.
func playlistQuery(a *audioInfo) string {
	if a == nil || a.master == "" {
		return ""
	}
	p := params{"master": a.master}
	if a.hiRes {
		p["device"] = "1"
	}
	return query(p)
}

// knownQ is what prepare found, so that a song is asked about once: its
// quality and how it was found. A song without ALAC is known as nil.
type knownQ struct {
	SampleRate int    `json:"sampleRate"`
	BitDepth   int    `json:"bitDepth"`
	Bandwidth  int    `json:"bandwidth"`
	Query      string `json:"query"`
}

// keptLossless is how known is kept in storage ("lossless") for a week, as
// amhelper keeps what it worked out: a song played since the app started is
// ready at once.
type keptLossless struct {
	Q  *knownQ `json:"q"`
	At int64   `json:"at"`
}

const (
	knownFor   = 7 * 24 * time.Hour
	knownMost  = 2000
	foundMost  = 2000
	saveLater  = 3 * time.Second
	wakeEvery  = time.Minute
	knownStore = losslessKey
)

// loadKnown reads what's kept, once. Under s.llMu.
func (s *Source) loadKnown() {
	if s.keptLL != nil {
		return
	}
	s.keptLL = newOmap[keptLossless]()
	stored := newOmap[keptLossless]()
	s.storage.get(knownStore, stored)
	now := s.now()
	for _, id := range stored.keys() {
		k, _ := stored.get(id)
		if now-k.At < knownFor.Milliseconds() {
			s.keptLL.set(id, k)
			s.known.set(id, k.Q)
		}
	}
}

// saveKnown writes what's known a few seconds on, once for all that came meanwhile. Under s.llMu.
func (s *Source) saveKnown() {
	if s.saving != nil {
		return
	}
	s.saving = s.clk.AfterFunc(saveLater, s.flushKnown)
}

func (s *Source) flushKnown() {
	s.llMu.Lock()
	defer s.llMu.Unlock()
	s.saving = nil
	if s.keptLL != nil {
		s.storage.set(knownStore, s.keptLL)
	}
}

func (s *Source) rememberKnown(id string, q *knownQ) {
	s.llMu.Lock()
	defer s.llMu.Unlock()
	s.loadKnown()
	s.known.set(id, q)
	s.keptLL.set(id, keptLossless{q, s.now()})
	for _, old := range s.known.trim(knownMost) {
		s.keptLL.delete(old)
	}
	s.saveKnown()
}

type prepareCall struct {
	done  chan struct{}
	ahead bool
	q     *knownQ
	err   error
}

// preparedOrPreparing says whether the song is ready to play lossless, has none, or is being asked about.
func (s *Source) preparedOrPreparing(id string) bool {
	s.llMu.Lock()
	defer s.llMu.Unlock()
	s.loadKnown()
	return s.known.has(id) || s.preparing[id] != nil
}

// prepare asks amhelper to work the song out: its ALAC and the keys, which
// take the wrapper a second or two. nil when the song has no ALAC. ahead,
// for a song the listener hasn't played yet, waits behind the songs they
// play; played while it's worked out ahead, it's asked for again, which
// amhelper takes as the same, now theirs.
func (s *Source) prepare(id string, ahead bool) (*knownQ, error) {
	s.llMu.Lock()
	s.loadKnown()
	if q, ok := s.known.get(id); ok {
		s.llMu.Unlock()
		return q, nil
	}
	if cur := s.preparing[id]; cur != nil && (ahead || !cur.ahead) {
		s.llMu.Unlock()
		<-cur.done
		return cur.q, cur.err
	}
	c := &prepareCall{done: make(chan struct{}), ahead: ahead}
	s.preparing[id] = c
	s.llMu.Unlock()

	c.q, c.err = s.prepareNow(id, ahead)
	s.llMu.Lock()
	if s.preparing[id] == c {
		delete(s.preparing, id)
	}
	s.llMu.Unlock()
	close(c.done)
	return c.q, c.err
}

func (s *Source) prepareNow(id string, ahead bool) (*knownQ, error) {
	var (
		wg sync.WaitGroup
		a  *audioInfo
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		a, _ = s.audioOf(id)
	}()
	base, err := s.helperBase()
	wg.Wait()
	if err != nil {
		return nil, err
	}
	if a != nil && !a.lossless {
		s.rememberKnown(id, nil)
		return nil, nil
	}
	q := playlistQuery(a)
	u := base + "/prepare/" + id + q
	if ahead {
		if q != "" {
			u += "&ahead=1"
		} else {
			u += "?ahead=1"
		}
	}
	res, err := s.helperDo("GET", u, nil)
	if err != nil {
		return nil, err
	}
	var b struct {
		knownQ
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	json.Unmarshal(res.body, &b)
	if res.status == 404 && b.Code == "noALAC" {
		s.rememberKnown(id, nil)
		return nil, nil
	}
	if !res.ok() {
		why := b.Message
		if why == "" {
			why = strconv.Itoa(res.status)
		}
		return nil, errors.New("amhelper: " + why)
	}
	found := &knownQ{SampleRate: b.SampleRate, BitDepth: b.BitDepth, Bandwidth: b.Bandwidth, Query: q}
	s.rememberKnown(id, found)
	return found, nil
}

// find asks amhelper to read the song's playlists from Apple, without its
// keys, so that when it's played only the keys are waited for. Only for a
// song the catalog said has ALAC (learnAudio).
func (s *Source) find(id string) {
	s.llMu.Lock()
	s.loadKnown()
	a, ok := s.audio.get(id)
	if !ok || !a.lossless || s.known.has(id) || s.preparing[id] != nil || s.found[id] {
		s.llMu.Unlock()
		return
	}
	if len(s.found) >= foundMost {
		s.found = map[string]bool{}
	}
	s.found[id] = true
	s.llMu.Unlock()
	failed := true
	if base, err := s.helperBase(); err == nil {
		q := playlistQuery(&a)
		sep := "?"
		if q != "" {
			sep = "&"
		}
		res, err := s.helperDo("GET", base+"/find/"+id+q+sep+"ahead=1", nil)
		failed = err != nil || !res.ok()
	}
	if failed {
		s.llMu.Lock()
		delete(s.found, id)
		s.llMu.Unlock()
	}
}

// warmLossless starts amhelper ahead of the first song, if it's going to be wanted.
func (s *Source) warmLossless() {
	if s.hasWrapper() {
		s.warmHelper()
	}
}

// A wrapper on Cloudflare sleeps after five idle minutes, and the song that
// wakes it waits for its container to start. So while the listener is about,
// amhelper asks it something now and then (it knows when it last did), and
// the first song finds it up. Not while songs play (prefetch.go#wake): kept
// up then, it would never sleep.

// wakeWrapper wakes the wrapper ahead of the songs, if it's going to be wanted.
func (s *Source) wakeWrapper() {
	if !s.hasWrapper() {
		return
	}
	s.llMu.Lock()
	now := s.clk.Now()
	if now.Sub(s.wokeAt) < wakeEvery {
		s.llMu.Unlock()
		return
	}
	s.wokeAt = now
	s.llMu.Unlock()
	s.spawn(func() {
		if base, err := s.helperBase(); err == nil {
			s.helperDo("POST", base+"/wake", nil)
		}
	})
}

// losslessStream is the song as ALAC, or nil when it should play as AAC: no
// wrapper, no ALAC of it, or the wrapper failing.
func (s *Source) losslessStream(id string) (*music.Stream, error) {
	if !s.hasWrapper() {
		return nil, nil
	}
	q, err := s.prepare(id, false)
	if err != nil {
		if errors.Is(err, music.ErrSetup) {
			return nil, err
		}
		log.Printf("Apple Music: playing %s as AAC: %v", id, err)
		return nil, nil
	}
	if q == nil {
		return nil, nil
	}
	base, err := s.helperBase()
	if err != nil {
		return nil, err
	}
	return &music.Stream{URL: base + "/song/" + id + ".m4a" + q.Query, Type: "m4a", Quality: "lossless", BitRate: q.Bandwidth}, nil
}
