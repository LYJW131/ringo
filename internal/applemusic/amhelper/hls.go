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
	"strconv"
	"strings"
	"time"
)

// track is one song's ALAC: the encrypted file and the keys of its sample
// descriptions, in order.
type track struct {
	MediaURL   string   `json:"-"`
	Keys       []string `json:"-"`
	SampleRate int      `json:"sampleRate"`
	BitDepth   int      `json:"bitDepth"`
	Bandwidth  int      `json:"bandwidth"`
	// Seconds is how long it plays, from its media playlist; 0 unknown.
	Seconds float64 `json:"-"`
}

var errNoALAC = errors.New("the song has no ALAC")

var alacGroup = regexp.MustCompile(`^audio-alac-stereo-\d{5,6}-\d{2}$`)

type variant struct {
	uri                             string
	audio                           string
	bandwidth, bitDepth, sampleRate int
}

// resolveTrack picks the best ALAC variant within the limits (0 for none)
// and reads its media playlist.
func resolveTrack(ctx context.Context, client *http.Client, masterURL string, maxRate, maxBits int, tm *timings) (track, error) {
	start := time.Now()
	master, err := getText(ctx, client, masterURL)
	tm.Master = time.Since(start).Milliseconds()
	if err != nil {
		return track{}, err
	}
	var best *variant
	for _, v := range parseMaster(master, masterURL) {
		if !alacGroup.MatchString(v.audio) ||
			(maxRate > 0 && v.sampleRate > maxRate) || (maxBits > 0 && v.bitDepth > maxBits) {
			continue
		}
		if best == nil || v.bandwidth > best.bandwidth {
			best = &v
		}
	}
	if best == nil {
		return track{}, errNoALAC
	}
	start = time.Now()
	media, err := getText(ctx, client, best.uri)
	tm.Media = time.Since(start).Milliseconds()
	if err != nil {
		return track{}, err
	}
	mediaURL, keys, seconds, err := parseMedia(media, best.uri)
	if err != nil {
		return track{}, err
	}
	return track{MediaURL: mediaURL, Keys: keys, SampleRate: best.sampleRate, BitDepth: best.bitDepth, Bandwidth: best.bandwidth, Seconds: seconds}, nil
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

// parseMedia returns the media file, its keys (the prefetch key, which
// opens every track, then ALAC's (c23) or the shared one (c6)) and how long
// it plays: its segments' lengths add up to within a frame of it.
func parseMedia(body, base string) (string, []string, float64, error) {
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
			if strings.HasPrefix(key, "skd://") && (strings.HasSuffix(key, "c23") || strings.HasSuffix(key, "c6")) {
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
