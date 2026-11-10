package app

import (
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/lyrics"
	"ringo/internal/music"
)

// sungLine is a line of a song made up for the frames, sung from start a
// character at a time, two or three tenths of a second each and the last
// held for a second; a space in text rests a tenth.
func sungLine(start int64, text, translation string) music.LyricLine {
	l := music.LyricLine{Start: start, Translation: translation}
	at := start
	for _, r := range text {
		if r == ' ' {
			if n := len(l.Words); n > 0 {
				l.Words[n-1].Text += " "
			}
			at += 100
			continue
		}
		d := int64(200)
		if len(l.Words)%3 == 0 {
			d = 300
		}
		l.Words = append(l.Words, music.LyricWord{Start: at, End: at + d, Text: string(r)})
		at += d
	}
	last := &l.Words[len(l.Words)-1]
	last.End = last.Start + 1000
	l.End = last.End
	return l
}

// motionSongs are the songs the frames of the lyrics' motion are of: one in
// Japanese and one in Chinese, both made up, a line every 5.2 seconds from
// the sixth, but for a break of some eight seconds after the fifth line.
func motionSongs() map[string][]music.LyricLine {
	words := map[string][][2]string{
		"ja": {
			{"夜明けの駅で 風を待っていた", "在黎明的车站 等着风来"},
			{"白い息が 空にほどけてゆく", "白色的呼吸 在天空里散开"},
			{"君の名前を 小さく呼んでみる", "试着轻轻唤你的名字"},
			{"遠い街の灯りが まだ揺れている", "远方城市的灯火 还在摇晃"},
			{"始発のベルが 今日を連れてくる", "首班车的铃声 把今天带来"},
			{"もう一度だけ 歩き出せる気がした", "觉得自己 还能再出发一次"},
			{"朝がホームに 降りてくる", "清晨落在了站台上"},
			{"窓の向こうで 街が目を覚ます", "车窗外 城市睁开了眼"},
			{"知らない誰かの 声がやさしい", "陌生人的声音 很温柔"},
			{"ポケットの中の 切符をにぎる", "握紧口袋里的那张车票"},
			{"行き先はまだ 決めていないけど", "要去哪里 还没有决定"},
			{"この風となら どこへでも行ける", "和这阵风一起 哪里都能去"},
		},
		"zh": {
			{"把晚风折成一只纸船", "Fold the evening wind into a paper boat"},
			{"让它顺着路灯漂远", "And let it drift away along the streetlights"},
			{"你说过的那些以后", "All those somedays you spoke of"},
			{"都停在了第几个夏天", "Which summer did they stop in"},
			{"我在潮水退去的岸边", "On the shore the tide has left"},
			{"捡起一枚发亮的昨天", "I pick up a yesterday that still shines"},
			{"放进口袋 慢慢走远", "Pocket it, and slowly walk away"},
			{"路过的风替我记得", "The passing wind remembers for me"},
			{"那条回家的小巷", "That little lane that leads home"},
			{"灯一盏一盏亮起来", "The lamps come on, one by one"},
			{"像你数过的星光", "Like the starlight you once counted"},
			{"我们在夜里慢慢发亮", "Slowly, we begin to glow in the night"},
		},
	}
	songs := map[string][]music.LyricLine{}
	for lang, lines := range words {
		for i, l := range lines {
			start := int64(6000 + 5200*i)
			if i >= 5 {
				start += 7000
			}
			songs[lang] = append(songs[lang], sungLine(start, l[0], l[1]))
		}
	}
	return songs
}

// motionBuilt is how long motionApp's views took to build, in all.
var motionBuilt time.Duration

// motionApp is an app with Now Playing open on lines, held at pos, in a
// dark window, and the number of times its view was built.
func motionApp(lines []music.LyricLine, pos time.Duration) (*app, *ui.Tester, *int) {
	a := newTestApp()
	a.lyricSoft = true
	tr := coveredSong(a)
	a.pl.Cue([]music.Track{tr})
	a.pl.SetLyrics(lyrics.FromSource(&music.Lyrics{Lines: lines}))
	a.pl.Hold(pos)
	a.nowPlaying, a.npPane = true, paneLyrics
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		start := time.Now()
		a.view(c)
		motionBuilt += time.Since(start)
	}, 1180, 780)
	tt.SetDark(true)
	tt.Frame()
	tt.Advance(3 * time.Second)
	tt.Frame()
	tt.Frame()
	return a, tt, &builds
}

// playFrames plays a from from, a frame every step, n of them, and has
// each seen. A view that asks for frames by itself has had them as the
// time went by; one at rest is built for where the song has got to.
func playFrames(a *app, tt *ui.Tester, from, step time.Duration, n int, seen func(i int)) {
	for i := range n {
		a.pl.Seek(from + time.Duration(i)*step)
		tt.Advance(step)
		if !tt.FramePending() {
			tt.Frame()
		}
		seen(i)
	}
}

