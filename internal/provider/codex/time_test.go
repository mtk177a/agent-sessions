package codex

import (
	"context"
	"os"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

func TestInteractionTimeUsesConversationRowsAcrossResume(t *testing.T) {
	home := t.TempDir()
	path := rolloutPath(home, "sessions", testThreadID)
	writeRollout(t, path, []string{
		header(testThreadID, "0.153.0", "legacy", ""),
		`{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
		`{"timestamp":"2026-09-03T10:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"hi"}}`,
		`{"timestamp":"2026-09-03T20:00:00Z","type":"event_msg","payload":{"type":"token_count"}}`,
		`{"timestamp":"2026-09-03T21:00:00Z","type":"token_usage_record","payload":{}}`,
	})
	adapter := New()
	first := adapter.List(context.Background(), testSource(home))
	if first.Status != contract.StatusComplete || first.Sources[0].LastInteractionAt == nil || *first.Sources[0].LastInteractionAt != "2026-09-03T10:00:02Z" {
		t.Fatalf("initial time = %#v", first)
	}
	fingerprint := contract.SourceFingerprint(testThreadID)
	if events := adapter.Events(context.Background(), testSource(home), fingerprint); events.Status != contract.StatusComplete || len(events.Events) != 2 {
		t.Fatalf("0.153.0 events = %#v", events)
	}
	if evidence := adapter.Evidence(context.Background(), testSource(home), fingerprint); evidence.Status != contract.StatusComplete {
		t.Fatalf("0.153.0 evidence = %#v", evidence)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"timestamp":"2026-09-04T09:00:00+09:00","type":"event_msg","payload":{"type":"user_message","message":"resumed"}}` + "\n"); err != nil {
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

func TestInteractionTimeIsUnavailableForUnsafeCodexRows(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra string
		row   string
	}{
		{"missing_timestamp", "", `{"type":"event_msg","payload":{"type":"user_message","message":"later"}}`},
		{"invalid_timestamp", "", `{"timestamp":"invalid","type":"event_msg","payload":{"type":"user_message","message":"later"}}`},
		{"unknown_row", "", `{"timestamp":"2026-09-04T00:00:00Z","type":"future_row","payload":{}}`},
		{"realtime_row", "", `{"timestamp":"2026-09-04T00:00:00Z","type":"realtime_item","payload":{}}`},
		{"inherited_history", `,"history_base":{"thread_id":"` + testThreadID + `"}`, `{"timestamp":"2026-09-04T00:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"later"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
				header(testThreadID, "0.153.0", "legacy", tc.extra),
				`{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
				tc.row,
			})
			listed := New().List(context.Background(), testSource(home))
			if listed.Status != contract.StatusPartial || listed.Sources[0].LastInteractionAt != nil || !hasOmission(listed.Omissions, "source_time_unavailable") {
				t.Fatalf("List() = %#v", listed)
			}
		})
	}
}

func TestInteractionTimeForVerifiedCodexVersionsAndItems(t *testing.T) {
	for _, version := range []string{"0.152.0", "0.153.3", "0.153.4", "0.154.0", "0.154.0-alpha.6.2", "0.155.0-alpha.9.2"} {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
				header(testThreadID, version, "paginated", ""),
				`{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"user-1","content":[{"type":"text","text":"hello"}]}}}`,
				`{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"FileChange","id":"change-1"}}}`,
				`{"timestamp":"2026-09-03T12:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CollabAgentToolCall","id":"collab-1"}}}`,
				`{"timestamp":"2026-09-03T13:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"SubAgentActivity","id":"activity-1"}}}`,
				`{"timestamp":"2026-09-03T14:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"FunctionCallOutput","id":"output-1"}}}`,
				`{"timestamp":"2026-09-03T15:00:00Z","type":"token_usage_record","payload":{}}`,
			})
			listed := New().List(context.Background(), testSource(home))
			if len(listed.Sources) != 1 || listed.Sources[0].LastInteractionAt == nil || *listed.Sources[0].LastInteractionAt != "2026-09-03T14:00:00Z" {
				t.Fatalf("List() = %#v", listed)
			}
		})
	}
}

func TestInteractionTimeForObservedLegacyAppVersion(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.155.0-alpha.9.2", "legacy", ""),
		`{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
		`{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"hi"}}`,
		`{"timestamp":"2026-09-03T12:00:00Z","type":"token_usage_record","payload":{}}`,
	})
	listed := New().List(context.Background(), testSource(home))
	if len(listed.Sources) != 1 || listed.Sources[0].LastInteractionAt == nil || *listed.Sources[0].LastInteractionAt != "2026-09-03T11:00:00Z" {
		t.Fatalf("List() = %#v", listed)
	}
}
