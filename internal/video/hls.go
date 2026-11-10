package video

import (
	"bufio"
	"bytes"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// A moving cover comes as an HLS master playlist of variants, H.264 and
// HEVC at sizes from a few hundred pixels to 4K. Each variant's media
// playlist is one fragmented MP4 cut into byte ranges, which this fetches
// whole: what the decoder reads.

// Variant is a stream of a master playlist.
type Variant struct {
	URI           string
	Width, Height int
	Bandwidth     int
	Codecs        string
}

// ParseMaster reads a master playlist's variants, their URIs resolved
// against base, its address.
func ParseMaster(base string, text []byte) ([]Variant, error) {
	var out []Variant
	var pending *Variant
	sc := bufio.NewScanner(bytes.NewReader(text))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			v := Variant{}
			for k, val := range attrs(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:")) {
				switch k {
				case "RESOLUTION":
					w, h, _ := strings.Cut(val, "x")
					v.Width, _ = strconv.Atoi(w)
					v.Height, _ = strconv.Atoi(h)
				case "BANDWIDTH":
					v.Bandwidth, _ = strconv.Atoi(val)
				case "CODECS":
					v.Codecs = val
				}
			}
			pending = &v
		case line == "" || strings.HasPrefix(line, "#"):
		case pending != nil:
			u, err := resolve(base, line)
			if err != nil {
				return nil, err
			}
			pending.URI = u
			out = append(out, *pending)
			pending = nil
		}
	}
	if len(out) == 0 {
		return nil, errors.New("video: no variants")
	}
	return out, sc.Err()
}

// attrs are an HLS tag's attributes, quotes taken off.
func attrs(s string) map[string]string {
	out := map[string]string{}
	for s != "" {
		k, rest, ok := strings.Cut(s, "=")
		if !ok {
			break
		}
		var v string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				break
			}
			v, rest = rest[1:1+end], rest[2+end:]
		} else {
			v, rest, _ = strings.Cut(rest, ",")
			rest = "," + rest
		}
		out[strings.TrimSpace(k)] = v
		s = strings.TrimPrefix(rest, ",")
	}
	return out
}

func resolve(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(r).String(), nil
}

// Pick is the H.264 variant to show at w×h pixels: the smallest that
// covers it, of those no bigger than maxW×maxH, else the biggest of those;
// of those the same size, the one of least bandwidth. HEVC is left out, as
// the decoder is H.264's alone.
func Pick(vs []Variant, w, h, maxW, maxH int) (Variant, bool) {
	var fits, under []Variant
	for _, v := range vs {
		if !strings.HasPrefix(v.Codecs, "avc1") || v.Width <= 0 || v.Height <= 0 {
			continue
		}
		if v.Width > maxW || v.Height > maxH {
			continue
		}
		if v.Width >= w && v.Height >= h {
			fits = append(fits, v)
		} else {
			under = append(under, v)
		}
	}
	better := func(a, b Variant, smaller bool) bool {
		if pa, pb := a.Width*a.Height, b.Width*b.Height; pa != pb {
			return (pa < pb) == smaller
		}
		return a.Bandwidth < b.Bandwidth
	}
	pick := func(list []Variant, smaller bool) (Variant, bool) {
		if len(list) == 0 {
			return Variant{}, false
		}
		best := list[0]
		for _, v := range list[1:] {
			if better(v, best, smaller) {
				best = v
			}
		}
		return best, true
	}
	if v, ok := pick(fits, true); ok {
		return v, true
	}
	return pick(under, false)
}

// mediaFiles are the files a media playlist's stream is in, in order: its
// map, then its segments, each file once however many byte ranges of it
// there are. Fetched whole and put together they make one fragmented MP4.
func mediaFiles(base string, text []byte) ([]string, error) {
	var out []string
	add := func(ref string) error {
		u, err := resolve(base, ref)
		if err != nil {
			return err
		}
		for _, have := range out {
			if have == u {
				return nil
			}
		}
		out = append(out, u)
		return nil
	}
	sc := bufio.NewScanner(bytes.NewReader(text))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "#EXT-X-KEY:") && !strings.Contains(line, "METHOD=NONE"):
			return nil, errors.New("video: an encrypted stream")
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			if u := attrs(strings.TrimPrefix(line, "#EXT-X-MAP:"))["URI"]; u != "" {
				if err := add(u); err != nil {
					return nil, err
				}
			}
		case line == "" || strings.HasPrefix(line, "#"):
		default:
			if err := add(line); err != nil {
				return nil, err
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("video: no segments")
	}
	return out, sc.Err()
}

// Fetch is the MP4 of the variant of the master playlist at master that
// Pick chooses, fetched with get.
func Fetch(master string, w, h, maxW, maxH int, get func(string) ([]byte, error)) ([]byte, error) {
	v, err := Choose(master, w, h, maxW, maxH, get)
	if err != nil {
		return nil, err
	}
	return FetchVariant(v, get)
}

// Choose is the variant of the master playlist at master that Pick
// chooses.
func Choose(master string, w, h, maxW, maxH int, get func(string) ([]byte, error)) (Variant, error) {
	text, err := get(master)
	if err != nil {
		return Variant{}, err
	}
	vs, err := ParseMaster(master, text)
	if err != nil {
		return Variant{}, err
	}
	v, ok := Pick(vs, w, h, maxW, maxH)
	if !ok {
		return Variant{}, errors.New("video: no H.264 variant")
	}
	return v, nil
}

// FetchVariant is a variant's stream as one fragmented MP4.
func FetchVariant(v Variant, get func(string) ([]byte, error)) ([]byte, error) {
	media, err := get(v.URI)
	if err != nil {
		return nil, err
	}
	files, err := mediaFiles(v.URI, media)
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, f := range files {
		b, err := get(f)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}
