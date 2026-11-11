package codex

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), nativeDirectoryRows(rows))
			listed := New().List(context.Background(), testSource(home))
			w := listed.Sources[0].WorkingDirectories
			if w.State != tc.state || len(w.Locations) != tc.want {
				t.Fatalf("%#v", w)
			}
			for _, loc := range w.Locations {
				if loc.Path == nativeDirectoryFixture("/fictional/copied-parent") {
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
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), nativeDirectoryRows(rows))
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
	base = nativeDirectoryRows(base)
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), base)
	cutoff := len(base[0]) + len(base[1]) + 2
	extra := nativeDirectoryFixture(fmt.Sprintf(`,"cwd":"/fictional/selected","history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, testThreadID, cutoff))
	writeRollout(t, rolloutPath(home, "sessions", secondThreadID), []string{withOrdinal(2, header(secondThreadID, "0.153.4", "paginated", extra)), withOrdinal(3, nativeDirectoryFixture(`{"type":"turn_context","payload":{"cwd":"/fictional/current"}}`))})
	shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(secondThreadID))
	w := shown.Sources[0].WorkingDirectories
	if w.State != "complete" {
		t.Fatalf("%#v", w)
	}
	got := map[string]bool{}
	for _, location := range w.Locations {
		got[location.Path] = true
	}
	if !got[nativeDirectoryFixture("/fictional/inherited")] || !got[nativeDirectoryFixture("/fictional/selected")] || !got[nativeDirectoryFixture("/fictional/current")] || got[nativeDirectoryFixture("/fictional/excluded")] || got[nativeDirectoryFixture("/fictional/base-header")] {
		t.Fatal(got)
	}
}

func TestCommandDirectoryPathAcceptsLocalPaths(t *testing.T) {
	posixURI := "file:///fictional/space%20%23%25"
	windowsURI := "file:///C:/fictional/space%20%23%25"
	localURI, foreignURI := posixURI, windowsURI
	path := "/fictional/space #%"
	if filepath.Separator == '\\' {
		localURI, foreignURI = windowsURI, posixURI
		path = filepath.FromSlash("C:" + path)
	}
	for _, tc := range []struct {
		name, input, want string
	}{
		{"absolute_path", path, path},
		{"local_uri", localURI, path},
		{"localhost_uri", strings.Replace(localURI, "file://", "file://localhost", 1), path},
		{"foreign_path_convention", foreignURI, ""},
		{"remote_authority", "file://example.invalid/fictional", ""},
		{"empty_fragment", localURI + "#", ""},
		{"encoded_separator", localURI + "%2fchild", ""},
		{"invalid_escape", localURI + "%zz", ""},
		{"opaque_uri", "file:fictional", ""},
		{"opaque_fallback", "file:///%00/bad/path/fictional", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := contract.NewWorkingDirectories()
			w.Add(contract.WorkingDirectory{Path: commandDirectoryPath(tc.input), Evidence: "tool"})
			if tc.want == "" {
				if len(w.Locations) != 0 || w.State != "partial" {
					t.Fatalf("expected unavailable location: %+v", w)
				}
			} else if len(w.Locations) != 1 || w.Locations[0].Path != tc.want || w.State != "complete" {
				t.Fatalf("expected %q: %+v", tc.want, w)
			}
		})
	}
}

// Synthetic provider paths must be absolute on the host that runs the test.
// Keep this conversion before history byte boundaries are calculated.
func nativeDirectoryFixture(value string) string {
	if filepath.Separator == '\\' {
		return strings.ReplaceAll(value, "/fictional/", "C:/fictional/")
	}
	return value
}
func nativeDirectoryRows(rows []string) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = nativeDirectoryFixture(row)
	}
	return result
}
