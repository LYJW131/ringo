package player

import (
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"

	"ringo/internal/i18n"
	"ringo/internal/lyrics"
	"ringo/internal/music"
)

// RepeatMode is music's, which the source is told it in.
type RepeatMode = music.RepeatMode

const (
	RepeatOff = music.RepeatOff
	RepeatAll = music.RepeatAll
	RepeatOne = music.RepeatOne
)

// Player plays a queue of tracks, from its source. Its
// methods run on the main thread; loading happens on goroutines, whose results
// post brings back there.
type Player struct {
	post func(func())
	// Quiet is post without a redraw, for what mostly changes nothing, if set.
	Quiet func(func())
	// Source is where the tracks come from: none, and nothing plays.
	Source music.Source
	// Failed, if set, hears that the source said the listener has something to
	// do first, ErrSignIn or ErrSetup, of a song they asked to play.
	Failed func(err error)
	// Quality returns what to ask the source for, as the settings say: a
	// music.Ask, of a level and how songs with Dolby Atmos play.
	Quality func() string
	// Normalize reports whether to even out the loudness of tracks, as
	// the settings say.
	Normalize func() bool
	// Autoplay, if set, reports whether AutoPlay is on, as the settings
	// say, for what the source is told of how it plays.
	Autoplay func() bool
	// Cache keeps the songs played, if set.
	Cache *Cache
	// Changed, if set, hears that what plays, where it is or whether it
	// plays changed.
	Changed func()
	// More, if set, is asked for what to play after the queue, once it
	// nears its end, with no radio playing and repeat off: AutoPlay. It
	// answers false at once if it has nothing to give, or true and then
	// calls got on the main thread, with none if nothing came. It's asked
	// once a queue, and again once what it gave nears its end.
	More func(got func([]music.Track)) bool

	queue   []music.Track
	order   []int // the queue's indices in play order: shuffled or not
	at      int   // where in order the current track is, -1 for none
	shuffle bool
	repeat  RepeatMode
	volume  float64

	// A radio goes on with the tracks its feed gives, as the queue nears
	// its end, for as long as nothing else is played.
	radio    string
	feed     func(got func([]music.Track))
	radioGen int  // counts radios, to drop what an old one gives
	feeding  bool // the feed, or More, is asked
	awaiting bool // at the queue's end, for what the feed gives
	// moreAsked says More was asked at this end of the queue.
	moreAsked bool

	track   *music.Track
	out     *audio.Player
	stream  stream
	watch   *watched      // stream, as out plays it
	info    *music.Stream // what the current track streams
	fade    fader
	gain    float64 // the current track's normalizing gain
	loading bool
	playing bool   // what the listener wants: false while paused
	quality string // what the current track was asked for at
	err     string
	gen     int // counts loads, to drop the results of stale ones

	lyrics       []lyrics.Line
	lyricsLoaded bool

	// pending holds a seek asked for while the track loads.
	pending time.Duration
	// A seek playing fades out, then lands, then fades in: meanwhile the
	// position is where it goes.
	seeking bool
	seekTo  time.Duration
	seekGen int

	// after runs fn after d, on another goroutine; tests replace it.
	after func(d time.Duration, fn func())
	now   func() time.Time

	// told says the source was told the current track it plays, and not yet
	// that it stopped: so that what stops is told once, and what never
	// started isn't told of.
	told bool

	ahead *ahead // the next track's audio, asked for before its turn
}

func New(post func(func())) *Player {
	p := &Player{post: post, at: -1, volume: 0.8, gain: 1}
	p.after = func(d time.Duration, fn func()) { time.AfterFunc(d, fn) }
	p.now = time.Now
	// Music, mixed far enough ahead to play on while macOS keeps the app
	// waiting, as it does while its appearance changes.
	audio.SetLatency(audio.LatencyPlayback)
	// The media keys, and the controls of Control Center.
	mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
		switch cmd {
		case mygo.MediaPlay, mygo.MediaPause:
			if (cmd == mygo.MediaPlay) != p.playing {
				p.TogglePlay()
			}
		case mygo.MediaNext:
			p.Next()
		case mygo.MediaPrevious:
			p.Prev()
		case mygo.MediaSeek:
			p.Seek(pos)
		}
		p.post(func() {})
	})
	return p
}

// sync shows the system, and Changed, where playback is.
func (p *Player) sync() {
	if p.Changed != nil {
		defer p.Changed()
	}
	if p.track == nil {
		mygo.NowPlaying.Clear()
		return
	}
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.SetPlayback(state, p.Position())
}