// With MM_FRAMES set, saves the lyrics as a line gives way to the next, a
// frame every 30th of a second, in Japanese and in Chinese; as a line is
// sung to the long note that ends it; and through a break in the song.
func TestLyricsMotionFrames(t *testing.T) {
	dir := framesDir(t)
	step := time.Second / 30
	for _, lang := range []string{"ja", "zh"} {
		// The eighth line begins at 49.4 s.
		a, tt, _ := motionApp(motionSongs()[lang], 49200*time.Millisecond)
		playFrames(a, tt, 49200*time.Millisecond, step, 40, func(i int) {
			saveFrame(t, dir, fmt.Sprintf("change-%s-%02d.png", lang, i), tt.Image())
		})
	}
	// The ninth line, from 54.6 s.
	a, tt, _ := motionApp(motionSongs()["ja"], 54400*time.Millisecond)
	playFrames(a, tt, 54400*time.Millisecond, 3*step, 50, func(i int) {
		saveFrame(t, dir, fmt.Sprintf("sweep-ja-%02d.png", i), tt.Image())
	})
	// The break after the fifth line, which ends at about 30.5 s, until
	// the sixth at 39 s.
	a, tt, _ = motionApp(motionSongs()["zh"], 30000*time.Millisecond)
	playFrames(a, tt, 30000*time.Millisecond, 6*step, 50, func(i int) {
		saveFrame(t, dir, fmt.Sprintf("break-zh-%02d.png", i), tt.Image())
	})
}

// With MM_FRAMES set, saves the lyrics of a duet, the other singer's lines
// on the right and the backing vocals under theirs, as one of each is
// sung.
func TestLyricsDuetFrames(t *testing.T) {
	dir := framesDir(t)
	word := func(start int64, text string) music.LyricWord {
		return music.LyricWord{Start: start, End: start + 380, Text: text}
	}
	line := func(start int64, other bool, translation string, words ...string) music.LyricLine {
		l := music.LyricLine{Start: start, Other: other, Translation: translation}
		for i, w := range words {
			if i+1 < len(words) {
				w += " "
			}
			l.Words = append(l.Words, word(start+int64(i)*400, w))
		}
		l.End = l.Words[len(l.Words)-1].End + 600
		return l
	}
	lines := []music.LyricLine{
		line(4000, false, "我为你留着门廊的灯", "I", "left", "the", "porch", "light", "on", "for", "you"),
		line(8500, true, "我从路上就看见它亮着", "I", "saw", "it", "burning", "from", "the", "road"),
		line(13000, false, "进来吧 夜里冷", "Come", "in,", "come", "in,", "the", "night", "is", "cold"),
		line(17500, true, "我从没想过要走这么久", "I", "never", "meant", "to", "stay", "away"),
		line(22000, false, "我们到家了", "We", "are", "home"),
	}
	lines[2].Background = []music.LyricWord{word(15400, "the "), word(15800, "night "), word(16200, "is "), word(16600, "cold")}
	lines[3].Background = []music.LyricWord{word(19900, "so "), word(20300, "long")}
	for name, pos := range map[string]time.Duration{"duet-1.png": 16300 * time.Millisecond, "duet-2.png": 20200 * time.Millisecond} {
		a, tt, _ := motionApp(lines, pos-time.Second)
		playFrames(a, tt, pos-time.Second, time.Second/30, 31, func(int) {})
		saveFrame(t, dir, name, tt.Image())
	}
}

// With MM_FRAMES set, saves the lyrics a moment after the listener scrolled
// them: every line sharp, to read.
func TestLyricsBrowsedFrame(t *testing.T) {
	dir := framesDir(t)
	a, tt, _ := motionApp(motionSongs()["zh"], 51*time.Second)
	wheel(tt, 800, 400, 120)
	playFrames(a, tt, 51*time.Second, time.Second/30, 20, func(int) {})
	saveFrame(t, dir, "browsed-zh.png", tt.Image())
}

