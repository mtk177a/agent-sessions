package safeio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListRegularFilesWithinIsSortedAndBounded(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "sessions")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"b.jsonl", "a.jsonl"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := ListRegularFilesWithin(root, "sessions", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Relative != filepath.Join("sessions", "a.jsonl") || entries[1].Relative != filepath.Join("sessions", "b.jsonl") {
		t.Fatalf("entries = %#v", entries)
	}
	if _, err := ListRegularFilesWithin(root, "sessions", 1); err == nil {
		t.Fatal("expected file-count limit error")
	}
}

func TestListRegularFilesWithinRejectsSymlinksAndEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "rollout.jsonl"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sessions")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ListRegularFilesWithin(root, "sessions", 10); err == nil {
		t.Fatal("expected provider-root escape error")
	}
}
