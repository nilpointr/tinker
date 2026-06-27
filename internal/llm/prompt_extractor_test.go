package llm

import (
	"strings"
	"testing"
)

func TestPromptExtractor_HappyPath(t *testing.T) {
	input := "```tool\n{\"name\": \"read_file\", \"args\": {\"path\": \"main.go\"}}\n```"
	tc, err := PromptExtractor{}.Extract(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.Name != "read_file" {
		t.Errorf("name: got %q, want %q", tc.Name, "read_file")
	}
	if tc.Args["path"] != "main.go" {
		t.Errorf("args.path: got %v, want %q", tc.Args["path"], "main.go")
	}
}

func TestPromptExtractor_ProsePreamble(t *testing.T) {
	input := "I'll read main.go to understand the structure.\n```tool\n{\"name\": \"read_file\", \"args\": {\"path\": \"main.go\"}}\n```"
	tc, err := PromptExtractor{}.Extract(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.Name != "read_file" {
		t.Errorf("name: got %q, want %q", tc.Name, "read_file")
	}
}

func TestPromptExtractor_TakesLastBlock(t *testing.T) {
	input := strings.Join([]string{
		"```tool",
		`{"name": "read_file", "args": {"path": "a.go"}}`,
		"```",
		"Now I'll write instead.",
		"```tool",
		`{"name": "write_file", "args": {"path": "b.go", "content": "x"}}`,
		"```",
	}, "\n")
	tc, err := PromptExtractor{}.Extract(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.Name != "write_file" {
		t.Errorf("name: got %q, want %q (should take last block)", tc.Name, "write_file")
	}
}

func TestPromptExtractor_DoneSignal(t *testing.T) {
	input := "```tool\n{\"name\": \"done\", \"args\": {\"summary\": \"All done.\"}}\n```"
	tc, err := PromptExtractor{}.Extract(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.Name != "done" {
		t.Errorf("name: got %q, want %q", tc.Name, "done")
	}
	if tc.Args["summary"] != "All done." {
		t.Errorf("args.summary: got %v, want %q", tc.Args["summary"], "All done.")
	}
}

func TestPromptExtractor_DoneNoArgs(t *testing.T) {
	input := "```tool\n{\"name\": \"done\"}\n```"
	tc, err := PromptExtractor{}.Extract(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.Name != "done" {
		t.Errorf("name: got %q, want %q", tc.Name, "done")
	}
	if tc.Args != nil {
		t.Errorf("args: got %v, want nil", tc.Args)
	}
}

func TestPromptExtractor_NoBlock(t *testing.T) {
	_, err := PromptExtractor{}.Extract("just some prose with no tool call")
	if err == nil {
		t.Fatal("expected error for missing tool block")
	}
}

func TestPromptExtractor_UnclosedBlock(t *testing.T) {
	_, err := PromptExtractor{}.Extract("```tool\n{\"name\": \"read_file\"}")
	if err == nil {
		t.Fatal("expected error for unclosed tool block")
	}
}

func TestPromptExtractor_InvalidJSON(t *testing.T) {
	_, err := PromptExtractor{}.Extract("```tool\nnot json\n```")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestPromptExtractor_MissingName(t *testing.T) {
	_, err := PromptExtractor{}.Extract("```tool\n{\"args\": {}}\n```")
	if err == nil {
		t.Fatal("expected error for missing name field")
	}
}
