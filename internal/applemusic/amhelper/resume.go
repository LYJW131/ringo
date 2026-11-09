package amhelper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// resumeWaits is how long to wait before each try at the rest of a file
// whose line broke; past the last, the song fails. A song comes down many
// times faster than it plays, so the app has seconds of audio to spare.
var resumeWaits = []time.Duration{0, 500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second}

// resumingBody reads Apple's media file and, when the line breaks midway (a
// network that drops a connection, a proxy, Apple resetting the stream),
// asks for the rest with a Range request and goes on, so that the song plays
// through instead of ending where the line broke.
type resumingBody struct {
	ctx    context.Context
	client *http.Client
	url    string
	id     string
	etag   string
	size   int64 // -1 when Apple didn't say
	body   io.ReadCloser
	offset int64
	// failed counts the tries since the file last came on.
	failed int
}

func newResumingBody(ctx context.Context, client *http.Client, url, id string, res *http.Response) *resumingBody {
	return &resumingBody{ctx: ctx, client: client, url: url, id: id, etag: res.Header.Get("ETag"), size: res.ContentLength, body: res.Body}
}

func (b *resumingBody) Read(p []byte) (int, error) {
	for {
		n, err := b.body.Read(p)
		b.offset += int64(n)
		if n > 0 {
			b.failed = 0
		}
		switch {
		case err == nil:
			return n, nil
		case err == io.EOF && (b.size < 0 || b.offset >= b.size):
			return n, io.EOF
		case b.ctx.Err() != nil:
			return n, b.ctx.Err()
		}
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		if rerr := b.resume(err); rerr != nil {
			return n, rerr
		}
		if n > 0 {
			return n, nil
		}
	}
}

// resume asks for the file from where it broke, waiting a little longer
// each time it doesn't come.
func (b *resumingBody) resume(cause error) error {
	b.body.Close()
	b.body = io.NopCloser(strings.NewReader(""))
	for b.failed < len(resumeWaits) {
		wait := resumeWaits[b.failed]
		b.failed++
		log.Printf("amhelper: song %s: the line broke at %d of %d bytes (%v); asking for the rest, try %d", b.id, b.offset, b.size, cause, b.failed)
		select {
		case <-time.After(wait):
		case <-b.ctx.Done():
			return b.ctx.Err()
		}
		body, err := b.rest()
		if err == nil {
			b.body = body
			return nil
		}
		if b.ctx.Err() != nil {
			return b.ctx.Err()
		}
		var changed errFileChanged
		if errors.As(err, &changed) {
			return err
		}
		cause = err
	}
	return fmt.Errorf("the line broke at %d of %d bytes and the rest didn't come: %w", b.offset, b.size, cause)
}

type errFileChanged struct{ status int }

func (e errFileChanged) Error() string {
	return fmt.Sprintf("the rest of the file: HTTP %d, not the part asked for", e.status)
}

func (b *resumingBody) rest() (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(b.ctx, http.MethodGet, b.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", "bytes="+strconv.FormatInt(b.offset, 10)+"-")
	if b.etag != "" {
		// A file changed since would come whole, as a 200: no use mid-song.
		req.Header.Set("If-Range", b.etag)
	}
	res, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusPartialContent {
		res.Body.Close()
		if res.StatusCode == http.StatusOK {
			return nil, errFileChanged{res.StatusCode}
		}
		return nil, fmt.Errorf("the rest of the file: HTTP %d", res.StatusCode)
	}
	if !strings.HasPrefix(res.Header.Get("Content-Range"), "bytes "+strconv.FormatInt(b.offset, 10)+"-") {
		res.Body.Close()
		return nil, errFileChanged{res.StatusCode}
	}
	return res.Body, nil
}

func (b *resumingBody) Close() error { return b.body.Close() }
