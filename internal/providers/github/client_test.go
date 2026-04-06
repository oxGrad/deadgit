package github

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
		w.Write([]byte(`[]`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newClient(srv.URL, "token")
	var out []any
	if err := c.get(srv.URL, &out); err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func Test403RateLimit_Retries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newClient(srv.URL, "token")
	var out []any
	if err := c.get(srv.URL, &out); err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func Test500Retry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newClient(srv.URL, "token")
	var out []any
	if err := c.get(srv.URL, &out); err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func Test500ExhaustsRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newClient(srv.URL, "token")
	_, err := c.getRaw(srv.URL)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls.Load() != maxRetries {
		t.Errorf("expected %d calls, got %d", maxRetries, calls.Load())
	}
}

func Test404NoRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newClient(srv.URL, "token")
	_, err := c.getRaw(srv.URL)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if calls.Load() != 1 {
		t.Errorf("404 should not retry, got %d calls", calls.Load())
	}
}

func TestDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not-valid-json`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newClient(srv.URL, "token")
	var out struct{ Name string }
	if err := c.get(srv.URL, &out); err == nil {
		t.Error("expected JSON decode error")
	}
}
