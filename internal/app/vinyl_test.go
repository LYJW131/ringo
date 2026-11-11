package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"ringo/internal/lyrics"
	"ringo/internal/music"
	"ringo/internal/player"
)

// vinylSong is a song of verses and a chorus sung twice, a line every four
// seconds, each word with its time, the chorus's last held long.
func vinylSong() []lyrics.Line {
	verses := [][]string{
		{"I left the porch light on for you", "and counted every car that passed", "the kettle sang itself to sleep"},
		{"the moths have made the lamp their own", "your coat still hangs behind the door", "I wound the clock and let it run"},
	}
	chorus := []string{"come home come home", "the night is long and I am here"}
	var lines []music.LyricLine
	at := int64(8000)
	sing := func(text string, hold bool) {
		l := music.LyricLine{Start: at}
		words := strings.Fields(text)
		for i, w := range words {
			start := at + int64(i)*300
			word := music.LyricWord{Start: start, End: start + 280, Text: w + " "}
			if hold && i == len(words)-1 {
				word.End = start + 2200
			}
			l.Words = append(l.Words, word)
		}
		l.End = l.Words[len(l.Words)-1].End
		lines = append(lines, l)
		at += 4000
	}
	for _, verse := range verses {
		for _, v := range verse {
			sing(v, false)
		}
		at += 3000 // a breath before the chorus
		for i, c := range chorus {
			sing(c, i == len(chorus)-1)
		}
		at += 30000 // and a long break after it
	}
	return lyrics.FromSource(&music.Lyrics{Lines: lines})
}

// vinylSet sets lines for a stage whose spokes take wide of the deck's
// DIPs, each character 15 of them.
func vinylSetOf(lines []lyrics.Line, wide float32) *vinylSet {
	return setVinyl(lines, vinylType, wide,
		func(text string, size float32) float32 { return float32(utf8.RuneCountInString(text)) * size * 0.6 })
}

// The wheel has the line sung level, and turns to the next from a little
// before its first word, most of the way within a third of a second.
func TestVinylWheelTurnsToTheLineSung(t *testing.T) {
	lines := vinylSong()
	set := vinylSetOf(lines, 600)
	if active, at := set.wheelAt(2 * time.Second); active != -1 || at != 0 {
		t.Errorf("before the first line: spoke %d, the wheel at %v", active, at)
	}
	if active, at := set.wheelAt(lines[1].At + 3*time.Second); active != 1 || at < 0.999 || at > 1.001 {
		t.Errorf("three seconds into the second line: spoke %d, the wheel at %v", active, at)
	}
	turn := lines[2].At - vinylLead
	if active, at := set.wheelAt(turn - time.Millisecond); active != 1 || at > 1.001 {
		t.Errorf("just before the third line's turn: spoke %d, the wheel at %v", active, at)
	}
	if _, at := set.wheelAt(turn + 100*time.Millisecond); at < 1.5 || at > 1.75 {
		t.Errorf("a tenth of a second into the turn: the wheel at %v, want it past halfway", at)
	}
	if active, at := set.wheelAt(turn + 330*time.Millisecond); active != 2 || at < 1.95 {
		t.Errorf("a third of a second into the turn: spoke %d, the wheel at %v", active, at)
	}
}

// A line too long for a spoke goes on to the next, each part within the
// spoke's width, and the wheel turns to a part as its first word is sung.
func TestVinylLongLineTakesMoreSpokes(t *testing.T) {
	lines := vinylSong()
	whole := vinylSetOf(lines, 2000)
	if len(whole.lines) != len(lines) {
		t.Fatalf("with room: %d spokes for %d lines", len(whole.lines), len(lines))
	}
	set := vinylSetOf(lines, 240)
	if len(set.lines) <= len(lines) {
		t.Fatalf("without room: %d spokes for %d lines", len(set.lines), len(lines))
	}
	for i, l := range set.lines {
		last := l.toks[len(l.toks)-1]
		if !last.rest && last.x+last.w > 240 {
			t.Errorf("spoke %d is %v wide", i, last.x+last.w)
		}
		if l.cont && l.start != l.toks[0].on {
			t.Errorf("spoke %d, a line's second part, starts at %v, its first word at %v", i, l.start, l.toks[0].on)
		}
	}
}

