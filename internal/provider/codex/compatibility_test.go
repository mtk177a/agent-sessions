package codex

import (
	"context"
	"fmt"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

func TestVerifiedVersionProfilesSupportMinimalObservedRollouts(t *testing.T) {
	for version, compatibility := range codexCompatibility {
		version, compatibility := version, compatibility
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			message := `{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"fictional-message","content":[{"type":"text","text":"hello"}]}}}`
			rows := []string{header(testThreadID, version, "paginated", ""), message}
			if compatibility.paginated == profileCanonicalizedLegacyPaginated {
				rows[0] = withOrdinal(0, rows[0])
				rows[1] = withOrdinal(1, rows[1])
			}
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), rows)
			adapter := New()
			source := testSource(home)
			fingerprint := contract.SourceFingerprint(testThreadID)
			shown := adapter.Show(context.Background(), source, fingerprint)
			if shown.Status != contract.StatusComplete || shown.Sources[0].LastInteractionAt == nil {
				t.Fatalf("Show() = %#v", shown)
			}
			events := adapter.Events(context.Background(), source, fingerprint)
			if events.Status != contract.StatusComplete || len(events.Events) != 1 || events.Events[0].Message == nil {
				t.Fatalf("Events() = %#v", events)
			}
			evidence := adapter.Evidence(context.Background(), source, fingerprint)
			if evidence.Status != contract.StatusComplete {
				t.Fatalf("Evidence() = %#v", evidence)
			}
		})
	}
}

func TestCanonicalizedLegacyToolCorrelationCrossesHistorySpans(t *testing.T) {
	home := t.TempDir()
	base := []string{
		withOrdinal(0, header(testThreadID, "0.117.0", "paginated", "")),
		withOrdinal(1, `{"timestamp":"2026-09-03T10:00:00Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"fictional-call"}}`),
	}
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), base)
	cutoff := len(base[0]) + len(base[1]) + 2
	extra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, testThreadID, cutoff)
	writeRollout(t, rolloutPath(home, "sessions", secondThreadID), []string{
		withOrdinal(2, header(secondThreadID, "0.117.0", "paginated", extra)),
		withOrdinal(3, `{"timestamp":"2026-09-03T10:00:01Z","type":"response_item","payload":{"type":"function_call_output","call_id":"fictional-call"}}`),
	})
	result := New().Events(context.Background(), testSource(home), contract.SourceFingerprint(secondThreadID))
	if len(result.Events) != 2 || result.Events[0].ToolCall == nil || !hasOmission(result.Omissions, "tool_outcome_unknown") || hasOmission(result.Omissions, "correlation_omitted") {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestCanonicalizedLegacyEventsUseRawToolRows(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		withOrdinal(0, header(testThreadID, "0.117.0", "paginated", "")),
		withOrdinal(1, `{"timestamp":"2026-09-03T10:00:00Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"fictional-call"}}`),
		withOrdinal(2, `{"timestamp":"2026-09-03T10:00:01Z","type":"response_item","payload":{"type":"function_call_output","call_id":"fictional-call"}}`),
		withOrdinal(3, `{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"fictional-completed","status":"completed"}}}`),
	})
	result := New().Events(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if len(result.Events) != 2 || result.Events[0].ToolCall == nil || result.Events[0].ToolCall.Category != "shell" || !hasOmission(result.Omissions, "tool_outcome_unknown") {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestFutureCodexVersionUsesStructurallyCompatibleTimeProfile(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		withOrdinal(0, header(testThreadID, "0.155.1", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
		withOrdinal(2, completedMessage("2026-09-03T11:00:00Z", "AgentMessage")),
		withOrdinal(3, `{"timestamp":"2026-09-03T12:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"fictional-command","command":[],"cwd":"file:///fictional","parsed_cmd":[],"source":"agent","status":"completed"}}}`),
		withOrdinal(4, `{"timestamp":"2026-09-03T13:00:00Z","type":"response_item","payload":{"type":"function_call","name":"fictional","arguments":"{}","call_id":"fictional-call","future_field":true}}`),
	})
	adapter := New()
	source := testSource(home)
	fingerprint := contract.SourceFingerprint(testThreadID)
	shown := adapter.Show(context.Background(), source, fingerprint)
	if shown.Status != contract.StatusComplete || len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt == nil || *shown.Sources[0].LastInteractionAt != "2026-09-03T13:00:00Z" || hasOmission(shown.Omissions, "unsupported_format") {
		t.Fatalf("Show() = %#v", shown)
	}
	if events := adapter.Events(context.Background(), source, fingerprint); !hasOmission(events.Omissions, "unsupported_format") {
		t.Fatalf("Events() = %#v", events)
	}
	if evidence := adapter.Evidence(context.Background(), source, fingerprint); !hasOmission(evidence.Omissions, "unknown_format") {
		t.Fatalf("Evidence() = %#v", evidence)
	}
}

func TestFutureCodexVersionRejectsIncompleteInteractionItems(t *testing.T) {
	for _, tc := range []struct {
		name string
		item string
	}{
		{name: "message", item: `{"type":"UserMessage"}`},
		{name: "tool", item: `{"type":"CommandExecution","id":"fictional-command"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
				withOrdinal(0, header(testThreadID, "0.155.1", "paginated", "")),
				withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
				withOrdinal(2, `{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":`+tc.item+`}}`),
			})
			shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
			if shown.Status != contract.StatusPartial || len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt != nil || !hasOmission(shown.Omissions, "source_time_unavailable") {
				t.Fatalf("Show() = %#v", shown)
			}
		})
	}
}

