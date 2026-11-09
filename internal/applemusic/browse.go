package applemusic

import (
	"strconv"
	"strings"
	"sync"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Home, and the source's own pages: the charts and what was added lately.

type chart struct {
	Name string      `json:"name"`
	Data []*resource `json:"data"`
}

type charts struct{ songs, albums, playlists *chart }

func (s *Source) charts(limit int) (charts, error) {
	var b struct {
		Results struct {
			Songs     []*chart `json:"songs"`
			Albums    []*chart `json:"albums"`
			Playlists []*chart `json:"playlists"`
		} `json:"results"`
	}
	if err := s.catalogInto("/charts", params{"types": "songs,albums,playlists", "limit": strconv.Itoa(limit)}, &b); err != nil {
		return charts{}, err
	}
	pick := func(c []*chart) *chart {
		if len(c) > 0 {
			return c[0]
		}
		return nil
	}
	return charts{pick(b.Results.Songs), pick(b.Results.Albums), pick(b.Results.Playlists)}, nil
}

func orT(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}

func (s *Source) home() (*music.Browse, error) {
	s.warm()
	var (
		wg              sync.WaitGroup
		chart           charts
		recommendations *body
		recent          []music.Track
	)
	signedIn := s.signedIn()
	wg.Add(1)
	go func() {
		defer wg.Done()
		chart, _ = s.charts(30)
	}()
	if signedIn {
		wg.Add(2)
		go func() {
			defer wg.Done()
			recommendations, _ = s.me("/recommendations", params{"limit": "12"}, reqOpts{})
		}()
		go func() {
			defer wg.Done()
			recent, _ = s.recentTracks()
		}()
	}
	wg.Wait()
	var sections []music.Section
	if chart.songs != nil {
		sections = append(sections, music.Section{Title: orT(chart.songs.Name, i18n.T("Top Songs")), Tracks: s.songsOf(chart.songs.Data), MorePage: "charts"})
	}
	if len(recent) > 0 {
		sections = append(sections, music.Section{Title: i18n.T("Recently Played"), Tracks: recent[:min(30, len(recent))]})
	}
	// The app plays a section's songs from the one picked on. It loads home
	// again by itself, every hour, songs playing or not: what's on it is
	// noted, and worked out only as it plays, so that it doesn't wake the
	// wrapper.
	for _, sec := range sections {
		s.shown(sec.Tracks)
	}
	if recommendations != nil {
		for _, rec := range recommendations.Data {
			if rec == nil {
				continue
			}
			var albums []music.Album
			var playlists []music.Playlist
			var stations []music.Station
			for _, i := range rec.related("contents").Data {
				if i == nil {
					continue
				}
				switch {
				case i.Type == "albums":
					albums = append(albums, toAlbum(i))
				case i.Type == "playlists":
					playlists = append(playlists, s.toPlaylist(i))
				case i.Type == "stations" && !i.attrs().IsLive:
					stations = append(stations, toStation(i))
				}
			}
			if len(albums) == 0 && len(playlists) == 0 && len(stations) == 0 {
				continue
			}
			title := ""
			if t := rec.attrs().Title; t != nil {
				title = t.StringForDisplay
			}
			sections = append(sections, music.Section{Title: title, Albums: albums, Playlists: playlists, Stations: stations})
		}
	}
	if chart.albums != nil {
		sections = append(sections, music.Section{Title: orT(chart.albums.Name, i18n.T("Top Albums")), Albums: toAlbums(chart.albums.Data), MorePage: "charts"})
	}
	if chart.playlists != nil {
		sections = append(sections, music.Section{Title: orT(chart.playlists.Name, i18n.T("Top Playlists")), Playlists: s.toPlaylists(chart.playlists.Data)})
	}
	return &music.Browse{Title: "Apple Music", Sections: sections}, nil
}

func (s *Source) browse(page string) (*music.Browse, error) {
	s.warm()
	switch page {
	case "charts":
		chart, err := s.charts(100)
		if err != nil {
			return nil, err
		}
		var sections []music.Section
		if chart.songs != nil {
			songs := s.songsOf(chart.songs.Data)
			s.listed(songs, nil)
			sections = append(sections, music.Section{Title: orT(chart.songs.Name, i18n.T("Top Songs")), Tracks: songs})
		}
		if chart.albums != nil {
			sections = append(sections, music.Section{Title: orT(chart.albums.Name, i18n.T("Top Albums")), Albums: toAlbums(chart.albums.Data)})
		}
		if chart.playlists != nil {
			sections = append(sections, music.Section{Title: orT(chart.playlists.Name, i18n.T("Top Playlists")), Playlists: s.toPlaylists(chart.playlists.Data)})
		}
		sf, err := s.storefront()
		if err != nil {
			return nil, err
		}
		return &music.Browse{Title: i18n.T("Charts"), Subtitle: strings.ToUpper(sf), Sections: sections}, nil
	case "recentlyAdded":
		return s.recentlyAdded(false)
	}
	return nil, fail("unavailable", i18n.T("No such page"))
}

// recentlyAdded is what the listener added lately, kept a while.
func (s *Source) recentlyAdded(ahead bool) (*music.Browse, error) {
	return kept(s, "recentlyAdded", freshness{maxAge: listKept}, func(pause func()) (*music.Browse, error) {
		items, err := s.allPages("/v1/me/library/recently-added", params{"limit": "25"}, 100, pause)
		if err != nil {
			return nil, err
		}
		var albums, playlists []*resource
		for _, i := range items {
			switch {
			case i == nil:
			case strings.HasSuffix(i.Type, "albums"):
				albums = append(albums, i)
			case strings.HasSuffix(i.Type, "playlists"):
				playlists = append(playlists, i)
			}
		}
		return &music.Browse{
			Title: i18n.T("Recently Added"),
			Sections: []music.Section{
				{Title: i18n.T("Albums"), Albums: toAlbums(albums), Empty: i18n.T("Nothing added lately")},
				{Title: i18n.T("Playlists"), Playlists: s.toPlaylists(playlists)},
			},
		}, nil
	}, ahead)
}
