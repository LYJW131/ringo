package app

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/i18n"
	"ringo/internal/music"
)

// Recently Added is cut into runs headed by when each thing was added, as
// of now: today, yesterday, earlier this week, this month, the last three
// and six months, this year, and each year before. A week starts on Monday
// in Chinese, so the Sunday before is last week's there and this week's in
// English. A thing that doesn't say when stays with the one before it.
func TestAddedSections(t *testing.T) {
	defer i18n.SetLanguage(i18n.English)
	i18n.SetLanguage(i18n.English)
	loc := time.FixedZone("SGT", 8*3600)
	// A Wednesday.
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, loc)
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 23, 0, 0, 0, loc) }
	items := []music.Item{
		{ID: "today", Added: time.Date(2026, 11, 11, 0, 30, 0, 0, loc)},
		{ID: "yesterday", Added: day(2026, 11, 10)},
		{ID: "monday", Added: day(2026, 11, 9)},
		{ID: "sunday", Added: day(2026, 11, 8)},
		{ID: "undated"},
		{ID: "month", Added: day(2026, 11, 2)},
		{ID: "three", Added: day(2026, 9, 1)},
		{ID: "six", Added: day(2026, 6, 1)},
		{ID: "year", Added: day(2026, 2, 1)},
		{ID: "before", Added: day(2025, 12, 31)},
		{ID: "long", Added: day(2019, 5, 1)},
	}
	page := []music.Section{{Style: music.ShelfGrid, Items: items}}
	heads := func() (out []string) {
		for _, sec := range addedSections(page, now) {
			ids := ""
			for _, it := range sec.Items {
				ids += " " + it.ID
			}
			out = append(out, sec.Title+":"+ids)
		}
		return out
	}
	want := []string{
		"Today: today", "Yesterday: yesterday", "This Week: monday sunday undated", "This Month: month",
		"Last 3 Months: three", "Last 6 Months: six", "This Year: year", "2025: before", "2019: long",
	}
	if got := heads(); !slices.Equal(got, want) {
		t.Errorf("in English:\n got %q\nwant %q", got, want)
	}
	i18n.SetLanguage(i18n.Chinese)
	if got := heads(); len(got) < 4 || got[2] != "本周: monday" || got[3] != "本月: sunday undated month" {
		t.Errorf("in Chinese, where a week starts on Monday: %q", got)
	}

	// A page whose things don't say when is left as it is.
	plain := []music.Section{{Style: music.ShelfGrid, Items: []music.Item{{ID: "a"}, {ID: "b"}}}}
	if got := addedSections(plain, now); len(got) != 1 || got[0].Title != "" || len(got[0].Items) != 2 {
		t.Errorf("undated: %+v", got)
	}
}

// With MM_FRAMES set, saves Recently Added with things added today, this
// week, this month and before, in Chinese and the dark.
func TestRecentlyAddedFrames(t *testing.T) {
	dir := framesDir(t)
	defer i18n.SetLanguage(i18n.English)
	i18n.SetLanguage(i18n.Chinese)
	a := newTestApp()
	a.account = &music.Account{ID: "1", Name: "Alex"}
	shelfArt(a)
	fake(a).browse = func(string) (*music.Browse, error) {
		now := time.Now()
		var items []music.Item
		for i, ago := range []int{0, 0, 1, 1, 1, 9, 10, 12, 40, 50, 60, 70, 80, 200, 400} {
			items = append(items, music.Item{Kind: music.ItemAlbum, ID: "a" + string(rune('a'+i)), Title: "Album " + string(rune('A'+i)), Subtitle: "Artist",
				Added: now.AddDate(0, 0, -ago)})
		}
		return &music.Browse{Title: "最近添加", Sections: []music.Section{{Style: music.ShelfGrid, Items: items}}}, nil
	}
	ch := posts(a)
	a.open(route{page: pageBrowse, id: music.PageRecentlyAdded})
	tt := ui.NewTester(a.view, 1180, 1000)
	tt.SetDark(true)
	tt.Frame()
	runPost(t, ch)
	for len(ch) > 0 {
		runPost(t, ch)
	}
	settled(tt)
	settled(tt)
	saveFrame(t, dir, "recently-added-dark.png", tt.Image())
}
