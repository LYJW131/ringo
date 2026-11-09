package applemusic

import (
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Covers. Apple's catalog ones are templates the app sizes; a private
// playlist's is kept on this machine (see playlistArt).

// art is a catalog cover, as the template the app fills in: {size} for both sides.
func art(a *artwork) string {
	if a == nil || a.URL == "" {
		return ""
	}
	u := strings.Replace(a.URL, "{w}x{h}", "{size}x{size}", 1)
	u = strings.Replace(u, "{w}", "{size}", 1)
	u = strings.Replace(u, "{h}", "{size}", 1)
	u = strings.Replace(u, "{c}", "cc", 1) // a crop, as profile pictures take
	return strings.Replace(u, "{f}", "jpg", 1)
}

// ImageHeaders answers the headers a picture at u is fetched with: Apple's
// image servers (*.mzstatic.com) are asked as Safari on music.apple.com
// would ask them. Other hosts get none.
func ImageHeaders(u *url.URL) map[string]string {
	if u == nil || !strings.HasSuffix(strings.ToLower(u.Hostname()), ".mzstatic.com") {
		return nil
	}
	return map[string]string{"user-agent": userAgent, "referer": "https://music.apple.com/"}
}

// A cover the listener chose for their own playlist is a presigned link to
// Apple's blobstore, at the size they uploaded it and no other. From some
// places it takes 20 to 35 seconds, past the 30 the app waits for a cover,
// so the bigger ones never show. amhelper fetches each once, with no
// deadline, and keeps it 600 pixels wide in the source's data; from then on
// the app reads it from there. Until then the cover is left out: given the
// link, the app would spend one of the few covers it loads at once on each
// for half a minute, and every other cover would wait behind them.

const (
	coversKept = 500
	// Each blobstore download is slow on its own, not for want of bandwidth.
	coverFetches = 4
)

var blobstore = regexp.MustCompile(`^https://[^/]+\.blobstore\.apple\.com/`)

type coverJob struct{ name, url string }

// playlistArt is a library playlist's cover.
func (s *Source) playlistArt(a *artwork) string {
	if a == nil || !blobstore.MatchString(a.URL) {
		return art(a)
	}
	name := coverName(a.URL)
	if name == "" {
		return ""
	}
	var kept map[string]int64
	s.storage.get(coversKey, &kept)
	if kept[name] != 0 {
		return fileURL(s.coverPath(name))
	}
	s.coverMu.Lock()
	queued := false
	for _, c := range s.coverQueue {
		if c.name == name {
			queued = true
			break
		}
	}
	if !queued {
		s.coverQueue = append(s.coverQueue, coverJob{name, a.URL})
	}
	start := coverFetches - s.coverFetching
	s.coverFetching = coverFetches
	s.coverMu.Unlock()
	for range start {
		s.spawn(s.keepCovers)
	}
	return ""
}

var coverObject = regexp.MustCompile(`(?i)/([0-9a-f]{8}-[0-9a-f-]{27})/image\b`)

// coverName is the blobstore object's own name, which stays as long as the
// picture does; the signature around it changes every day.
func coverName(u string) string {
	if m := coverObject.FindStringSubmatch(u); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

func (s *Source) coverPath(name string) string {
	return filepath.Join(s.dataDir, "cover-"+name+".jpg")
}

func fileURL(path string) string {
	path = strings.ReplaceAll(path, `\`, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// keepCovers is one of the few fetching covers, until there are none left.
func (s *Source) keepCovers() {
	defer func() {
		s.coverMu.Lock()
		s.coverFetching--
		s.coverMu.Unlock()
	}()
	base, err := s.helperBase()
	if err != nil {
		s.coverMu.Lock()
		s.coverQueue = nil // without amhelper there's no keeping them
		s.coverMu.Unlock()
		return
	}
	for {
		s.coverMu.Lock()
		if len(s.coverQueue) == 0 {
			s.coverMu.Unlock()
			return
		}
		next := s.coverQueue[0]
		s.coverQueue = s.coverQueue[1:]
		s.coverMu.Unlock()
		res, err := s.helperDo("POST", base+"/cover/"+next.name, map[string]string{"url": next.url})
		if err != nil || res.status/100 != 2 {
			continue
		}
		s.coverKeptMu.Lock()
		kept := map[string]int64{}
		s.storage.get(coversKey, &kept)
		kept[next.name] = s.now()
		if len(kept) > coversKept {
			names := make([]string, 0, len(kept))
			for n := range kept {
				names = append(names, n)
			}
			sort.Slice(names, func(i, j int) bool { return kept[names[i]] < kept[names[j]] })
			for _, old := range names[:len(names)-coversKept] {
				delete(kept, old)
			}
		}
		s.storage.set(coversKey, kept)
		s.coverKeptMu.Unlock()
	}
}