// announce shows the system the track, and its cover once loaded.
func (p *Player) announce(t *music.Track, artwork []byte) {
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.Set(mygo.NowPlayingInfo{
		Title:    t.Name,
		Artist:   t.ArtistNames(),
		Album:    t.Album.Name,
		Artwork:  artwork,
		Duration: t.Length(),
		Position: p.Position(),
		State:    state,
		Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek,
	})
}

func (p *Player) ended() {
	// At its end, as it ends, not as the next one starts.
	p.stopped(music.StopEnded)
	if p.repeat == RepeatOne {
		p.out.Seek(0)
		p.out.Play()
		p.started(music.Started, 0)
		return
	}
	if p.at+1 >= len(p.order) && p.awaitMore() {
		return
	}
	if p.at+1 >= len(p.order) && p.repeat == RepeatOff {
		p.playing = false
		p.sync()
		return
	}
	p.Next()
}

// PlayQueue replaces the queue with tracks, which from is where they're
// played from, and plays the one at start, or the first after it that can
// play.
func (p *Player) PlayQueue(tracks []music.Track, start int, from music.Origin) {
	p.playQueue(stamped(tracks, from), start, forward)
}

// PlayTrack replaces the queue with tracks and plays the one at i, the
// listener's pick: one that can't play stays, saying why, rather than another
// playing in its place.
func (p *Player) PlayTrack(tracks []music.Track, i int, from music.Origin) {
	p.playQueue(stamped(tracks, from), i, stay)
}

// PlayShuffled replaces the queue with tracks in a random order, any of them
// first, and plays it with shuffle on.
func (p *Player) PlayShuffled(tracks []music.Track, from music.Origin) {
	p.shuffle = true
	p.playQueue(stamped(tracks, from), -1, forward)
}

// stamped copies tracks as played from from.
func stamped(tracks []music.Track, from music.Origin) []music.Track {
	out := append([]music.Track(nil), tracks...)
	for i := range out {
		out[i].Origin = from
	}
	return out
}

// playQueue plays tracks from start; a start below 0, when shuffled, lets
// any track come first.
func (p *Player) playQueue(tracks []music.Track, start, dir int) {
	if len(tracks) == 0 {
		return
	}
	p.stopped(music.StopChanged)
	p.endRadio()
	start = min(start, len(tracks)-1)
	p.queue = tracks
	p.order = make([]int, len(p.queue))
	for i := range p.order {
		p.order[i] = i
	}
	if p.shuffle {
		p.shuffleOrder(start)
		start = 0
	}
	p.loadSkipping(max(start, 0), 0, dir, true, 0)
}

// PlayRadio plays tracks in their order, then goes on with those feed
// gives as the queue nears its end, until something else is played. The
// radio's name says which it is. feed calls got on the main thread. The
// tracks, and what feed gives, say what they're played from themselves.
func (p *Player) PlayRadio(name string, tracks []music.Track, feed func(got func([]music.Track))) {
	if len(tracks) == 0 {
		return
	}
	shuffle := p.shuffle
	p.shuffle = false
	p.playQueue(append([]music.Track(nil), tracks...), 0, forward)
	p.shuffle = shuffle
	p.AttachRadio(name, feed)
}

// AttachRadio makes the queue the radio name's, which feed goes on with,
// as after restoring a session that played it.
func (p *Player) AttachRadio(name string, feed func(got func([]music.Track))) {
	p.endRadio()
	p.radio, p.feed = name, feed
	p.refill()
}

// EndRadio stops the radio playing: the song playing plays on, but the
// songs the radio queued after it go, and it adds no more.
func (p *Player) EndRadio() {
	if p.radio == "" && p.feed == nil {
		return
	}
	awaiting := p.awaiting
	p.endRadio()
	if p.at >= 0 && p.at+1 < len(p.order) {
		kept := make([]music.Track, p.at+1)
		for i, k := range p.order[:p.at+1] {
			kept[i] = p.queue[k]
		}
		p.queue, p.order = kept, make([]int, len(kept))
		for i := range p.order {
			p.order[i] = i
		}
	}
	if awaiting {
		// At its end, waiting for more: nothing plays on.
		p.playing = false
		p.sync()
	}
}

func (p *Player) endRadio() {
	p.radioGen++
	p.radio, p.feed, p.feeding, p.awaiting = "", nil, false, false
	p.moreAsked = false
}

