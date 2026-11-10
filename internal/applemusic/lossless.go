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

	"ringo/internal/applemusic/amhelper"
	"ringo/internal/music"
)

// Lossless: Apple's ALAC when the app's quality is Lossless, up to 24-bit/48
// kHz, or Hi-Res Lossless, up to 24-bit/192 kHz, and the listener has a
// wrapper. The wrapper, a wrapper-manager
// holding their Apple Music account, gives the playlist and the FairPlay key
// templates; amhelper fetches the audio from Apple, decrypts it and serves it
// on this machine as a fragmented MP4 the app plays as it comes. Without a
// wrapper, or for a song it can't play, the song plays as AAC.
//
// The same way come High Efficiency's HE-AAC, and, with Dolby Atmos on, a
// song's Dolby Atmos version as one of its two stereo versions: Apple's
// binaural rendering for headphones, or Dolby's downmix for speakers. The
// web player's own AAC is the only kind that comes without the wrapper.

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
	atmos    bool // it has a Dolby Atmos version
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
		atmos:    slices.Contains(a.AudioTraits, "atmos"),
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

// lossless48 is the most Lossless plays: 48 kHz, as Apple Music has it.
const lossless48 = 48000

// playsAs is what a song plays as, asked for as ask (music.Ask), as far as
// what's known of its audio says: its Dolby Atmos version, by the setting,
// where it has one, or else the level. A song whose audio isn't known plays
// at the level. It's what the stream says it is (music.Stream.Quality).
func playsAs(a *audioInfo, ask string) string {
	level, atmos := music.SplitAsk(ask)
	if atmos != music.AtmosOff && a != nil && a.atmos {
		return music.AtmosQualities[atmos]
	}
	return level
}

// viaWrapper says whether what a song plays as comes through the wrapper:
// all but High Quality, which is the web player's own AAC.
func viaWrapper(quality string) bool { return quality != "high" }

// helperKinds are what amhelper is asked for (its kind) to play as each
// quality, besides ALAC, which it's asked for without one.
var helperKinds = map[string]string{
	"efficient":      "he",
	"atmos-binaural": "binaural",
	"atmos-downmix":  "downmix",
}

// playlistQuery is how amhelper is to find the song's playlist to play as
// quality. Lossless has no use for the wrapper's own playlist, which is only
// for the rates past 48 kHz that the catalog's may leave out, so it asks for
// the catalog's and caps the rate; Hi-Res Lossless asks for the most there
// is. The other kinds are all in the catalog's.
func playlistQuery(a *audioInfo, quality string) string {
	p := params{}
	if a != nil && a.master != "" {
		p["master"] = a.master
		if a.hiRes && quality == "hires" {
			p["device"] = "1"
		}
	}
	if quality == "lossless" {
		p["maxRate"] = strconv.Itoa(lossless48)
	}
	if kind := helperKinds[quality]; kind != "" {
		p["kind"] = kind
	}
	return query(p)
}

// knownQ is what prepare found, so that a song is asked about once: its
// quality and how it was found. A song without what it's asked for is known
// as nil. Of ALAC, that's at either level: Apple's lossless songs all have
// ALAC within 48 kHz, so one with none at Hi-Res Lossless has none at
// Lossless either. The other kinds are each known apart (knownKey).
type knownQ struct {
	// Level is what it was found to play as, which is what the stream it
	// makes says it is (music.Stream.Quality). What was found at another
	// level is asked again: a song found at Hi-Res Lossless is not within
	// 48 kHz.
	Level      string `json:"level"`
	SampleRate int    `json:"sampleRate"`
	BitDepth   int    `json:"bitDepth"`
	Bandwidth  int    `json:"bandwidth"`
	// BitRate is what Apple calls an AAC kind, as 256000; 0 of ALAC.
	BitRate int    `json:"bitRate,omitempty"`
	Query   string `json:"query"`
	// Loudness is what its file says, for Sound Check: nil where amhelper
	// didn't read it, as of one known from before it did.
	Loudness *amhelper.Loudness `json:"loudness,omitempty"`
}

