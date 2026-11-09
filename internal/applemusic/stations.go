package applemusic

import (
	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Stations: the listener's own, and those they played lately. Live radio
// isn't one: it has no next tracks to play.

func (s *Source) stations(ahead bool) ([]music.Station, error) {
	if !s.signedIn() {
		return []music.Station{}, nil
	}
	return kept(s, "stations", freshness{maxAge: listKept}, s.fetchStations, ahead)
}

func (s *Source) fetchStations(pause func()) ([]music.Station, error) {
	out := []music.Station{}
	if own, err := s.catalog("/stations", params{"filter[identity]": "personal"}); err == nil {
		for _, st := range own.Data {
			if st == nil {
				continue
			}
			station := toStation(st)
			if station.Name == "" {
				station.Name = i18n.T("Your Station")
			}
			out = append(out, station)
		}
	}
	pause()
	if recent, err := s.me("/recent/radio-stations", params{"limit": "10"}, reqOpts{}); err == nil {
	next:
		for _, st := range recent.Data {
			if st == nil || st.Type != "stations" || st.attrs().IsLive {
				continue
			}
			for _, o := range out {
				if o.ID == st.ID {
					continue next
				}
			}
			out = append(out, toStation(st))
		}
	}
	return out, nil
}

func (s *Source) stationNext(id string) ([]music.Track, error) {
	b, err := s.me("/stations/next-tracks/"+id, params{"limit": "10"}, reqOpts{method: "POST"})
	if err != nil {
		return nil, err
	}
	songs := s.songsOf(b.Data)
	s.stationSongs(id, songs)
	return songs, nil
}
