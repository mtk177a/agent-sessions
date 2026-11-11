package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

func TestGitOutputBound(t *testing.T) {
	var out boundedBuffer
	if n, err := out.Write([]byte(strings.Repeat("x", 1<<20))); err != nil || n != 1<<20 {
		t.Fatal(n, err)
	}
	if _, err := out.Write([]byte("extra")); err == nil {
		t.Fatal("Git output limit ignored")
	}
}
func TestLookupLimitKeepsCandidateUnknown(t *testing.T) {
	resolver, _ := New(context.Background(), "")
	for i := 0; i < 4096; i++ {
		resolver.cache[string(rune(i))] = unknown("directory_unavailable")
	}
	source := contract.Source{WorkingDirectories: contract.NewWorkingDirectories()}
	source.WorkingDirectories.Add(contract.WorkingDirectory{Path: "/fictional/not-looked-up", Evidence: "turn"})
	if resolver.Observe(&source) != "unknown" || Uncertainty(source) != "git_lookup_limit" {
		t.Fatal(source.WorkingDirectories)
	}
}
