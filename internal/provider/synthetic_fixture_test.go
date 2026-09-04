package provider

import (
	"path/filepath"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/safeio"
)

func TestSyntheticFixtureExercisesStrictBoundedShape(t *testing.T) {
	var fixture struct {
		SchemaVersion string `json:"schema_version"`
		Source        struct {
			NativeID string `json:"native_id"`
			Kind     string `json:"kind"`
		} `json:"source"`
		Events []struct {
			Kind     string `json:"kind"`
			Role     string `json:"role,omitempty"`
			Text     string `json:"text,omitempty"`
			CallID   string `json:"call_id,omitempty"`
			Category string `json:"category,omitempty"`
			Success  bool   `json:"success,omitempty"`
			ExitCode *int   `json:"exit_code,omitempty"`
		} `json:"events"`
	}
	if err := safeio.DecodeJSONFile(filepath.Join("testdata", "synthetic-source.json"), 64<<20, 64, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != "v1" || len(fixture.Events) != 3 {
		t.Fatalf("unexpected fixture: %#v", fixture)
	}
}
