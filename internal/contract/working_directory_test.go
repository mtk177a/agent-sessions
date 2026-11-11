package contract

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkingDirectoriesBoundsAndValidation(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first")
	w := NewWorkingDirectories()
	w.Add(WorkingDirectory{Path: first, Evidence: "turn"})
	w.Add(WorkingDirectory{Path: first, Evidence: "turn"})
	if len(w.Locations) != 1 || ValidateWorkingDirectories(w) != nil {
		t.Fatal(w)
	}
	w.Add(WorkingDirectory{Path: "relative", Evidence: "turn"})
	if w.State != "partial" {
		t.Fatal("invalid path hidden")
	}
	e := NewEnvelope("show", "test", StatusComplete)
	e.Data = &Data{Source: &Source{WorkingDirectories: w}}
	w.Locations[0].Path = "/" + strings.Repeat("x", MaxStringBytes)
	if err := EnforceBounds(&e); !errors.Is(err, ErrStructuralStringBound) {
		t.Fatalf("path was truncated: %v", err)
	}
	w.Locations[0].Path = first
	w.Locations[0].Repository = &RepositoryMembership{State: "unknown", CommonDir: filepath.Join(t.TempDir(), "repo"), Reason: "conflicting_evidence"}
	if ValidateWorkingDirectories(w) == nil {
		t.Fatal("unknown association retained a resolved identity")
	}
}
