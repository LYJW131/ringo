package app

import (
	"log"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
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

// autoplaySwitch ends Up Next with the switch of AutoPlay.
func (a *app) autoplaySwitch(c *ui.Context) {
	tr := a.pl.Current()
	if tr == nil {
		return
	}
	ui.Row(c.Key("autoplay")).Padding(18, 10, 0, 10).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, T("AutoPlay")).FontSize(14).FontWeight(600)
			ui.Text(c, T("Similar music will keep playing")).FontSize(12.5).TextColor(white.Alpha(0.55)).SingleLine()
		})
		if ui.Switch(c.Key("switch"), &a.settings.Autoplay).Label(T("AutoPlay")).Changed() {
			a.settings.save()
		}
	})
}
