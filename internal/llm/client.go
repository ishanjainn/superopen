package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
)

// Client is a minimal chat client (OpenAI-compatible + Anthropic Messages).
type Client struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

func New(provider, model, apiKey string) *Client {
	return NewFromResolved(config.ResolvedLLM{
		Provider: provider,
		Model:    model,
		APIKey:   apiKey,
	})
}

// NewFromConfig builds a client from harness config + env autodetection.
func NewFromConfig(cfg config.Config) *Client {
	return NewFromResolved(cfg.ResolveLLM())
}

func NewFromResolved(r config.ResolvedLLM) *Client {
	provider := strings.ToLower(r.Provider)
	base := strings.TrimRight(r.BaseURL, "/")
	if base == "" {
		switch provider {
		case "anthropic":
			base = "https://api.anthropic.com/v1"
		case "openrouter":
			base = "https://openrouter.ai/api/v1"
		case "local", "ollama":
			base = "http://127.0.0.1:11434/v1"
		default:
			base = "https://api.openai.com/v1"
		}
	}
	// Env still wins for OpenAI-compatible base when not anthropic.
	if provider != "anthropic" {
		if v := os.Getenv("SUPEROPEN_LLM_BASE_URL"); v != "" {
			base = strings.TrimRight(v, "/")
		} else if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
			base = strings.TrimRight(v, "/")
		}
	}
	return &Client{Provider: provider, Model: r.Model, APIKey: r.APIKey, BaseURL: base}
}

func (c *Client) Available() bool {
	if c == nil {
		return false
	}
	if c.Provider == "local" || c.Provider == "ollama" {
		return c.BaseURL != ""
	}
	return c.APIKey != ""
}

type Options struct {
	MaxTokens int
	Timeout   time.Duration
}

type chatReq struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResp struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (c *Client) Complete(system, user string) (string, error) {
	return c.CompleteOpts(system, user, Options{MaxTokens: 2048, Timeout: 60 * time.Second})
}

func (c *Client) CompleteOpts(system, user string, opts Options) (string, error) {
	if !c.Available() {
		return "", fmt.Errorf("LLM not configured - set OPENAI_API_KEY, ANTHROPIC_API_KEY, OPENROUTER_API_KEY, or OPENAI_BASE_URL for a local/gateway server")
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 2048
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if c.Provider == "anthropic" {
		return c.completeAnthropic(system, user, opts)
	}
	return c.completeOpenAICompat(system, user, opts)
}

func (c *Client) completeOpenAICompat(system, user string, opts Options) (string, error) {
	body, _ := json.Marshal(chatReq{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens:   opts.MaxTokens,
		Temperature: 0.2,
	})
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Provider == "openrouter" {
		req.Header.Set("HTTP-Referer", "https://github.com/ishanjainn/superopen")
		req.Header.Set("X-Title", "Superopen")
	}
	httpClient := &http.Client{Timeout: opts.Timeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("llm http %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	var cr chatResp
	if err := json.Unmarshal(data, &cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("empty llm response")
	}
	return cr.Choices[0].Message.Content, nil
}

func (c *Client) completeAnthropic(system, user string, opts Options) (string, error) {
	payload := map[string]any{
		"model":       c.Model,
		"max_tokens":  opts.MaxTokens,
		"temperature": 0.2,
		"system":      system,
		"messages":    []map[string]string{{"role": "user", "content": user}},
	}
	body, _ := json.Marshal(payload)
	url := c.BaseURL
	if !strings.HasSuffix(url, "/messages") {
		url = strings.TrimRight(url, "/") + "/messages"
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	httpClient := &http.Client{Timeout: opts.Timeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("anthropic http %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	var parsed struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Content) == 0 {
		return "", fmt.Errorf("empty anthropic response")
	}
	return parsed.Content[0].Text, nil
}

// ExtractJSON pulls the first JSON object/array from an LLM reply (handles fences).
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	startObj := strings.Index(s, "{")
	startArr := strings.Index(s, "[")
	start := -1
	if startObj >= 0 && (startArr < 0 || startObj < startArr) {
		start = startObj
	} else if startArr >= 0 {
		start = startArr
	}
	if start < 0 {
		return s
	}
	s = s[start:]
	open, close := byte('{'), byte('}')
	if s[0] == '[' {
		open, close = '[', ']'
	}
	depth := 0
	inStr := false
	esc := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
