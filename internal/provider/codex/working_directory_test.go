package codex

import (
	"context"
	"fmt"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

func TestWorkingDirectoriesUseLogicalRowsAndKnownFields(t *testing.T) {
	for _, tc := range []struct {
		name, mode, extra string
		rows              []string
		want              int
		state             string
	}{
		{"legacy_mid_session", "legacy", "", []string{`{"type":"turn_context","payload":{"cwd":"/fictional/second"}}`}, 2, "complete"},
		{"unknown", "legacy", "", []string{`{"type":"future_row","payload":{"cwd":"/fictional/ignored"}}`}, 1, "partial"},
		{"malformed", "legacy", "", []string{`{"type":"turn_context","payload":{"cwd":17}}`}, 1, "partial"},
		{"missing", "legacy", "", []string{`{"type":"turn_context","payload":{}}`}, 1, "partial"},
		{"native_command", "paginated", "", []string{`{"ordinal":1,"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","cwd":"/fictional/second"}}}`}, 2, "complete"},
		{"subagent_boundary", "paginated", `,"subagent_history_start_ordinal":2`, []string{`{"ordinal":1,"type":"turn_context","payload":{"cwd":"/fictional/copied-parent"}}`, `{"ordinal":2,"type":"turn_context","payload":{"cwd":"/fictional/child"}}`}, 2, "complete"},
		{"bad_ordinals", "paginated", `,"subagent_history_start_ordinal":2`, []string{`{"ordinal":3,"type":"turn_context","payload":{"cwd":"/fictional/not-trusted"}}`}, 0, "partial"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			head := `{"ordinal":0,"type":"session_meta","payload":{"id":"` + testThreadID + `","cli_version":"0.153.0","history_mode":"` + tc.mode + `","cwd":"/fictional/first"` + tc.extra + `}}`
			rows := append([]string{head}, tc.rows...)
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), rows)
			listed := New().List(context.Background(), testSource(home))
			w := listed.Sources[0].WorkingDirectories
			if w.State != tc.state || len(w.Locations) != tc.want {
				t.Fatalf("%#v", w)
			}
			for _, loc := range w.Locations {
				if loc.Path == "/fictional/copied-parent" {
					t.Fatal("copied parent context leaked")
				}
			}
		})
	}
}
func TestWorkingDirectoryLimitPreservesUncertainty(t *testing.T) {
	home := t.TempDir()
	rows := []string{header(testThreadID, "0.153.0", "legacy", `,"cwd":"/fictional/start"`)}
	for i := 0; i < contract.MaxWorkingDirectories+1; i++ {
		rows = append(rows, fmt.Sprintf(`{"type":"turn_context","payload":{"cwd":"/fictional/path-%d"}}`, i))
	}
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), rows)
	w := New().List(context.Background(), testSource(home)).Sources[0].WorkingDirectories
	if w.State != "partial" || len(w.Locations) != contract.MaxWorkingDirectories {
		t.Fatalf("%#v", w)
	}
}

func TestWorkingDirectoriesFollowInheritedPrefixOnly(t *testing.T) {
	home := t.TempDir()
	base := []string{
		withOrdinal(0, header(testThreadID, "0.153.4", "paginated", `,"cwd":"/fictional/base-header"`)),
		withOrdinal(1, `{"type":"turn_context","payload":{"cwd":"/fictional/inherited"}}`),
		withOrdinal(2, `{"type":"turn_context","payload":{"cwd":"/fictional/excluded"}}`),
	}
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), base)
	cutoff := len(base[0]) + len(base[1]) + 2
	extra := fmt.Sprintf(`,"cwd":"/fictional/selected","history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, testThreadID, cutoff)
	writeRollout(t, rolloutPath(home, "sessions", secondThreadID), []string{withOrdinal(2, header(secondThreadID, "0.153.4", "paginated", extra)), withOrdinal(3, `{"type":"turn_context","payload":{"cwd":"/fictional/current"}}`)})
	shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(secondThreadID))
	w := shown.Sources[0].WorkingDirectories
	if w.State != "complete" {
		t.Fatalf("%#v", w)
	}
	got := map[string]bool{}
	for _, location := range w.Locations {
		got[location.Path] = true
	}
	if !got["/fictional/inherited"] || !got["/fictional/selected"] || !got["/fictional/current"] || got["/fictional/excluded"] || got["/fictional/base-header"] {
		t.Fatal(got)
	}
}