// Radio returns the name of the radio playing, or "".
func (p *Player) Radio() string { return p.radio }

// refill asks the radio, or else More, for what follows once the queue is
// near its end.
func (p *Player) refill() {
	if p.feeding || p.at < len(p.order)-2 {
		return
	}
	gen := p.radioGen
	got := func(more []music.Track) {
		if gen != p.radioGen {
			return
		}
		p.feeding = false
		n := len(p.order)
		p.appendTracks(more)
		if len(p.order) > n && p.feed == nil {
			// What More gave may run out too: it's asked again then.
			p.moreAsked = false
		}
		switch {
		case !p.awaiting:
		case p.at+1 < len(p.order):
			p.awaiting = false
			p.load(p.at + 1)
		default:
			// Nothing more: it stops at its end, saying so of a station,
			// but as any queue does after AutoPlay.
			p.awaiting, p.playing = false, false
			if p.feed != nil {
				p.err = i18n.T("The station has nothing more right now")
			}
			p.sync()
		}
	}
	if p.feed != nil {
		p.feeding = true
		p.feed(got)
		return
	}
	if !p.mayAskMore() {
		return
	}
	p.feeding, p.moreAsked = true, true
	if !p.More(got) && gen == p.radioGen {
		// Nothing to give for now, as with AutoPlay off: asked again at the
		// end, it may have something then.
		p.feeding, p.moreAsked = false, false
	}
}

// mayAskMore reports whether More may be asked for what follows the queue.
func (p *Player) mayAskMore() bool {
	return p.feed == nil && p.More != nil && p.repeat == RepeatOff && !p.moreAsked
}

// awaitMore waits at the end of the queue for what the radio, or More, gives,
// and reports whether anything may come.
func (p *Player) awaitMore() bool {
	if p.feed == nil && !p.feeding && !p.mayAskMore() {
		return false
	}
	gen := p.gen
	p.awaiting = true
	p.refill()
	switch {
	case p.gen != gen:
		// It gave at once, and what it gave loads.
		return true
	case p.feed == nil && !p.feeding:
		p.awaiting = false
		return false
	}
	return true
}

// appendTracks adds tracks to the end of the queue, but those in it.
func (p *Player) appendTracks(tracks []music.Track) {
	in := make(map[string]bool, len(p.queue))
	for _, t := range p.queue {
		in[t.ID] = true
	}
	for _, t := range tracks {
		if !in[t.ID] {
			in[t.ID] = true
			p.order = append(p.order, len(p.queue))
			p.queue = append(p.queue, t)
		}
	}
}

// PlayNext puts tracks after the one playing, to play next in the order they
// come, and PlayLast after the rest of the queue: what plays plays on. With
// nothing queued, they play now. Shuffled, they still play next, and
// shuffle off, they're where they were put.
func (p *Player) PlayNext(tracks []music.Track, from music.Origin) { p.insert(tracks, true, from) }

func (p *Player) PlayLast(tracks []music.Track, from music.Origin) { p.insert(tracks, false, from) }

func (p *Player) insert(tracks []music.Track, next bool, from music.Origin) {
	if len(tracks) == 0 {
		return
	}
	tracks = stamped(tracks, from)
	if p.at < 0 || len(p.order) == 0 {
		p.playQueue(tracks, 0, forward)
		return
	}
	// Into the queue after the current track, or at its end; the order
	// after where it is.
	q, o := len(p.queue), len(p.order)
	if next {
		q, o = p.order[p.at]+1, p.at+1
	}
	for k, i := range p.order {
		if i >= q {
			p.order[k] = i + len(tracks)
		}
	}
	add := make([]int, len(tracks))
	for i := range add {
		add[i] = q + i
	}
	p.queue = slices.Insert(slices.Clip(p.queue), q, tracks...)
	p.order = slices.Insert(slices.Clip(p.order), o, add...)
	p.moreAsked = false
	if p.awaiting {
		// At the end, waiting on a radio for more: these come first.
		p.awaiting = false
		p.load(p.at + 1)
		return
	}
	if p.out != nil {
		p.getAhead(p.quality)
	}
	p.sync()
}

