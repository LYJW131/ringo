package app

import (
	"log"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// AutoPlay goes on with music like what played once the queue ends, as Apple
// Music's does, for a source that has some (music.Autoplayer). The player
// asks for it as its queue nears its end (player.More), but not under
// repeat, which goes round the queue instead, or while a station plays,
// which goes on with its own songs.

// autoplayFrom is how many of the last songs queued autoplay goes on from.
const autoplayFrom = 5

// station is the name of the station playing, as the player knows it, or "".
func (a *app) station() string { return a.pl.Radio() }

// autoplay gives the player songs like the last ones of its queue, from the
// source of the song playing, if AutoPlay is on and the source has it.
func (a *app) autoplay(got func([]music.Track)) bool {
	tr := a.pl.Current()
	if tr == nil || !a.settings.Autoplay {
		return false
	}
	lib := a.trackLib(tr)
	auto, ok := lib.src.(music.Autoplayer)
	if !ok || !lib.can("autoplay") {
		return false
	}
	var ids []string
	order, _ := a.pl.PlayOrder()
	queue := a.pl.Queue()
	for i := len(order) - 1; i >= 0 && len(ids) < autoplayFrom; i-- {
		if tr := &queue[order[i]]; tr.Source == "" || tr.Source == lib.id {
			ids = append([]string{tr.ID}, ids...)
		}
	}
	go func() {
		tracks, err := auto.Autoplay(ids)
		a.post(func() {
			if err != nil {
				log.Printf("source %s: autoplay: %v", lib.id, err)
			}
			if !a.settings.Autoplay {
				// Switched off meanwhile.
				tracks = nil
			}
			for i := range tracks {
				if tracks[i].Source == "" {
					tracks[i].Source = lib.id
				}
			}
			got(playable(tracks))
		})
	}()
	return true
}

// autoplaySwitch ends Up Next with the switch of AutoPlay, for a source
// that has it.
func (a *app) autoplaySwitch(c *ui.Context) {
	tr := a.pl.Current()
	if tr == nil || !a.trackLib(tr).can("autoplay") {
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
