package applemusic

import (
	"crypto/rand"
	"fmt"
	"log"
	"regexp"
	"strings"

	"mygo-music/internal/music"
)

// Plays, reported to Apple as its own players report them, so what's played
// here shows in Recently Played and counts toward Replay and recommendations.
// The web player's MusicKit posts every song's start and end to Apple's
// activity service. The app only says when a song starts, so a song's end
// goes with the next one's start, timed by the clock: a pause counts as
// playing, up to the song's length. The listener stops it with "off" in the
// Use Listening History setting.

// MusicKit's codes: an event is a start or an end, and an end has a reason.
const (
	playEnd             = 0
	playStart           = 1
	naturalEndOfTrack   = 7
	trackSkippedForward = 2
	itunesStoreContent  = 1
	musicKit            = 10
	durationsMost       = 3000
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

type nowPlaying struct {
	id       string
	duration int64
	started  int64
}

func (s *Source) event(song nowPlaying, typ int, fields map[string]any) (map[string]any, error) {
	dev, err := s.developerToken(false)
	if err != nil {
		return nil, err
	}
	sf, err := s.storefront()
	if err != nil {
		return nil, err
	}
	_, offset := s.clk.Now().Zone()
	e := map[string]any{
		"build-version":                  "MusicKitApp/1.0 macOS/10.15.7 model/Macintosh build/1.0",
		"developer-token":                dev,
		"user-token":                     s.userToken(),
		"user-agent":                     userAgent,
		"device-name":                    "Macintosh",
		"guid":                           s.guid,
		"event-type":                     typ,
		"type":                           itunesStoreContent,
		"media-type":                     0,
		"container-type":                 0,
		"feature-name":                   "",
		"ids":                            map[string]string{"subscription-adam-id": song.id},
		"media-duration-in-milliseconds": song.duration,
		"persistent-id":                  uuid(),
		"private-enabled":                false,
		"sb-enabled":                     true,
		"siri-initiated":                 false,
		"source-type":                    musicKit,
		"store-front":                    sf,
		"offline":                        false,
		"utc-offset-in-seconds":          offset,
		"start-position-in-milliseconds": 0,
	}
	for k, v := range fields {
		e[k] = v
	}
	return e, nil
}

// ended is the end of the song that played before, as far as the clock tells.
func (s *Source) ended(song nowPlaying, now int64) (map[string]any, error) {
	played := now - song.started
	whole := song.duration > 0 && played >= song.duration-2000
	end := played
	if song.duration > 0 {
		end = min(played, song.duration)
	}
	reason := trackSkippedForward
	if whole {
		reason = naturalEndOfTrack
	}
	return s.event(song, playEnd, map[string]any{
		"end-position-in-milliseconds": end,
		"end-reason-type":              reason,
		"timestamp":                    now,
	})
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

// Played is the app's word that a song started playing. Apple is told in
// the background.
func (s *Source) Played(track music.Track) {
	defer s.asked(false)()
	s.noteDuration(track.ID, track.Duration)
	now := s.now()
	// The song playing is swapped before anything is waited on, so plays
	// that come close together each end the one just before them.
	on := s.signedIn() && s.reporting()
	var song *nowPlaying
	s.actMu.Lock()
	var before *nowPlaying
	if s.nowPlaying != nil {
		was := *s.nowPlaying
		before = &was
	}
	if on && isCatalogID(track.ID) {
		d, _ := s.durations.get(track.ID)
		song = &nowPlaying{id: track.ID, duration: d, started: now}
	}
	s.nowPlaying = song
	s.actMu.Unlock()
	if !on {
		return
	}
	s.spawn(func() {
		if err := s.report(before, song, now); err != nil {
			log.Printf("Apple Music: reporting a play: %v", err)
		}
	})
}

func (s *Source) report(before, song *nowPlaying, now int64) error {
	var data []map[string]any
	if before != nil {
		e, err := s.ended(*before, now)
		if err != nil {
			return err
		}
		data = append(data, e)
	}
	if song != nil {
		s.actMu.Lock()
		this := *song
		s.actMu.Unlock()
		if this.duration == 0 {
			// Kept on what plays, for its end to say.
			this.duration = s.lengthOf(this.id)
			s.actMu.Lock()
			song.duration = this.duration
			s.actMu.Unlock()
		}
		e, err := s.event(this, playStart, map[string]any{"timestamp": now})
		if err != nil {
			return err
		}
		data = append(data, e)
	}
	if len(data) == 0 {
		return nil
	}
	_, err := s.request(s.activityURL, reqOpts{method: "POST", body: map[string]any{"client_id": "JSCLIENT", "event_type": "JSPLAY", "data": data}})
	return err
}
