package claude

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

func timedRow(version, at, kind, message, extra string) string {
	return strings.Replace(rowVersion(version, kind, message, extra), "2026-09-03T10:00:00Z", at, 1)
}

func TestInteractionTimeIgnoresBookkeepingAndAdvancesOnResume(t *testing.T) {
	home := t.TempDir()
	path := writeTranscript(t, home, "fictional-project", testSessionID, []string{
		timedRow("2.1.260", "2026-09-03T10:00:00Z", "user", `{"role":"user","content":"hello"}`, ""),
		timedRow("2.1.260", "2026-09-03T11:00:00Z", "assistant", `{"role":"assistant","content":[{"type":"text","text":"hi"}]}`, ""),
		`{"timestamp":"2026-09-03T22:00:00Z","type":"file-history-snapshot","sessionId":"` + testSessionID + `","version":"2.1.260"}`,
		timedRow("2.1.260", "2026-09-03T23:00:00Z", "system", `{}`, `,"subtype":"turn_duration"`),
	})
	adapter := New()
	first := adapter.List(context.Background(), testSource(home))
	if first.Status != contract.StatusComplete || first.Sources[0].LastInteractionAt == nil || *first.Sources[0].LastInteractionAt != "2026-09-03T11:00:00Z" {
		t.Fatalf("initial time = %#v", first)
	}
	fingerprint := contract.SourceFingerprint(testSessionID)
	if events := adapter.Events(context.Background(), testSource(home), fingerprint); events.Status != contract.StatusComplete || len(events.Events) != 2 {
		t.Fatalf("2.1.260 events = %#v", events)
	}
	if evidence := adapter.Evidence(context.Background(), testSource(home), fingerprint); evidence.Status != contract.StatusComplete {
		t.Fatalf("2.1.260 evidence = %#v", evidence)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(timedRow("2.1.260", "2026-09-04T09:00:00+09:00", "user", `{"role":"user","content":"resumed"}`, "") + "\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	second := adapter.Show(context.Background(), testSource(home), fingerprint)
	if second.Status != contract.StatusComplete || second.Sources[0].LastInteractionAt == nil || *second.Sources[0].LastInteractionAt != "2026-09-04T00:00:00Z" {
		t.Fatalf("resumed time = %#v", second)
	}
}

func TestInteractionTimeIsUnavailableForUnsafeClaudeRows(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
	}{
		{"missing_timestamp", strings.Replace(row("assistant", `{"role":"assistant","content":[{"type":"text","text":"later"}]}`, ""), `"timestamp":"2026-09-03T10:00:00Z",`, "", 1)},
		{"invalid_timestamp", timedRow("2.1.260", "invalid", "assistant", `{"role":"assistant","content":[{"type":"text","text":"later"}]}`, "")},
		{"unknown_row", `{"timestamp":"2026-09-04T00:00:00Z","type":"future_row","sessionId":"` + testSessionID + `","version":"2.1.260"}`},
		{"unknown_block", timedRow("2.1.260", "2026-09-04T00:00:00Z", "assistant", `{"role":"assistant","content":[{"type":"future_block"}]}`, "")},
		{"older_unverified_version", timedRow("2.1.259", "2026-09-04T00:00:00Z", "assistant", `{"role":"assistant","content":[{"type":"text","text":"later"}]}`, "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeTranscript(t, home, "fictional-project", testSessionID, []string{
				timedRow("2.1.260", "2026-09-03T10:00:00Z", "user", `{"role":"user","content":"hello"}`, ""), tc.row,
			})
			listed := New().List(context.Background(), testSource(home))
			if listed.Status != contract.StatusPartial || listed.Sources[0].LastInteractionAt != nil || !hasOmission(listed.Omissions, "source_time_unavailable") {
				t.Fatalf("List() = %#v", listed)
			}
		})
	}
}

