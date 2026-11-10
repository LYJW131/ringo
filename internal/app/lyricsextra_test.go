package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ringo/internal/i18n"
	"ringo/internal/lyrics"
	"ringo/internal/music"
)

// What the lines of the songs made up for these tests are said as, and
// translated to.
const (
	extraSaid       = "yoake no eki"
	extraTranslated = "At the station at dawn"
)

// extraApp shows pane of Now Playing a second and a half into a song of two
// lines, made up, the first of which comes with how it is said, if said,
// and its translation, if translated; the pointer is in the window, for the
// button over the lyrics to show.
func extraApp(pane int, said, translated bool) (*app, *ui.Tester) {
	first := music.LyricLine{Start: 1000, End: 4000, Text: "夜明けの駅"}
	if said {
		first.Pronunciation = extraSaid
	}
	if translated {
		first.Translation = extraTranslated
	}
	a := newTestApp()
	a.pl.Cue([]music.Track{song("7", "宝船", "DOUDOU")})
	a.pl.SetLyrics(lyrics.FromSource(&music.Lyrics{Lines: []music.LyricLine{first, {Start: 5000, End: 8000, Text: "白い息"}}}))
	a.pl.Hold(1500 * time.Millisecond)
	a.nowPlaying, a.npPane = true, pane
	tt := ui.NewTester(a.view, 1200, 800)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Move(600, 400)
	tt.Advance(400 * time.Millisecond) // Now Playing fades in
	return a, tt
}

// saidShown reports whether the lyrics beside the cover show how their
// first line is said: set in type, as the light crosses it.
func saidShown(a *app) bool {
	return len(a.lyricType.said) > 0 && a.lyricType.said[0] != nil
}

// The stage and the wall show under the line sung what the lyrics do: its
// translation, how it is said, or neither.
func TestLyricsExtraOnTheStageAndTheWall(t *testing.T) {
	for _, pane := range []int{paneStage, paneWall} {
		a, tt := extraApp(pane, true, true)
		for _, c := range []struct {
			extra            string
			said, translated bool
		}{
			{extraTranslation, false, true},
			{extraPronunciation, true, false},
			{extraOff, false, false},
		} {
			a.settings.LyricsExtra = c.extra
			tt.Frame()
			if tt.HasText(extraSaid) != c.said || tt.HasText(extraTranslated) != c.translated {
				t.Errorf("pane %d, %s: how it is said shows %v, the translation %v", pane, c.extra, tt.HasText(extraSaid), tt.HasText(extraTranslated))
			}
		}
	}
}

// A song that hasn't what the listener chose to see under its lines shows
// them bare, in every pane, and not the other of the two, which wasn't
// asked for.
func TestLyricsExtraShowsOnlyWhatWasChosen(t *testing.T) {
	for _, pane := range []int{paneLyrics, paneStage, paneWall} {
		for _, c := range []struct {
			extra            string
			said, translated bool
		}{
			{extraPronunciation, false, true},
			{extraTranslation, true, false},
			{extraOff, true, true},
		} {
			a, tt := extraApp(pane, c.said, c.translated)
			a.settings.LyricsExtra = c.extra
			tt.Frame()
			if saidShown(a) || tt.HasText(extraSaid) || tt.HasText(extraTranslated) {
				t.Errorf("pane %d, %s chosen: how it is said shows %v, the translation %v", pane, c.extra, saidShown(a) || tt.HasText(extraSaid), tt.HasText(extraTranslated))
			}
		}
	}
}

