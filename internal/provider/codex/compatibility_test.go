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
	if len(result.Events) != 1 || result.Events[0].ToolCall == nil || !hasOmission(result.Omissions, "unsupported_tool_result") || hasOmission(result.Omissions, "correlation_omitted") {
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
	if len(result.Events) != 1 || result.Events[0].ToolCall == nil || result.Events[0].ToolCall.Category != "shell" || !hasOmission(result.Omissions, "unsupported_tool_result") {
		t.Fatalf("Events() = %#v", result)
	}
}