func TestInteractionTimeIgnoresObservedNonInteractiveAttachments(t *testing.T) {
	home := t.TempDir()
	lines := []string{timedRow("2.1.177", "2026-09-03T10:00:00Z", "user", `{"role":"user","content":"hello"}`, "")}
	for _, kind := range []string{"agent_listing_delta", "command_permissions", "deferred_tools_delta", "diagnostics", "edited_text_file", "opened_file_in_ide", "plan_mode", "plan_mode_exit", "selected_lines_in_ide", "skill_listing", "task_reminder"} {
		lines = append(lines, `{"timestamp":"2026-09-03T12:00:00Z","type":"attachment","sessionId":"`+testSessionID+`","version":"2.1.177","attachment":{"type":"`+kind+`"}}`)
	}
	for _, subtype := range []string{"away_summary", "informational"} {
		lines = append(lines, `{"timestamp":"2026-09-03T13:00:00Z","type":"system","sessionId":"`+testSessionID+`","version":"2.1.177","subtype":"`+subtype+`","content":"fictional notice"}`)
	}
	writeTranscript(t, home, "fictional-project", testSessionID, lines)
	listed := New().List(context.Background(), testSource(home))
	if len(listed.Sources) != 1 || listed.Sources[0].LastInteractionAt == nil || *listed.Sources[0].LastInteractionAt != "2026-09-03T10:00:00Z" {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestInteractionTimeRemainsUnknownForCommandAttachment(t *testing.T) {
	for _, version := range []string{"2.1.177", "2.1.270", "9.0.0"} {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			writeTranscript(t, home, "fictional-project", testSessionID, []string{
				timedRow(version, "2026-09-03T10:00:00Z", "user", `{"role":"user","content":"hello"}`, ""),
				`{"timestamp":"2026-09-03T12:00:00Z","type":"attachment","sessionId":"` + testSessionID + `","version":"` + version + `","attachment":{"type":"queued_command","prompt":"fictional task"}}`,
			})
			listed := New().List(context.Background(), testSource(home))
			if len(listed.Sources) != 1 || listed.Sources[0].LastInteractionAt != nil || !hasOmission(listed.Omissions, "source_time_unavailable") {
				t.Fatalf("List() = %#v", listed)
			}
		})
	}
}

func TestFutureClaudeVersionUsesStructurallyCompatibleTimeProfile(t *testing.T) {
	for _, version := range []string{"2.1.270", "9.0.0"} {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			writeTranscript(t, home, "fictional-project", testSessionID, []string{
				timedRow("2.1.177", "2026-09-03T09:00:00Z", "user", `{"role":"user","content":"earlier"}`, ""),
				timedRow(version, "2026-09-03T10:00:00Z", "user", `{"role":"user","content":"hello"}`, ""),
				timedRow(version, "2026-09-03T11:00:00Z", "assistant", `{"role":"assistant","content":[{"type":"text","text":"hi"}]}`, ""),
				`{"timestamp":"2026-09-03T12:00:00Z","type":"attachment","sessionId":"` + testSessionID + `","version":"` + version + `","attachment":{"type":"task_reminder"}}`,
				`{"timestamp":"2026-09-03T13:00:00Z","type":"system","sessionId":"` + testSessionID + `","version":"` + version + `","subtype":"informational","content":"fictional notice"}`,
			})
			adapter := New()
			source := testSource(home)
			listed := adapter.List(context.Background(), source)
			if listed.Status != contract.StatusComplete || len(listed.Sources) != 1 || listed.Sources[0].LastInteractionAt == nil || *listed.Sources[0].LastInteractionAt != "2026-09-03T11:00:00Z" || hasOmission(listed.Omissions, "unsupported_format") {
				t.Fatalf("List() = %#v", listed)
			}
			fingerprint := listed.Sources[0].Identity.ProviderSourceFingerprint
			if events := adapter.Events(context.Background(), source, fingerprint); !hasOmission(events.Omissions, "unsupported_format") {
				t.Fatalf("Events() = %#v", events)
			}
			if evidence := adapter.Evidence(context.Background(), source, fingerprint); !hasOmission(evidence.Omissions, "unsupported_format") {
				t.Fatalf("Evidence() = %#v", evidence)
			}
		})
	}
}
