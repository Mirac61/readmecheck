package rules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPLinkCheckerStatuses(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/missing", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/forbidden", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	mux.HandleFunc("/server-error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	mux.HandleFunc("/head-not-allowed", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok", http.StatusFound)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	checker := NewHTTPLinkChecker()
	checker.Client = server.Client()
	checker.Workers = 4
	checker.Timeout = time.Second
	urls := []string{
		server.URL + "/ok",
		server.URL + "/missing",
		server.URL + "/forbidden",
		server.URL + "/server-error",
		server.URL + "/head-not-allowed",
		server.URL + "/redirect",
		server.URL + "/missing",
	}
	results := checker.Check(context.Background(), urls)
	if len(results) != len(urls)-1 {
		t.Fatalf("results = %d, want %d after deduplication", len(results), len(urls)-1)
	}
	byURL := make(map[string]LinkCheck)
	for _, result := range results {
		byURL[result.URL] = result
	}
	tests := []struct {
		path string
		ok   bool
		code int
	}{
		{"/ok", true, 200},
		{"/missing", false, 404},
		{"/forbidden", true, 403},
		{"/server-error", false, 500},
		{"/head-not-allowed", true, 200},
		{"/redirect", true, 200},
	}
	for _, tt := range tests {
		result := byURL[server.URL+tt.path]
		if result.OK != tt.ok || result.StatusCode != tt.code {
			t.Errorf("%s = %+v, want ok=%v code=%d", tt.path, result, tt.ok, tt.code)
		}
	}
}

func TestHTTPLinkCheckerConcurrencyLimit(t *testing.T) {
	t.Parallel()
	var active, maximum atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	checker := NewHTTPLinkChecker()
	checker.Client = server.Client()
	checker.Workers = 2
	checker.Timeout = time.Second
	urls := make([]string, 8)
	for i := range urls {
		urls[i] = server.URL + "/?i=" + string(rune('a'+i))
	}
	results := checker.Check(context.Background(), urls)
	if len(results) != 8 {
		t.Fatalf("results = %d", len(results))
	}
	for _, result := range results {
		if !result.OK {
			t.Fatalf("result = %+v", result)
		}
	}
	if got := maximum.Load(); got > 2 {
		t.Fatalf("maximum concurrent requests = %d, want <= 2", got)
	}
}

func TestHTTPLinkCheckerTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	checker := NewHTTPLinkChecker()
	checker.Client = server.Client()
	checker.Workers = 1
	checker.Timeout = 10 * time.Millisecond
	results := checker.Check(context.Background(), []string{server.URL})
	if len(results) != 1 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].OK || results[0].Err != "timed out" {
		t.Fatalf("result = %+v", results[0])
	}
}

func TestHTTPLinkCheckerMaxLinks(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	checker := NewHTTPLinkChecker()
	checker.MaxLinks = 2
	checker.Client = server.Client()
	results := checker.Check(context.Background(), []string{
		server.URL + "/a",
		server.URL + "/b",
		server.URL + "/c",
	})
	if len(results) != 2 {
		t.Fatalf("results = %+v", results)
	}
}
