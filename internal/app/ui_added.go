package app

import (
	"fmt"
	"strconv"
	"time"

	"ringo/internal/i18n"
	"ringo/internal/music"
)

// Recently Added is headed by when things came into the library, as Music's
// is on the Mac: Today, Yesterday, This Week, This Month, Last 3 Months,
// Last 6 Months, This Year, and then each year before. The words are
// Music's own. Without them the page read as Albums does, sorted by when
// each was added.

// addedHeading is the heading over what was added at t, as of now.
func addedHeading(t, now time.Time) string {
	t = t.In(now.Location())
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	// A week starts on Monday where the app speaks Chinese, and on Sunday
	// in English, as calendars there do.
	back := int(today.Weekday())
	if i18n.Language() == i18n.Chinese {
		back = (back + 6) % 7
	}
	switch {
	case !t.Before(today):
		return T("Today")
	case !t.Before(today.AddDate(0, 0, -1)):
		return T("Yesterday")
	case !t.Before(today.AddDate(0, 0, -back)):
		return T("This Week")
	case !t.Before(time.Date(y, m, 1, 0, 0, 0, 0, now.Location())):
		return T("This Month")
	case !t.Before(today.AddDate(0, -3, 0)):
		return T("Last 3 Months")
	case !t.Before(today.AddDate(0, -6, 0)):
		return T("Last 6 Months")
	case t.Year() == y:
		return T("This Year")
	}
	return fmt.Sprintf(TC("%s", "year"), strconv.Itoa(t.Year()))
}

// addedSections is a page of what was added, newest first, as sections
// headed by when: each a run of the page's items, which stay where they are.
// One that doesn't say when goes with the one before it, and a page none of
// whose items say is left as it is.
func addedSections(secs []music.Section, now time.Time) []music.Section {
	var out []music.Section
	for _, sec := range secs {
		if sec.Style != music.ShelfGrid || sec.Title != "" {
			out = append(out, sec)
			continue
		}
		from, heading, dated := 0, "", false
		cut := func(to int) {
			if to > from {
				part := sec
				part.Title, part.Items = heading, sec.Items[from:to:to]
				out = append(out, part)
			}
			from = to
		}
		for i := range sec.Items {
			at := sec.Items[i].Added
			if at.IsZero() {
				continue
			}
			dated = true
			if h := addedHeading(at, now); h != heading {
				cut(i)
				heading = h
			}
		}
		if !dated {
			out = append(out, sec)
			continue
		}
		cut(len(sec.Items))
	}
	return out
}
