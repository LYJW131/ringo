package app

import (
	"log"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// AutoPlay goes on with music like what played once the queue ends, as Apple
// Music's does. The player asks for it as its queue nears its end
// (player.More), but not under repeat, which goes round the queue instead,
// or while a station plays, which goes on with its own songs.

// autoplayFrom is how many of the last songs queued autoplay goes on from.
const autoplayFrom = 5

// station is the name of the station playing, as the player knows it, or "".
func (a *app) station() string { return a.pl.Radio() }

// autoplay gives the player songs like the last ones of its queue, if
// AutoPlay is on.
func (a *app) autoplay(got func([]music.Track)) bool {
	tr := a.pl.Current()
	if tr == nil || !a.settings.Autoplay {
		return false
	}
	var ids []string
	order, _ := a.pl.PlayOrder()
	queue := a.pl.Queue()
	for i := len(order) - 1; i >= 0 && len(ids) < autoplayFrom; i-- {
		ids = append([]string{queue[order[i]].ID}, ids...)
	}
	go func() {
		tracks, err := a.src.Autoplay(ids)
		a.post(func() {
			if err != nil {
				log.Printf("autoplay: %v", err)
			}
			if !a.settings.Autoplay {
				// Switched off meanwhile.
				tracks = nil
			}
			got(playable(tracks))
		})
	}()
	return true
}

// queueFoot ends Up Next with two buttons on glass. Clear takes out what was
// to play after the song playing, which plays on: with nothing after it,
// there is nothing to clear. AutoPlay is lit while it is on, as shuffle and
// repeat are under the cover, and a click turns it on and off; what it
// does is said under the pointer.
func (a *app) queueFoot(c *ui.Context) {
	if a.pl.Current() == nil {
		return
	}
	ui.Row(c.Key("queue-foot")).Padding(18, 10, 0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
		order, at := a.pl.PlayOrder()
		if glassPill(c.Key("clear"), nil, T("Clear"), false, a.npInk()).Disabled(at+1 >= len(order)).Clicked() {
			a.pl.ClearNext()
		}
		auto := glassPill(c.Key("autoplay"), iconInfinity, T("AutoPlay"), a.settings.Autoplay, a.npInk()).
			Checked(a.settings.Autoplay).Tooltip(T("Similar music will keep playing"))
		if auto.Clicked() {
			a.settings.Autoplay = !a.settings.Autoplay
			a.settings.save()
		}
	})
}
