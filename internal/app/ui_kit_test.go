package app

import (
	"testing"
	"time"

	"mygo-music/internal/i18n"
	"mygo-music/internal/music"
)

// Durations read as minutes and seconds.
func TestClock(t *testing.T) {
	if s := clock(185 * time.Second); s != "3:05" {
		t.Error(s)
	}
}

// The foot of an album's page says when it came out and how long it plays,
// in the listener's language: a day as the language writes one, a year
// alone as it is, and hours only past one.
func TestAlbumFootText(t *testing.T) {
	defer i18n.SetLanguage(i18n.English)
	long := make([]music.Track, 24)
	for i := range long {
		long[i].Duration = 340000 // 136 minutes in all
	}
	short := []music.Track{{Duration: 150000}, {Duration: 150000}}
	for _, c := range []struct {
		language, day, year, long, short string
	}{
		{i18n.English, "January 1, 2010", "2010", "24 Songs, 2 hr 16 min", "2 Songs, 5 min"},
		{i18n.Chinese, "2010年1月1日", "2010", "24 首歌曲、2 小时 16 分钟", "2 首歌曲、5 分钟"},
	} {
		i18n.SetLanguage(c.language)
		if s := released("2010-01-01"); s != c.day {
			t.Errorf("%s: released %q, want %q", c.language, s, c.day)
		}
		if s := released("2010"); s != c.year {
			t.Errorf("%s: released %q, want %q", c.language, s, c.year)
		}
		if s := listLength(long); s != c.long {
			t.Errorf("%s: %q, want %q", c.language, s, c.long)
		}
		if s := listLength(short); s != c.short {
			t.Errorf("%s: %q, want %q", c.language, s, c.short)
		}
	}
}

// A playlist says when it changed as Apple Music does: today, yesterday, the
// day of the week within a week, last week, or else the day.
func TestPlaylistUpdated(t *testing.T) {
	now := time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC) // a Friday
	for _, c := range []struct{ modified, want string }{
		{"2026-11-13T01:00:00Z", "Updated Today"},
		{"2026-11-12T23:00:00Z", "Updated Yesterday"},
		{"2026-11-09T08:00:00Z", "Updated Monday"},
		{"2026-11-03T08:00:00Z", "Updated Last Week"},
		{"2026-03-01T08:00:00Z", "Updated March 1, 2026"},
		{"soon", ""},
	} {
		if s := updated(c.modified, now); s != c.want {
			t.Errorf("%s: %q, want %q", c.modified, s, c.want)
		}
	}
}
