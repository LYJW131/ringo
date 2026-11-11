// Package telemetry sends what plays to a collector of the listener's own,
// over OpenTelemetry's OTLP/HTTP, as log events: the same starts, ends and
// jumps that Apple hears of a play, so a listening history can be kept
// outside Apple. Nothing is sent until the listener gives an endpoint, and
// nothing that says whose account it is.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"ringo/internal/music"
)

// Event names, one to each thing that happens to the song that plays.
const (
	EventStart = "ringo.play.start"
	EventEnd   = "ringo.play.end"
	EventSeek  = "ringo.play.seek"
)

// Plays sends what the player says plays. Its zero value sends nothing; a
// nil one too.
type Plays struct {
	// Version is the app's, sent as service.version.
	Version string

	mu       sync.Mutex
	provider *sdklog.LoggerProvider
	logger   log.Logger
	// from is where in the song the part now playing began, for an end to
	// say how much of it was heard.
	song string
	from time.Duration
}

// Configure sends from now on to endpoint, with headers, "k1=v1,k2=v2" as
// OTEL_EXPORTER_OTLP_HEADERS has them; an empty endpoint, or one that can't
// be used, sends nothing. What was waiting to go to the one before is sent
// there first, which takes as long as it does: it's called off the main
// thread.
func (p *Plays) Configure(endpoint, headers string) error {
	if p == nil {
		return nil
	}
	next, err := newProvider(endpoint, headers, p.Version)
	p.mu.Lock()
	old := p.provider
	p.provider = next
	p.logger = nil
	if next != nil {
		p.logger = next.Logger("ringo/plays")
	}
	p.mu.Unlock()
	if old != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		old.Shutdown(ctx)
	}
	return err
}

// newProvider sends to endpoint, or nowhere for an empty one or one that
// can't be used.
func newProvider(endpoint, headers, version string) (*sdklog.LoggerProvider, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, nil
	}
	u, err := endpointURL(endpoint)
	if err != nil {
		return nil, err
	}
	h, err := parseHeaders(headers)
	if err != nil {
		return nil, err
	}
	exp, err := otlploghttp.New(context.Background(), otlploghttp.WithEndpointURL(u), otlploghttp.WithHeaders(h))
	if err != nil {
		return nil, err
	}
	// Only what names the app: no host or process detectors, which would
	// send the computer's and the user's names.
	res := resource.NewSchemaless(semconv.ServiceName("ringo"), semconv.ServiceVersion(version))
	return sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(exp))), nil
}

// endpointURL is the URL logs are posted to: a collector's address alone
// gets OTLP's path for logs, as OTEL_EXPORTER_OTLP_ENDPOINT does.
func endpointURL(endpoint string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%q is not an http:// or https:// address", endpoint)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/logs"
	}
	return u.String(), nil
}

// parseHeaders reads "k1=v1,k2=v2", values percent-encoded or not.
func parseHeaders(s string) (map[string]string, error) {
	h := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			// Not the pair itself, which may hold a key.
			return nil, errors.New("headers are written as key=value, separated by commas")
		}
		if dv, err := url.PathUnescape(strings.TrimSpace(v)); err == nil {
			v = dv
		}
		h[k] = strings.TrimSpace(v)
	}
	return h, nil
}

// Played is the player's word of what happens to the song that plays,
// told on the main thread. It answers at once: the batch is sent after.
func (p *Plays) Played(ev music.PlayEvent) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.logger == nil {
		return
	}
	var r log.Record
	now := time.Now()
	r.SetTimestamp(now)
	r.SetObservedTimestamp(now)
	r.SetSeverity(log.SeverityInfo)
	t := ev.Track
	r.AddAttributes(
		attribute.String("music.track.id", t.ID),
		attribute.String("music.track.name", t.Name),
		attribute.String("music.artist.name", t.ArtistNames()),
		attribute.String("music.album.name", t.Album.Name),
		attribute.Int64("music.track.duration_ms", t.Duration),
		attribute.Int64("music.position_ms", ev.Position.Milliseconds()),
		attribute.String("music.origin.kind", originKinds[t.Origin.Kind]),
		attribute.Bool("music.mode.shuffle", ev.Mode.Shuffle),
		attribute.String("music.mode.repeat", repeatModes[ev.Mode.Repeat]),
		attribute.Bool("music.mode.autoplay", ev.Mode.Autoplay),
	)
	if t.Origin.ID != "" {
		r.AddAttributes(attribute.String("music.origin.id", t.Origin.ID))
	}
	if t.URL != "" {
		r.AddAttributes(attribute.String("music.track.url", t.URL))
	}
	switch ev.Action {
	case music.Started, music.Resumed:
		r.SetEventName(EventStart)
		r.SetBody(attribute.StringValue("start"))
		r.AddAttributes(attribute.Bool("music.resumed", ev.Action == music.Resumed))
		p.song, p.from = t.ID, ev.Position
	case music.Stopped:
		r.SetEventName(EventEnd)
		r.SetBody(attribute.StringValue("end"))
		r.AddAttributes(attribute.String("music.stop.reason", stopReasons[ev.Why]))
		if p.song == t.ID {
			r.AddAttributes(
				attribute.Int64("music.play.from_ms", p.from.Milliseconds()),
				attribute.Int64("music.play.heard_ms", max(0, ev.Position-p.from).Milliseconds()),
			)
		}
		p.song = ""
	case music.Sought:
		r.SetEventName(EventSeek)
		r.SetBody(attribute.StringValue("seek"))
		r.AddAttributes(attribute.Int64("music.seek.from_ms", ev.From.Milliseconds()))
		if p.song == t.ID {
			r.AddAttributes(
				attribute.Int64("music.play.from_ms", p.from.Milliseconds()),
				attribute.Int64("music.play.heard_ms", max(0, ev.From-p.from).Milliseconds()),
			)
			p.from = ev.Position
		}
	}
	p.logger.Emit(context.Background(), r)
}

// Flush waits, up to d, for what was told to be sent, as the app quits.
func (p *Plays) Flush(d time.Duration) {
	if p == nil {
		return
	}
	p.mu.Lock()
	prov := p.provider
	p.mu.Unlock()
	if prov == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	prov.ForceFlush(ctx)
}

// Close stops sending.
func (p *Plays) Close() { p.Configure("", "") }

var originKinds = map[music.OriginKind]string{
	music.FromNothing:  "none",
	music.FromAlbum:    "album",
	music.FromPlaylist: "playlist",
	music.FromStation:  "station",
	music.FromArtist:   "artist",
}

var repeatModes = map[music.RepeatMode]string{
	music.RepeatOff: "off",
	music.RepeatAll: "all",
	music.RepeatOne: "one",
}

var stopReasons = map[music.StopReason]string{
	music.StopPaused:  "paused",
	music.StopEnded:   "ended",
	music.StopNext:    "next",
	music.StopPrev:    "previous",
	music.StopChanged: "changed",
	music.StopQuit:    "quit",
	music.StopOther:   "other",
}
