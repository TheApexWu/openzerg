// Tests per PRD data_contracts.nimble_test_schema. Light coverage of the
// four failure modes plus the hard-requirement key-leak canary.

package nimble

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newFakeNimbleServer returns an httptest.Server that responds to /extract
// and /search with the supplied status and body. Single-purpose helper kept
// tiny per PRD.
func newFakeNimbleServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

func TestFetchRenderedPage_MissingKey(t *testing.T) {
	httpHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		httpHits++
	}))
	defer server.Close()

	client := New("").WithBaseURL(server.URL)
	_, err := client.FetchRenderedPage(context.Background(), "https://example.com", FetchOptions{})
	if !errors.Is(err, ErrMissingKey) {
		t.Fatalf("expected ErrMissingKey, got %v", err)
	}
	if httpHits != 0 {
		t.Fatalf("client made %d HTTP requests without a key; want 0", httpHits)
	}
}

func TestFetchRenderedPage_HappyPath(t *testing.T) {
	cannedBody := `{
	  "url": "https://example.com/",
	  "task_id": "abc",
	  "status": "success",
	  "status_code": 200,
	  "data": {"html": "<html>ok</html>", "markdown": "ok"}
	}`
	server := newFakeNimbleServer(t, 200, cannedBody)
	defer server.Close()

	client := New("test-key").WithBaseURL(server.URL)
	page, err := client.FetchRenderedPage(context.Background(), "https://example.com", FetchOptions{Render: true})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if page.URL != "https://example.com/" || page.StatusCode != 200 ||
		page.HTML == "" || page.Markdown == "" {
		t.Fatalf("unexpected RenderedPage: %+v", page)
	}
}

func TestFetchRenderedPage_NetworkError(t *testing.T) {
	// Point at a closed port; net.Dial returns "connection refused" fast.
	client := New("test-key").WithBaseURL("http://127.0.0.1:1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.FetchRenderedPage(ctx, "https://example.com", FetchOptions{})
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "nimble") {
		t.Fatalf("error should mention nimble for traceability: %v", err)
	}
}

func TestFetchRenderedPage_MalformedResponse(t *testing.T) {
	server := newFakeNimbleServer(t, 200, "not json at all {{{")
	defer server.Close()

	client := New("test-key").WithBaseURL(server.URL)
	_, err := client.FetchRenderedPage(context.Background(), "https://example.com", FetchOptions{})
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("expected json.SyntaxError wrapped in err, got %v", err)
	}
}

func TestFetchRenderedPage_HTTP5xx(t *testing.T) {
	for _, status := range []int{500, 502, 503} {
		statusCopy := status
		t.Run(http.StatusText(statusCopy), func(t *testing.T) {
			server := newFakeNimbleServer(t, statusCopy, `{"status":"failed","msg":"boom"}`)
			defer server.Close()

			client := New("test-key").WithBaseURL(server.URL)
			_, err := client.FetchRenderedPage(context.Background(), "https://example.com", FetchOptions{})
			if !errors.Is(err, ErrUpstream) {
				t.Fatalf("expected ErrUpstream, got %v", err)
			}
			if !strings.Contains(err.Error(), "status ") {
				t.Fatalf("error should embed status code: %v", err)
			}
		})
	}
}

// TestKeyNeverLogged is the hard-requirement canary from PRD M6 acceptance.
// It runs FetchRenderedPage with a known sentinel API key against a server
// that returns 500 (the failure path most likely to log auth headers), then
// asserts the sentinel never appears in slog output or the returned error.
func TestKeyNeverLogged(t *testing.T) {
	const sentinel = "test-sentinel-xyz123"

	server := newFakeNimbleServer(t, 500, `{"status":"failed","msg":"upstream sad"}`)
	defer server.Close()

	// Swap the default slog handler so any incidental Info/Error call inside
	// the package is captured. We restore it on test exit.
	var logBuffer bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuffer, nil)))
	defer slog.SetDefault(previousLogger)

	client := New(sentinel).WithBaseURL(server.URL)
	_, err := client.FetchRenderedPage(context.Background(), "https://example.com", FetchOptions{})
	if err == nil {
		t.Fatal("expected upstream error, got nil")
	}
	if strings.Contains(logBuffer.String(), sentinel) {
		t.Fatalf("slog output leaked the API key sentinel")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("error message leaked the API key sentinel: %v", err)
	}
}

func TestSearchWeb_HappyPath(t *testing.T) {
	cannedBody := `{
	  "total_results": 3,
	  "results": [
	    {"title":"a","description":"snip-a","url":"https://a","content":""},
	    {"title":"b","description":"snip-b","url":"https://b","content":""},
	    {"title":"c","description":"snip-c","url":"https://c","content":""}
	  ],
	  "request_id":"req-1"
	}`
	server := newFakeNimbleServer(t, 200, cannedBody)
	defer server.Close()

	client := New("test-key").WithBaseURL(server.URL)
	results, err := client.SearchWeb(context.Background(), "owasp juice shop cve", SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Title == "" || r.Snippet == "" {
			t.Fatalf("unexpected result: %+v", r)
		}
	}
}
