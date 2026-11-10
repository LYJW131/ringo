package applemusic

import (
	"crypto/rand"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mygo-music/internal/music"
)

// Plays, reported to Apple as the web player at music.apple.com reports
// them, so what's played here shows in Recently Played and counts toward
// Replay and recommendations. Its MusicKit posts an event to Apple's
// activity service each time a song starts, is paused, played on, jumped
// within or comes to an end, one to a request and in the order they happen,
// and says in each which album, playlist or station the song is played from:
// that is what Recently Played lists. The player says all this as it
// happens (music.Recorder), where in the song it is; the source keeps what
// Apple was told, as MusicKit does, which an end's starting point and a
// start's hint depend on. The listener stops it with "off" in the Use
// Listening History setting.

// MusicKit's codes: an event is a start or an end, and an end has a reason.
const (
	playEnd   = 0
	playStart = 1

	endOther           = 1
	endSkippedForward  = 2
	endPaused          = 3
	endChangedItem     = 5
	endNaturalEnd      = 7
	endScrubBegin      = 12
	endScrubEnd        = 13
	endSkippedBackward = 14
	endExited          = 17
)

// What a start says of why it began: its container is not that of the event
// before it, or it is, or it has none.
const (
	hintNotSpecified     = 0
	hintContainerChanged = 1
)

const (
	itunesStoreContent = 1
	// webApp is what the web player says it is, among MusicKit's source types.
	webApp = 16

	containerNone     = 0
	containerRadio    = 1
	containerPlaylist = 2
	containerAlbum    = 3
	containerArtist   = 4

	// What the web player names where a song was started, for the pages it
	// was seen to: MusicKit's own name is for the rest.
	defaultFeature  = "music_kit-integration"
	albumFeature    = "new / album_detail"
	playlistFeature = "new / playlist_detail"

	durationsMost = 3000
)

// noteDuration notes a song's length, from the pages it was listed on, as
// the app's word that it plays may come without one.
func (s *Source) noteDuration(id string, ms int64) {
	if ms == 0 || !isCatalogID(id) {
		return
	}
	s.actMu.Lock()
	defer s.actMu.Unlock()
	s.durations.set(id, ms)
	s.durations.trim(durationsMost)
}

var offWords = regexp.MustCompile(`(?i)^(off|no|false|0)$`)

// reporting says whether plays are reported: unless the setting says off.
func (s *Source) reporting() bool {
	return !offWords.MatchString(strings.TrimSpace(s.Setting("listeningHistory")))
}

func uuid() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// persistentID is the 16 hex digits each event has of its own.
func persistentID() string {
	var b [8]byte
	rand.Read(b[:])
	return fmt.Sprintf("%x", b[:])
}

// told is what Apple was told of the song that plays, as MusicKit's own
// state of it is: an end is only sent for a song that started, and starts
// where the event before it left it.
type told struct {
	id      string
	playing bool  // started, and not ended
	pos     int64 // where the last event left it, in milliseconds
}

// playEvent is an event to send, with all that was true as it happened.
type playEvent struct {
	at       int64 // when, as Date.now()
	typ      int
	song     string
	duration int64 // 0 where it's not known yet: asked for when it's sent
	token    string
	origin   music.Origin
	mode     music.Mode
	start    int64
	end      int64
	reason   int
	hint     int
}

// whole is a position of a song, as MusicKit has it: in whole seconds.
func whole(d time.Duration) int64 { return d.Milliseconds() / 1000 * 1000 }

// stopReasons are the codes of what the player says of why a song stopped.
var stopReasons = map[music.StopReason]int{
	music.StopPaused:  endPaused,
	music.StopEnded:   endNaturalEnd,
	music.StopNext:    endSkippedForward,
	music.StopPrev:    endSkippedBackward,
	music.StopChanged: endChangedItem,
	music.StopQuit:    endExited,
	music.StopOther:   endOther,
}

