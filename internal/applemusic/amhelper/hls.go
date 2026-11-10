package amhelper

// The playlist parsing follows amdl's internal/media/hls.go.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// track is one song's audio, as a variant of its master playlist: the
// encrypted file and the keys of its sample descriptions, in order.
type track struct {
	MediaURL string   `json:"-"`
	Keys     []string `json:"-"`
	// Codec is what the samples are: "alac", or "aac" for every other kind,
	// HE-AAC among them. It says how a key decrypting them is checked.
	Codec      string `json:"codec"`
	SampleRate int    `json:"sampleRate"`
	BitDepth   int    `json:"bitDepth"`
	Bandwidth  int    `json:"bandwidth"`
	// BitRate is what Apple calls an AAC variant, from its group, as 256000
	// of audio-stereo-256-binaural; 0 of ALAC.
	BitRate int `json:"bitRate,omitempty"`
	// Seconds is how long it plays, from its media playlist; 0 unknown.
	Seconds float64 `json:"-"`
}

// describe says what the track is, for the log: 24-bit/96 kHz ALAC, or
// AAC at 256 kbps.
func (t track) describe() string {
	if t.Codec == "alac" {
		return fmt.Sprintf("%d Hz / %d bit ALAC", t.SampleRate, t.BitDepth)
	}
	return fmt.Sprintf("AAC at %d kbps", t.BitRate/1000)
}

// errNoAudio is the song lacking the kind asked for: no ALAC, or no Dolby
// Atmos version.
var errNoAudio = errors.New("the song has no such audio")

// The kinds of a song's audio that can be asked for, besides ALAC, by the
// groups of its master playlist, which Apple names for what they hold. Only
// a song with a Dolby Atmos version has the binaural and downmix ones, of
// that version: Apple's rendering of it for headphones, and Dolby's mix of
// it down to stereo. Both are AAC, as is HE-AAC.
const (
	kindALAC     = ""
	kindBinaural = "binaural"
	kindDownmix  = "downmix"
	kindHE       = "he"
)

// kinds are the groups of each kind, by the first that a song has: within
// one, the variant of most bandwidth. A few older songs have AAC at 64 kbps
// where others have HE-AAC, which is HE-AAC all the same, whatever its
// group says.
var kinds = map[string][]*regexp.Regexp{
	kindALAC:     {regexp.MustCompile(`^audio-alac-stereo-\d{5,6}-\d{2}$`)},
	kindBinaural: {regexp.MustCompile(`^audio-stereo-\d{2,3}-binaural$`)},
	kindDownmix:  {regexp.MustCompile(`^audio-stereo-\d{2,3}-downmix$`)},
	kindHE:       {regexp.MustCompile(`^audio-HE-stereo-\d{2,3}$`), regexp.MustCompile(`^audio-stereo-64$`)},
}

// knownKind says whether kind is one that can be asked for.
func knownKind(kind string) bool {
	_, ok := kinds[kind]
	return ok
}

// groupRate is the bit rate an AAC group is named for, as 256000 of
// audio-stereo-256-binaural, or 0.
var groupRate = regexp.MustCompile(`^audio-(?:HE-)?stereo-(\d{2,3})(?:-|$)`)

type variant struct {
	uri                             string
	audio                           string
	bandwidth, bitDepth, sampleRate int
}

// pickVariant returns the variant of the kind asked for, within the limits on
// ALAC (0 for none), or nil.
func pickVariant(variants []variant, kind string, maxRate, maxBits int) *variant {
	for _, group := range kinds[kind] {
		var best *variant
		for _, v := range variants {
			if !group.MatchString(v.audio) ||
				(kind == kindALAC && ((maxRate > 0 && v.sampleRate > maxRate) || (maxBits > 0 && v.bitDepth > maxBits))) {
				continue
			}
			if best == nil || v.bandwidth > best.bandwidth {
				best = &v
			}
		}
		if best != nil {
			return best
		}
	}
	return nil
}

