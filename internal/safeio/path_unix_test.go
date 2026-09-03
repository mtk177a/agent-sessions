//go:build linux || darwin

package safeio

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadFileWithinRejectsFIFOWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input.jsonl")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ReadFileWithin(root, "input.jsonl", 1024)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO was accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO open blocked")
	}
}

func TestDecodeJSONFileRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		var target map[string]any
		done <- DecodeJSONFile(path, 1024, 8, &target)
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO was accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO open blocked")
	}
}

func TestReadFileWithinRejectsParentSymlinkReplacementDuringOpen(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "nested")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inside, "input.jsonl"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "input.jsonl"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	replaced := false
	opener := func(path string) (*os.File, error) {
		if !replaced {
			replaced = true
			if err := os.Rename(inside, inside+"-original"); err != nil {
				return nil, err
			}
			if err := os.Symlink(outside, inside); err != nil {
				return nil, err
			}
		}
		return openRegularNoFollow(path)
	}
	if data, err := readFileWithin(root, filepath.Join("nested", "input.jsonl"), 1024, opener); err == nil {
		t.Fatalf("replacement escaped provider root: %q", data)
	}
}
