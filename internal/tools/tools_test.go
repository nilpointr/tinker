package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- sandbox ---

func TestSafePath_Valid(t *testing.T) {
	root := t.TempDir()
	cases := []string{"file.go", "sub/file.go", "."}
	for _, p := range cases {
		if _, err := safePath(root, p); err != nil {
			t.Errorf("safePath(%q): unexpected error: %v", p, err)
		}
	}
}

func TestSafePath_Escapes(t *testing.T) {
	root := t.TempDir()
	cases := []string{"../secret", "../../etc/passwd", "sub/../../.."}
	for _, p := range cases {
		if _, err := safePath(root, p); err == nil {
			t.Errorf("safePath(%q): expected error for path escaping root", p)
		}
	}
}

func TestSafePath_AbsolutePathTreatedAsRelative(t *testing.T) {
	root := t.TempDir()
	// An absolute path from the model should be joined under root, not
	// treated as a filesystem-absolute path.
	safe, err := safePath(root, "/etc/passwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(safe, root) {
		t.Errorf("expected %q to be under root %q", safe, root)
	}
}

// --- ReadFile ---

func TestReadFile_Success(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	tool := ReadFile{root: root}
	got, err := tool.Execute(context.Background(), map[string]any{"path": "hello.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestReadFile_NotFound(t *testing.T) {
	root := t.TempDir()
	tool := ReadFile{root: root}
	_, err := tool.Execute(context.Background(), map[string]any{"path": "missing.txt"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadFile_PathEscape(t *testing.T) {
	root := t.TempDir()
	tool := ReadFile{root: root}
	_, err := tool.Execute(context.Background(), map[string]any{"path": "../outside.txt"})
	if err == nil {
		t.Fatal("expected error for path escaping root")
	}
}

func TestReadFile_MissingArg(t *testing.T) {
	tool := ReadFile{root: t.TempDir()}
	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing path arg")
	}
}

func TestReadFile_BinaryRejected(t *testing.T) {
	root := t.TempDir()
	// null byte makes it binary
	data := []byte("text\x00binary")
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), data, 0644); err != nil {
		t.Fatal(err)
	}
	tool := ReadFile{root: root}
	_, err := tool.Execute(context.Background(), map[string]any{"path": "bin.dat"})
	if err == nil {
		t.Fatal("expected error for binary file")
	}
}

func TestReadFile_TooLargeRejected(t *testing.T) {
	root := t.TempDir()
	data := make([]byte, maxReadBytes+1)
	for i := range data {
		data[i] = 'a'
	}
	if err := os.WriteFile(filepath.Join(root, "big.txt"), data, 0644); err != nil {
		t.Fatal(err)
	}
	tool := ReadFile{root: root}
	_, err := tool.Execute(context.Background(), map[string]any{"path": "big.txt"})
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
}

// --- WriteFile ---

func TestWriteFile_Success(t *testing.T) {
	root := t.TempDir()
	tool := WriteFile{root: root}
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    "out.txt",
		"content": "written",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "out.txt"))
	if string(data) != "written" {
		t.Errorf("file content: got %q, want %q", string(data), "written")
	}
}

func TestWriteFile_CreatesParentDirs(t *testing.T) {
	root := t.TempDir()
	tool := WriteFile{root: root}
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    "sub/dir/out.txt",
		"content": "deep",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "sub/dir/out.txt"))
	if string(data) != "deep" {
		t.Errorf("file content: got %q, want %q", string(data), "deep")
	}
}

func TestWriteFile_PathEscape(t *testing.T) {
	root := t.TempDir()
	tool := WriteFile{root: root}
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    "../outside.txt",
		"content": "evil",
	})
	if err == nil {
		t.Fatal("expected error for path escaping root")
	}
}

// --- ListDir ---

func TestListDir_Success(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte(""), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	tool := ListDir{root: root}
	out, err := tool.Execute(context.Background(), map[string]any{"path": "."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "a.go") {
		t.Errorf("expected a.go in output: %q", out)
	}
	if !strings.Contains(out, "sub/") {
		t.Errorf("expected sub/ (with trailing slash) in output: %q", out)
	}
}

func TestListDir_NotFound(t *testing.T) {
	tool := ListDir{root: t.TempDir()}
	_, err := tool.Execute(context.Background(), map[string]any{"path": "missing"})
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestListDir_PathEscape(t *testing.T) {
	tool := ListDir{root: t.TempDir()}
	_, err := tool.Execute(context.Background(), map[string]any{"path": "../.."})
	if err == nil {
		t.Fatal("expected error for path escaping root")
	}
}

// --- Registry ---

func TestRegistry_KnownTool(t *testing.T) {
	r := NewRegistry(t.TempDir())
	if !r.Has("read_file") || !r.Has("write_file") || !r.Has("list_dir") {
		t.Error("registry missing expected tools")
	}
}

func TestRegistry_UnknownTool(t *testing.T) {
	r := NewRegistry(t.TempDir())
	_, err := r.Execute(context.Background(), "no_such_tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestRegistry_Dispatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	r := NewRegistry(root)
	out, err := r.Execute(context.Background(), "read_file", map[string]any{"path": "file.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "content" {
		t.Errorf("got %q, want %q", out, "content")
	}
}
