package agent

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nilpointr/tinker/internal/llm"
	"github.com/nilpointr/tinker/internal/tools"
)

// mockChatter returns preset responses in order, then errors if exhausted.
type mockChatter struct {
	responses []string
	index     int
}

func (m *mockChatter) Chat(_ context.Context, _ []llm.Message, onToken func(string)) (llm.Message, error) {
	if m.index >= len(m.responses) {
		return llm.Message{}, errors.New("mockChatter: no more responses")
	}
	resp := m.responses[m.index]
	m.index++
	if onToken != nil {
		onToken(resp)
	}
	return llm.Message{Role: llm.RoleAssistant, Content: resp}, nil
}

func toolBlock(name, argsJSON string) string {
	return "```tool\n{\"name\": \"" + name + "\", \"args\": " + argsJSON + "}\n```"
}

func doneBlock(summary string) string {
	return toolBlock("done", "{\"summary\": \""+summary+"\"}")
}

// newAgent builds an Agent with a mock chatter and the given responses.
// Pass a nil registry for tests that only exercise done/repair (no file tools).
func newAgent(registry *tools.Registry, responses ...string) *Agent {
	return New(&mockChatter{responses: responses}, llm.PromptExtractor{}, registry)
}

// --- done signal ---

func TestRun_Done(t *testing.T) {
	a := newAgent(nil, doneBlock("all done"))
	var got string
	if err := a.Run(context.Background(), "task", RunOptions{OnDone: func(s string) { got = s }}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "all done" {
		t.Errorf("summary: got %q, want %q", got, "all done")
	}
}

func TestRun_DoneNoSummary(t *testing.T) {
	a := newAgent(nil, toolBlock("done", "{}"))
	var called bool
	if err := a.Run(context.Background(), "task", RunOptions{OnDone: func(string) { called = true }}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("OnDone not called")
	}
}

// --- context cancellation ---

func TestRun_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := newAgent(nil, doneBlock(""))
	err := a.Run(ctx, "task", RunOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// --- callbacks ---

func TestRun_OnTokenCalled(t *testing.T) {
	a := newAgent(nil, doneBlock("ok"))
	var tokens []string
	err := a.Run(context.Background(), "task", RunOptions{
		OnToken: func(tok string) { tokens = append(tokens, tok) },
		OnDone:  func(string) {},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) == 0 {
		t.Error("OnToken was never called")
	}
}

func TestRun_ToolCallbacks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/a.go", []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry(root)
	a := newAgent(reg,
		toolBlock("list_dir", `{"path": "."}`),
		doneBlock("listed"),
	)

	var gotName string
	var gotResult string
	err := a.Run(context.Background(), "list files", RunOptions{
		OnToolCall:   func(name string, _ map[string]any) { gotName = name },
		OnToolResult: func(r string) { gotResult = r },
		OnDone:       func(string) {},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotName != "list_dir" {
		t.Errorf("OnToolCall: got %q, want %q", gotName, "list_dir")
	}
	if gotResult == "" {
		t.Error("OnToolResult: got empty string")
	}
}

// --- approval ---

func TestRun_ApprovalDenied(t *testing.T) {
	reg := tools.NewRegistry(t.TempDir())
	a := newAgent(reg,
		toolBlock("list_dir", `{"path": "."}`),
		doneBlock("done"),
	)

	var toolResult string
	err := a.Run(context.Background(), "task", RunOptions{
		ShouldApprove: func(string, map[string]any) bool { return false },
		OnToolResult:  func(r string) { toolResult = r },
		OnDone:        func(string) {},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(toolResult, "denied") {
		t.Errorf("expected denied message in result, got %q", toolResult)
	}
}

// --- repair loop ---

func TestRun_RepairSucceeds(t *testing.T) {
	// First response has no tool block; second is valid after repair.
	a := newAgent(nil,
		"I need to think about this...",
		doneBlock("repaired"),
	)
	var got string
	err := a.Run(context.Background(), "task", RunOptions{
		OnDone: func(s string) { got = s },
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "repaired" {
		t.Errorf("summary: got %q, want %q", got, "repaired")
	}
}

func TestRun_RepairExhausted(t *testing.T) {
	// All responses are unparseable — should fail after maxRepairAttempts.
	bad := "no tool block here"
	responses := make([]string, maxRepairAttempts+2)
	for i := range responses {
		responses[i] = bad
	}
	a := newAgent(nil, responses...)
	err := a.Run(context.Background(), "task", RunOptions{})
	if err == nil {
		t.Fatal("expected error after repair exhaustion")
	}
	if !strings.Contains(err.Error(), "repair attempts") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --- message history ---

func TestRun_HistoryGrows(t *testing.T) {
	reg := tools.NewRegistry(t.TempDir())
	a := newAgent(reg,
		toolBlock("list_dir", `{"path": "."}`),
		doneBlock("done"),
	)
	_ = a.Run(context.Background(), "task", RunOptions{OnDone: func(string) {}})

	// system + user(task) + assistant(list_dir) + user(result) + assistant(done) = 5
	if len(a.messages) != 5 {
		t.Errorf("message history: got %d messages, want 5", len(a.messages))
	}
}
