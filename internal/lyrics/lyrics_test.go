package lyrics

import (
	"testing"
	"time"

	"ringo/internal/music"
)

// Index finds the line being sung at a time, and none before the first.
func TestIndex(t *testing.T) {
	lines := FromSource(&music.Lyrics{Lines: []music.LyricLine{
		{Start: 1000, Text: "作词 : 某人"},
		{Start: 30540, Text: "故事的小黄花"},
		{Start: 34165, Text: "从出生那年就飘着"},
	}})
	if i := Index(lines, 33*time.Second); i != 1 {
		t.Errorf("index at 33s = %d", i)
	}
	if i := Index(lines, 0); i != -1 {
		t.Errorf("index at 0 = %d", i)
	}
}

func TestSpreadWords(t *testing.T) {
	w := SpreadWords("我爱 you all", 0, 10*time.Second)
	got := []string{}
	for _, x := range w {
		got = append(got, x.Text)
	}
	if len(w) != 4 || got[0] != "我" || got[1] != "爱" || got[2] != "you " || got[3] != "all" {
		t.Fatalf("words %q", got)
	}
	if w[0].Start != 0 || w[3].End > 10*time.Second || w[1].Start != w[0].End {
		t.Errorf("times %+v", w)
	}
}

// A source's own timed lines keep the times of their words, and a line given
// without words or an end still gets both.
func TestFromSourceLines(t *testing.T) {
	lines := FromSource(&music.Lyrics{Lines: []music.LyricLine{
		{Start: 13010, End: 17790, Translation: "Never", Words: []music.LyricWord{
			{Start: 13010, End: 13670, Text: "Hold "},
			{Start: 13670, End: 14340, Text: "on"},
		}},
		{Start: 20000, Text: "Plain line"},
		{Start: 30000, Text: "   "},
	}})
	if len(lines) != 2 {
		t.Fatalf("lines %+v", lines)
	}
	l := lines[0]
	if l.Text != "Hold on" || l.At != 13010*time.Millisecond || l.End != 17790*time.Millisecond || l.Translation != "Never" {
		t.Errorf("line %+v", l)
	}
	if len(l.Words) != 2 || l.Words[1].Start != 13670*time.Millisecond {
		t.Errorf("words %+v", l.Words)
	}
	if lines[1].End != 26*time.Second || len(lines[1].Words) != 2 {
		t.Errorf("plain line %+v", lines[1])
	}
}

func TestLang(t *testing.T) {
	for _, c := range []struct {
		lines []Line
		want  string
	}{
		{[]Line{{Text: "春日影"}, {Text: "悴んだ心 ふるえる眼差し"}}, "ja"},
		{[]Line{{Text: "カタカナだけ"}}, "ja"},
		{[]Line{{Text: "春日影"}, {Text: "我想要的只是"}}, ""},
		{[]Line{{Text: "Hello ・ world ー"}}, ""},
		{nil, ""},
	} {
		if got := Lang(c.lines); got != c.want {
			t.Errorf("Lang(%v) = %q, want %q", c.lines, got, c.want)
		}
	}
}

// Words the source timed are told from those that share their line's time,
// and the words sung behind a line come with it.
func TestFromSourceSung(t *testing.T) {
	lines := FromSource(&music.Lyrics{Lines: []music.LyricLine{
		{Start: 1000, End: 3000, Other: true,
			Words:      []music.LyricWord{{Start: 1000, End: 2000, Text: "Hello "}, {Start: 2000, End: 3000, Text: "world"}},
			Background: []music.LyricWord{{Start: 2500, End: 2900, Text: "yeah"}}},
		{Start: 4000, End: 6000, Text: "Line two"},
	}})
	if len(lines) != 2 {
		t.Fatalf("lines: %+v", lines)
	}
	if l := lines[0]; !l.Sung || !l.Other || len(l.Behind) != 1 || l.Behind[0].Start != 2500*time.Millisecond {
		t.Errorf("a line the source timed word by word: %+v", l)
	}
	if l := lines[1]; l.Sung || len(l.Words) != 2 {
		t.Errorf("a line with no words of its own: %+v", l)
	}
}

// How a line is said keeps the times the source gives its words, and where
// it gives none they share the line's, as the line's own do.
func TestFromSourcePronounced(t *testing.T) {
	lines := FromSource(&music.Lyrics{Lines: []music.LyricLine{
		{Start: 1000, End: 3000, Text: "夜明け", Pronunciation: "yoake", Pronounced: []music.LyricWord{
			{Start: 1000, End: 1400, Text: "yo"},
			{Start: 1400, End: 3000, Text: "ake"},
		}},
		{Start: 4000, End: 6000, Text: "白い息", Pronunciation: "shiroi iki"},
		{Start: 7000, End: 9000, Text: "Stay here"},
	}})
	if l := lines[0]; l.Pronunciation != "yoake" || len(l.Pronounced) != 2 || l.Pronounced[1].Start != 1400*time.Millisecond {
		t.Errorf("timed: %q %+v", l.Pronunciation, l.Pronounced)
	}
	if l := lines[1]; len(l.Pronounced) != 2 || l.Pronounced[0].Start != 4*time.Second || l.Pronounced[1].End > 6*time.Second {
		t.Errorf("not timed: %+v", l.Pronounced)
	}
	if l := lines[2]; l.Pronunciation != "" || len(l.Pronounced) != 0 {
		t.Errorf("none: %q %+v", l.Pronunciation, l.Pronounced)
	}
}
