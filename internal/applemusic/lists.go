package applemusic

import (
	"net/url"
	"strings"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// The catalog's pages made of lists: the charts, a search and what Search
// shows before anything is typed. Each list comes with its name in the
// language asked for, and its next part as Apple's own address, which the
// page that goes on with it carries (Browse.Next), as it is.

// group is a list of an answer that has several: a chart, or what a search
// found of one kind.
type group struct {
	Chart   string      `json:"chart"`
	ChartID flexString  `json:"chartId"`
	Name    string      `json:"name"`
	Href    string      `json:"href"`
	Next    string      `json:"next"`
	Data    []*resource `json:"data"`
}

// groups is an answer of lists: the charts' (several to a key) or a
// search's (one).
type groups[G any] struct {
	Results   map[string]G                    `json:"results"`
	Resources map[string]map[string]*resource `json:"resources"`
	Meta      *struct {
		Results struct {
			Order []string `json:"order"`
		} `json:"results"`
	} `json:"meta"`
}

func (g *groups[G]) body() *body { return &body{Resources: g.Resources} }

func (g *groups[G]) order() []string {
	if g.Meta == nil {
		return nil
	}
	return g.Meta.Results.Order
}

// chartGroups are the charts in the order the web player shows them when
// Apple doesn't say.
var chartGroups = []string{"songs", "cityCharts", "dailyGlobalTopCharts", "playlists", "albums", "music-videos"}

func chartParams() params {
	return params{
		"types":                 "albums,songs,music-videos,playlists",
		"limit":                 "50",
		"include[songs]":        "artists",
		"relate[songs]":         "albums",
		"include[albums]":       "artists",
		"include[music-videos]": "artists",
		"with":                  "cityCharts,dailyGlobalTopCharts",
		"extend":                "artistUrl",
		"art[url]":              "f",
		"platform":              "web",
		"format[resources]":     "map",
	}
}

// charts is the charts of all music, or of a genre: a section a chart, in
// Apple's order. A city's and the world's daily charts are playlists.
func (s *Source) charts(genre string) (*music.Browse, error) {
	p := chartParams()
	if genre != "" {
		p["genre"] = genre
	}
	var g groups[[]*group]
	if err := s.catalogInto("/charts", p, &g); err != nil {
		return nil, err
	}
	b := g.body()
	type entry struct {
		key string
		c   *group
	}
	var entries []entry
	seen := map[*group]bool{}
	add := func(key string, c *group) {
		if c != nil && !seen[c] {
			seen[c] = true
			entries = append(entries, entry{key, c})
		}
	}
	// "most-played:songs" is the chart most-played of the songs.
	for _, o := range g.order() {
		chart, key, ok := strings.Cut(o, ":")
		if !ok {
			continue
		}
		for _, c := range g.Results[key] {
			if c.Chart == chart {
				add(key, c)
			}
		}
	}
	for _, key := range chartGroups {
		for _, c := range g.Results[key] {
			add(key, c)
		}
	}
	out := &music.Browse{Title: i18n.T("Charts")}
	for _, e := range entries {
		items := s.items(b, e.c.Data)
		style := shelfStyle(items)
		if e.key == "songs" {
			style = music.ShelfTracks
			s.listed(songItems(items), nil)
		}
		sec := music.Section{Title: e.c.Name, Style: style, Items: items}
		if all := chartFrom(e.c); all != "" {
			sec.MorePage = "chart|" + all
		}
		if len(items) > 0 {
			out.Sections = append(out.Sections, sec)
		}
	}
	return out, nil
}

// chartFrom is where all of a chart is, from its start: its next part's
// address without the offset, which is the chart itself.
func chartFrom(c *group) string {
	next := orT(c.Next, c.Href)
	if next == "" {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Del("offset")
	u.RawQuery = q.Encode()
	return u.RequestURI()
}

// chart is a chart whole, or its next part: path is Apple's.
func (s *Source) chart(path string) (*music.Browse, error) {
	base := s.mapped(chartParams())
	delete(base, "types")
	delete(base, "with")
	var g groups[[]*group]
	if err := s.requestJSON(s.amp+continued(path, base), reqOpts{}, &g); err != nil {
		return nil, err
	}
	out := &music.Browse{}
	for _, list := range g.Results {
		for _, c := range list {
			if len(c.Data) == 0 || len(out.Sections) > 0 {
				continue
			}
			items := s.items(g.body(), c.Data)
			out.Title = c.Name
			out.Sections = []music.Section{{Style: gridStyle(items), Items: items}}
			if c.Next != "" {
				out.Next = "chart|" + c.Next
			}
			s.listed(songItems(items), nil)
		}
	}
	return out, nil
}

// The kinds a search looks for, and how the web player asks for them.
func searchParams(term string) params {
	return params{
		"term":                  term,
		"limit":                 "21",
		"types":                 "activities,albums,apple-curators,artists,curators,editorial-items,music-movies,music-videos,playlists,record-labels,songs,stations,tv-episodes,uploaded-videos",
		"extend":                "artistUrl",
		"with":                  "serverBubbles",
		"art[url]":              "f",
		"art[music-videos:url]": "c",
		"relate[albums]":        "artists",
		"relate[songs]":         "albums",
		"include[albums]":       "artists",
		"include[music-videos]": "artists",
		"include[stations]":     "radio-show",
		"include[songs]":        "artists",
		"fields[artists]":       "url,name,artwork",
		"omit[resource]":        "autos",
		"platform":              "web",
		"format[resources]":     "map",
	}
}

// searchGroups are a search's in the order the web player shows them when
// Apple doesn't say.
var searchGroups = []string{"top", "artist", "album", "song", "playlist", "radio_episode", "station", "music_video", "video_extra", "category"}

// searchStyle is how a search's group shows.
var searchStyle = map[string]string{
	"top":           music.ShelfSquare,
	"artist":        music.ShelfCircle,
	"album":         music.ShelfSquare,
	"playlist":      music.ShelfSquare,
	"song":          music.ShelfTracks,
	"music_video":   music.ShelfBrick,
	"video_extra":   music.ShelfBrick,
	"station":       music.ShelfSquare,
	"radio_episode": music.ShelfSquare,
	"category":      music.ShelfBrick,
}

// searchPage is all a search finds: the top results, of every kind, then a
// section a kind, each with the rest a page of its own.
func (s *Source) searchPage(term string) (*music.Browse, error) {
	var g groups[*group]
	if err := s.catalogInto("/search", searchParams(term), &g); err != nil {
		return nil, err
	}
	order := g.order()
	if len(order) == 0 {
		order = searchGroups
	}
	out := &music.Browse{Title: term}
	for _, key := range order {
		c := g.Results[key]
		if c == nil {
			continue
		}
		items := s.items(g.body(), c.Data)
		if len(items) == 0 {
			continue
		}
		style := searchStyle[key]
		if style == "" {
			style = shelfStyle(items)
		}
		if style == music.ShelfTracks {
			s.listed(songItems(items), nil)
		}
		sec := music.Section{Title: c.Name, Style: style, Items: items}
		if c.Next != "" {
			sec.MorePage = "search-more:" + key + ":" + term
		}
		out.Sections = append(out.Sections, sec)
	}
	return out, nil
}

// searchMore is all a search found of one kind (key, as Apple groups them),
// or, with next, its next part.
func (s *Source) searchMore(key, term, next string) (*music.Browse, error) {
	p := searchParams(term)
	var g groups[*group]
	var err error
	if next == "" {
		p["groups"] = key
		p["limit"] = "25"
		err = s.catalogInto("/search", p, &g)
	} else {
		err = s.requestJSON(s.amp+continued(next, s.withLanguage(p)), reqOpts{}, &g)
	}
	if err != nil {
		return nil, err
	}
	out := &music.Browse{Title: term}
	c := g.Results[key]
	if c == nil {
		return out, nil
	}
	items := s.items(g.body(), c.Data)
	out.Title = orT(c.Name, term)
	out.Sections = []music.Section{{Style: gridStyle(items), Items: items}}
	if c.Next != "" {
		out.Next = "search-more:" + key + ":" + term + "|" + c.Next
	}
	s.listed(songItems(items), nil)
	return out, nil
}

// recommendationStyle is how each of the shelves of recommendations shows,
// by display.kind; "" leaves the shelf out, as the web player does what it
// asks only to fill in with what the listener has.
var recommendationStyle = map[string]string{
	"MusicSuperHeroShelf":       music.ShelfPortrait,
	"MusicNotesHeroShelf":       music.ShelfPortrait,
	"MusicCoverShelf":           music.ShelfSquare,
	"MusicCardShelf":            music.ShelfSquare,
	"MusicHorizontalLockupGrid": music.ShelfSquare,
	"MusicCoverGrid":            music.ShelfGrid,
	"MusicCircleCoverShelf":     music.ShelfCircle,
	"MusicCircleCoverGrid":      music.ShelfCircle,
	"MusicNamedBrickShelf":      music.ShelfBrick,
	"MusicSocialCardShelf":      "",
	"MusicConcertsEmptyShelf":   "",
	"MusicUpsellShelf":          "",
}

// recommendations are the shelves of personal recommendations, Home's or
// Search's, in order.
func (s *Source) recommendations(b *body) []music.Section {
	var out []music.Section
	for _, rec := range b.all(b.Data) {
		a := rec.attrs()
		kind := ""
		if a.Display != nil {
			kind = a.Display.Kind
		}
		style, known := recommendationStyle[kind]
		if !known {
			unknown("recommendation shelf", kind)
		}
		if known && style == "" {
			continue
		}
		items := s.recommended(b, rec.related("contents").Data)
		if len(items) == 0 {
			continue
		}
		if !known || shelfStyle(items) == music.ShelfTracks {
			style = shelfStyle(items)
		}
		sec := music.Section{Title: string(a.Title), Style: style, Items: items}
		// "More like" a thing has it as its own: it shows its cover and
		// name, with what Apple says before the name over them.
		if a.TitleWithoutName != "" {
			if seeds := s.items(b, rec.related("primary-content").Data); len(seeds) > 0 {
				sec.Seed, sec.Kicker = &seeds[0], string(a.TitleWithoutName)
			}
		}
		if a.HasSeeAll {
			sec.MorePage = "rec:" + rec.ID
		}
		out = append(out, sec)
	}
	return out
}

// recommended is items of a recommendation's refs, each with the reason
// Apple gives for it, which sits on the ref and not on what it points at.
func (s *Source) recommended(b *body, refs []*resource) []music.Item {
	out := []music.Item{}
	for _, ref := range refs {
		it, ok := s.item(b, b.get(ref))
		if !ok {
			continue
		}
		if ref.Meta != nil {
			it.Badge = string(ref.Meta.Reason)
		}
		out = append(out, it)
	}
	return out
}

// searchLanding is what Search shows before a word is typed: the
// categories to browse.
func (s *Source) searchLanding() (*music.Browse, error) {
	sf, err := s.storefront()
	if err != nil {
		return nil, err
	}
	b, err := s.api("/v1/recommendations/"+sf, s.mapped(params{
		"name":           "search-landing",
		"extend":         "editorialArtwork",
		"art[url]":       "c,f",
		"types":          "activities,apple-curators,editorial-items",
		"omit[resource]": "autos",
	}), reqOpts{})
	if err != nil {
		return nil, err
	}
	return &music.Browse{Title: i18n.T("Search"), Sections: s.recommendations(b)}, nil
}