// resolveTrack picks the song's variant of the kind, and reads its media
// playlist.
func resolveTrack(ctx context.Context, client *http.Client, masterURL, kind string, maxRate, maxBits int, tm *timings) (track, error) {
	start := time.Now()
	master, err := getText(ctx, client, masterURL)
	tm.Master = time.Since(start).Milliseconds()
	if err != nil {
		return track{}, err
	}
	best := pickVariant(parseMaster(master, masterURL), kind, maxRate, maxBits)
	if best == nil {
		return track{}, errNoAudio
	}
	start = time.Now()
	media, err := getText(ctx, client, best.uri)
	tm.Media = time.Since(start).Milliseconds()
	if err != nil {
		return track{}, err
	}
	codec := "aac"
	if kind == kindALAC {
		codec = "alac"
	}
	mediaURL, keys, seconds, err := parseMedia(media, best.uri, codec)
	if err != nil {
		return track{}, err
	}
	t := track{MediaURL: mediaURL, Keys: keys, Codec: codec, Bandwidth: best.bandwidth, Seconds: seconds}
	if codec == "alac" {
		t.SampleRate, t.BitDepth = best.sampleRate, best.bitDepth
	} else if m := groupRate.FindStringSubmatch(best.audio); m != nil {
		t.BitRate = atoi(m[1]) * 1000
	}
	return t, nil
}

func getText(ctx context.Context, client *http.Client, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("playlist: HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return string(b), err
}

func parseMaster(body, base string) []variant {
	var out []variant
	var pending map[string]string
	groups := map[string]map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA:"):
			attrs := parseAttrs(strings.TrimPrefix(line, "#EXT-X-MEDIA:"))
			if attrs["GROUP-ID"] != "" {
				groups[attrs["GROUP-ID"]] = attrs
			}
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			pending = parseAttrs(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"))
		case pending != nil && line != "" && !strings.HasPrefix(line, "#"):
			audio := pending["AUDIO"]
			g := groups[audio]
			out = append(out, variant{
				uri:        absURL(base, line),
				audio:      audio,
				bandwidth:  atoi(firstOf(pending["AVERAGE-BANDWIDTH"], pending["BANDWIDTH"])),
				bitDepth:   atoi(firstOf(g["BIT-DEPTH"], g["bit_depth"])),
				sampleRate: atoi(firstOf(g["SAMPLE-RATE"], g["sample_rate"])),
			})
			pending = nil
		}
	}
	return out
}

// keySuffixes are the ends of the keys of each codec's samples, as amdl
// has them: ALAC's own (c23), AAC's (c22, or c24 of a Dolby Atmos version's),
// and the one they share (c6), which is what Apple's playlists name today.
var keySuffixes = map[string][]string{"alac": {"c23", "c6"}, "aac": {"c22", "c24", "c6"}}

// parseMedia returns the media file, its keys (the prefetch key, which
// opens every track, then the codec's) and how long it plays: its
// segments' lengths add up to within a frame of it.
func parseMedia(body, base, codec string) (string, []string, float64, error) {
	keys := []string{prefetchKey}
	mediaURL := ""
	seconds := 0.0
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, "#EXTINF:"); ok {
			v, _, _ = strings.Cut(v, ",")
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 {
				seconds += f
			}
		}
		if strings.HasPrefix(line, "#EXT-X-KEY:") {
			key := parseAttrs(strings.TrimPrefix(line, "#EXT-X-KEY:"))["URI"]
			if strings.HasPrefix(key, "skd://") && slices.ContainsFunc(keySuffixes[codec], func(end string) bool { return strings.HasSuffix(key, end) }) {
				keys = append(keys, key)
			}
		}
		if strings.HasPrefix(line, "#EXT-X-MAP:") {
			if u := parseAttrs(strings.TrimPrefix(line, "#EXT-X-MAP:"))["URI"]; u != "" {
				mediaURL = absURL(base, u)
			}
		}
	}
	if mediaURL == "" {
		return "", nil, 0, errors.New("the playlist names no media file")
	}
	return mediaURL, keys, seconds, sc.Err()
}

func parseAttrs(v string) map[string]string {
	out := map[string]string{}
	var key, val strings.Builder
	inKey, inQuote := true, false
	flush := func() {
		if k := strings.TrimSpace(key.String()); k != "" {
			out[k] = strings.Trim(strings.TrimSpace(val.String()), `"`)
		}
		key.Reset()
		val.Reset()
		inKey = true
	}
	for _, r := range v {
		switch {
		case inKey && r == '=':
			inKey = false
		case !inKey && r == '"':
			inQuote = !inQuote
			val.WriteRune(r)
		case !inKey && r == ',' && !inQuote:
			flush()
		case inKey:
			key.WriteRune(r)
		default:
			val.WriteRune(r)
		}
	}
	flush()
	return out
}

func absURL(base, ref string) string {
	b, err1 := url.Parse(base)
	r, err2 := url.Parse(ref)
	if err1 != nil || err2 != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
