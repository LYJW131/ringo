package amhelper

// Songs the listener uploaded to their library are plain .m4a files on
// Apple's blobstore, behind links that last a day. The blobstore hands each
// connection 13 to 16 KB a second, half what a 256 kbps song plays at, and
// more connections add up: one 13, four 62, six 76, eight 125 (2026-11). So
// each song is fetched in parts over several connections at once, into a
// file here, and served from it in order as the parts come in, the part a
// request starts at fetched first. Its start comes in small parts, all at
// once, as it's handed over, so that it plays in seconds rather than the
// twenty a whole part takes. Once all of it is in, the file is kept
// under the song's library ID, which outlives the link, and served from
// there from then on.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// uploadConnections is how many parts of a song come at once: the
	// fewest that came at more than twice the speed the song plays at.
	uploadConnections = 6
	// uploadStart is the start of a song, fetched in a small part per
	// connection so that it begins to play in seconds; uploadPart is each
	// part after it.
	uploadStart = 288 << 10
	uploadPart  = 256 << 10
	// uploadTries is how many times a part is asked for before the song fails.
	uploadTries = 3
	// uploadPartTime is how long a part may take.
	uploadPartTime = 2 * time.Minute
	// uploadsKept is how many whole songs are kept.
	uploadsKept = 20
)

var uploadID = regexp.MustCompile(`^i\.[A-Za-z0-9_-]+$`)

type uploads struct {
	ctx  context.Context // ends the parts coming in, with the server
	dir  string          // "" keeps none
	http *http.Client

	mu    sync.Mutex
	songs map[string]*upload // those coming in, by library ID
}

func newUploads(ctx context.Context, dir string, client *http.Client) *uploads {
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return &uploads{ctx: ctx, dir: dir, http: client, songs: map[string]*upload{}}
}

// upload is a song coming in.
type upload struct {
	id    string
	size  int64
	file  *os.File // the parts, each at its place
	spans []span

	mu      sync.Mutex
	cond    sync.Cond
	link    string // the latest, as each handing over brings a fresh one
	state   []partState
	want    int64 // where the latest request reads, fetched first
	readers int
	workers int
	err     error
	whole   bool
}

type span struct{ off, n int64 }

type partState byte

const (
	partWanted partState = iota
	partComing
	partIn
)

// uploadParts cuts size bytes into the parts fetched.
func uploadParts(size int64) []span {
	var out []span
	small := int64(uploadStart / uploadConnections)
	for off := int64(0); off < size; {
		n := int64(uploadPart)
		if off < uploadStart {
			n = small
		}
		n = min(n, size-off)
		out = append(out, span{off, n})
		off += n
	}
	return out
}

func (u *uploads) keptPath(id string) string {
	return filepath.Join(u.dir, "upload-"+id+".m4a")
}

// uploadRegister is POST /upload/<id> {url}: the song's link, fresh. It
// answers {size} once the song's first part is in, or at once if the song
// is kept.
func (s *server) uploadRegister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		reply(w, http.StatusBadRequest, errorBody{"badRequest", err.Error()})
		return
	}
	switch {
	case !uploadID.MatchString(id):
		reply(w, http.StatusBadRequest, errorBody{"badRequest", "not a library song's ID"})
		return
	case !blobstoreURL(body.URL):
		reply(w, http.StatusBadRequest, errorBody{"badRequest", "not one of Apple's blobstore links"})
		return
	}
	size, err := s.uploads.register(r.Context(), id, body.URL)
	if err != nil {
		reply(w, http.StatusBadGateway, errorBody{"media", err.Error()})
		return
	}
	reply(w, http.StatusOK, map[string]int64{"size": size})
}

