package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxReadBytes = 256 * 1024 // 256 KB

// ReadFile reads a file within the sandboxed root and returns its contents.
type ReadFile struct{ root string }

func (t ReadFile) Name() string { return "read_file" }

func (t ReadFile) Execute(_ context.Context, args map[string]any) (string, error) {
	path, err := stringArg(args, "path")
	if err != nil {
		return "", err
	}
	safe, err := safePath(t.root, path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(safe)
	if err != nil {
		return "", err
	}
	if len(data) > maxReadBytes {
		return "", fmt.Errorf("file too large to read (%d bytes); maximum is %d bytes", len(data), maxReadBytes)
	}
	if bytes.IndexByte(data, 0) != -1 {
		return "", fmt.Errorf("file appears to be binary and cannot be read as text")
	}
	return string(data), nil
}

// WriteFile writes content to a file within the sandboxed root,
// creating parent directories as needed.
type WriteFile struct{ root string }

func (t WriteFile) Name() string { return "write_file" }

func (t WriteFile) Execute(_ context.Context, args map[string]any) (string, error) {
	path, err := stringArg(args, "path")
	if err != nil {
		return "", err
	}
	content, err := stringArg(args, "content")
	if err != nil {
		return "", err
	}
	safe, err := safePath(t.root, path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(safe), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(safe, []byte(content), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %s", path), nil
}

// ListDir lists the contents of a directory within the sandboxed root.
// Directories are suffixed with / to distinguish them from files.
type ListDir struct{ root string }

func (t ListDir) Name() string { return "list_dir" }

func (t ListDir) Execute(_ context.Context, args map[string]any) (string, error) {
	path, err := stringArg(args, "path")
	if err != nil {
		return "", err
	}
	safe, err := safePath(t.root, path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(safe)
	if err != nil {
		return "", err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
		if e.IsDir() {
			names[i] += "/"
		}
	}
	return strings.Join(names, "\n"), nil
}
