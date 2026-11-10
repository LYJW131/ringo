package player

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"ringo/internal/music"
)

// onServer answers every track as the same song, of which the server sends a
// quarter, and the rest once rest closes.
type onServer struct {
	nothing
	url string
}

func (s onServer) Stream(string, string) (*music.Stream, error) {
	return &music.Stream{URL: s.url, Type: "m4a"}, nil
}

// A seek past what has come in says it's loading until it has, as the stream
// waits for it: it said nothing, and the song only went quiet.
func TestSeekPastWhatsInLoads(t *testing.T) {
	b, err := os.ReadFile("testdata/seek-aac-frag.m4a")
	if err != nil {
		t.Fatal(err)
	}
	rest := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(b[:len(b)/4])
		w.(http.Flusher).Flush()
		<-rest
		w.Write(b[len(b)/4:])
	}))
	defer srv.Close()
	released := false
	release := func() {
		if !released {
			released = true
			close(rest)
		}
	}
	defer release()

	ch := make(chan func(), 64)
	p := New(func(fn func()) { ch <- fn })
	p.SetVolume(0)
	var clock atomic.Int64
	clock.Store(time.Unix(1, 0).UnixNano())
	p.now = func() time.Time { return time.Unix(0, clock.Load()) }
	p.Source = onServer{url: srv.URL}
	defer p.stop()
	p.PlayQueue([]music.Track{{ID: "a", Duration: 5000}}, 0, music.Origin{})
	settle(t, ch, p)

	p.Seek(4 * time.Second)
	// The seek lands once what plays has faded out, and the stream waits.
	eventually(t, "the seek didn't land", func() bool {
		select {
		case fn := <-ch:
			fn()
		default:
		}
		return p.watch.since.Load() != 0 && !p.seeking
	})
	clock.Add(int64(starvedAfter))
	if !p.Loading() {
		t.Fatal("doesn't say it's loading")
	}
	release()
	eventually(t, "still says it's loading", func() bool { return !p.Loading() })
}
