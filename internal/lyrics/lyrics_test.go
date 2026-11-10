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
