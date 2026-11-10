package applemusic

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"ringo/internal/i18n"
	"ringo/internal/music"
)

// Lyrics: Apple's TTML, word by word where Apple has it, with the
// translation into the listener's language when there is one.

func lyricsLanguage() string {
	if i18n.Language() == i18n.Chinese {
		return "zh-Hans-CN"
	}
	return "en-US"
}

func (s *Source) lyrics(id string) (*music.Lyrics, error) {
	if !isCatalogID(id) || !s.signedIn() {
		return nil, nil
	}
	language := lyricsLanguage()
	p := params{"l[lyrics]": language, "extend": "ttmlLocalizations", "l[script]": "en-Latn"}
	for _, kind := range []string{"syllable-lyrics", "lyrics"} {
		b, err := s.catalog("/songs/"+id+"/"+kind, p)
		if err != nil || len(b.Data) == 0 || b.Data[0] == nil {
			continue
		}
		a := b.Data[0].attrs()
		ttml := orT(a.TTMLLocalizations, a.TTML)
		if ttml != "" {
			return parseTTML(ttml, strings.Split(language, "-")[0]), nil
		}
	}
	return nil, nil
}

// milliseconds is TTML's 1.5, 1:02.25 or 1.5s, in milliseconds.
func milliseconds(t string) int64 {
	if t == "" {
		return 0
	}
	s := 0.0
	for _, p := range strings.Split(strings.TrimSuffix(t, "s"), ":") {
		n, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if strings.TrimSpace(p) == "" {
			n = 0 // as JavaScript's Number("")
		} else if err != nil {
			n = math.NaN()
		}
		s = s*60 + n
	}
	if math.IsNaN(s) {
		return 0
	}
	return int64(math.Round(s * 1000))
}

var (
	decimalRef = regexp.MustCompile(`&#(\d+);`)
	hexRef     = regexp.MustCompile(`(?i)&#x([0-9a-f]+);`)
	anyTag     = regexp.MustCompile(`<[^>]+>`)
	spaces     = regexp.MustCompile(`\s+`)

	translationBlock = regexp.MustCompile(`<translation\b([^>]*)>([\s\S]*?)</translation>`)
	textBlock        = regexp.MustCompile(`<text\b([^>]*)>([\s\S]*?)</text>`)
	paragraph        = regexp.MustCompile(`<p\b([^>]*)>([\s\S]*?)</p>`)
	background       = regexp.MustCompile(`<span\b[^>]*ttm:role="x-bg"[^>]*>([\s\S]*?)</span>\s*</span>|<span\b[^>]*ttm:role="x-bg"[^>]*>([\s\S]*?)</span>`)
	agentTag         = regexp.MustCompile(`<ttm:agent\b([^>]*)>`)
	wordSpan         = regexp.MustCompile(`<span\b([^>]*)>([^<]*)</span>(\s*)`)
)

func unescapeXML(s string) string {
	s = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'").Replace(s)
	codePoint := func(digits string, base int) string {
		n, err := strconv.ParseInt(digits, base, 32)
		if err != nil {
			return ""
		}
		return string(rune(n))
	}
	s = decimalRef.ReplaceAllStringFunc(s, func(m string) string { return codePoint(decimalRef.FindStringSubmatch(m)[1], 10) })
	s = hexRef.ReplaceAllStringFunc(s, func(m string) string { return codePoint(hexRef.FindStringSubmatch(m)[1], 16) })
	return strings.ReplaceAll(s, "&amp;", "&")
}

