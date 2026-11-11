package claude

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

func TestWorkingDirectoryCoverageAndStateTransitions(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		want        int
		state       string
	}{
		{"mid_session", rowVersion("2.1.260", "user", `{"role":"user","content":"synthetic"}`, `,"cwd":"/fictional/second"`), 2, "complete"},
		{"clear", `{"type":"worktree-state","sessionId":"` + testSessionID + `","worktreeSession":null}`, 1, "complete"},
		{"missing_state", `{"type":"worktree-state","sessionId":"` + testSessionID + `"}`, 1, "partial"},
		{"malformed_state", `{"type":"worktree-state","sessionId":"` + testSessionID + `","worktreeSession":{"originalCwd":12}}`, 1, "partial"},
		{"unknown_row", `{"type":"future-row"}`, 1, "partial"},
		{"invalid_json", `{broken`, 1, "partial"},
		{"missing_cwd", rowVersion("2.1.260", "user", `{"role":"user","content":"synthetic"}`, ""), 1, "partial"},
		{"invalid_cwd", rowVersion("2.1.260", "user", `{"role":"user","content":"synthetic"}`, `,"cwd":false`), 1, "partial"},
		{"forward_directory", strings.ReplaceAll(rowVersion("2.1.260", "user", `{"role":"user","content":"synthetic"}`, `,"cwd":"/fictional/second"`), "2.1.260", "2.2.0"), 2, "complete"},
		{"wrong_session", strings.ReplaceAll(rowVersion("2.1.260", "user", `{"role":"user","content":"synthetic"}`, `,"cwd":"/fictional/second"`), testSessionID, "99999999-2222-4333-8444-555555555555"), 1, "partial"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeTranscript(t, home, "synthetic", testSessionID, []string{rowVersion("2.1.260", "user", `{"role":"user","content":"synthetic cd /fictional/ignored"}`, `,"cwd":"/fictional/first"`), tc.extra})
			result := New().List(context.Background(), testSource(home))
			w := result.Sources[0].WorkingDirectories
			if w == nil || len(w.Locations) != tc.want || w.State != tc.state {
				t.Fatalf("%#v", w)
			}
			show := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testSessionID))
			if show.Sources[0].WorkingDirectories.State != tc.state {
				t.Fatal("show differs")
			}
		})
	}
}

func TestWorkingDirectoryStateVersionAndCreationFlags(t *testing.T) {
	state := `{"type":"worktree-state","sessionId":"` + testSessionID + `","worktreeSession":{"sessionId":"` + testSessionID + `","originalCwd":"/fictional/original","preEnterOriginalCwd":"/fictional/previous","worktreePath":"/fictional/worktree"%s}}`
	for _, tc := range []struct {
		name, flags, version, kind, wantState string
		count                                 int
	}{
		{"normal", "", "2.1.260", "git_created", "complete", 2},
		{"current_distribution", "", "2.1.322", "git_created", "complete", 2},
		{"existing", `,"enteredExisting":true`, "2.1.228", "existing", "complete", 2},
		{"hook", `,"hookBased":true`, "2.1.260", "hook", "complete", 2},
		{"invalid_flag", `,"hookBased":null`, "2.1.260", "", "partial", 1},
		{"unknown_creation", `,"madeElsewhere":"future-mode"`, "2.1.260", "unknown", "partial", 2},
		{"future_resume", "", "2.2.0", "", "partial", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			rows := []string{rowVersion("2.1.260", "user", `{"role":"user","content":"synthetic"}`, `,"cwd":"/fictional/first"`)}
			if tc.version != "2.1.260" {
				rows = append(rows, rowVersion(tc.version, "user", `{"role":"user","content":"synthetic"}`, `,"cwd":"/fictional/first"`))
			}
			rows = append(rows, fmt.Sprintf(state, tc.flags))
			writeTranscript(t, home, "synthetic", testSessionID, rows)
			w := New().List(context.Background(), testSource(home)).Sources[0].WorkingDirectories
			if w.State != tc.wantState || len(w.Locations) != tc.count {
				t.Fatalf("%#v", w)
			}
			if tc.kind != "" && w.Locations[1].Mapping.Kind != tc.kind {
				t.Fatal(w.Locations[1])
			}
		})
	}
}
