package amhelper

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// flakyCDN serves file as Apple's CDN does (ETag, ranges), breaking the line
// partway through the first breaks answers.
func flakyCDN(t *testing.T, file []byte, breaks int, ignoreRange bool) (*httptest.Server, *atomic.Int32) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := n.Add(1)
		w.Header().Set("ETag", `"abc-6"`)
		if ignoreRange && i > 1 {
			r.Header.Del("Range")
		}
		if int(i) <= breaks {
			start := 0
			if rg := r.Header.Get("Range"); rg != "" {
				start, _ = strconv.Atoi(rg[len("bytes=") : len(rg)-1])
			}
			rest := file[start:]
			if start > 0 {
				w.Header().Set("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(len(file)-1)+"/"+strconv.Itoa(len(file)))
				w.Header().Set("Content-Length", strconv.Itoa(len(rest)))
				w.WriteHeader(http.StatusPartialContent)
			} else {
				w.Header().Set("Content-Length", strconv.Itoa(len(rest)))
			}
			_, _ = w.Write(rest[:len(rest)/3])
			w.(http.Flusher).Flush()
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(file))
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func openFlaky(t *testing.T, ctx context.Context, url string) *resumingBody {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return newResumingBody(ctx, http.DefaultClient, url, "1", res)
}

func quickResume(t *testing.T) {
	old := resumeWaits
	resumeWaits = []time.Duration{0, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { resumeWaits = old })
}

func testFile() []byte {
	file := make([]byte, 3<<20)
	for i := range file {
		file[i] = byte(i * 7 % 251)
	}
	return file
}

func TestResumeGoesOnWhereTheLineBroke(t *testing.T) {
	quickResume(t)
	file := testFile()
	srv, n := flakyCDN(t, file, 2, false)
	b := openFlaky(t, context.Background(), srv.URL)
	got, err := io.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, file) {
		t.Fatalf("got %d bytes, not the file's %d", len(got), len(file))
	}
	if n.Load() != 3 {
		t.Fatalf("%d requests, want 3", n.Load())
	}
}

func TestResumeGivesUpOnAChangedFile(t *testing.T) {
	quickResume(t)
	srv, _ := flakyCDN(t, testFile(), 1, true)
	_, err := io.ReadAll(openFlaky(t, context.Background(), srv.URL))
	var changed errFileChanged
	if !errors.As(err, &changed) {
		t.Fatalf("err = %v, want the file changed", err)
	}
}

func TestResumeGivesUpWhenTheRestNeverComes(t *testing.T) {
	quickResume(t)
	file := testFile()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) > 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(file)))
		_, _ = w.Write(file[:1000])
		w.(http.Flusher).Flush()
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	_, err := io.ReadAll(openFlaky(t, context.Background(), srv.URL))
	if err == nil {
		t.Fatal("read the file whole from a line that never comes back")
	}
	if got, want := int(n.Load()), 1+len(resumeWaits); got != want {
		t.Fatalf("%d requests, want %d", got, want)
	}
}

func TestResumeStopsWhenTheAppHangsUp(t *testing.T) {
	old := resumeWaits
	resumeWaits = []time.Duration{0, time.Hour}
	t.Cleanup(func() { resumeWaits = old })
	srv, _ := flakyCDN(t, testFile(), 100, false)
	ctx, cancel := context.WithCancel(context.Background())
	b := openFlaky(t, ctx, srv.URL)
	time.AfterFunc(100*time.Millisecond, cancel)
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(b); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("still trying after the app hung up")
	}
}
