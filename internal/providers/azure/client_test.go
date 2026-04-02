package azure

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func Test429Retry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"value":[]}`))
	}))
	defer srv.Close()

	c := newClient("test-pat")
	var out struct{ Value []interface{} }
	if err := c.get(srv.URL, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func Test404NoRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newClient("test-pat")
	_, err := c.getRaw(srv.URL)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if calls.Load() != 1 {
		t.Errorf("404 should not retry, got %d calls", calls.Load())
	}
}
