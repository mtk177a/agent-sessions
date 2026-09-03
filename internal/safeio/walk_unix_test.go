//go:build unix

package safeio

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestListRegularFilesWithinRejectsSpecialFiles(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "sessions")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(directory, "rollout.jsonl"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ListRegularFilesWithin(root, "sessions", 10); err == nil {
		t.Fatal("expected special-file error")
	}
}