func TestFutureCodexVersionRejectsIncompleteResponseInteraction(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		withOrdinal(0, header(testThreadID, "0.155.1", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
		withOrdinal(2, `{"timestamp":"2026-09-03T11:00:00Z","type":"response_item","payload":{"type":"function_call"}}`),
	})
	shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if shown.Status != contract.StatusPartial || len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt != nil || !hasOmission(shown.Omissions, "source_time_unavailable") {
		t.Fatalf("Show() = %#v", shown)
	}
}

func TestFutureCodexVersionUsesInheritedHistoryForTime(t *testing.T) {
	home := t.TempDir()
	base := []string{
		withOrdinal(0, header(testThreadID, "0.153.4", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
	}
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), base)
	cutoff := len(base[0]) + len(base[1]) + 2
	extra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, testThreadID, cutoff)
	writeRollout(t, rolloutPath(home, "sessions", secondThreadID), []string{
		withOrdinal(2, header(secondThreadID, "0.155.1", "paginated", extra)),
		withOrdinal(3, completedMessage("2026-09-03T11:00:00Z", "AgentMessage")),
	})
	shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(secondThreadID))
	if shown.Status != contract.StatusComplete || len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt == nil || *shown.Sources[0].LastInteractionAt != "2026-09-03T11:00:00Z" || shown.Sources[0].VersionHint == nil || shown.Sources[0].VersionHint.Kind != "content_hash" {
		t.Fatalf("Show() = %#v", shown)
	}
}

func TestFutureCodexVersionAcceptsKnownBookkeepingShapes(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		withOrdinal(0, header(testThreadID, "9.0.0", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
		withOrdinal(2, `{"timestamp":"2026-09-03T11:00:00Z","type":"world_state","payload":{"full":true,"state":{},"future_field":true}}`),
		withOrdinal(3, `{"timestamp":"2026-09-03T12:00:00Z","type":"inter_agent_communication_metadata","payload":{"trigger_turn":false,"future_field":true}}`),
		withOrdinal(4, `{"timestamp":"2026-09-03T13:00:00Z","type":"token_usage_record","payload":{"thread_id":"fictional-thread","turn_id":"fictional-turn","session_id":"fictional-session","root_turn_id":"fictional-root","response_id":"fictional-response","usage":{},"turn_token_usage":{},"thread_token_usage":{},"future_field":true}}`),
		withOrdinal(5, `{"timestamp":"2026-09-03T14:00:00Z","type":"turn_context","payload":{"turn_id":"fictional-turn","future_field":true}}`),
		withOrdinal(6, `{"timestamp":"2026-09-03T15:00:00Z","type":"compacted","payload":{"message":"fictional summary","future_field":true}}`),
	})
	shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if shown.Status != contract.StatusComplete || shown.Sources[0].LastInteractionAt == nil || *shown.Sources[0].LastInteractionAt != "2026-09-03T10:00:00Z" {
		t.Fatalf("Show() = %#v", shown)
	}
}

func TestFutureCodexLegacyVersionUsesKnownInteractionRows(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "9.0.0", "legacy", ""),
		`{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
		`{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"hi"}}`,
	})
	shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if shown.Status != contract.StatusComplete || shown.Sources[0].LastInteractionAt == nil || *shown.Sources[0].LastInteractionAt != "2026-09-03T11:00:00Z" {
		t.Fatalf("Show() = %#v", shown)
	}
}

func TestFutureCodexLegacyVersionRejectsIncompleteMessageEvent(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "9.0.0", "legacy", ""),
		`{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"user_message"}}`,
	})
	shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if shown.Status != contract.StatusPartial || len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt != nil || !hasOmission(shown.Omissions, "source_time_unavailable") {
		t.Fatalf("Show() = %#v", shown)
	}
}

func TestFutureCodexTimeProfileFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name        string
		version     string
		rows        []string
		unsupported bool
	}{
		{
			name:    "missing_paginated_ordinal",
			version: "9.0.0",
			rows:    []string{header(testThreadID, "9.0.0", "paginated", ""), completedMessage("2026-09-03T10:00:00Z", "UserMessage")},
		},
		{
			name:    "invalid_bookkeeping_shape",
			version: "9.0.0",
			rows: []string{
				withOrdinal(0, header(testThreadID, "9.0.0", "paginated", "")),
				withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
				withOrdinal(2, `{"timestamp":"2026-09-03T11:00:00Z","type":"world_state","payload":{"full":true}}`),
			},
		},
		{
			name:        "older_unverified_version",
			version:     "0.151.0",
			unsupported: true,
			rows: []string{
				withOrdinal(0, header(testThreadID, "0.151.0", "paginated", "")),
				withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
			},
		},
		{
			name:    "unknown_event_type",
			version: "9.0.0",
			rows: []string{
				withOrdinal(0, header(testThreadID, "9.0.0", "paginated", "")),
				withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
				withOrdinal(2, `{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"future_event"}}`),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), tc.rows)
			shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
			if len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt != nil || !hasOmission(shown.Omissions, "source_time_unavailable") || hasOmission(shown.Omissions, "unsupported_format") != tc.unsupported {
				t.Fatalf("Show() = %#v", shown)
			}
		})
	}
}