func (u *uploads) register(ctx context.Context, id, link string) (int64, error) {
	if u.dir != "" {
		if fi, err := os.Stat(u.keptPath(id)); err == nil {
			return fi.Size(), nil
		}
	}
	u.mu.Lock()
	song := u.songs[id]
	if song != nil {
		u.mu.Unlock()
		song.mu.Lock()
		song.link = link
		song.mu.Unlock()
		return song.size, nil
	}
	u.mu.Unlock()

	// The start's parts, all at once: any of them says how big the song is,
	// and the song plays from them while the rest comes.
	starts := uploadParts(uploadStart)
	got := make([][]byte, len(starts))
	sizes := make([]int64, len(starts))
	errs := make([]error, len(starts))
	var wg sync.WaitGroup
	for i, p := range starts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], sizes[i], errs[i] = u.fetch(ctx, link, p)
		}()
	}
	wg.Wait()
	if errs[0] != nil {
		return 0, errs[0]
	}
	size := sizes[0]
	var f *os.File
	var err error
	if u.dir != "" {
		f, err = os.Create(filepath.Join(u.dir, "upload-"+id+".part"))
	} else {
		f, err = os.CreateTemp("", "upload-*.part")
	}
	if err != nil {
		return 0, err
	}
	song = &upload{id: id, size: size, file: f, spans: uploadParts(size), link: link}
	song.cond.L = &song.mu
	song.state = make([]partState, len(song.spans))
	// Those that failed, or lie past a song shorter than the start, are
	// fetched again as any other part, or not at all.
	for i, p := range starts {
		if errs[i] != nil || i >= len(song.spans) || song.spans[i] != (span{p.off, int64(len(got[i]))}) {
			continue
		}
		if _, err := f.WriteAt(got[i], p.off); err != nil {
			f.Close()
			os.Remove(f.Name())
			return 0, err
		}
		song.state[i] = partIn
	}
	u.mu.Lock()
	if again := u.songs[id]; again != nil {
		// Handed over twice at once: the other one stays.
		u.mu.Unlock()
		f.Close()
		os.Remove(f.Name())
		return again.size, nil
	}
	u.songs[id] = song
	u.mu.Unlock()
	return size, nil
}

// fetch asks for one part of link, answering its bytes and the whole song's size.
func (u *uploads) fetch(ctx context.Context, link string, p span) ([]byte, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, uploadPartTime)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, 0, errors.New("upload: a bad link")
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", p.off, p.off+p.n-1))
	res, err := u.http.Do(req)
	if err != nil {
		return nil, 0, withoutLink(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent {
		return nil, 0, fmt.Errorf("upload: HTTP %d", res.StatusCode)
	}
	cr := res.Header.Get("Content-Range")
	size, err := strconv.ParseInt(cr[strings.LastIndex(cr, "/")+1:], 10, 64)
	if err != nil || size <= 0 {
		return nil, 0, errors.New("upload: no size")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, p.n+1))
	if err != nil {
		return nil, 0, withoutLink(err)
	}
	if int64(len(b)) != min(p.n, size-p.off) {
		return nil, 0, fmt.Errorf("upload: %d bytes of a part of %d", len(b), p.n)
	}
	return b, size, nil
}

// withoutLink is err without the link it may quote, whose signature is the
// listener's.
func withoutLink(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("upload: %w", ue.Err)
	}
	return err
}

// uploadFile is GET /upload/<id>.m4a: the song, from the byte a Range asks
// for, as it comes in.
func (s *server) uploadFile(w http.ResponseWriter, r *http.Request) {
	id, ok := strings.CutSuffix(r.PathValue("file"), ".m4a")
	if !ok || !uploadID.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	u := s.uploads
	if u.dir != "" {
		if f, err := os.Open(u.keptPath(id)); err == nil {
			defer f.Close()
			now := time.Now()
			_ = os.Chtimes(f.Name(), now, now) // the songs played last are kept longest
			w.Header().Set("Content-Type", "audio/mp4")
			http.ServeContent(w, r, "", time.Time{}, f)
			return
		}
	}
	u.mu.Lock()
	song := u.songs[id]
	u.mu.Unlock()
	if song == nil {
		reply(w, http.StatusNotFound, errorBody{"media", "not handed over"})
		return
	}
	from, to, ok := byteRange(r.Header.Get("Range"), song.size)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", song.size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	w.Header().Set("Content-Type", "audio/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(to-from, 10))
	if r.Header.Get("Range") != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to-1, song.size))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	song.read(r.Context(), u, w, from, to)
}

// byteRange is the bytes a Range header asks for, of size, as [from, to):
// all of them without one. Only a single range is read.
func byteRange(h string, size int64) (int64, int64, bool) {
	if h == "" {
		return 0, size, true
	}
	spec, ok := strings.CutPrefix(h, "bytes=")
	if !ok || strings.Contains(spec, ",") {
		return 0, 0, false
	}
	a, b, _ := strings.Cut(spec, "-")
	var from, to int64 = 0, size
	var err error
	switch {
	case a == "": // the last b bytes
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		from = max(0, size-n)
	default:
		if from, err = strconv.ParseInt(a, 10, 64); err != nil {
			return 0, 0, false
		}
		if b != "" {
			last, err := strconv.ParseInt(b, 10, 64)
			if err != nil || last < from {
				return 0, 0, false
			}
			to = min(size, last+1)
		}
	}
	if from < 0 || from >= size {
		return 0, 0, false
	}
	return from, to, true
}

