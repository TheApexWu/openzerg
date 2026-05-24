// Package openrouter is a thin client for OpenRouter's chat-completions
// API. It is used by the control plane for the optional LLM-driven mutation
// step. Free-tier models are deliberately not pinned here; callers supply
// the model ID per call so the paid google/gemma-4 variants from PRD.json
// can be passed in.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const DefaultBaseURL = "https://openrouter.ai/api/v1"

// Client is a minimal chat-completions client. It is safe for concurrent use
// by multiple goroutines because http.Client is.
type Client struct {
	APIKey      string
	BaseURL     string
	HTTP        *http.Client
	UserAgent   string
	// HTTPReferer and Title populate the optional OpenRouter ranking
	// headers documented at https://openrouter.ai/docs/quickstart.
	HTTPReferer string
	AppTitle    string
}

// New constructs a Client with sensible defaults. The caller must supply a
// non-empty APIKey; callers without a key should not invoke the LLM mutation
// path at all.
//
// The HTTP client has no timeout: model calls run until the agent finishes
// or the parent context is cancelled. The caller controls deadlines via the
// context passed to CreateChatCompletion.
func New(apiKey string) *Client {
	return &Client{
		APIKey:    apiKey,
		BaseURL:   DefaultBaseURL,
		HTTP:      &http.Client{}, // no timeout; caller controls via ctx
		UserAgent: "openzerg-control-plane/0.1",
		AppTitle:  "OpenZerg",
	}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Index   int         `json:"index"`
		Message ChatMessage `json:"message"`
		Finish  string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// FirstMessageContent is the convenience accessor most callers want; it
// returns the assistant message text from the first choice, or empty string.
func (response ChatResponse) FirstMessageContent() string {
	if len(response.Choices) == 0 {
		return ""
	}
	return response.Choices[0].Message.Content
}

// CreateChatCompletion POSTs to /chat/completions and returns the decoded
// response. Non-2xx responses become errors that include the response body
// for debugging (OpenRouter returns a JSON error object).
func (c *Client) CreateChatCompletion(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if c.APIKey == "" {
		return ChatResponse{}, fmt.Errorf("openrouter: missing API key")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openrouter: marshal: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openrouter: new request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		httpRequest.Header.Set("User-Agent", c.UserAgent)
	}
	if c.HTTPReferer != "" {
		httpRequest.Header.Set("HTTP-Referer", c.HTTPReferer)
	}
	if c.AppTitle != "" {
		httpRequest.Header.Set("X-Title", c.AppTitle)
	}

	httpResponse, err := c.HTTP.Do(httpRequest)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openrouter: http: %w", err)
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("openrouter: read body: %w", err)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return ChatResponse{}, fmt.Errorf("openrouter: status %d: %s",
			httpResponse.StatusCode, truncate(string(responseBody), 512))
	}
	var decoded ChatResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return ChatResponse{}, fmt.Errorf("openrouter: decode: %w (body=%s)", err, truncate(string(responseBody), 256))
	}
	return decoded, nil
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
