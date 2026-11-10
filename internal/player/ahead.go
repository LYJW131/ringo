package player

import "ringo/internal/music"

// The next track's audio is asked for once the current one plays, and comes in
// meanwhile, so that it starts at once when its turn comes: a source may take
// seconds to give a stream, as one that works it out with programs does.

// ahead is a track's audio asked for before its turn.
type ahead struct {
	id, quality string
	done        chan struct{} // closed once info, dl and err are in
	info        *music.Stream
	dl          *download
	err         error
}

func (a *ahead) is(t music.Track, quality string) bool {
	return a.id == t.ID && a.quality == quality
}

// upNext is the track that plays when the current one ends, if one does by
// itself: not a radio's next, which its feed may not have given yet.
func (p *Player) upNext() (music.Track, bool) {
	if p.at < 0 || p.at >= len(p.order) || p.repeat == RepeatOne {
		return music.Track{}, false
	}
	at := p.at + 1
	if at >= len(p.order) {
		if p.repeat != RepeatAll || p.feed != nil {
			return music.Track{}, false
		}
		at = 0
	}
	if at == p.at {
		return music.Track{}, false
	}
	return p.queue[p.order[at]], true
}

// getAhead asks for the next track's audio at quality, unless it's kept
// already. What was asked for before that
// isn't next any more is let go.
func (p *Player) getAhead(quality string) {
	t, ok := p.upNext()
	if a := p.ahead; a != nil {
		if ok && a.is(t, quality) {
			return
		}
		p.dropAhead()
	}
	if !ok {
		return
	}
	src := p.Source
	if src == nil || p.Cache.has(t.ID, quality) {
		return
	}
	a := &ahead{id: t.ID, quality: quality, done: make(chan struct{})}
	p.ahead = a
	go func() {
		defer close(a.done)
		info, err := src.Stream(t.ID, quality)
		if err == nil && info == nil {
			err = music.ErrUnavailable
		}
		if err != nil {
			a.err = err
			return
		}
		a.info = info
		a.dl = p.Cache.download(t.ID, quality, info)
	}()
}

// takeAhead returns what was asked for ahead of t at quality, if it was, for
// the caller to open or close.
func (p *Player) takeAhead(t music.Track, quality string) *ahead {
	a := p.ahead
	if a == nil || !a.is(t, quality) {
		return nil
	}
	p.ahead = nil
	return a
}

// dropAhead lets go of what was asked for ahead, stopping it if it's still
// coming in. What has all come in stays in the cache.
func (p *Player) dropAhead() {
	a := p.ahead
	p.ahead = nil
	if a == nil {
		return
	}
	go a.close()
}

// close stops what was asked for ahead once it's in, if it's still coming in.
func (a *ahead) close() {
	<-a.done
	if a.dl != nil {
		a.dl.Close()
	}
}