// partAt is the part that byte off is in.
func (song *upload) partAt(off int64) int {
	return sort.Search(len(song.spans), func(i int) bool { return song.spans[i].off+song.spans[i].n > off })
}

// read writes the song's bytes [from, to) to w as their parts come in, and
// has the parts from where it reads fetched first.
func (song *upload) read(ctx context.Context, u *uploads, w io.Writer, from, to int64) {
	song.mu.Lock()
	song.readers++
	song.want = from
	song.startWorkers(u)
	song.mu.Unlock()
	defer func() {
		song.mu.Lock()
		song.readers--
		last := song.readers == 0
		song.mu.Unlock()
		if last {
			u.finish(song)
		}
	}()
	// A request that goes wakes the wait below.
	stop := context.AfterFunc(ctx, func() {
		song.mu.Lock()
		song.cond.Broadcast()
		song.mu.Unlock()
	})
	defer stop()
	buf := make([]byte, 64<<10)
	for pos := from; pos < to; {
		i := song.partAt(pos)
		song.mu.Lock()
		song.want = pos
		for song.state[i] != partIn && song.err == nil && ctx.Err() == nil {
			song.cond.Wait()
		}
		err := song.err
		song.mu.Unlock()
		if err != nil || ctx.Err() != nil {
			return
		}
		end := min(to, song.spans[i].off+song.spans[i].n)
		for pos < end {
			n, err := song.file.ReadAt(buf[:min(int64(len(buf)), end-pos)], pos)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
				pos += int64(n)
			}
			if err != nil && n == 0 {
				return
			}
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// startWorkers has as many parts coming as there are connections. Under song.mu.
func (song *upload) startWorkers(u *uploads) {
	for song.workers < uploadConnections && !song.whole && song.err == nil {
		song.workers++
		go song.work(u)
	}
}

// next is the part to fetch next: the first wanted from where the latest
// request reads, else the first wanted at all, or -1. Under song.mu.
func (song *upload) next() int {
	start := song.partAt(song.want)
	for i := start; i < len(song.state); i++ {
		if song.state[i] == partWanted {
			return i
		}
	}
	for i := 0; i < start; i++ {
		if song.state[i] == partWanted {
			return i
		}
	}
	return -1
}

// work fetches parts while anyone reads the song, until none is left.
func (song *upload) work(u *uploads) {
	song.mu.Lock()
	defer song.mu.Unlock()
	for {
		i := song.next()
		if i < 0 || song.readers == 0 || song.err != nil {
			song.workers--
			if i < 0 && song.err == nil && !song.whole && song.allIn() {
				song.whole = true
				song.cond.Broadcast()
				if song.readers == 0 {
					go u.finish(song)
				}
			}
			return
		}
		song.state[i] = partComing
		link := song.link
		song.mu.Unlock()
		var b []byte
		var err error
		for range uploadTries {
			if b, _, err = u.fetch(u.ctx, link, song.spans[i]); err == nil {
				_, err = song.file.WriteAt(b, song.spans[i].off)
				break
			}
		}
		song.mu.Lock()
		if err != nil {
			song.state[i] = partWanted
			song.err = err
		} else {
			song.state[i] = partIn
		}
		song.cond.Broadcast()
	}
}

func (song *upload) allIn() bool {
	for _, st := range song.state {
		if st != partIn {
			return false
		}
	}
	return true
}

// finish keeps a song that has all come in and that nobody reads, or lets
// go of one that failed. A song only partly in waits for its next request.
func (u *uploads) finish(song *upload) {
	song.mu.Lock()
	done := song.readers == 0 && song.workers == 0 && (song.whole || song.err != nil)
	whole := song.whole
	song.mu.Unlock()
	if !done {
		return
	}
	u.mu.Lock()
	if u.songs[song.id] != song {
		u.mu.Unlock()
		return
	}
	delete(u.songs, song.id)
	u.mu.Unlock()
	name := song.file.Name()
	song.file.Close()
	if !whole || u.dir == "" || os.Rename(name, u.keptPath(song.id)) != nil {
		os.Remove(name)
		return
	}
	u.trim()
}

// trim keeps the songs played last, uploadsKept of them.
func (u *uploads) trim() {
	kept, _ := filepath.Glob(filepath.Join(u.dir, "upload-*.m4a"))
	if len(kept) <= uploadsKept {
		return
	}
	at := map[string]time.Time{}
	for _, p := range kept {
		if fi, err := os.Stat(p); err == nil {
			at[p] = fi.ModTime()
		}
	}
	sort.Slice(kept, func(i, j int) bool { return at[kept[i]].After(at[kept[j]]) })
	for _, p := range kept[uploadsKept:] {
		os.Remove(p)
	}
}