// With MM_BENCH set, says how long a refresh of the display takes the lyrics
// off screen, through a line giving way to the next, a line being sung and
// a break in the song, with their translation under the lines and with how
// they are said: the frames the view asks for, built, laid out and drawn on
// the CPU, how many of them built the view, and how long a build took.
func TestLyricsMotionCost(t *testing.T) {
	if os.Getenv("MM_BENCH") == "" {
		t.Skip()
	}
	const refresh = time.Second / 60
	cost := func(name string, from time.Duration, n int, soft bool, extra string) {
		a, tt, builds := motionApp(saidSong(), from-time.Second/2)
		a.lyricSoft, a.settings.LyricsExtra = soft, extra
		tt.Frame()
		for i := range 30 { // warmed
			a.pl.Seek(from - time.Second/2 + time.Duration(i)*refresh)
			tt.Advance(refresh)
		}
		was, view := *builds, motionBuilt
		start := time.Now()
		for i := range n {
			a.pl.Seek(from + time.Duration(i)*refresh)
			tt.Advance(refresh)
		}
		took, built := time.Since(start), *builds-was
		each := 0.0
		if built > 0 {
			each = float64((motionBuilt - view).Microseconds()) / 1000 / float64(built)
		}
		t.Logf("%s (blurred: %v, under the lines: %s): %.2f ms a refresh, %d builds of %.2f ms in %d refreshes", name, soft, extra, float64(took.Microseconds())/1000/float64(n), built, each, n)
	}
	// Under the lines, their translation, which is text, and how they are
	// said, which the light crosses as it does the line.
	for _, extra := range []string{extraTranslation, extraPronunciation} {
		for _, soft := range []bool{false, true} {
			cost("a line giving way to the next", 49400*time.Millisecond, 72, soft, extra)
			cost("a line being sung", 51000*time.Millisecond, 120, soft, extra)
			cost("a break in the song", 34000*time.Millisecond, 120, soft, extra)
		}
	}
}

// With MM_FRAMES set, saves the lyrics as a song begins: before its first
// line, as the first is sung, and as the second is.
func TestLyricsFirstLineFrames(t *testing.T) {
	dir := framesDir(t)
	// The first line begins at 6 s, the second at 11.2 s.
	for name, pos := range map[string]time.Duration{"first-waiting.png": 3 * time.Second, "first-sung.png": 7500 * time.Millisecond, "second-sung.png": 12700 * time.Millisecond} {
		a, tt, _ := motionApp(motionSongs()["ja"], pos-time.Second)
		playFrames(a, tt, pos-time.Second, time.Second/30, 31, func(int) {})
		saveFrame(t, dir, name, tt.Image())
	}
}

// thrice is lines three times over, one after another, for a song long
// enough to have lines past both ends of the pane at once.
func thrice(lines []music.LyricLine) []music.LyricLine {
	length := lines[len(lines)-1].End + 4000
	var out []music.LyricLine
	for n := range int64(3) {
		for _, l := range lines {
			l.Start, l.End = l.Start+n*length, l.End+n*length
			l.Words, l.Pronounced = slices.Clone(l.Words), slices.Clone(l.Pronounced)
			for _, words := range [][]music.LyricWord{l.Words, l.Pronounced} {
				for k := range words {
					words[k].Start, words[k].End = words[k].Start+n*length, words[k].End+n*length
				}
			}
			out = append(out, l)
		}
	}
	return out
}

// With MM_FRAMES set, saves the lyrics where their lines meet the pane's
// top and bottom, in a tall window: as a line is sung, with a translation
// under the lines and with how they are said; a moment after the listener
// scrolled them; and lyrics without times, at their top and scrolled a
// little.
func TestLyricsEdgesFrames(t *testing.T) {
	dir := framesDir(t)
	const w, h = 1180, 1000
	tall := func(lines []music.LyricLine, extra string) (*app, *ui.Tester, time.Duration) {
		// A second and a half into the seventeenth line.
		lines = thrice(lines)
		pos := time.Duration(lines[16].Start+1500) * time.Millisecond
		a, tt, _ := motionApp(lines, pos)
		a.settings.LyricsExtra = extra
		tt.SetSize(w, h)
		tt.Frame()
		playFrames(a, tt, pos, time.Second/30, 45, func(int) {})
		return a, tt, pos
	}
	_, tt, _ := tall(motionSongs()["zh"], extraTranslation)
	saveFrame(t, dir, "edges-translation.png", tt.Image())
	_, tt, _ = tall(saidSong(), extraPronunciation)
	saveFrame(t, dir, "edges-pronunciation.png", tt.Image())
	// Scrolled as far as leaves a line across the pane's top, and its foot.
	var pos time.Duration
	for name, by := range map[string]float32{"edges-scrolled-top.png": 122, "edges-scrolled-foot.png": 150} {
		var a *app
		a, tt, pos = tall(motionSongs()["zh"], extraTranslation)
		wheel(tt, 800, 500, by)
		playFrames(a, tt, pos, time.Second/30, 20, func(int) {})
		saveFrame(t, dir, name, tt.Image())
	}

	a := newTestApp()
	a.pl.Cue([]music.Track{coveredSong(a)})
	a.pl.SetLyrics(stillWords(40))
	a.pl.Hold(pos)
	a.nowPlaying, a.npPane = true, paneLyrics
	tt = ui.NewTester(a.view, w, h)
	tt.SetDark(true)
	tt.Advance(2 * time.Second)
	tt.Frame()
	saveFrame(t, dir, "edges-still-top.png", tt.Image())
	wheel(tt, 800, 500, 137)
	tt.Advance(2 * time.Second) // the scroll bar goes
	tt.Frame()
	saveFrame(t, dir, "edges-still.png", tt.Image())
}