// Remove takes the queue's track i out of it, if it hasn't played: one that is
// after the track playing, in the order it plays. It reports whether it did.
// What plays isn't touched; what was asked for ahead goes if it was this one,
// and the one after takes its place.
func (p *Player) Remove(i int) bool {
	k := p.orderAt(i)
	if k < 0 || k <= p.at {
		return false
	}
	// New slices, not the old ones changed: Session may hold them.
	queue := slices.Delete(slices.Clone(p.queue), i, i+1)
	order := make([]int, 0, len(p.order)-1)
	for _, v := range p.order {
		switch {
		case v < i:
			order = append(order, v)
		case v > i:
			order = append(order, v-1)
		}
	}
	p.queue, p.order = queue, order
	p.afterReorder()
	return true
}

// Move puts the queue's track i at place at in the order it plays, the places
// of the rest after it giving way, if it hasn't played and the place is after
// the track playing. It reports whether anything moved. Shuffled, only the
// order changes, and the queue is as it was given when shuffle goes off; not
// shuffled, the queue is the order, and moves with it.
func (p *Player) Move(i, at int) bool {
	k := p.orderAt(i)
	if k <= p.at || at <= p.at || at >= len(p.order) || at == k {
		return false
	}
	// New slices, not the old ones changed: Session may hold them.
	order := slices.Clone(p.order)
	order = slices.Insert(slices.Delete(order, k, k+1), at, i)
	if p.shuffle {
		p.order = order
	} else {
		queue := make([]music.Track, len(order))
		for j, v := range order {
			queue[j] = p.queue[v]
		}
		p.queue = queue
		p.order = make([]int, len(queue))
		for j := range p.order {
			p.order[j] = j
		}
	}
	p.afterReorder()
	return true
}

// orderAt returns where the queue's track i is in the order, or -1.
func (p *Player) orderAt(i int) int {
	for k, v := range p.order {
		if v == i {
			return k
		}
	}
	return -1
}

// afterReorder follows a change to what comes after the track playing: the
// next may be another now, so what was asked for ahead is asked again, and
// the end of the queue may be near enough for a radio or AutoPlay to give more.
func (p *Player) afterReorder() {
	if p.out != nil {
		p.getAhead(p.quality)
	} else if a := p.ahead; a != nil {
		// Still loading: it's asked for once the track has loaded, but what
		// isn't next any more needn't wait there until then.
		if t, ok := p.upNext(); !ok || !a.is(t, p.quality) {
			p.dropAhead()
		}
	}
	p.refill()
	p.sync()
}

// shuffleOrder puts the order in a random one with first at its head, or
// with any at its head when first is below 0.
func (p *Player) shuffleOrder(first int) {
	rand.Shuffle(len(p.order), func(i, j int) { p.order[i], p.order[j] = p.order[j], p.order[i] })
	for i, v := range p.order {
		if v == first {
			p.order[0], p.order[i] = p.order[i], p.order[0]
		}
	}
}

func (p *Player) ToggleShuffle() {
	p.shuffle = !p.shuffle
	if len(p.order) == 0 {
		return
	}
	cur := p.order[max(p.at, 0)]
	if p.shuffle {
		p.shuffleOrder(cur)
		p.at = 0
	} else {
		for i := range p.order {
			p.order[i] = i
		}
		p.at = cur
	}
}

func (p *Player) CycleRepeat() { p.repeat = (p.repeat + 1) % 3 }

// Current returns the track playing or loading, or nil.
func (p *Player) Current() *music.Track { return p.track }

// Queue returns the tracks queued, in the order they were given.
func (p *Player) Queue() []music.Track { return p.queue }

// ClearNext takes out of the queue what was to play after the track
// playing, in the order it plays in: that track plays on, what played
// before it stays behind it, and nothing is after it. What was asked for
// ahead of the next one is let go. The queue then ends with the track
// playing, and what goes on from a queue's end, a radio or AutoPlay, is
// asked anew: what it was asked for before followed songs that are gone.
func (p *Player) ClearNext() {
	if p.at < 0 || p.at+1 >= len(p.order) {
		return
	}
	// Those kept stay in the order they were given, which shuffle going
	// off puts them back in.
	order := slices.Clone(p.order[:p.at+1])
	given := slices.Clone(order)
	slices.Sort(given)
	queue, to := make([]music.Track, len(given)), make(map[int]int, len(given))
	for n, i := range given {
		queue[n], to[i] = p.queue[i], n
	}
	for k, i := range order {
		order[k] = to[i]
	}
	p.queue, p.order = queue, order
	p.radioGen++
	p.feeding, p.moreAsked = false, false
	if p.out != nil {
		p.getAhead(p.quality)
	} else {
		p.dropAhead()
	}
	p.refill()
	p.sync()
}

