package amhelper

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestWakeAsksOnceWhileTheWrapperIsAwake(t *testing.T) {
	var keys atomic.Int32
	asked := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/key" {
			keys.Add(1)
			asked <- r.URL.Query().Get("adamId") + " " + r.URL.Query().Get("uri") + " " + r.Header.Get("Authorization")
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{}}`))
	}))
	defer srv.Close()
	w, err := newWrapper(srv.URL, "k", &store{dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !w.wake() {
		t.Fatal("a wrapper never asked wasn't woken")
	}
	select {
	case got := <-asked:
		if got != "0 "+prefetchKey+" Bearer k" {
			t.Fatalf("asked %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the wrapper wasn't asked")
	}
	if w.wake() {
		t.Fatal("woken again while awake")
	}
	// Asked something else lately: awake too. Long ago: asleep.
	w.asked.Store(time.Now().Add(-awake - time.Second).UnixNano())
	if _, err := w.status(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !w.wake() {
		t.Fatal("/status kept it awake, but the gateway answers it from its cache")
	}
}
