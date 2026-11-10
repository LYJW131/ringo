package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/lyrics"
	"ringo/internal/music"
)

// lyricsApp is an app with Now Playing open on the lyrics of a song of 30
// lines, 第00句 to 第29句, one every 5 seconds and sung for 4, held at pos
// and come to rest there.
func lyricsApp(pos time.Duration) (*app, *ui.Tester) {
	var lines []lyrics.Line
	for i := range 30 {
		at := time.Duration(i) * 5 * time.Second
		lines = append(lines, lyrics.Line{At: at, End: at + 4*time.Second, Text: fmt.Sprintf("第%02d句", i)})
	}
	a := newTestApp()
	a.pl.Cue([]music.Track{song("1", "改变", "与非门")})
	a.pl.SetLyrics(lines)
	a.pl.Hold(pos)
	a.nowPlaying, a.npPane = true, paneLyrics
	tt := ui.NewTester(a.view, 1200, 800)
	settled(tt)
	settled(tt)
	return a, tt
}

// The lyrics are on the line sung, and on a break's dots from a moment
// after a line ends until the next begins, where the two are far enough
// apart, and before the first line of a song that starts late.
func TestLyricFocus(t *testing.T) {
	s := time.Second
	lines := []lyrics.Line{
		{At: 10 * s, End: 14 * s, Text: "一"},
		{At: 15 * s, End: 19 * s, Text: "二"},
		{At: 30 * s, End: 34 * s, Text: "三"},
	}
	for _, c := range []struct {
		pos  time.Duration
		want lyricFocus
		next time.Duration
	}{
		{2 * s, lyricFocus{line: -1, rest: true}, 8 * s},
		{12 * s, lyricFocus{line: 0}, 3 * s},
		{14500 * time.Millisecond, lyricFocus{line: 0}, 500 * time.Millisecond}, // a second apart: no break
		{19 * s, lyricFocus{line: 1}, breakLead},
		{19*s + breakLead, lyricFocus{line: 1, rest: true}, 11*s - breakLead},
		{31 * s, lyricFocus{line: 2}, 0},
	} {
		if got, next := focusAt(lines, c.pos); got != c.want || next != c.next {
			t.Errorf("at %v: on %+v for %v, want %+v for %v", c.pos, got, next, c.want, c.next)
		}
	}
	// A song that starts at once has no dots before its first line.
	soon := []lyrics.Line{{At: 3 * s, End: 6 * s, Text: "一"}, {At: 7 * s, End: 9 * s, Text: "二"}}
	if got, next := focusAt(soon, s); got != (lyricFocus{line: -1}) || next != 2*s {
		t.Errorf("before a first line at 3 s: on %+v for %v", got, next)
	}
}

// A line set in type has each of its words where the text has it, in
// order, though the words leave out a space the text has, as Japanese
// lines shared out by character do; and a note counts as long only where
// the words' times are the source's.
func TestLyricWordsFound(t *testing.T) {
	text := "夜明けの 駅で"
	words := lyrics.SpreadWords(text, 0, 6*time.Second)
	font := ui.Font{Size: 30, Weight: 800, Lang: "ja"}
	set := setLyric(text, words, false, font, 0, 600, ui.Start)
	if len(set.words) != len(words) {
		t.Fatalf("%d words set, of %d", len(set.words), len(words))
	}
	for k := 1; k < len(set.words); k++ {
		if w, before := set.words[k], set.words[k-1]; w.left < before.left+10 || w.left != before.right {
			t.Errorf("word %d reaches from %v, the one before it from %v to %v", k, w.left, before.left, before.right)
		}
		if set.words[k].long {
			t.Errorf("word %d of a line with no times of its words is a long note", k)
		}
	}
	held := []lyrics.Word{{Text: "あ", Start: 0, End: 300 * time.Millisecond}, {Text: "あ", Start: 300 * time.Millisecond, End: 2 * time.Second}}
	if set := setLyric("", held, true, font, 0, 600, ui.Start); len(set.words) != 2 || set.words[0].long || !set.words[1].long {
		t.Errorf("a short note and a long one: %+v", set.words)
	}
	// Words that aren't the text's are shown as they are.
	if set := setLyric("Another line", held, true, font, 0, 600, ui.Start); len(set.words) != 2 {
		t.Errorf("words of another text: %+v", set.words)
	}
}

