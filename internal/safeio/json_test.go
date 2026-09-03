package safeio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeJSONFileRejectsMalformedOversizedAndDeepInput(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name     string
		data     string
		maxBytes int64
		maxDepth int
	}{
		{"malformed", `{"value":`, 1024, 8},
		{"oversized", `{"value":"` + strings.Repeat("x", 128) + `"}`, 32, 8},
		{"deep", strings.Repeat(`{"x":`, 9) + `0` + strings.Repeat(`}`, 9), 1024, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			var target struct {
				Value string `json:"value"`
			}
			if err := DecodeJSONFile(path, tc.maxBytes, tc.maxDepth, &target); err == nil {
				t.Fatal("unsafe input was accepted")
			}
		})
	}
}

func TestReadFileWithinRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("fictional"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFileWithin(root, filepath.Join("..", filepath.Base(outside), "outside.txt"), 1024); err == nil {
		t.Fatal("path traversal was accepted")
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outsideFile, link); err == nil {
		if _, err := ReadFileWithin(root, "link.txt", 1024); err == nil {
			t.Fatal("symlink escape was accepted")
		}
	}
}