// Playing reports what the listener wants: false while paused.
func (p *Player) Playing() bool { return p.playing }

// Loading reports whether the current track is still loading.
// Loading reports whether the current track is on its way: loading, or
// waiting for the next of its bytes, after a seek past what has come in.
func (p *Player) Loading() bool {
	return p.loading || p.awaiting || p.watch != nil && p.watch.starved()
}

// Ready reports whether the current track has loaded, to play.
func (p *Player) Ready() bool { return p.out != nil }

// Err returns why the current track did not play, or "".
func (p *Player) Err() string { return p.err }

// Stream returns what the current track streams, once loaded, or nil.
func (p *Player) Stream() *music.Stream { return p.info }

func (p *Player) Shuffle() bool      { return p.shuffle }
func (p *Player) Repeat() RepeatMode { return p.repeat }

// Lyrics returns the current track's lyrics, and whether they have loaded.
func (p *Player) Lyrics() ([]lyrics.Line, bool) { return p.lyrics, p.lyricsLoaded }

// Downloaded returns the share of the current track downloaded so far, or
// false before it has started to.
func (p *Player) Downloaded() (float64, bool) {
	if p.stream == nil {
		return 0, false
	}
	dl, _ := p.stream.source()
	return dl.Progress(), true
}

// PlayOrder returns the queue's indices in play order, and where the
// current track is among them, -1 for none.
func (p *Player) PlayOrder() ([]int, int) { return p.order, p.at }

func (p *Player) Next() {
	if len(p.order) == 0 {
		return
	}
	p.stopped(music.StopNext)
	at := p.at + 1
	if at >= len(p.order) {
		if p.feed != nil || p.feeding || p.mayAskMore() {
			p.stop()
			if p.awaitMore() {
				return
			}
		}
		at = 0
	}
	p.load(at)
}

// Prev plays the track before this one, however far this one has played:
// once a few seconds in, going back to its start only kept the listener
// pressing twice.
func (p *Player) Prev() {
	if len(p.order) == 0 {
		return
	}
	p.stopped(music.StopPrev)
	at := p.at - 1
	if at < 0 {
		at = len(p.order) - 1
	}
	// One that can't play is passed going back, as going forward.
	p.loadSkipping(at, 0, back, true, 0)
}

// Jump plays the queue's track i, the listener's pick: one that can't play
// stays, saying why.
func (p *Player) Jump(i int) {
	for k, v := range p.order {
		if v == i {
			p.stopped(music.StopChanged)
			p.loadSkipping(k, 0, stay, true, 0)
			return
		}
	}
}

// TogglePlay pauses, fading out, or plays, fading in.
func (p *Player) TogglePlay() {
	if p.out == nil {
		switch {
		case p.loading:
			// It plays once loaded, or not.
			p.playing = !p.playing
			p.sync()
		case len(p.order) > 0:
			// Again, as for a track that couldn't play.
			p.loadSkipping(max(p.at, 0), 0, stay, true, p.pending)
		}
		return
	}
	out := p.out
	if p.playing {
		p.playing = false
		p.stopped(music.StopPaused)
		p.fade.fade(0, fadeOut, func() {
			p.post(func() {
				if p.out == out && !p.playing {
					out.Pause()
					p.sync()
				}
			})
		})
		p.sync()
		return
	}
	p.playing = true
	// Played again from its start, if it had ended, is a new play.
	action := music.Resumed
	if !p.seeking && !out.Playing() {
		if out.Position() >= out.Duration()-time.Second {
			out.Seek(0)
			action = music.Started
		}
		out.Play()
	}
	p.started(action, p.Position())
	p.fade.fade(1, fadeIn, nil)
	p.sync()
}

// seekGap is how long a seek waits for what plays to fade out, which the
// audio player does over 5 ms, as it pauses: within the next buffer or
// two it sends the system.
const seekGap = 30 * time.Millisecond

