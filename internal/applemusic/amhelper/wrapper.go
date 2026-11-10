package amhelper

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mygo-music/internal/applemusic/fairplay"
)

// wrapper talks to a wrapper-manager (or a wrapper-lite of AMDL-Web's fork):
// the master playlist and the key templates. Media comes from Apple's CDN.
type wrapper struct {
	base  *url.URL
	token string
	http  *http.Client

	queue *queue
	store *store
	ctx   context.Context // work that outlives its request ends with it; Start sets it

	mu        sync.Mutex
	templates *lru[*fairplay.Template]
	inflight  map[string]*templateCall

	// Whether it gives its accounts' tokens (account.go).
	tokens atomic.Int32

	// When the wrapper was last asked something /status isn't, in Unix ns.
	asked atomic.Int64
}

type templateCall struct {
	done chan struct{}
	t    *fairplay.Template
	err  error
}

// prefetchKey opens every enhanced-HLS track; wrapper-lite serves it for
// adamId 0 only, and it never changes.
const prefetchKey = "skd://itunes.apple.com/P000000000/s1/e1"

func newWrapper(address, token string, st *store) (*wrapper, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(address), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("not a wrapper address: %q", address)
	}
	return &wrapper{
		base:      u,
		token:     token,
		http:      &http.Client{Timeout: 30 * time.Second},
		ctx:       context.Background(),
		queue:     newQueue(),
		store:     st,
		templates: newLRU[*fairplay.Template](64),
		inflight:  map[string]*templateCall{},
	}, nil
}

// errWrapper is the wrapper failing, as opposed to the song lacking ALAC.
var errWrapper = errors.New("wrapper")

// call GETs endpoint and returns the data of wrapper-lite's {code, msg, data}.
func (w *wrapper) call(ctx context.Context, endpoint string, query url.Values) (json.RawMessage, error) {
	target := w.base.JoinPath(endpoint)
	target.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	// /status and /token don't wait behind songs, and don't wake a wrapper
	// on Cloudflare, whose gateway answers them from its cache.
	if endpoint != "/status" && endpoint != "/token" {
		release, err := w.queue.acquire(ctx, query.Get("adamId"))
		if err != nil {
			return nil, err
		}
		defer release()
		w.asked.Store(time.Now().UnixNano())
	}
	if w.token != "" {
		req.Header.Set("Authorization", "Bearer "+w.token)
	}
	res, err := w.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", errWrapper, endpoint, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", errWrapper, endpoint, err)
	}
	var reply struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if res.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("%w %s: the key was refused", errWrapper, endpoint)
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		if res.StatusCode == http.StatusNotFound {
			// A route this wrapper doesn't have.
			return nil, &replyError{endpoint: endpoint, status: res.StatusCode, msg: "not found"}
		}
		return nil, fmt.Errorf("%w %s: HTTP %d: %.200s", errWrapper, endpoint, res.StatusCode, raw)
	}
	if res.StatusCode != http.StatusOK || reply.Code != 0 {
		return nil, &replyError{endpoint: endpoint, status: res.StatusCode, code: reply.Code, msg: reply.Msg}
	}
	return reply.Data, nil
}

type replyError struct {
	endpoint     string
	status, code int
	msg          string
}

func (e *replyError) Error() string {
	return fmt.Sprintf("wrapper %s: HTTP %d, code %d: %s", e.endpoint, e.status, e.code, e.msg)
}

// status is what the app shows: whether there is an account to play with.
type status struct {
	Ready   bool     `json:"ready"`
	Regions []string `json:"regions"`
}

func (w *wrapper) status(ctx context.Context) (status, error) {
	data, err := w.call(ctx, "/status", nil)
	if err != nil {
		return status{}, err
	}
	var s status
	if err := json.Unmarshal(data, &s); err != nil {
		return status{}, fmt.Errorf("%w /status: %w", errWrapper, err)
	}
	return s, nil
}

// master returns adamID's enhanced-HLS master playlist URL.
func (w *wrapper) master(ctx context.Context, adamID string) (string, error) {
	data, err := w.call(ctx, "/m3u8", url.Values{"adamId": {adamID}})
	if err != nil {
		return "", err
	}
	var body struct {
		M3U8 string `json:"m3u8"`
	}
	if err := json.Unmarshal(data, &body); err != nil || body.M3U8 == "" {
		return "", fmt.Errorf("%w /m3u8: no playlist for %s", errWrapper, adamID)
	}
	return body.M3U8, nil
}

