package applemusic

import (
	"strings"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// The editors' pages: New, Radio, a genre's, a See All, a feature of many
// rooms and a curator's, as the web player asks for them. A page is a tree of
// editorial elements; each element's editorialElementKind says what it is,
// which the web player's script names (api.md §4.1), and so how it shows.

// The kinds of editorial element, by the web player's names for them.
const (
	kindHeroList    = "316" // HeroListShelf: the heroes that lead the page
	kindHero        = "317" // Hero: one, of an album, playlist or station
	kindHeroCustom  = "320" // HeroCustom: a picture of its own, linking to a feature
	kindSwoosh      = "326" // CopperSwoosh: a row, with a room for See All
	kindTrackSwoosh = "327" // CopperTrackSwoosh: songs in columns
	kindRecentRadio = "332" // RecentlyPlayedRadioMarker: what the listener played lately, filled in by the player
	kindRoomSwoosh  = "345" // RoomSwoosh: a row in a feature
	kindTabRoot     = "382" // TabRoot: a page's sections
	kindAMOneHero   = "383" // AppleMusicOneHero: Radio's lead
	kindBrickSwoosh = "385" // BrickSwoosh, Brick and CustomBrick, by their names only
	kindBrick       = "386"
	kindCustomBrick = "394"
	kindLinkList    = "391" // LinkList: links a line each
	kindGenreLinks  = "322" // links to genres' pages
	kindParagraph   = "404" // Paragraph: a feature's text
	kindLiveShelf   = "488" // LiveRadioShelf, of LiveRadioShelfItem (489)
)

// sections are the page's, of the elements refs point at, in order.
func (s *Source) sections(b *body, refs []*resource) []music.Section {
	var out []music.Section
	for _, e := range b.all(refs) {
		out = append(out, s.elementSections(b, e)...)
	}
	return out
}

// elementSections is what one element adds to its page: a section, none, or,
// for a tab, all of its own.
func (s *Source) elementSections(b *body, e *resource) []music.Section {
	a := e.attrs()
	title := orT(a.Name, string(a.Title))
	switch k := a.EditorialElementKind; k {
	case kindTabRoot:
		return s.sections(b, e.related("children").Data)
	case kindHeroList:
		var items []music.Item
		for _, child := range b.all(e.related("children").Data) {
			if it, ok := s.hero(b, child); ok {
				items = append(items, it)
			}
		}
		return one(music.Section{Title: title, Style: music.ShelfHero, Items: items})
	case kindHero, kindHeroCustom, kindAMOneHero:
		if it, ok := s.hero(b, e); ok {
			return one(music.Section{Title: title, Style: music.ShelfHero, Items: []music.Item{it}})
		}
		return nil
	case kindSwoosh, kindRoomSwoosh, kindTrackSwoosh, kindBrickSwoosh, kindBrick, kindCustomBrick:
		items := s.items(b, e.related("contents").Data)
		style := shelfStyle(items)
		switch k {
		case kindTrackSwoosh:
			style = music.ShelfTracks
		case kindBrickSwoosh, kindBrick, kindCustomBrick:
			style = music.ShelfBrick
		}
		sec := music.Section{Title: title, Style: style, Items: items}
		if room := e.related("room").Data; len(room) > 0 && room[0] != nil {
			// The room is the element's own ID, not its collectionId.
			sec.MorePage = "room:" + room[0].ID
		}
		return one(sec)
	case kindLinkList, kindGenreLinks:
		var items []music.Item
		for _, l := range a.Links {
			items = append(items, music.Item{Kind: music.ItemLink, Title: l.Label, Page: linkPage(l.URL), URL: l.URL})
		}
		return one(music.Section{Title: title, Style: music.ShelfLinks, Items: items})
	case kindLiveShelf:
		items := s.items(b, e.related("contents").Data)
		for _, child := range b.all(e.related("children").Data) {
			items = append(items, s.items(b, child.related("contents").Data)...)
		}
		return one(music.Section{Title: title, Style: music.ShelfLive, Items: items})
	case kindParagraph:
		return one(music.Section{Title: title, Style: music.ShelfText, Subtitle: plain(a.Description.long())})
	case kindRecentRadio:
		return nil
	default:
		unknown("editorial element kind", k)
		return nil
	}
}

// one is sec as a page's sections, none when it has nothing to show.
func one(sec music.Section) []music.Section {
	if len(sec.Items) == 0 && sec.Subtitle == "" {
		return nil
	}
	return []music.Section{sec}
}

// hero is a hero element as the item it shows: what it's of, with the
// editors' badge and words, over a picture wider than tall.
func (s *Source) hero(b *body, e *resource) (music.Item, bool) {
	a := e.attrs()
	var it music.Item
	contents := b.all(e.related("contents").Data)
	ok := false
	if len(contents) > 0 {
		it, ok = s.item(b, contents[0])
	}
	if !ok {
		// A picture linking to a feature, of nothing to play.
		page := ""
		if a.Link != nil {
			page = linkPage(a.Link.URL)
			it.URL = a.Link.URL
		}
		if page == "" && it.URL == "" {
			return music.Item{}, false
		}
		it = music.Item{Kind: music.ItemLink, ID: e.ID, Title: orT(a.Name, a.DesignBadge), Page: page, URL: it.URL}
	}
	it.Badge = a.DesignBadge
	if a.DesignTag != "" {
		it.Note = a.DesignTag
	}
	// The element's own picture first: it's the one made for the hero.
	var w *artwork
	switch {
	case a.Artwork != nil && a.Artwork.URL != "":
		w = a.Artwork
	default:
		if _, w = wide(a, heroArt...); w == nil && len(contents) > 0 {
			_, w = wide(contents[0].attrs(), heroArt...)
		}
	}
	if w != nil {
		it.WideURL, it.Colors = w.URL, colors(w)
	}
	if len(contents) > 0 {
		if v := motion(contents[0].attrs(), wideMove...); v != "" {
			it.VideoURL = v
		}
	}
	return it, true
}

// The parameters of an editors' page, as the web player sends them.
func groupingParams() params {
	return params{
		"art[url]":               "f",
		"extend":                 "editorialArtwork,artistUrl,plainEditorialNotes",
		"omit[resource:artists]": "autos",
		"relate[songs]":          "albums",
		"include[albums]":        "artists",
		"include[songs]":         "artists",
		"include[music-videos]":  "artists",
		"include[stations]":      "events,radio-show",
		"include[groupings]":     "curator",
		"extend[station-events]": "editorialVideo",
		"fields[artists]":        "name,url,artwork,editorialArtwork,genreNames,plainEditorialNotes",
		"fields[albums]":         "artistName,artistUrl,artwork,contentRating,editorialArtwork,plainEditorialNotes,name,playParams,releaseDate,url,trackCount",
	}
}

func (s *Source) tabs() string {
	if s.subscriber() {
		return "subscriber"
	}
	return "nonsubscriber"
}

// groupingPage is New, Radio (by name) or a genre's (by id): the sections of
// its tab.
func (s *Source) groupingPage(name, id string) (*music.Browse, error) {
	p := groupingParams()
	p["tabs"] = s.tabs()
	path := "/groupings"
	if id != "" {
		path += "/" + id
	} else {
		p["name"] = name
	}
	b, err := s.editorial(path, p)
	if err != nil {
		return nil, err
	}
	g, err := first(b)
	if err != nil {
		return nil, err
	}
	if g = b.get(g); g == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	out := &music.Browse{Title: g.attrs().Name}
	for _, tab := range b.all(g.related("tabs").Data) {
		// A tab is a root; its children are the page.
		if tab.attrs().EditorialElementKind == kindTabRoot || len(tab.related("children").Data) > 0 {
			out.Sections = append(out.Sections, s.sections(b, tab.related("children").Data)...)
		} else {
			out.Sections = append(out.Sections, s.elementSections(b, tab)...)
		}
	}
	return out, nil
}

func roomParams() params {
	return params{
		"omit[resource:artists]": "autos",
		"relate[songs]":          "albums",
		"include[albums]":        "artists",
		"include[songs]":         "artists",
		"fields[albums]":         "artistName,artistUrl,artwork,contentRating,editorialArtwork,editorialNotes,name,playParams,releaseDate,url,trackCount",
		"extend[albums]":         "artistUrl",
		"art[url]":               "f",
	}
}

// room is a See All: all of a row, a page at a time.
func (s *Source) room(id string) (*music.Browse, error) {
	b, err := s.editorial("/rooms/"+id, roomParams())
	if err != nil {
		return nil, err
	}
	r, err := first(b)
	if err != nil {
		return nil, err
	}
	if r = b.get(r); r == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	contents := r.related("contents")
	items := s.items(b, contents.Data)
	out := &music.Browse{Title: string(r.attrs().Title), Sections: []music.Section{{Style: gridStyle(items), Items: items}}}
	if contents.Next != "" {
		out.Next = "room:" + id + "|" + contents.Next
	}
	s.listed(songItems(items), nil)
	return out, nil
}

// more is the next part of a list, at next, for the page prefix: a section
// that goes on from the last one's.
func (s *Source) more(prefix, next string, base params) (*music.Browse, error) {
	if !strings.HasPrefix(next, "/v1/me") {
		base = s.mapped(base)
	}
	b, err := s.follow(next, base)
	if err != nil {
		return nil, err
	}
	items := s.items(b, b.Data)
	out := &music.Browse{Sections: []music.Section{{Style: gridStyle(items), Items: items}}}
	if b.Next != "" {
		out.Next = prefix + "|" + b.Next
	}
	s.listed(songItems(items), nil)
	return out, nil
}

// multiroom is a feature: rooms and words under a picture of its own.
func (s *Source) multiroom(id string) (*music.Browse, error) {
	b, err := s.editorial("/multirooms/"+id, params{"extend": "editorialArtwork,uber,lockupStyle", "art[url]": "f"})
	if err != nil {
		return nil, err
	}
	m, err := first(b)
	if err != nil {
		return nil, err
	}
	if m = b.get(m); m == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	a := m.attrs()
	out := &music.Browse{Title: string(a.Title), Sections: s.sections(b, m.related("children").Data)}
	if u := a.Uber; u != nil && u.MasterArt != nil {
		out.WideURL, out.Colors = u.MasterArt.URL, colors(u.MasterArt)
		if out.Colors.Text1 == "" {
			out.Colors.Text1 = orT(u.TitleTextColor, u.PrimaryTextColor)
		}
	}
	return out, nil
}

// curator is a curator's or a show's page: the editors' page Apple keeps of
// it, if there's one, and else its playlists.
func (s *Source) curator(id string) (*music.Browse, error) {
	sf, err := s.storefront()
	if err != nil {
		return nil, err
	}
	b, err := s.api("/v1/catalog/"+sf, s.mapped(params{
		"ids[apple-curators]":    id,
		"include":                "grouping,playlists",
		"extend":                 "editorialArtwork",
		"extend[apple-curators]": "playlistCount",
		"art[url]":               "f",
		"omit[resource]":         "autos",
	}), reqOpts{})
	if err == nil && len(b.Data) == 0 {
		// Not one of Apple's own: a curator's.
		b, err = s.api("/v1/catalog/"+sf, s.mapped(params{"ids[curators]": id, "include": "grouping,playlists", "extend": "editorialArtwork", "art[url]": "f"}), reqOpts{})
	}
	if err != nil {
		return nil, err
	}
	c, err := first(b)
	if err != nil {
		return nil, err
	}
	if c = b.get(c); c == nil {
		return nil, fail("unavailable", i18n.T("Apple Music doesn't have this"))
	}
	a := c.attrs()
	out := &music.Browse{Title: a.Name, Description: plain(a.PlainEditorialNotes.brief())}
	if out.Description == "" {
		out.Description = plain(a.EditorialNotes.brief())
	}
	if u, w := wide(a, "bannerUber", "epicStageWide", "musicFullscreenStatic16x9", "subscriptionHero"); u != "" {
		out.WideURL, out.Colors = u, colors(w)
	} else {
		out.ImageURL = art(a.Artwork)
	}
	if g := c.related("grouping").Data; len(g) > 0 && g[0] != nil {
		page, err := s.groupingPage("", g[0].ID)
		if err == nil && len(page.Sections) > 0 {
			out.Sections = page.Sections
			return out, nil
		}
	}
	items := s.items(b, c.related("playlists").Data)
	if len(items) > 0 {
		out.Sections = []music.Section{{Title: i18n.T("Playlists"), Style: music.ShelfGrid, Items: items}}
	}
	return out, nil
}
