package amhelper

// The covers listeners give their own playlists are presigned links to
// Apple's blobstore, at the size they were uploaded, and from some places
// take half a minute to come: past what the app waits for a cover. The
// applemusic package has each fetched once, with no hurry, and kept 600 pixels wide in
// its data, where the app reads it from then on.

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // the blobstore's are JPEG, but say they're PNG
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// coverWidth is as wide as a kept cover gets: the app shows none bigger.
const coverWidth = 600

// coverTime is how long a cover may take to come.
const coverTime = 3 * time.Minute

// coverMost is the biggest picture fetched, in bytes.
const coverMost = 40 << 20

var coverName = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f-]{27}$`)

type covers struct {
	dir  string // "" keeps none
	http *http.Client
}

func newCovers(dir string) *covers {
	return &covers{dir: dir, http: &http.Client{Timeout: coverTime}}
}

// cover fetches {url} and keeps it as <covers>/cover-<name>.jpg, answering
// once it's there.
func (s *server) cover(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		reply(w, http.StatusBadRequest, errorBody{"badRequest", err.Error()})
		return
	}
	switch {
	case s.covers.dir == "":
		reply(w, http.StatusBadRequest, errorBody{"badRequest", "no folder for covers"})
		return
	case !coverName.MatchString(name):
		reply(w, http.StatusBadRequest, errorBody{"badRequest", "not a cover's name"})
		return
	case !blobstoreURL(body.URL):
		reply(w, http.StatusBadRequest, errorBody{"badRequest", "not one of Apple's blobstore links"})
		return
	}
	// Detached: a cover asked for is wanted even if the caller stops waiting.
	ctx, cancel := context.WithTimeout(s.base(), coverTime)
	defer cancel()
	if err := s.covers.keep(ctx, name, body.URL); err != nil {
		reply(w, http.StatusBadGateway, errorBody{"failed", err.Error()})
		return
	}
	reply(w, http.StatusOK, map[string]bool{"ok": true})
}

func blobstoreURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && strings.HasSuffix(u.Hostname(), ".blobstore.apple.com")
}

func (c *covers) keep(ctx context.Context, name, link string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("cover: HTTP %d", res.StatusCode)
	}
	img, _, err := image.Decode(io.LimitReader(res.Body, coverMost))
	if err != nil {
		return fmt.Errorf("cover: %w", err)
	}
	if img.Bounds().Dx() > coverWidth {
		img = shrink(img, coverWidth)
	}
	// Written aside and moved in, so the app never reads half of one.
	f, err := os.CreateTemp(c.dir, ".cover-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 88}); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_ = os.Chmod(f.Name(), 0o644)
	return os.Rename(f.Name(), filepath.Join(c.dir, "cover-"+name+".jpg"))
}

// shrink scales img down to width pixels wide, each pixel of it the average
// of the part of img it covers.
func shrink(img image.Image, width int) image.Image {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	height := max(1, (sh*width+sw/2)/sw)
	// Across, then down: each pass weighs the source pixels by how much of
	// them a destination pixel takes in.
	across := make([]float64, width*sh*3)
	xw := spans(sw, width)
	for y := range sh {
		for x, span := range xw {
			var r, g, bl float64
			for _, p := range span {
				cr, cg, cb, _ := img.At(b.Min.X+p.at, b.Min.Y+y).RGBA()
				r += float64(cr) * p.w
				g += float64(cg) * p.w
				bl += float64(cb) * p.w
			}
			i := (y*width + x) * 3
			across[i], across[i+1], across[i+2] = r, g, bl
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	yw := spans(sh, height)
	for y, span := range yw {
		for x := range width {
			var r, g, bl float64
			for _, p := range span {
				i := (p.at*width + x) * 3
				r += across[i] * p.w
				g += across[i+1] * p.w
				bl += across[i+2] * p.w
			}
			o := out.PixOffset(x, y)
			out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = to8(r), to8(g), to8(bl), 0xff
		}
	}
	return out
}

type weight struct {
	at int
	w  float64
}

// spans says, for each of n pixels across a line of from pixels, which of
// those it takes in and how much of each, the weights adding up to 1.
func spans(from, n int) [][]weight {
	scale := float64(from) / float64(n)
	out := make([][]weight, n)
	for i := range n {
		lo, hi := float64(i)*scale, float64(i+1)*scale
		for p := int(lo); p < from && float64(p) < hi; p++ {
			w := min(hi, float64(p+1)) - max(lo, float64(p))
			if w > 0 {
				out[i] = append(out[i], weight{p, w / scale})
			}
		}
	}
	return out
}

func to8(v float64) uint8 {
	v = v/257 + 0.5
	if v > 255 {
		return 255
	}
	return uint8(v)
}
