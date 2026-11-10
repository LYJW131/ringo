package applemusic

import "ringo/internal/music"

// Stations go on with what Apple answers next. Live radio has no next
// tracks: it plays as it's on the air.

func (s *Source) stationNext(id string) ([]music.Track, error) {
	b, err := s.me("/stations/next-tracks/"+id, params{"limit": "10"}, reqOpts{method: "POST"})
	if err != nil {
		return nil, err
	}
	songs := s.songsOf(b.Data)
	for i := range songs {
		songs[i].Origin = music.Origin{Kind: music.FromStation, ID: id}
	}
	s.stationSongs(id, songs)
	return songs, nil
}