// The chorus coming in gets the full accent, the first time and the last;
// lines sung on from the one before get none; and the chorus's long last
// word swells the wheel.
func TestVinylAccentsTheChorus(t *testing.T) {
	lines := vinylSong()
	set := vinylSetOf(lines, 2000)
	hit := func(at time.Duration) *vinylHit {
		for i := range set.hits {
			if set.hits[i].at == at {
				return &set.hits[i]
			}
		}
		return nil
	}
	for _, i := range []int{3, 8} {
		if h := hit(lines[i].At); h == nil || h.tier != 3 || h.pulse != 1 || h.glow != 1 || h.tear != 1 {
			t.Errorf("the chorus comes in at line %d with %+v", i, h)
		}
	}
	for _, i := range []int{1, 2, 6, 7} {
		if h := hit(lines[i].At); h != nil && h.tier >= 2 {
			t.Errorf("line %d, sung on from the one before, is accented %+v", i, h)
		}
	}
	if len(set.swells) != 2 {
		t.Errorf("%d swells, want the chorus's last word, twice", len(set.swells))
	}
	// The wheel is larger for it while it is held, and not after.
	held := lines[4].Words[len(lines[4].Words)-1]
	if m := set.motionAt(held.Start + time.Second); m.zoom <= 0 {
		t.Errorf("a second into the word held: the wheel %v larger", m.zoom)
	}
	if m := set.motionAt(held.End + time.Second); m.zoom != 0 {
		t.Errorf("a second after the word held: the wheel %v larger", m.zoom)
	}
}

// Vinyl builds its view only as its wheel turns: between lines the record
// goes on turning in frames that only paint.
func TestVinylBuildBudget(t *testing.T) {
	a, tt, builds := playingApp()
	a.nowPlaying, a.npPane = true, paneStage
	a.settings.StageStyle = "vinyl"
	a.pl.Seek(13 * time.Second) // into the second line, sung from 10 s to 15 s
	tt.Frame()
	tt.Advance(2 * time.Second)
	n := *builds
	tt.Advance(time.Second)
	if got := *builds - n; got > 6 {
		t.Errorf("Vinyl, a line being sung: %d builds a second", got)
	}
	if !tt.FramePending() {
		t.Error("the record asks for no frames to turn in")
	}
}

// vinylChange is an app playing a song on the stage, in Vinyl, held two
// seconds in, with its lyrics. change plays another, whose lyrics are next,
// nil for none, and come once release is closed.
func vinylChange(t *testing.T, next []music.LyricLine) (a *app, tt *ui.Tester, ch chan func(), change func(), release chan struct{}) {
	t.Helper()
	a = newTestApp()
	ch = posts(a)
	a.pl = player.New(a.post)
	a.pl.Source = a.src
	a.pl.More = a.autoplay
	release = make(chan struct{})
	fake(a).lyrics = func(id string) (*music.Lyrics, error) {
		if id != "8" {
			return &music.Lyrics{Lines: []music.LyricLine{{Start: 1000, End: 3000, Text: "hello world"}, {Start: 4000, End: 6000, Text: "the next line"}}}, nil
		}
		<-release
		if next == nil {
			return nil, nil
		}
		return &music.Lyrics{Lines: next}, nil
	}
	first, second := coveredSong(a), song("8", "Southern Cross", "Solveig")
	second.Album = first.Album
	a.pl.PlayQueue([]music.Track{first}, 0, music.Origin{})
	until(t, ch, func() bool { _, loaded := a.pl.Lyrics(); return loaded })
	change = func() {
		a.pl.PlayQueue([]music.Track{second}, 0, music.Origin{})
		a.pl.Hold(0)
	}
	a.pl.Hold(2 * time.Second)
	a.nowPlaying, a.npPane = true, paneStage
	a.settings.StageStyle = "vinyl"
	tt = ui.NewTester(a.view, 1180, 780)
	tt.SetDark(true)
	tt.Frame()
	tt.Advance(time.Second) // Now Playing slides into place
	return a, tt, ch, change, release
}

// Vinyl's deck is painted in every frame through the change from a song
// to the next: while the next one's lyrics are on their way, and once they
// have come, with times, without, or not at all. The stage showed its
// skeleton until they came, and a song without timed lyrics its text
// alone, each without the deck: going to the next song, the record, the
// tonearm and the ring went and came back.
func TestVinylDeckStaysThroughASongChange(t *testing.T) {
	timed := []music.LyricLine{{Start: 1000, End: 3000, Text: "another line"}, {Start: 4000, End: 6000, Text: "and one more"}}
	untimed := []music.LyricLine{{Text: "another line"}, {Text: "and one more"}}
	for name, next := range map[string][]music.LyricLine{"with times": timed, "without times": untimed, "not at all": nil} {
		a, tt, ch, change, release := vinylChange(t, next)
		// The deck's ring is painted with the rest of it.
		painted := func(when string) {
			t.Helper()
			for i := range 30 {
				before := a.deck.levels.last
				tt.Advance(time.Second / 60)
				if !a.deck.levels.last.After(before) {
					t.Fatalf("lyrics %s: %s, the deck isn't painted in frame %d", name, when, i)
				}
			}
		}
		painted("in the first song")
		change()
		if _, loaded := a.pl.Lyrics(); loaded {
			t.Fatalf("lyrics %s: they came at once", name)
		}
		tt.Frame()
		painted("as the next song's lyrics are on their way")
		close(release)
		until(t, ch, func() bool { _, loaded := a.pl.Lyrics(); return loaded })
		tt.Frame()
		painted("once the next song's lyrics have come")
		if next != nil && len(next[0].Text) > 0 && next[1].Start > 0 && (a.vinyl == nil || len(a.vinyl.lines) != 2) {
			t.Errorf("lyrics %s: the wheel has %+v", name, a.vinyl)
		}
	}
}

