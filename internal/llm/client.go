package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	defaultBaseURL = "http://localhost:11434"
	defaultNumCtx  = 8192
)

type Client struct {
	baseURL string
	model   string
	numCtx  int
	http    *http.Client
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

func WithNumCtx(n int) Option {
	return func(c *Client) { c.numCtx = n }
}

func NewClient(model string, opts ...Option) *Client {
	c := &Client{
		baseURL: defaultBaseURL,
		model:   model,
		numCtx:  defaultNumCtx,
		http:    &http.Client{},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Options  chatOpts  `json:"options"`
}

type chatOpts struct {
	NumCtx int `json:"num_ctx"`
}

type chatChunk struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}

// Chat sends messages to Ollama and streams tokens to onToken (may be nil).
// Returns the complete assistant message once the stream is done.
func (c *Client) Chat(ctx context.Context, messages []Message, onToken func(string)) (Message, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
		Options:  chatOpts{NumCtx: c.numCtx},
	})
	if err != nil {
		return Message{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Message{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("ollama: unexpected status %d", resp.StatusCode)
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return Message{}, err
		}
		var chunk chatChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			return Message{}, fmt.Errorf("decode chunk: %w", err)
		}
		if token := chunk.Message.Content; token != "" {
			if onToken != nil {
				onToken(token)
			}
			sb.WriteString(token)
		}
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return Message{}, fmt.Errorf("read stream: %w", err)
	}

	return Message{Role: RoleAssistant, Content: sb.String()}, nil
}
