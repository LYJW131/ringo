package lyrics

import (
	"regexp"
	"sort"
	"strconv"
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

var lrcTime = regexp.MustCompile(`\[(\d+):(\d+)(?:[.:](\d+))?\]`)

// parseLRC reads lyrics in the LRC format: a line may carry several times,
// and credits come as lines of their own.
func parseLRC(lrc string) []Line {
	var out []Line
	for _, raw := range strings.Split(lrc, "\n") {
		times := lrcTime.FindAllStringSubmatchIndex(raw, -1)
		if len(times) == 0 {
			continue
		}
		text := strings.TrimSpace(raw[times[len(times)-1][1]:])
		for _, m := range times {
			min, _ := strconv.Atoi(raw[m[2]:m[3]])
			sec, _ := strconv.Atoi(raw[m[4]:m[5]])
			var frac time.Duration
			if m[6] >= 0 {
				f := raw[m[6]:m[7]]
				n, _ := strconv.Atoi(f)
				frac = time.Duration(n) * time.Second
				for range len(f) {
					frac /= 10
				}
			}
			at := time.Duration(min)*time.Minute + time.Duration(sec)*time.Second + frac
			out = append(out, Line{At: at, Text: text})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// FromSource makes the lines to show from what a source answers: its own timed
// lines where it has them, else its LRC.
func FromSource(l *music.Lyrics) []Line {
	if l == nil {
		return nil
	}
	if len(l.Lines) == 0 {
		return Build(l.LRC, l.Translation)
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

// Build makes the lines to show from timed lines and their translation, the
// words of each sharing its time.
func Build(lrc, translation string) []Line {
	var lines []Line
	for _, l := range parseLRC(lrc) {
		if l.Text != "" {
			lines = append(lines, l)
		}
	}
	for i := range lines {
		end := lines[i].At + 6*time.Second
		if i+1 < len(lines) {
			end = min(end, lines[i+1].At)
		}
		lines[i].End = end
		lines[i].Words = SpreadWords(lines[i].Text, lines[i].At, end)
	}
	// Translations go to the line nearest their time.
	for _, t := range parseLRC(translation) {
		if t.Text == "" {
			continue
		}
		best, bestGap := -1, 1500*time.Millisecond
		for i := range lines {
			gap := lines[i].At - t.At
			if gap < 0 {
				gap = -gap
			}
			if gap < bestGap {
				best, bestGap = i, gap
			}
		}
		if best >= 0 && lines[best].Translation == "" {
			lines[best].Translation = t.Text
		}
	}
	return lines
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

// A Quiet is a stretch of a song with nothing sung: its intro, an
// interlude, or its outro.
type Quiet struct {
	From, To time.Duration
}

// Quiets returns the stretches of a song length long, of at least least,
// in which none of lines is sung. Credits, as 作词 and 作曲, are not sung.
func Quiets(lines []Line, length, least time.Duration) []Quiet {
	var out []Quiet
	add := func(from, to time.Duration) {
		if to-from >= least {
			out = append(out, Quiet{from, to})
		}
	}
	at := time.Duration(0)
	sung := false
	for _, l := range lines {
		if IsCredit(l.Text) {
			continue
		}
		add(at, l.At)
		at, sung = max(at, l.End), true
	}
	if sung && length > 0 {
		add(at, length)
	}
	return out
}

// creditNames are what credit lines begin with.
var creditNames = []string{"作词", "作曲", "编曲", "制作", "监制", "混音", "录音", "母带", "和声", "吉他", "贝斯", "鼓", "键盘", "弦乐", "出品", "发行", "OP", "SP", "词", "曲", "Lyrics", "Music", "Composer", "Arranger", "Producer"}

// IsCredit reports whether a line names who made the song, as "作词 :
// 林夕", rather than being sung.
func IsCredit(text string) bool {
	i := strings.IndexAny(text, ":：")
	if i <= 0 || utf8.RuneCountInString(text[:i]) > 12 {
		return false
	}
	head := strings.TrimSpace(text[:i])
	for _, n := range creditNames {
		// A name of one character, as 词, is the whole of the head.
		if head == n || utf8.RuneCountInString(n) > 1 && strings.HasPrefix(head, n) {
			return true
		}
	}
	return false
}
