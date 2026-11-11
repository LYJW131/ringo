package telemetry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/proto"

	"ringo/internal/music"
)

// collector is an OTLP/HTTP collector that keeps the records posted to it.
type collector struct {
	mu      sync.Mutex
	path    string
	auth    string
	service string
	records []*logspb.LogRecord
}

func newCollector(t *testing.T) (*collector, string) {
	t.Helper()
	c := &collector{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req collogs.ExportLogsServiceRequest
		if err := proto.Unmarshal(b, &req); err != nil {
			t.Errorf("not OTLP: %v", err)
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		c.path, c.auth = r.URL.Path, r.Header.Get("Authorization")
		for _, rl := range req.ResourceLogs {
			for _, kv := range rl.Resource.GetAttributes() {
				if kv.Key == "service.name" {
					c.service = kv.Value.GetStringValue()
				}
			}
			for _, sl := range rl.ScopeLogs {
				c.records = append(c.records, sl.LogRecords...)
			}
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		b, _ = proto.Marshal(&collogs.ExportLogsServiceResponse{})
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return c, srv.URL
}

func attr(r *logspb.LogRecord, key string) any {
	for _, kv := range r.Attributes {
		if kv.Key == key {
			switch v := kv.Value.Value.(type) {
			case *commonpb.AnyValue_StringValue:
				return v.StringValue
			case *commonpb.AnyValue_IntValue:
				return v.IntValue
			case *commonpb.AnyValue_BoolValue:
				return v.BoolValue
			}
		}
	}
	return nil
}

// A play's start, a jump within it and its end reach the collector at OTLP's
// path for logs, with the headers given, saying what played, from where, and
// how much of each part was heard.
func TestPlaysReachTheCollector(t *testing.T) {
	c, url := newCollector(t)
	p := &Plays{Version: "1.0"}
	if err := p.Configure(url, "Authorization=Bearer%20abc, X-Other = 1"); err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	song := music.Track{ID: "1", Name: "Song", Artists: []music.Artist{{Name: "A"}, {Name: "B"}}, Album: music.Album{Name: "Album"}, Duration: 200000, Origin: music.Origin{Kind: music.FromAlbum, ID: "9"}}
	p.Played(music.PlayEvent{Action: music.Started, Track: song})
	p.Played(music.PlayEvent{Action: music.Sought, Track: song, From: 30 * time.Second, Position: 60 * time.Second})
	p.Played(music.PlayEvent{Action: music.Stopped, Track: song, Position: 90 * time.Second, Why: music.StopNext})
	p.Flush(5 * time.Second)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path != "/v1/logs" || c.auth != "Bearer abc" || c.service != "ringo" {
		t.Errorf("posted to %q with Authorization %q as %q", c.path, c.auth, c.service)
	}
	if len(c.records) != 3 {
		t.Fatalf("%d records, want 3", len(c.records))
	}
	start, seek, end := c.records[0], c.records[1], c.records[2]
	if start.EventName != EventStart || seek.EventName != EventSeek || end.EventName != EventEnd {
		t.Errorf("events %q %q %q", start.EventName, seek.EventName, end.EventName)
	}
	if attr(start, "music.track.id") != "1" || attr(start, "music.artist.name") != song.ArtistNames() || attr(start, "music.origin.kind") != "album" || attr(start, "music.origin.id") != "9" {
		t.Errorf("the start says %v", start.Attributes)
	}
	if attr(seek, "music.play.heard_ms") != int64(30000) || attr(seek, "music.seek.from_ms") != int64(30000) {
		t.Errorf("the jump says %v", seek.Attributes)
	}
	if attr(end, "music.play.heard_ms") != int64(30000) || attr(end, "music.play.from_ms") != int64(60000) || attr(end, "music.stop.reason") != "next" {
		t.Errorf("the end says %v", end.Attributes)
	}
}

// An address that isn't one, or headers that aren't pairs, are refused, and
// what was sent before stops being sent.
func TestPlaysRefuseWhatCantBeUsed(t *testing.T) {
	c, url := newCollector(t)
	p := &Plays{}
	if err := p.Configure(url, ""); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][2]string{{"127.0.0.1:4318", ""}, {url, "Authorization"}} {
		if err := p.Configure(bad[0], bad[1]); err == nil {
			t.Errorf("%q with headers %q kept", bad[0], bad[1])
		}
	}
	p.Played(music.PlayEvent{Action: music.Started, Track: music.Track{ID: "1"}})
	p.Flush(time.Second)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.records) != 0 {
		t.Errorf("%d records sent after the endpoint was refused", len(c.records))
	}
}