// keyRetryDelays: wrapper-lite fails /key a few times in a row around its
// periodic token refresh.
var keyRetryDelays = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// template returns key's decrypter, from memory, the disk or the wrapper.
// Callers asking for the same key at once share one request.
func (w *wrapper) template(ctx context.Context, adamID, key string) (*fairplay.Template, error) {
	if key == prefetchKey {
		adamID = "0"
	}
	w.mu.Lock()
	if t, ok := w.templates.get(key); ok {
		w.mu.Unlock()
		return t, nil
	}
	if raw := w.store.key(key); raw != nil {
		if t, err := fairplay.ParseTemplate(raw); err == nil {
			w.templates.put(key, t)
			w.mu.Unlock()
			return t, nil
		}
		w.store.dropKey(key)
	}
	call, ok := w.inflight[key]
	if !ok {
		call = &templateCall{done: make(chan struct{})}
		w.inflight[key] = call
		go func() {
			// Detached from ctx: a listener skipping the song shouldn't waste
			// the key another play of it will want.
			base := w.ctx
			if isAhead(ctx) {
				base = ahead(base)
			}
			fetchCtx, cancel := context.WithTimeout(base, time.Minute)
			defer cancel()
			var raw []byte
			call.t, raw, call.err = w.fetchTemplate(fetchCtx, adamID, key)
			w.mu.Lock()
			delete(w.inflight, key)
			if call.err == nil {
				w.templates.put(key, call.t)
				w.store.putKey(key, raw)
			}
			w.mu.Unlock()
			close(call.done)
		}()
	}
	w.mu.Unlock()
	select {
	case <-call.done:
		return call.t, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// awake is how long a wrapper stays up after a request. A wrapper-manager on
// Cloudflare sleeps after five idle minutes, and the request that wakes it
// waits for the container to start and its accounts to be restored: some
// seconds, which the listener's first song shouldn't spend.
const awake = 4 * time.Minute

// wake asks the wrapper for the key every track opens with, unless it was
// asked something within awake, and reports whether it did. On Cloudflare
// that starts the container while the listener browses; /status wouldn't, as
// the gateway answers it from its cache. The answer is kept, fresh.
func (w *wrapper) wake() bool {
	last := w.asked.Load()
	if time.Since(time.Unix(0, last)) < awake || !w.asked.CompareAndSwap(last, time.Now().UnixNano()) {
		return false
	}
	go func() {
		ctx, cancel := context.WithTimeout(w.ctx, 2*time.Minute)
		defer cancel()
		t, raw, err := w.fetchTemplate(ctx, "0", prefetchKey)
		if err != nil {
			log.Printf("amhelper: waking the wrapper: %v", err)
			return
		}
		w.mu.Lock()
		w.templates.put(prefetchKey, t)
		w.mu.Unlock()
		w.store.putKey(prefetchKey, raw)
	}()
	return true
}

// forget drops key's template, which decrypted to noise, from memory and
// the disk, so that the next ask goes to the wrapper.
func (w *wrapper) forget(key string) {
	w.mu.Lock()
	w.templates.remove(key)
	w.mu.Unlock()
	w.store.dropKey(key)
}

func (w *wrapper) fetchTemplate(ctx context.Context, adamID, key string) (*fairplay.Template, []byte, error) {
	for attempt := 0; ; attempt++ {
		data, err := w.call(ctx, "/key", url.Values{"adamId": {adamID}, "uri": {key}})
		if err == nil {
			t, perr := fairplay.ParseTemplate(data)
			if perr == nil {
				return t, data, nil
			}
			err = perr
		}
		if attempt == len(keyRetryDelays) || ctx.Err() != nil {
			return nil, nil, fmt.Errorf("key %s: %w", key, err)
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(keyRetryDelays[attempt]):
		}
	}
}

// lru keeps the n values used last. Not safe for concurrent use.
type lru[V any] struct {
	n     int
	order *list.List
	items map[string]*list.Element
}

type lruEntry[V any] struct {
	key string
	v   V
}

func newLRU[V any](n int) *lru[V] {
	return &lru[V]{n: n, order: list.New(), items: map[string]*list.Element{}}
}

func (c *lru[V]) get(key string) (V, bool) {
	if e, ok := c.items[key]; ok {
		c.order.MoveToFront(e)
		return e.Value.(lruEntry[V]).v, true
	}
	var zero V
	return zero, false
}

func (c *lru[V]) remove(key string) {
	if e, ok := c.items[key]; ok {
		c.order.Remove(e)
		delete(c.items, key)
	}
}

// all returns every entry, the one used last first.
func (c *lru[V]) all() map[string]V {
	out := make(map[string]V, len(c.items))
	for e := c.order.Front(); e != nil; e = e.Next() {
		out[e.Value.(lruEntry[V]).key] = e.Value.(lruEntry[V]).v
	}
	return out
}

func (c *lru[V]) put(key string, v V) {
	if e, ok := c.items[key]; ok {
		e.Value = lruEntry[V]{key, v}
		c.order.MoveToFront(e)
		return
	}
	c.items[key] = c.order.PushFront(lruEntry[V]{key, v})
	for c.order.Len() > c.n {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(lruEntry[V]).key)
	}
}