// knownKey is what's known of a song to play as quality is kept under: the
// song's ID for ALAC, as both levels share what's known (knownQ), and the ID
// and the quality for the others.
func knownKey(id, quality string) string {
	if music.IsLossless(quality) {
		return id
	}
	return id + "/" + quality
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
		// Known without its loudness, it's asked about again, to have it.
		if now-k.At < knownFor.Milliseconds() && (k.Q == nil || k.Q.Loudness != nil) {
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

// knownAt is what's known of the song to play as quality, if it is: a song
// known to have none of it is, and one found at another level isn't. Under
// s.llMu.
func (s *Source) knownAt(id, quality string) (*knownQ, bool) {
	q, ok := s.known.get(knownKey(id, quality))
	if ok && q != nil && q.Level != quality {
		return nil, false
	}
	return q, ok
}

func (s *Source) rememberKnown(id, quality string, q *knownQ) {
	key := knownKey(id, quality)
	s.llMu.Lock()
	defer s.llMu.Unlock()
	s.loadKnown()
	s.known.set(key, q)
	s.keptLL.set(key, keptLossless{q, s.now()})
	for _, old := range s.known.trim(knownMost) {
		s.keptLL.delete(old)
	}
	s.saveKnown()
}

type prepareCall struct {
	done  chan struct{}
	level string
	ahead bool
	q     *knownQ
	err   error
}

// preparedOrPreparing says whether the song is ready to play as quality through the wrapper, has none of it, or is being asked about.
func (s *Source) preparedOrPreparing(id, quality string) bool {
	s.llMu.Lock()
	defer s.llMu.Unlock()
	s.loadKnown()
	if _, known := s.knownAt(id, quality); known {
		return true
	}
	cur := s.preparing[knownKey(id, quality)]
	return cur != nil && cur.level == quality
}

// playsAsKnown is what the song plays as, asked for as ask, by what's known
// of its audio already.
func (s *Source) playsAsKnown(id, ask string) string {
	if a, ok := s.audioKnown(id); ok {
		return playsAs(&a, ask)
	}
	return playsAs(nil, ask)
}

// prepare asks amhelper to work the song out to play as quality, one that
// comes through the wrapper: the variant and its keys, which take the
// wrapper a second or two. nil when the song has none of it. ahead, for a
// song the listener hasn't played yet, waits behind the songs they play;
// played while it's worked out ahead, it's asked for again, which amhelper
// takes as the same, now theirs.
func (s *Source) prepare(id, quality string, ahead bool) (*knownQ, error) {
	key := knownKey(id, quality)
	s.llMu.Lock()
	s.loadKnown()
	if q, ok := s.knownAt(id, quality); ok {
		s.llMu.Unlock()
		return q, nil
	}
	if cur := s.preparing[key]; cur != nil && cur.level == quality && (ahead || !cur.ahead) {
		s.llMu.Unlock()
		<-cur.done
		return cur.q, cur.err
	}
	c := &prepareCall{done: make(chan struct{}), level: quality, ahead: ahead}
	s.preparing[key] = c
	s.llMu.Unlock()

	c.q, c.err = s.prepareNow(id, quality, ahead)
	s.llMu.Lock()
	if s.preparing[key] == c {
		delete(s.preparing, key)
	}
	s.llMu.Unlock()
	close(c.done)
	return c.q, c.err
}

func (s *Source) prepareNow(id, quality string, ahead bool) (*knownQ, error) {
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
	// What the catalog says the song lacks isn't asked for.
	if a != nil && ((music.IsLossless(quality) && !a.lossless) || (music.IsAtmos(quality) && !a.atmos)) {
		s.rememberKnown(id, quality, nil)
		return nil, nil
	}
	q := playlistQuery(a, quality)
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
	if res.status == 404 && b.Code == "noAudio" {
		s.rememberKnown(id, quality, nil)
		return nil, nil
	}
	if !res.ok() {
		why := b.Message
		if why == "" {
			why = strconv.Itoa(res.status)
		}
		return nil, errors.New("amhelper: " + why)
	}
	found := &knownQ{Level: quality, SampleRate: b.SampleRate, BitDepth: b.BitDepth, Bandwidth: b.Bandwidth, BitRate: b.BitRate, Query: q, Loudness: b.Loudness}
	s.rememberKnown(id, quality, found)
	return found, nil
}

// find asks amhelper to read the song's playlists from Apple, without its
// keys, so that when it's played only the keys are waited for. Only for a
// song the catalog said has what it's to play as (learnAudio).
func (s *Source) find(id string) {
	ask, ok := s.wrapperTier()
	if !ok {
		return
	}
	s.llMu.Lock()
	s.loadKnown()
	a, ok := s.audio.get(id)
	level := playsAs(&a, ask)
	_, known := s.knownAt(id, level)
	if !ok || !viaWrapper(level) || (music.IsLossless(level) && !a.lossless) || known ||
		s.preparing[knownKey(id, level)] != nil || s.found[id] == level {
		s.llMu.Unlock()
		return
	}
	if len(s.found) >= foundMost {
		s.found = map[string]string{}
	}
	s.found[id] = level
	s.llMu.Unlock()
	failed := true
	if base, err := s.helperBase(); err == nil {
		q := playlistQuery(&a, level)
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

// wrapperStream is the song through the wrapper, as asked (music.Ask), or
// nil when it should play as the web player's AAC: no wrapper, nothing of
// it to play through one, or the wrapper failing. A song whose Dolby Atmos
// version doesn't come plays at the level, through the wrapper still if
// the level comes that way.
func (s *Source) wrapperStream(id, ask string, a *audioInfo) (*music.Stream, error) {
	if !s.hasWrapper() {
		return nil, nil
	}
	level, _ := music.SplitAsk(ask)
	tries := []string{playsAs(a, ask)}
	if tries[0] != level {
		tries = append(tries, level)
	}
	for _, quality := range tries {
		if !viaWrapper(quality) {
			continue
		}
		q, err := s.prepare(id, quality, false)
		if err != nil {
			if errors.Is(err, music.ErrSetup) {
				return nil, err
			}
			log.Printf("Apple Music: playing %s other than as %s: %v", id, quality, err)
			continue
		}
		if q == nil {
			continue
		}
		base, err := s.helperBase()
		if err != nil {
			return nil, err
		}
		st := &music.Stream{URL: base + "/song/" + id + ".m4a" + q.Query, Type: "m4a", Quality: q.Level, BitRate: q.Bandwidth,
			SampleRate: q.SampleRate, BitDepth: q.BitDepth}
		if q.BitRate > 0 {
			st.BitRate = q.BitRate
		}
		soundCheck(st, q.Loudness)
		return st, nil
	}
	return nil, nil
}