// Seek moves to t. Playing, it pauses and plays again around the jump,
// which the audio player fades over a few milliseconds, rather than
// cutting from one sound to the other with a click.
func (p *Player) Seek(t time.Duration) {
	if p.out == nil {
		p.pending = t
		return
	}
	out := p.out
	if p.told {
		// From where it is, which a seek still on its way has moved.
		p.tell(music.PlayEvent{Action: music.Sought, From: p.Position(), Position: t})
	}
	// One past what has come in waits for it: a frame once that would show.
	p.after(seekGap+starvedAfter+20*time.Millisecond, func() { p.post(func() {}) })
	// Paused, it jumps at once; but a seek on its way paused it only for
	// the jump, and this one takes its place, playing on from there.
	if !out.Playing() && !p.seeking {
		out.Seek(t)
		p.sync()
		return
	}
	p.seekGen++
	gen := p.seekGen
	p.seeking, p.seekTo = true, t
	out.Pause()
	time.AfterFunc(seekGap, func() {
		p.post(func() {
			if gen != p.seekGen || p.out != out {
				return
			}
			p.seeking = false
			out.Seek(t)
			if p.playing || !p.fade.silent() {
				out.Play()
			}
			p.sync()
		})
	})
	p.sync()
}

func (p *Player) SetVolume(v float64) {
	p.volume = v
	p.applyGain()
}

// applyGain sets the volume of what plays: the listener's, which the ear
// hears in powers, and the track's gain, if normalizing.
func (p *Player) applyGain() {
	g := p.volume * p.volume
	if p.Normalize != nil && p.Normalize() {
		g *= p.gain
	}
	p.fade.setGain(g)
}

// albumLoudness reports whether track t, of the stream st, plays at its
// album's loudness rather than its own, as Apple Music's Sound Check does:
// only when it's played from its album, in the album's order, so that the
// album sounds as it was mastered, its quiet songs quieter than its loud
// ones, and every song of it at the one gain. Shuffled, or from a playlist,
// a station, an artist, a song on its own or AutoPlay, songs from many
// albums mix, and each plays at its own. An album whose loudness the source
// doesn't know plays each song at its own too.
func albumLoudness(t music.Track, shuffle bool, st *music.Stream) bool {
	return t.Origin.Kind == music.FromAlbum && !shuffle && st != nil && (st.AlbumGain != 0 || st.AlbumPeak != 0)
}

// normGain is the gain that brings a track as loud as others, as its source
// measured it (music.Stream.Gain), or its album as loud as others
// (music.Stream.AlbumGain), but never so loud that its peaks would clip.
func normGain(st *music.Stream, album bool) float64 {
	if st == nil {
		return 1
	}
	gain, peak := st.Gain, st.Peak
	// The album's peak with its gain, so that every song of it is held
	// back alike: each song's own would undo the album's balance.
	if album {
		gain, peak = st.AlbumGain, st.AlbumPeak
	}
	if gain == 0 && peak == 0 {
		return 1
	}
	g := math.Pow(10, gain/20)
	// The peak holds back a lift only: a true peak past full scale, as a
	// loud master's can be, doesn't turn a track down below its gain.
	if peak > 0 {
		g = min(g, max(1, 1/peak))
	}
	return g
}

func (p *Player) Position() time.Duration {
	switch {
	case p.out == nil:
		return p.pending
	case p.seeking:
		return p.seekTo
	}
	return p.out.Position()
}

func (p *Player) Duration() time.Duration {
	switch {
	case p.track == nil:
		return 0
	case p.track.Duration > 0 || p.out == nil:
		return p.track.Length()
	}
	return p.out.Duration() // a source that didn't say
}

// Levels returns the spectrum of what plays now.
func (p *Player) Levels() ([Bands]float32, bool) {
	if p.stream == nil || p.out == nil || !p.playing {
		return [Bands]float32{}, false
	}
	_, spec := p.stream.source()
	frame := int64(p.Position().Seconds() * float64(p.stream.SampleRate()))
	return spec.at(frame)
}

func (p *Player) stop() {
	if out := p.out; out != nil {
		// Paused first, it fades out over a few milliseconds, rather than
		// stopping with a click.
		out.Pause()
		time.AfterFunc(100*time.Millisecond, out.Close)
	}
	p.out, p.stream, p.watch, p.info = nil, nil, nil, nil
	p.seeking = false
	p.fade.attach(nil, 1)
}

// Which way a load goes past a track that can't play: on to the next, back to
// the one before, or nowhere, staying on it to say why.
const (
	forward = 1
	back    = -1
	stay    = 0
)

// load plays the track at order[at], skipping those that cannot play.
func (p *Player) load(at int) {
	p.loadSkipping(at, 0, forward, true, 0)
}

// recorder returns the source, if it wants to know what's played and a
// track plays.
func (p *Player) recorder() music.Recorder {
	if p.track == nil {
		return nil
	}
	rec, _ := p.Source.(music.Recorder)
	return rec
}

