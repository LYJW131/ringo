package lyrics

import (
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mygo-music/internal/music"
)

// A Line is a line of timed lyrics, with its translation if any, and
// when each of its words is sung.
type Line struct {
	At          time.Duration
	End         time.Duration
	Text        string
	Translation string
	Words       []Word
}

// A Word is a word of a line, or a character of Chinese or Japanese,
// with when it is sung.
type Word struct {
	Text       string
	Start, End time.Duration
}

// FromSource makes the lines to show from what a source answers: its timed
// lines, each word with its time or, where it has none, sharing the line's.
func FromSource(l *music.Lyrics) []Line {
	if l == nil {
		return nil
	}
	ms := func(n int64) time.Duration { return time.Duration(n) * time.Millisecond }
	out := make([]Line, 0, len(l.Lines))
	for _, sl := range l.Lines {
		line := Line{At: ms(sl.Start), End: ms(sl.End), Text: strings.TrimSpace(sl.Text), Translation: sl.Translation}
		for _, w := range sl.Words {
			line.Words = append(line.Words, Word{Text: w.Text, Start: ms(w.Start), End: ms(w.End)})
		}
		if line.Text == "" {
			for _, w := range sl.Words {
				line.Text += w.Text
			}
			line.Text = strings.TrimSpace(line.Text)
		}
		if line.Text == "" {
			continue
		}
		out = append(out, line)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	// A line without an end lasts until the next; one without words shares
	// its time among them.
	for i := range out {
		l := &out[i]
		if l.End <= l.At {
			l.End = l.At + 6*time.Second
			if i+1 < len(out) {
				l.End = min(l.End, out[i+1].At)
			}
		}
		if len(l.Words) == 0 {
			l.Words = SpreadWords(l.Text, l.At, l.End)
		}
	}
	return out
}

// SpreadWords splits text into words, each Chinese, Japanese or Korean
// character one of them, and shares the time from start to end among them
// by their lengths, for lines whose words have no times of their own.
func SpreadWords(text string, start, end time.Duration) []Word {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			if cur.Len() > 0 {
				cur.WriteRune(' ')
				flush()
			}
		case isCJK(r):
			flush()
			words = append(words, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	if len(words) == 0 {
		return nil
	}
	total := 0
	for _, w := range words {
		total += utf8.RuneCountInString(strings.TrimSpace(w))
	}
	// Singing takes most of the line, not the pause before the next.
	span := time.Duration(float64(end-start) * 0.85)
	out := make([]Word, len(words))
	at := start
	for i, w := range words {
		d := span * time.Duration(utf8.RuneCountInString(strings.TrimSpace(w))) / time.Duration(max(total, 1))
		out[i] = Word{Text: w, Start: at, End: at + d}
		at += d
	}
	return out
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// Lang answers "ja" for lyrics in Japanese, any of whose lines has kana,
// and "" for any others, which take the app's language. Their kanji are
// then drawn in a Japanese font along with their kana: in a Chinese app
// they took a Chinese one, heavier and larger than the Japanese font the
// kana fell back to.
func Lang(lines []Line) string {
	for _, l := range lines {
		for _, r := range l.Text {
			if unicode.In(r, unicode.Hiragana, unicode.Katakana) && r != 'ー' && r != '・' {
				return "ja"
			}
		}
	}
	return ""
}

// Still reports whether lines have no times: lyrics a source has as words
// alone, every line at the song's start. There is no line of them being
// sung, and none to follow.
func Still(lines []Line) bool {
	return len(lines) > 1 && lines[len(lines)-1].At == 0
}

// Index returns the line sung at pos, or -1 before the first.
func Index(lines []Line, pos time.Duration) int {
	i := sort.Search(len(lines), func(i int) bool { return lines[i].At > pos })
	return i - 1
}
