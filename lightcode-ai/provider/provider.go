// Package ai is the app's assistant client: an OpenAI-compatible chat
// completions stream, read event by event so the panel shows text as it
// arrives. The key comes from the system's store (mygo.Secrets) and the
// HTTP client follows the desktop's proxy settings.
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrNoKey is returned when no API key is configured.
var ErrNoKey = errors.New("no API key")

// Message is one turn of a conversation.
type Message struct {
	// Role is "system", "user" or "assistant".
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Config is where and how to ask.
type Config struct {
	// BaseURL ends before /chat/completions, as https://api.openai.com/v1.
	BaseURL string
	Model   string
	APIKey  string
	// Temperature is the sampling temperature, 0 for the API's default.
	Temperature float32
	// MaxTokens caps the reply, 0 for the API's default.
	MaxTokens int
}

// DefaultConfig is the OpenAI endpoint with a small, cheap model.
func DefaultConfig() Config {
	return Config{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini", Temperature: 0.2}
}

// Normalize fills what the config leaves out.
func (c Config) Normalize() Config {
	if c.BaseURL == "" {
		c.BaseURL = DefaultConfig().BaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.Model == "" {
		c.Model = DefaultConfig().Model
	}
	return c
}

// Client streams replies from an OpenAI-compatible endpoint.
type Client struct {
	cfg  Config
	http *http.Client
}

// New makes a client; it follows the proxy environment variables and
// leaves timeouts to the context.
func New(cfg Config) *Client {
	return &Client{
		cfg: cfg.Normalize(),
		// The default transport follows HTTP(S)_PROXY; the editor's own
		// system-proxy setting is not carried across the process boundary.
		http: &http.Client{},
	}
}

// Config is the client's configuration.
func (c *Client) Config() Config { return c.cfg }

// Stream asks for a reply to msgs and calls onDelta with each new piece of
// text; it returns the whole reply. Cancel ctx to stop: the call fails
// with ctx's error.
func (c *Client) Stream(ctx context.Context, msgs []Message, onDelta func(string)) (string, error) {
	if c.cfg.APIKey == "" {
		return "", ErrNoKey
	}
	stream := true
	body := map[string]any{
		"model":       c.cfg.Model,
		"messages":    msgs,
		"stream":      stream,
		"temperature": c.cfg.Temperature,
	}
	if c.cfg.MaxTokens > 0 {
		body["max_tokens"] = c.cfg.MaxTokens
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("AI 服务返回 %s：%s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		if event.Error != nil {
			return full.String(), errors.New(event.Error.Message)
		}
		if len(event.Choices) == 0 {
			continue
		}
		piece := event.Choices[0].Delta.Content
		if piece == "" {
			continue
		}
		full.WriteString(piece)
		if onDelta != nil {
			onDelta(piece)
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return full.String(), ctx.Err()
		}
		return full.String(), err
	}
	if ctx.Err() != nil {
		return full.String(), ctx.Err()
	}
	return full.String(), nil
}
