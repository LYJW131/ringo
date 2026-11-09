package app

import (
	"log"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
	"mygo-music/internal/player"
)

// AutoPlay goes on with music like what played once the queue ends, as Apple
// Music's does, for a source that has some (music.Autoplayer). The player
// goes on past its queue only with a radio's feed, so autoplay is one: a
// radio by a name no station's can be (radioName), whose feed asks the
// source for songs like the last ones queued. A new queue drops it, as it
// drops a station, and the view attaches it again (keepAutoplay).

// autoplayRadio is the name autoplay plays by, as a radio.
const autoplayRadio = "autoplay"

// autoplayFrom is how many of the last songs queued autoplay goes on from.
const autoplayFrom = 5

// station is the name of the station playing, as the player knows it, or
// "": autoplay is none.
func (a *app) station() string {
	if r := a.pl.Radio(); r != autoplayRadio {
		return r
	}
	return ""
}

// keepAutoplay attaches autoplay to the queue playing, where it goes on, or
// takes it off where it doesn't: switched off, under repeat, which goes
// round the queue instead, or for a source without it. A station playing
// goes on with its own songs.
func (a *app) keepAutoplay() {
	pl := a.pl
	tr := pl.Current()
	if tr == nil {
		return
	}
	lib := a.trackLib(tr)
	on := a.settings.Autoplay && pl.Repeat() == player.RepeatOff && lib.can("autoplay")
	switch r := pl.Radio(); {
	case on && r == "":
		pl.AttachRadio(autoplayRadio, a.autoplayFeed(lib))
	case !on && r == autoplayRadio:
		// What it added stays, but no more comes. EndRadio would drop the
		// songs after the one playing, which may be the listener's.
		pl.AttachRadio("", nil)
	}
}

// autoplayFeed gives the player songs like the last ones of its queue, from
// lib, as it nears its end.
func (a *app) autoplayFeed(lib *library) func(got func([]music.Track)) {
	return func(got func([]music.Track)) {
		auto, ok := lib.src.(music.Autoplayer)
		if !ok || !a.settings.Autoplay {
			got(nil)
			return
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
				for i := range tracks {
					if tracks[i].Source == "" {
						tracks[i].Source = lib.id
					}
				}
				got(playable(tracks))
			})
		}()
	}
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