// mode is how the player is set to play.
func (p *Player) mode() music.Mode {
	return music.Mode{Shuffle: p.shuffle, Repeat: p.repeat, Autoplay: p.Autoplay != nil && p.Autoplay()}
}

// tell tells the source of the current track what happens to it, with where
// in it that is, as the player has it and not by the clock.
func (p *Player) tell(ev music.PlayEvent) {
	rec := p.recorder()
	if rec == nil {
		return
	}
	ev.Track, ev.Mode = *p.track, p.mode()
	rec.Played(ev)
}

// started tells the source the current track plays, from pos.
func (p *Player) started(action music.PlayAction, pos time.Duration) {
	if p.track == nil || !p.playing || p.recorder() == nil {
		return
	}
	p.told = true
	p.tell(music.PlayEvent{Action: action, Position: pos})
}

// stopped tells the source the current track no longer plays, if it was told
// that it does.
func (p *Player) stopped(why music.StopReason) {
	if !p.told {
		return
	}
	p.told = false
	p.tell(music.PlayEvent{Action: music.Stopped, Position: p.Position(), Why: why})
}

// Exit tells the source what plays that the app is closing, before it does.
func (p *Player) Exit() { p.stopped(music.StopQuit) }

// StopTelling tells the source what plays is done with, though it plays on,
// and tells it no more of it: for a listener who turns their history off.
func (p *Player) StopTelling() { p.stopped(music.StopOther) }

// loadSkipping loads the track at order[at], to play or not, from from,
// skipping those that cannot play the way dir goes: skipped of them so far.
func (p *Player) loadSkipping(at, skipped, dir int, play bool, from time.Duration) {
	t := p.queue[p.order[at]]
	src := p.Source
	// Whatever was playing is over; those who ask for another say why first.
	p.stopped(music.StopChanged)
	p.stop()
	p.gen++
	gen := p.gen
	p.at = at
	p.track = &t
	p.loading, p.playing, p.err = true, play, ""
	p.awaiting = false
	p.lyrics, p.lyricsLoaded = nil, false
	p.pending = from
	p.announce(&t, nil)
	p.refill()

	p.loadCover(gen, &t)
	p.loadLyrics(gen, src, t)
	quality := "high"
	if p.Quality != nil {
		quality = p.Quality()
	}
	p.quality = quality
	pre := p.takeAhead(t, quality)
	go func() {
		info, s, err := p.open(src, t, quality, pre)
		p.post(func() {
			if gen != p.gen {
				if s != nil {
					s.Close()
				}
				return
			}
			p.loading = false
			if err != nil {
				p.loadFailed(err, at, skipped, dir)
				return
			}
			p.stream, p.info = s, info
			// Chosen as the song starts, and kept to its end: shuffle
			// switched meanwhile changes the next song's, not this one's
			// volume.
			p.gain = normGain(info, albumLoudness(t, p.shuffle, info))
			p.watch = &watched{stream: s, now: p.now, ended: func() { p.post(func() {}) }}
			p.out = audio.NewPlayer(p.watch)
			// From its middle, as a session picked up again, it fades in.
			level := 1.0
			if p.pending > 0 {
				level = 0
			}
			p.fade.attach(p.out, level)
			p.applyGain()
			p.out.OnEnd(func() {
				p.post(func() {
					if gen == p.gen {
						p.ended()
					}
				})
			})
			from := p.pending
			if p.pending > 0 {
				p.out.Seek(p.pending)
				p.pending = 0
			}
			if p.playing {
				p.out.Play()
				p.fade.fade(1, fadeIn, nil)
				p.started(music.Started, from)
			}
			p.sync()
			p.getAhead(quality)
		})
	}()
}

// loadCover shows the system the cover of t, once it has it.
func (p *Player) loadCover(gen int, t *music.Track) {
	url := t.Album.ImageURL
	if url == "" {
		return
	}
	go func() {
		// The system shows it small, as Now Playing does.
		cover, err := music.Fetch(music.SizedImage(url, 600))
		if err == nil {
			p.post(func() {
				if gen == p.gen {
					p.announce(t, cover)
				}
			})
		}
	}()
}

// loadLyrics asks the source for t's lyrics.
func (p *Player) loadLyrics(gen int, src music.Source, t music.Track) {
	go func() {
		var lines []lyrics.Line
		if src != nil {
			if l, err := src.Lyrics(t.ID); err == nil && l != nil {
				lines = lyrics.FromSource(l)
			}
		}
		p.post(func() {
			if gen == p.gen {
				p.lyrics = lines
				p.lyricsLoaded = true
			}
		})
	}()
}