// Played is the player's word of what happens to the song that plays. It
// answers at once: Apple is told in the background, in order.
func (s *Source) Played(ev music.PlayEvent) {
	id := ev.Track.ID
	s.noteDuration(id, ev.Track.Duration)
	token := s.userToken()
	on := token != "" && s.reporting() && isCatalogID(id)
	s.actMu.Lock()
	defer s.actMu.Unlock()
	if !on {
		// Not told of what isn't reported, and what was is over.
		s.reported = told{}
		return
	}
	duration, _ := s.durations.get(id)
	if ev.Track.Duration > 0 {
		duration = ev.Track.Duration
	}
	add := func(e playEvent) {
		e.at, e.song, e.duration, e.token, e.origin, e.mode = s.now(), id, duration, token, ev.Track.Origin, ev.Mode
		if e.typ == playEnd && duration > 0 {
			e.end = min(e.end, duration)
		}
		s.outbox = append(s.outbox, e)
		s.hasLast, s.last = true, ev.Track.Origin
		if !s.sending {
			s.sending = true
			s.spawn(s.drain)
		}
	}
	h := &s.reported
	switch ev.Action {
	case music.Started, music.Resumed:
		hint := hintNotSpecified
		resumed := ev.Action == music.Resumed && h.id == id && !h.playing
		if o := ev.Track.Origin; !resumed && o.Kind != music.FromNothing && (!s.hasLast || !o.Same(s.last)) {
			hint = hintContainerChanged
		}
		pos := whole(ev.Position)
		add(playEvent{typ: playStart, start: pos, hint: hint})
		*h = told{id: id, playing: true, pos: pos}
	case music.Stopped:
		if h.id != id || !h.playing {
			return
		}
		end := whole(ev.Position)
		if ev.Why == music.StopEnded {
			// As far as it goes, which isn't a whole second.
			end = duration
			if end == 0 {
				end = ev.Position.Milliseconds()
			}
		}
		add(playEvent{typ: playEnd, start: h.pos, end: end, reason: stopReasons[ev.Why]})
		*h = told{id: id, pos: end}
	case music.Sought:
		if h.id != id || !h.playing {
			return
		}
		from, to := whole(ev.From), whole(ev.Position)
		add(playEvent{typ: playEnd, start: h.pos, end: from, reason: endScrubBegin})
		add(playEvent{typ: playEnd, start: from, end: to, reason: endScrubEnd})
		h.pos = to
	}
}

// drain sends what Played queued, one event to a request, in order, as
// long as there is any. Those that fail are dropped, as the web player's do.
func (s *Source) drain() {
	defer s.asked(false)()
	for {
		s.actMu.Lock()
		if len(s.outbox) == 0 {
			s.sending = false
			for _, c := range s.flushed {
				close(c)
			}
			s.flushed = nil
			s.actMu.Unlock()
			return
		}
		e := s.outbox[0]
		s.outbox = s.outbox[1:]
		s.actMu.Unlock()
		if err := s.send(e); err != nil {
			log.Printf("Apple Music: reporting a play: %v", err)
		}
	}
}

// Flush waits, up to d, for what the player has said to be sent, for the
// app to quit without losing the end of what plays. It's asked before Close,
// which cancels what is still being sent.
func (s *Source) Flush(d time.Duration) {
	s.actMu.Lock()
	if !s.sending && len(s.outbox) == 0 {
		s.actMu.Unlock()
		return
	}
	c := make(chan struct{})
	s.flushed = append(s.flushed, c)
	s.actMu.Unlock()
	select {
	case <-c:
	case <-time.After(d):
	}
}

// send posts one event.
func (s *Source) send(e playEvent) error {
	dev, err := s.developerToken(false)
	if err != nil {
		return err
	}
	sf, err := s.storefront()
	if err != nil {
		return err
	}
	if e.duration == 0 {
		s.actMu.Lock()
		e.duration, _ = s.durations.get(e.song)
		s.actMu.Unlock()
		if e.duration == 0 {
			e.duration = s.lengthOf(e.song)
		}
	}
	_, offset := s.clk.Now().Zone()
	body := s.playBody(e, dev, sf, offset)
	_, err = s.request(s.activityURL, reqOpts{method: "POST", token: e.token, body: map[string]any{"client_id": "JSCLIENT", "event_type": "JSPLAY", "data": []map[string]any{body}}})
	return err
}

