package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeChunks encodes a slice of token strings as a newline-delimited
// Ollama streaming response. The last token is marked done.
func makeChunks(tokens []string) string {
	var sb strings.Builder
	for i, tok := range tokens {
		chunk := chatChunk{
			Message: Message{Role: RoleAssistant, Content: tok},
			Done:    i == len(tokens)-1,
		}
		b, _ := json.Marshal(chunk)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func testServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestChat_Success(t *testing.T) {
	tokens := []string{"Hello", ", ", "world", "!"}
	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, makeChunks(tokens))
	})

	client := NewClient("test-model", WithBaseURL(srv.URL))
	var received []string
	msg, err := client.Chat(context.Background(), []Message{
		{Role: RoleUser, Content: "hi"},
	}, func(tok string) {
		received = append(received, tok)
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Role != RoleAssistant {
		t.Errorf("role: got %q, want %q", msg.Role, RoleAssistant)
	}
	if msg.Content != "Hello, world!" {
		t.Errorf("content: got %q, want %q", msg.Content, "Hello, world!")
	}
	if len(received) != len(tokens) {
		t.Errorf("callback calls: got %d, want %d", len(received), len(tokens))
	}
}

func TestChat_NilCallbackSucceeds(t *testing.T) {
	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, makeChunks([]string{"ok"}))
	})
	client := NewClient("test-model", WithBaseURL(srv.URL))
	msg, err := client.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "ok" {
		t.Errorf("content: got %q, want %q", msg.Content, "ok")
	}
}

func TestChat_RequestShape(t *testing.T) {
	var got chatRequest
	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = fmt.Fprint(w, makeChunks([]string{"ok"}))
	})

	client := NewClient("qwen2.5-coder:14b", WithBaseURL(srv.URL), WithNumCtx(16384))
	_, _ = client.Chat(context.Background(), []Message{{Role: RoleUser, Content: "test"}}, nil)

	if got.Model != "qwen2.5-coder:14b" {
		t.Errorf("model: got %q, want %q", got.Model, "qwen2.5-coder:14b")
	}
	if !got.Stream {
		t.Error("expected stream: true")
	}
	if got.Options.NumCtx != 16384 {
		t.Errorf("num_ctx: got %d, want %d", got.Options.NumCtx, 16384)
	}
}

func TestChat_NonOKStatus(t *testing.T) {
	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	client := NewClient("test-model", WithBaseURL(srv.URL))
	_, err := client.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestChat_MalformedChunk(t *testing.T) {
	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "not valid json")
	})
	client := NewClient("test-model", WithBaseURL(srv.URL))
	_, err := client.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for malformed JSON chunk")
	}
}

func TestChat_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, makeChunks([]string{"token"}))
	})
	client := NewClient("test-model", WithBaseURL(srv.URL))
	_, err := client.Chat(ctx, nil, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