// With MM_FRAMES set, saves the stage's Vinyl as a line is sung and gives
// way to the next, a frame every 30th of a second, in Japanese and in
// Chinese, and one of each with the record as a GPU draws it; each of the
// pressings; the change from a song to the next; and a song in English as
// its chorus comes in and its last word is held.
func TestVinylFrames(t *testing.T) {
	dir := framesDir(t)
	step := time.Second / 30
	open := func(a *app, tt *ui.Tester) {
		a.npPane = paneStage
		a.settings.StageStyle = "vinyl"
		tt.Frame()
	}
	for _, lang := range []string{"ja", "zh"} {
		// The eighth line begins at 49.4 s.
		a, tt, _ := motionApp(motionSongs()[lang], 48200*time.Millisecond)
		open(a, tt)
		playFrames(a, tt, 48200*time.Millisecond, step, 70, func(i int) {
			saveFrame(t, dir, fmt.Sprintf("vinyl-%s-%02d.png", lang, i), tt.Image())
		})
		vinylMarbleOnCPU = true
		playFrames(a, tt, 51*time.Second, step, 2, func(int) {})
		saveFrame(t, dir, fmt.Sprintf("vinyl-%s-marbled.png", lang), tt.Image())
		vinylMarbleOnCPU = false
	}
	// Each pressing, as a GPU draws it.
	a, tt, _ := motionApp(motionSongs()["ja"], 50*time.Second)
	open(a, tt)
	playFrames(a, tt, 50*time.Second, step, 20, func(int) {}) // the stage takes the lyrics' place
	vinylMarbleOnCPU = true
	for i := range pressings {
		vinylPressed = i
		a.pl.Seek(50*time.Second + time.Duration(i)*200*time.Millisecond)
		tt.Advance(step)
		tt.Frame()
		saveFrame(t, dir, fmt.Sprintf("vinyl-pressing-%02d.png", i), tt.Image())
	}
	vinylMarbleOnCPU, vinylPressed = false, -1

	// Through the change to the next song, whose lyrics come half a
	// second after it starts.
	{
		a, tt, ch, change, release := vinylChange(t, []music.LyricLine{{Start: 1000, End: 3000, Text: "another line"}, {Start: 4000, End: 6000, Text: "and one more"}})
		n := 0
		frames := func(count int) {
			for range count {
				tt.Advance(step)
				saveFrame(t, dir, fmt.Sprintf("vinyl-change-%02d.png", n), tt.Image())
				n++
			}
		}
		frames(6)
		change()
		tt.Frame()
		frames(15)
		close(release)
		until(t, ch, func() bool { _, loaded := a.pl.Lyrics(); return loaded })
		tt.Frame()
		frames(15)
	}

	lines := vinylSong()
	a = newTestApp()
	a.pl.Cue([]music.Track{coveredSong(a)})
	a.pl.SetLyrics(lines)
	a.pl.Hold(lines[3].At - 500*time.Millisecond)
	a.nowPlaying = true
	a.settings.StageSparks = true
	tt = ui.NewTester(a.view, 1180, 780)
	tt.SetDark(true)
	open(a, tt)
	tt.Advance(3 * time.Second)
	// The chorus comes in, and then its second line to the end of the
	// word it holds.
	playFrames(a, tt, lines[3].At-500*time.Millisecond, step, 45, func(i int) {
		saveFrame(t, dir, fmt.Sprintf("vinyl-chorus-%02d.png", i), tt.Image())
	})
	held := lines[4].Words[len(lines[4].Words)-1]
	playFrames(a, tt, held.Start-300*time.Millisecond, 3*step, 30, func(i int) {
		saveFrame(t, dir, fmt.Sprintf("vinyl-held-%02d.png", i), tt.Image())
	})
}