// playBody is the event as Apple has it, the fields the web player gives
// one and no others.
func (s *Source) playBody(e playEvent, dev, storefront string, offset int) map[string]any {
	typ, ids, feature := containerOf(e.origin)
	body := map[string]any{
		"build-version":                  "AppleMusic/1.0 macOS/10.15.7 model/MacIntel build/" + s.webBuild(),
		"developer-token":                dev,
		"user-token":                     e.token,
		"user-agent":                     userAgent,
		"event-type":                     e.typ,
		"type":                           itunesStoreContent,
		"media-type":                     0,
		"container-type":                 typ,
		"feature-name":                   feature,
		"ids":                            map[string]string{"subscription-adam-id": e.song},
		"media-duration-in-milliseconds": e.duration,
		"persistent-id":                  persistentID(),
		"play-mode":                      playMode(e.mode),
		"private-enabled":                false,
		"sb-enabled":                     true,
		"siri-initiated":                 false,
		"internal-build":                 false,
		"source-type":                    webApp,
		"store-front":                    storefront,
		"offline":                        false,
		"utc-offset-in-seconds":          offset,
		"start-position-in-milliseconds": e.start,
		// How long ago it happened, which it was a moment ago when MusicKit sends it.
		"milliseconds-since-play": max(s.now()-e.at, 0),
	}
	if len(ids) > 0 {
		body["container-ids"] = ids
	}
	if e.typ == playStart {
		body["event-reason-hint-type"] = e.hint
	} else {
		body["end-position-in-milliseconds"] = e.end
		body["end-reason-type"] = e.reason
	}
	return body
}

// playMode is how the player is set to play, in MusicKit's numbers: 1 for
// off, and for repeat 2 is one song and 3 is all.
func playMode(m music.Mode) map[string]int {
	auto, shuffle, repeat := 1, 1, 1
	if m.Autoplay {
		auto = 2
	}
	if m.Shuffle {
		shuffle = 2
	}
	switch m.Repeat {
	case music.RepeatOne:
		repeat = 2
	case music.RepeatAll:
		repeat = 3
	}
	return map[string]int{"auto-play-mode": auto, "repeat-play-mode": repeat, "shuffle-play-mode": shuffle}
}

var stationNumber = regexp.MustCompile(`^st\.([0-9]+)$`)

// containerOf is what MusicKit says of where a song is played from:
// the container's type, its IDs, and the name of where in the app it was
// started, which was only seen of the web player's album and playlist
// pages.
func containerOf(o music.Origin) (typ int, ids map[string]any, feature string) {
	ids = map[string]any{}
	feature = defaultFeature
	if o.ID == "" {
		o = music.Origin{}
	}
	// A library thing's ID starts with one letter and a dot, a catalog's does not.
	library := len(o.ID) > 2 && o.ID[1] == '.'
	switch o.Kind {
	case music.FromAlbum:
		typ = containerAlbum
		if library {
			ids["cloud-album-id"] = o.ID
		} else {
			ids["album-adam-id"] = o.ID
			feature = albumFeature
		}
	case music.FromPlaylist:
		typ = containerPlaylist
		switch {
		case library && o.CatalogID != "":
			ids["global-playlist-id"] = o.CatalogID
			ids["universal-library-id"] = o.ID
		case library:
			ids["universal-library-id"] = o.ID
		default:
			ids["global-playlist-id"] = o.ID
			feature = playlistFeature
		}
		if o.Version != "" {
			ids["playlist-version-hash"] = o.Version
		}
	case music.FromStation:
		typ = containerRadio
		ids["station-id"] = o.ID
		if m := stationNumber.FindStringSubmatch(o.ID); m != nil {
			n, _ := strconv.Atoi(m[1])
			ids["station-personalized-id"] = n
		}
	case music.FromArtist:
		typ = containerArtist
	default:
		typ = containerNone
	}
	return typ, ids, feature
}

// lengthOf is a song's length, from the catalog, for one the app plays
// without listing it first, as after a restart.
func (s *Source) lengthOf(id string) int64 {
	b, err := s.catalog("/songs/"+id, nil)
	if err != nil {
		return 0
	}
	r, err := first(b)
	if err != nil {
		return 0
	}
	ms := r.attrs().DurationInMillis
	s.noteDuration(id, ms)
	return ms
}