// attribute is the value of name in tag's attributes, and whether it's there.
func attribute(tag, name string) (string, bool) {
	m := regexp.MustCompile(`\s` + regexp.QuoteMeta(name) + `="([^"]*)"`).FindStringSubmatch(tag)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func attr(tag, name string) string {
	v, _ := attribute(tag, name)
	return v
}

func stripTags(s string) string { return unescapeXML(anyTag.ReplaceAllString(s, "")) }

// parseTTML is TTML's lines, with the translation into language (zh, en) where there is one.
func parseTTML(ttml, language string) *music.Lyrics {
	// Translations, by the line they're for.
	translations := map[string]string{}
	for _, b := range translationBlock.FindAllStringSubmatch(ttml, -1) {
		lang, ok := attribute(b[1], "xml:lang")
		if !ok {
			lang = attr(b[1], "lang")
		}
		if !strings.HasPrefix(lang, language) {
			continue
		}
		for _, m := range textBlock.FindAllStringSubmatch(b[2], -1) {
			if key := attr(m[1], "for"); key != "" {
				translations[key] = strings.TrimSpace(stripTags(m[2]))
			}
		}
		break
	}
	b := ttml
	if at := strings.Index(ttml, "<body"); at >= 0 {
		b = ttml[at:]
	} else if len(ttml) > 0 {
		// As JavaScript's slice(-1) does: only the last character is left.
		b = ttml[len(ttml)-1:]
	}
	singers := singersOf(ttml)
	var lines []music.LyricLine
	for _, m := range paragraph.FindAllStringSubmatch(b, -1) {
		key := attr(m[1], "itunes:key")
		inner := m[2]
		// Background vocals are spans inside a span: kept apart from the
		// line's own words, which they're sung behind.
		var behind []music.LyricWord
		for _, bg := range background.FindAllStringSubmatch(inner, -1) {
			if bg[1] != "" {
				behind = append(behind, timedWords(bg[1]+"</span>")...)
			} else if text := strings.TrimSpace(stripTags(bg[2])); text != "" {
				behind = append(behind, music.LyricWord{Text: text})
			}
		}
		main := background.ReplaceAllString(inner, "")
		words := timedWords(main)
		text := strings.TrimSpace(spaces.ReplaceAllString(stripTags(main), " "))
		if text == "" {
			// A line of nothing but backing vocals is theirs, as written.
			if text = strings.TrimSpace(spaces.ReplaceAllString(stripTags(inner), " ")); text == "" {
				continue
			}
			words, behind = behind, nil
		}
		line := music.LyricLine{Start: milliseconds(attr(m[1], "begin")), End: milliseconds(attr(m[1], "end")), Text: text}
		line.Words, line.Background = words, unbracketed(behind, line.Start, line.End)
		agent := attr(m[1], "ttm:agent")
		if singers.first == "" {
			// A head that names no singers: the first to sing is the first.
			singers.first = agent
		}
		line.Other = singers.other(agent)
		if key != "" && translations[key] != "" {
			line.Translation = translations[key]
		}
		lines = append(lines, line)
	}
	// Lyrics that aren't timed at all come as lines at 0: shown still.
	if len(lines) == 0 {
		return nil
	}
	return &music.Lyrics{Lines: lines}
}

// timedWords are the words of a line's spans that say when they're sung, a
// space after each that has one after it, but not after the last.
func timedWords(spans string) []music.LyricWord {
	var words []music.LyricWord
	for _, w := range wordSpan.FindAllStringSubmatch(spans, -1) {
		begin := attr(w[1], "begin")
		if begin == "" {
			continue
		}
		text := unescapeXML(w[2])
		if w[3] != "" {
			text += " "
		}
		words = append(words, music.LyricWord{Start: milliseconds(begin), End: milliseconds(attr(w[1], "end")), Text: text})
	}
	if len(words) > 0 {
		last := &words[len(words)-1]
		last.Text = strings.TrimRight(last.Text, " \t\r\n")
	}
	return words
}

// unbracketed are the words sung behind a line without the brackets Apple
// writes them in, which Apple Music doesn't show: its smaller type says
// what they are. Words with no times of their own share the line's.
func unbracketed(words []music.LyricWord, start, end int64) []music.LyricWord {
	if len(words) == 0 {
		return nil
	}
	first, last := &words[0], &words[len(words)-1]
	first.Text = strings.TrimLeft(first.Text, "(（")
	last.Text = strings.TrimRight(last.Text, ")）")
	for i := range words {
		if w := &words[i]; w.Start == 0 && w.End == 0 {
			w.Start, w.End = start, end
		}
	}
	return words
}

// singers tells the lines of a duet's other singer from the rest: the
// song's first singer is the one its head names first, and a group, as
// everyone in the chorus, sings on that singer's side.
type singers struct {
	first  string
	groups map[string]bool
}

func singersOf(ttml string) singers {
	s := singers{groups: map[string]bool{}}
	for _, m := range agentTag.FindAllStringSubmatch(ttml, -1) {
		id, _ := attribute(m[1], "xml:id")
		switch {
		case id == "":
		case attr(m[1], "type") != "person":
			s.groups[id] = true
		case s.first == "":
			s.first = id
		}
	}
	return s
}

// other reports whether agent, who sings a line, is the other singer.
func (s singers) other(agent string) bool {
	return agent != "" && s.first != "" && agent != s.first && !s.groups[agent]
}