// loadFailed says why the track at order[at] can't play, and skips it the way
// dir goes, unless it stays, nothing in the queue can play, or the listener has
// to do something first, which would stop the next track too.
func (p *Player) loadFailed(err error, at, skipped, dir int) {
	p.err = errorText(err)
	if errors.Is(err, music.ErrSignIn) || errors.Is(err, music.ErrSetup) {
		// Only a song the listener asked to play takes them to fix it: one
		// loaded paused, as a session picked up is, just says why.
		asked := p.playing
		p.playing = false
		if p.Failed != nil && asked {
			p.Failed(err)
		}
		p.sync()
		return
	}
	if dir != stay && skipped+1 < len(p.order) {
		next := (at + dir + len(p.order)) % len(p.order)
		p.loadSkipping(next, skipped+1, dir, p.playing, 0)
		return
	}
	p.playing = false
	p.sync()
}

// errorText says what went wrong with a track, in the listener's terms where
// the source's error is one the app knows.
func errorText(err error) string {
	switch {
	case errors.Is(err, errNoSource):
		return i18n.T("This song isn't available")
	case errors.Is(err, music.ErrUnavailable):
		return i18n.T("This song isn't available")
	case errors.Is(err, music.ErrSignIn):
		return i18n.T("Sign in to play this song")
	case errors.Is(err, music.ErrSetup):
		if reason := music.Reason(err); reason != "" {
			return reason
		}
		return i18n.T("Apple Music needs setting up")
	}
	return err.Error()
}

var errNoSource = errors.New("player: no such source")

// open opens track t's audio at quality: what was asked for ahead of it, if
// that worked; or kept in the cache, or else streamed into the cache, and
// kept once it has all come in. What failed ahead is asked for again: it may
// have been a moment's.
func (p *Player) open(src music.Source, t music.Track, quality string, pre *ahead) (*music.Stream, stream, error) {
	if src == nil {
		if pre != nil {
			pre.close()
		}
		return nil, nil, errNoSource
	}
	var dl *download
	var info *music.Stream
	ok := false
	if pre != nil {
		<-pre.done
		if pre.err == nil {
			info, dl, ok = pre.info, pre.dl, true
		}
	}
	if !ok {
		dl, info, ok = p.Cache.lookup(t.ID, quality)
	}
	if !ok {
		var err error
		info, err = src.Stream(t.ID, quality)
		if err != nil {
			return nil, nil, err
		}
		if info == nil {
			return nil, nil, music.ErrUnavailable
		}
		dl = p.Cache.download(t.ID, quality, info)
	}
	s, err := openMP4Stream(dl)
	if err != nil {
		dl.Close()
		return nil, nil, err
	}
	return info, s, nil
}

// Session is where playback was left, to pick up again.
type Session struct {
	Queue    []music.Track `json:"queue"`
	Order    []int         `json:"order"`
	At       int           `json:"at"`
	Shuffle  bool          `json:"shuffle"`
	Repeat   RepeatMode    `json:"repeat"`
	Position int64         `json:"position"` // milliseconds
	Radio    string        `json:"radio,omitempty"`
}

// Session returns where playback is, or false with nothing queued.
func (p *Player) Session() (Session, bool) {
	if p.track == nil || p.at < 0 {
		return Session{}, false
	}
	return Session{
		Queue:    p.queue,
		Order:    p.order,
		At:       p.at,
		Shuffle:  p.shuffle,
		Repeat:   p.repeat,
		Position: p.Position().Milliseconds(),
		Radio:    p.radio,
	}, true
}

// Restore picks up a session where it was left, paused, and loads its
// track to play at once when asked.
func (p *Player) Restore(s Session) {
	n := len(s.Queue)
	if n == 0 || len(s.Order) != n || s.At < 0 || s.At >= n {
		return
	}
	seen := make([]bool, n)
	for _, i := range s.Order {
		if i < 0 || i >= n || seen[i] {
			return
		}
		seen[i] = true
	}
	p.queue, p.order = s.Queue, s.Order
	p.shuffle, p.repeat = s.Shuffle, s.Repeat%3
	p.loadSkipping(s.At, 0, stay, false, time.Duration(s.Position)*time.Millisecond)
}
