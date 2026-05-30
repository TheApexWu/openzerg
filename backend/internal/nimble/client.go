// Package nimble is the Go client for the Nimble.ai SDK that the control
// plane uses for optional CVE-seed lookups at startup. The attacker pods make
// their own Nimble calls via a small shell wrapper baked into the
// pi-attacker image (see backend/docker/pi-attacker/tools/nimble_fetch.sh);
// this package is consumed only by cmd/openzerg.
//
// API surface (PRD M6 acceptance):
//   - FetchRenderedPage(ctx, url, opts) -> RenderedPage  (uses /v1/extract)
//   - SearchWeb(ctx, query, opts)        -> []SearchResult (uses /v1/search)
//
// Hard guarantees:
//   - The API key is never written to slog, never wrapped into errors, and
//     never persisted on returned structs. The TestKeyNeverLogged test in
//     client_test.go is the canary; do not soften it without re-reading the
//     PRD M6 acceptance criteria.
//   - Sentinel errors (errors.Is-able): ErrMissingKey, ErrUpstream.
//   - Every public method takes a context.Context and respects its deadline.
package nimble

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the production Nimble SDK base URL.
const DefaultBaseURL = "https://sdk.nimbleway.com/v1"

// ErrMissingKey is returned when the client is constructed without a key
// and a caller invokes a method that needs one. Callers gate live calls on
// this error via errors.Is rather than scraping error strings.
var ErrMissingKey = errors.New("nimble: NIMBLE_API_KEY not set")

// ErrUpstream wraps non-2xx HTTP responses from the Nimble API. The error
// message embeds the status code; the API key is never embedded.
var ErrUpstream = errors.New("nimble: upstream error")

// Client is a tiny Nimble client safe for concurrent use. The zero value is
// not valid; always construct with New.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// New constructs a Nimble Client. apiKey may be empty: in that case every
// method returns ErrMissingKey (no HTTP request is made). This shape keeps
// the doctor command and --disable-nimble path simple.
func New(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		http:    &http.Client{Timeout: 45 * time.Second},
	}
}

// WithBaseURL returns a shallow clone with baseURL overridden. Used by tests
// to point the client at httptest servers without exporting the field.
func (c *Client) WithBaseURL(baseURL string) *Client {
	clone := *c
	clone.baseURL = baseURL
	return &clone
}

// WithHTTPClient lets callers (mainly tests) inject an http.Client with a
// custom transport, dialer, or timeout. The returned client is a clone.
func (c *Client) WithHTTPClient(httpClient *http.Client) *Client {
	clone := *c
	clone.http = httpClient
	return &clone
}

// HasKey reports whether the client has any API key configured.
func (c *Client) HasKey() bool { return c.apiKey != "" }

// RenderedPage is the typed shape FetchRenderedPage returns. It exposes only
// what the control plane and the attacker tool wrapper actually consume.
// Specifically, the API key never appears here.
type RenderedPage struct {
	URL        string
	StatusCode int
	HTML       string
	Markdown   string
}

// FetchOptions tunes the /v1/extract request. Zero values are sensible
// defaults for SPA and server-rendered targets alike.
type FetchOptions struct {
	// Render forces JavaScript execution. Default true for SPAs.
	Render bool
	// Formats requests specific output formats. Default ["html","markdown"].
	Formats []string
	// Country pins the egress IP geo. Optional.
	Country string
}

// FetchRenderedPage calls /v1/extract and returns the rendered page.
//
// Failure modes (all return non-nil err, no panics):
//   - apiKey empty                  -> ErrMissingKey, zero RenderedPage
//   - targetURL empty               -> validation error, no HTTP call
//   - network failure / ctx cancel  -> wrapped transport error
//   - HTTP 4xx/5xx                  -> wraps ErrUpstream with status code
//   - 200 with malformed JSON       -> json decode error
func (c *Client) FetchRenderedPage(ctx context.Context, targetURL string, opts FetchOptions) (RenderedPage, error) {
	if c.apiKey == "" {
		return RenderedPage{}, ErrMissingKey
	}
	if targetURL == "" {
		return RenderedPage{}, fmt.Errorf("nimble: targetURL is required")
	}
	formats := opts.Formats
	if len(formats) == 0 {
		formats = []string{"html", "markdown"}
	}
	payload := map[string]any{
		"url":     targetURL,
		"render":  opts.Render || true,
		"formats": formats,
	}
	if opts.Country != "" {
		payload["country"] = opts.Country
	}
	var decoded extractResponseEnvelope
	if err := c.post(ctx, "/extract", payload, &decoded); err != nil {
		return RenderedPage{}, err
	}
	return RenderedPage{
		URL:        decoded.URL,
		StatusCode: decoded.StatusCode,
		HTML:       decoded.Data.HTML,
		Markdown:   decoded.Data.Markdown,
	}, nil
}

// SearchResult is one hit from /v1/search.
type SearchResult struct {
	Title   string
	Snippet string
	URL     string
	Content string
}

// SearchOptions tunes /v1/search. Defaults: max_results=5, search_depth=lite.
type SearchOptions struct {
	MaxResults  int
	SearchDepth string // "lite" | "fast" | "deep"
	Focus       string // "general" | "news" | ...
}

// SearchWeb queries Nimble's /v1/search endpoint and returns parsed results.
// Used by the optional --enable-cve-seed startup hook to fetch CVE snippets
// for Gen-1 prompt seeding. Same failure-mode contract as FetchRenderedPage.
func (c *Client) SearchWeb(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	if c.apiKey == "" {
		return nil, ErrMissingKey
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("nimble: query is required")
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}
	depth := opts.SearchDepth
	if depth == "" {
		depth = "lite"
	}
	focus := opts.Focus
	if focus == "" {
		focus = "general"
	}
	payload := map[string]any{
		"query":        query,
		"max_results":  maxResults,
		"search_depth": depth,
		"focus":        focus,
	}
	var decoded searchResponseEnvelope
	if err := c.post(ctx, "/search", payload, &decoded); err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(decoded.Results))
	for _, hit := range decoded.Results {
		out = append(out, SearchResult{
			Title:   hit.Title,
			Snippet: hit.Description,
			URL:     hit.URL,
			Content: hit.Content,
		})
	}
	return out, nil
}

// post is the single networking entry point. Every public method funnels
// through here so the key-handling and error-shape guarantees only need to
// be audited in one place.
func (c *Client) post(ctx context.Context, path string, payload any, decoded any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("nimble: marshal: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("nimble: new request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		// Critical: do NOT wrap err with anything that has touched c.apiKey.
		// http.Client errors carry the request URL but not the auth header.
		return fmt.Errorf("nimble: transport: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("nimble: read body: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Truncate body so error stays bounded; never include c.apiKey.
		return fmt.Errorf("%w: status %d: %s",
			ErrUpstream, response.StatusCode, truncate(string(responseBody), 256))
	}
	if err := json.Unmarshal(responseBody, decoded); err != nil {
		return fmt.Errorf("nimble: decode: %w", err)
	}
	return nil
}

// extractResponseEnvelope mirrors the /v1/extract response shape.
type extractResponseEnvelope struct {
	URL        string `json:"url"`
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code"`
	Data       struct {
		HTML     string `json:"html"`
		Markdown string `json:"markdown"`
	} `json:"data"`
}

// searchResponseEnvelope mirrors the /v1/search response shape.
type searchResponseEnvelope struct {
	TotalResults int `json:"total_results"`
	Results      []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Content     string `json:"content"`
	} `json:"results"`
	RequestID string `json:"request_id"`
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
