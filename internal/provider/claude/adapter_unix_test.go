//go:build !windows

package claude

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSpecialFileIsRejectedWithoutOpeningIt(t *testing.T) {
	home := t.TempDir()
	projects := filepath.Join(home, "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(projects, "fictional.fifo"), 0o600); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	if listed := New().List(context.Background(), testSource(home)); listed.Err == nil {
		t.Fatalf("List() = %#v", listed)
	}
}
