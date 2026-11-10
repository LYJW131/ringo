package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"ringo/internal/music"
)

// Radios play on without end: a station of a source, whose StationNext goes on
// giving tracks. The player knows one by a name, which says all the app needs
// after a restart: the source, the station, and what it's called.

// radioName names a station for the player.
func radioName(st *music.Station) string {
	return st.ID + "\x00" + st.Name
}

// radioParts takes a radio's name apart.
func radioParts(name string) (id, label string) {
	parts := strings.Split(name, "\x00")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// radioLabel names a radio as the listener knows it.
func radioLabel(name string) string {
	_, label := radioParts(name)
	return label
}

func playable(tracks []music.Track) []music.Track {
	var out []music.Track
	for _, t := range tracks {
		if !t.Unavailable {
			out = append(out, t)
		}
	}
	return out
}

// radioFeed returns what gives the radio name more tracks: it loads them
// on a goroutine, and gives them to got on the main thread, none if it
// cannot.
func (a *app) radioFeed(name string) func(got func([]music.Track)) {
	return func(got func([]music.Track)) {
		id, _ := radioParts(name)
		lib := a.library
		if id == "" {
			got(nil)
			return
		}
		go func() {
			// A batch may hold only what the listener can't play: ask again
			// for those.
			var tracks []music.Track
			var err error
			for tries := 0; tries < 3 && len(tracks) == 0; tries++ {
				var more []music.Track
				more, err = lib.src.StationNext(id)
				if err != nil {
					break
				}
				tracks = playable(more)
			}
			a.post(func() {
				if err != nil {
					a.problem(err, T("The station didn't answer"))
				}
				got(tracks)
			})
		}()
	}
}

// playStation plays a station, or goes on playing it.
func (a *app) playStation(st *music.Station) {
	if a.library.signedOut() {
		a.promptSignIn()
		return
	}
	name := radioName(st)
	if a.pl.Radio() == name {
		if !a.pl.Playing() {
			a.pl.TogglePlay()
		}
		return
	}
	a.radioStarting = name
	feed := a.radioFeed(name)
	feed(func(tracks []music.Track) {
		if a.radioStarting != name {
			return
		}
		a.radioStarting = ""
		if len(tracks) == 0 {
			a.toast = T("This station can't play right now. Try again later.")
			return
		}
		a.pl.PlayRadio(name, tracks, feed)
	})
}

// exitRadio stops the radio playing, but not the song it plays.
func (a *app) exitRadio() {
	name := a.station()
	if name == "" {
		return
	}
	a.pl.EndRadio()
	a.toast = Tf("Left %s", radioLabel(name))
}

// stationCard is a station's cover over its name: a click plays it.
func (a *app) stationCard(c *ui.Context, st *music.Station) {
	if a.coverCardWith(c, st.Name, st.ImageURL, iconRadio, st.Description, func() { a.playStation(st) }) {
		a.playStation(st)
	}
}