// The button over the lyrics chooses what shows under their lines: how
// they are said in place of their translation, neither, and the
// translation again.
func TestLyricsExtraMenu(t *testing.T) {
	a, tt := extraApp(paneLyrics, true, true)
	if !tt.HasText(extraTranslated) || saidShown(a) {
		t.Fatalf("before any choice, the translation shows %v, how it is said %v", tt.HasText(extraTranslated), saidShown(a))
	}
	for _, c := range []struct {
		item, want       string
		said, translated bool
	}{
		{"Pronunciation", extraPronunciation, true, false},
		{"Off", extraOff, false, false},
		{"Translation", extraTranslation, false, true},
	} {
		if err := tt.Click("Translation and Pronunciation"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		settled(tt)
		if a.settings.LyricsExtra != c.want {
			t.Errorf("chose %s: the settings keep %q, want %q", c.item, a.settings.LyricsExtra, c.want)
		}
		if saidShown(a) != c.said || tt.HasText(extraTranslated) != c.translated {
			t.Errorf("chose %s: how it is said shows %v, the translation %v", c.item, saidShown(a), tt.HasText(extraTranslated))
		}
	}
}

// What the song playing hasn't can't be chosen from the button's menu:
// there would be nothing of it to show.
func TestLyricsExtraMenuOffersWhatTheSongHas(t *testing.T) {
	for _, c := range []struct {
		said, translated bool
		item             string
	}{
		{false, true, "Pronunciation"},
		{true, false, "Translation"},
	} {
		a, tt := extraApp(paneLyrics, c.said, c.translated)
		if err := tt.Click("Translation and Pronunciation"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err == nil || !strings.Contains(err.Error(), "disabled") {
			t.Errorf("%s, which the song hasn't, was chosen: %v", c.item, err)
		}
		tt.CloseMenu()
		if a.settings.LyricsExtra != "" {
			t.Errorf("the settings keep %q", a.settings.LyricsExtra)
		}
	}
}

// saidSong is the song in Japanese that the frames of the lyrics' motion
// are of, with how each of its lines is said, made up as the song is: as
// Apple sends it, a piece for each of the line's words, sung when the word
// is. A dot parts the pieces of a word.
func saidSong() []music.LyricLine {
	said := []string{
		"yo·a·ke no eki de kaze wo ma·t·te i·ta",
		"shiro·i iki ga sora ni ho·do·ke·te yu·ku",
		"kimi no na·mae wo chii·sa·ku yo·n·de mi·ru",
		"too·i machi no aka·ri ga ma·da yu·re·te i·ru",
		"shi·hatsu no be·ru ga kyo·u wo tsu·re·te ku·ru",
		"mo·u ichi·do da·ke aru·ki·da·se·ru ki ga shi·ta",
		"asa ga ho·o·mu ni o·ri·te ku·ru",
		"mado no mu·ko·u de machi ga me wo sa·ma·su",
		"shi·ra·na·i dare·ka no koe ga ya·sa·shi·i",
		"po·ke·t·to no naka no kip·pu wo ni·gi·ru",
		"yu·ki·saki wa ma·da ki·me·te i·na·i ke·do",
		"ko·no kaze to na·ra do·ko e de·mo i·ke·ru",
	}
	lines := motionSongs()["ja"]
	for i := range lines {
		l := &lines[i]
		for _, word := range strings.Fields(said[i]) {
			pieces := strings.Split(word, "·")
			pieces[len(pieces)-1] += " "
			for _, piece := range pieces {
				sung := l.Words[len(l.Pronounced)]
				l.Pronounced = append(l.Pronounced, music.LyricWord{Start: sung.Start, End: sung.End, Text: piece})
			}
		}
		if len(l.Pronounced) != len(l.Words) {
			panic(fmt.Sprintf("line %d: %d pieces for %d words", i, len(l.Pronounced), len(l.Words)))
		}
		l.Pronunciation = strings.ReplaceAll(said[i], "·", "")
	}
	return lines
}

// With MM_FRAMES set, saves the app in Chinese with each of what can show
// under a line of lyrics, nothing, how it is said, and its translation: the
// lyrics, the stage and the wall as a line is sung; the lyrics from one line
// through the next, a frame every tenth of a second; and the settings' page
// with the same choice.
func TestLyricsExtraFrames(t *testing.T) {
	dir := framesDir(t)
	defer i18n.SetLanguage(i18n.English)
	i18n.SetLanguage(i18n.Chinese)
	a := newTestApp()
	a.settingsPage = "playback"
	tt := ui.NewTester(a.settingsView, 780, 760)
	tt.SetDark(true)
	settled(tt)
	saveFrame(t, dir, "extra-settings.png", tt.Image())

	shown := func(pane int, extra string, from time.Duration) (*app, *ui.Tester) {
		a, tt, _ := motionApp(saidSong(), from)
		a.settings.LyricsExtra, a.npPane = extra, pane
		tt.Move(240, 700) // the pointer in the window shows the button
		tt.Frame()
		return a, tt
	}
	for _, extra := range []string{extraOff, extraPronunciation, extraTranslation} {
		// The eighth line begins at 49.4 s.
		for name, pane := range map[string]int{"lyrics": paneLyrics, "stage": paneStage, "wall": paneWall} {
			a, tt := shown(pane, extra, 50*time.Second)
			playFrames(a, tt, 50*time.Second, time.Second/30, 45, func(int) {})
			saveFrame(t, dir, fmt.Sprintf("extra-%s-%s.png", name, extra), tt.Image())
		}
		a, tt := shown(paneLyrics, extra, 49*time.Second)
		playFrames(a, tt, 49*time.Second, time.Second/10, 50, func(i int) {
			saveFrame(t, dir, fmt.Sprintf("extra-sung-%s-%02d.png", extra, i), tt.Image())
		})
	}
}
