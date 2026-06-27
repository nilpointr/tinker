package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// safePath resolves path relative to root and verifies it stays within root.
// Absolute paths in path are treated as relative to root, not the filesystem
// root, so the model cannot escape the working directory regardless of what
// it passes.
func safePath(root, path string) (string, error) {
	abs := filepath.Clean(filepath.Join(root, path))
	rootClean := filepath.Clean(root)
	if abs != rootClean && !strings.HasPrefix(abs, rootClean+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes working directory", path)
	}
	return abs, nil
}