// As a line gives way to the next, the lines go to their places one after
// another, the line sung first: a moment in, it is nearer its place than
// the third under it, which hasn't set off; and they all come to rest.
func TestLyricsFollowAsAWave(t *testing.T) {
	a, tt := lyricsApp(52 * time.Second) // 第10句
	a.pl.Seek(55100 * time.Millisecond)
	tt.Frame()
	tt.Advance(120 * time.Millisecond)
	held := func(i int) float64 {
		x, _ := a.lyricFlow.rows[i].value(tt.Now())
		return x
	}
	if a.lyricFlow.focus.line != 11 {
		t.Fatalf("the lyrics are on line %d, want 11", a.lyricFlow.focus.line)
	}
	if lead, third := held(11), held(14); lead <= 0 || third < lead+10 {
		t.Errorf("a moment in, the line sung is %v from its place and the third under it %v", lead, third)
	}
	tt.Advance(3 * time.Second)
	tt.Frame()
	for i := range a.lyricFlow.rows {
		if a.lyricFlow.rows[i].on {
			t.Fatalf("line %d is still on its way after three seconds", i)
		}
	}
	if lit, ok := tt.Find("第11句"); !ok || lit.Y < 150 || lit.Y > 400 {
		t.Errorf("the line sung came to rest at %v", lit.Y)
	}
}

// With less motion, the lines are at their places at once.
func TestLyricsFollowAtOnceWithLessMotion(t *testing.T) {
	a, tt := lyricsApp(52 * time.Second)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	before := a.lyricScroll.Y
	a.pl.Seek(55100 * time.Millisecond)
	tt.Frame()
	tt.Frame()
	if a.lyricScroll.Y <= before {
		t.Fatalf("the lyrics didn't follow: at %v, were at %v", a.lyricScroll.Y, before)
	}
	for i := range a.lyricFlow.rows {
		if a.lyricFlow.rows[i].on {
			t.Fatalf("line %d is held back from its place", i)
		}
	}
}

// Scrolled by the listener, the lyrics stay where they were put while the
// song goes on to its next line, and follow it again a moment after; a
// click on a line seeks to it, and they follow there at once.
func TestLyricsStayWhereScrolled(t *testing.T) {
	a, tt := lyricsApp(52 * time.Second)
	following := a.lyricScroll.Y
	wheel(tt, 900, 400, 160)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	put := a.lyricScroll.Y
	if put == following {
		t.Fatal("the wheel didn't scroll the lyrics")
	}
	a.pl.Seek(55100 * time.Millisecond)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if a.lyricScroll.Y != put {
		t.Errorf("a second after the listener scrolled them, the lyrics went from %v to %v", put, a.lyricScroll.Y)
	}
	tt.Advance(lyricBrowse)
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	if a.lyricFlow.focus.line != 11 || a.lyricScroll.Y <= following {
		t.Errorf("left alone, the lyrics are on line %d at %v, were at %v on line 10", a.lyricFlow.focus.line, a.lyricScroll.Y, following)
	}
	// Scrolled again, and a line clicked.
	wheel(tt, 900, 400, 60)
	if err := tt.Click("第13句"); err != nil {
		t.Fatal(err)
	}
	if pos := a.pl.Position(); pos != 65*time.Second {
		t.Fatalf("a click on the line at 1:05 seeks to %v", pos)
	}
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	if a.lyricFlow.focus.line != 13 {
		t.Errorf("after a click on line 13, the lyrics are on line %d", a.lyricFlow.focus.line)
	}
}

// The listener scrolling while the lines are on their way takes them from
// where they show: the offset isn't taken back to where they were going.
func TestLyricsScrolledOnTheirWay(t *testing.T) {
	a, tt := lyricsApp(52 * time.Second)
	a.pl.Seek(55100 * time.Millisecond)
	tt.Frame()
	tt.Advance(100 * time.Millisecond)
	going := a.lyricScroll.Y
	wheel(tt, 900, 400, -300)
	tt.Advance(time.Second)
	tt.Frame()
	if a.lyricScroll.Y >= going-100 {
		t.Errorf("scrolled up on their way to %v, the lyrics are at %v", going, a.lyricScroll.Y)
	}
}

// Now Playing opened on a line has it where the lyrics bring a line as the
// song gets to it. The pane is a little taller in its first frames than it
// comes to be, and the line stayed a third of the way down that, some 15
// DIPs too low, until the next line.
func TestLyricsOpenWhereTheyFollow(t *testing.T) {
	at := func(a *app, tt *ui.Tester) float32 {
		r, ok := tt.Find("第05句")
		if !ok {
			t.Fatalf("no line: %q", tt.Texts())
		}
		return r.Y
	}
	a, tt := lyricsApp(26 * time.Second)
	opened := at(a, tt)
	a, tt = lyricsApp(21 * time.Second)
	a.pl.Seek(26 * time.Second)
	settled(tt)
	settled(tt)
	if followed := at(a, tt); opened < followed-1 || opened > followed+1 {
		t.Errorf("opened on it, the line is at %.1f; followed to, at %.1f", opened, followed)
	}
}
