package codex

import (
	"context"
	"fmt"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/cli"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/provider"
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
		withOrdinal(1, `{"timestamp":"2026-09-03T10:00:00Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{}","call_id":"fictional-call"}}`),
		withOrdinal(2, `{"timestamp":"2026-09-03T10:00:01Z","type":"response_item","payload":{"type":"function_call_output","call_id":"fictional-call","output":"Fictional output"}}`),
		withOrdinal(3, `{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"fictional-call","status":"completed","exit_code":0,"aggregated_output":"Fictional output"}}}`),
	})
	result := New().Events(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if len(result.Events) != 2 || result.Events[0].ToolCall == nil || result.Events[0].ToolCall.Category != "shell" || !hasOmission(result.Omissions, "tool_outcome_unknown") {
		t.Fatalf("Events() = %#v", result)
	}
	call, output := result.Events[0].ToolCall, result.Events[1].ToolResult
	if call.Name != "exec_command" || output == nil || output.CallID != call.CallID || output.Content == nil || output.Content.Text != "Fictional output" {
		t.Fatalf("Expected one tool execution from the legacy response rows: %#v", result.Events)
	}
}

func TestUnlistedLegacyNormalizesToolRows(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.151.0", "legacy", ""),
		`{"timestamp":"2026-09-03T10:00:00Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{}","call_id":"fictional-call"}}`,
		`{"timestamp":"2026-09-03T10:00:01Z","type":"response_item","payload":{"type":"function_call_output","call_id":"fictional-call","output":"Fictional output"}}`,
	})
	result := New().Events(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if result.Status != contract.StatusPartial || len(result.Events) != 2 || !hasOmission(result.Omissions, "tool_outcome_unknown") || hasOmission(result.Omissions, "unknown_format") {
		t.Fatalf("Events() = %#v", result)
	}
	call, output := result.Events[0].ToolCall, result.Events[1].ToolResult
	if call == nil || call.Name != "exec_command" || output == nil || output.CallID != call.CallID || output.Content == nil || output.Content.Text != "Fictional output" {
		t.Fatalf("Expected a correlated legacy tool execution: %#v", result.Events)
	}
}

func TestUnlistedPaginatedReportsOmittedRawToolRows(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(fmt.Sprintf("completed=%t", completed), func(t *testing.T) {
			home := t.TempDir()
			rows := []string{
				withOrdinal(0, header(testThreadID, "0.143.0", "paginated", "")),
				withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "AgentMessage")),
				withOrdinal(2, `{"timestamp":"2026-09-03T10:00:01Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{}","call_id":"fictional-call"}}`),
				withOrdinal(3, `{"timestamp":"2026-09-03T10:00:02Z","type":"response_item","payload":{"type":"function_call_output","call_id":"fictional-call","output":"Fictional output"}}`),
			}
			wantEvents := 1
			if completed {
				rows = append(rows, withOrdinal(4, `{"timestamp":"2026-09-03T10:00:03Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"fictional-item","command":["echo","Fictional output"],"cwd":"file:///fictional","parsed_cmd":[],"source":"agent","status":"completed","exit_code":0,"aggregated_output":"Fictional output"}}}`))
				wantEvents = 3
			}
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), rows)

			result := New().Events(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
			if result.Status != contract.StatusPartial || len(result.Events) != wantEvents || countOmissions(result.Omissions, "unsupported_event") != 2 {
				t.Fatalf("Events() = %#v", result)
			}
			if completed && (result.Events[1].ToolCall == nil || result.Events[2].ToolResult == nil || result.Events[2].ToolResult.CallID != result.Events[1].ToolCall.CallID) {
				t.Fatalf("Expected one correlated execution from the completed item: %#v", result.Events)
			}
		})
	}
}

func TestUnlistedLegacyRetainsCompletedFunctionCallOutput(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.167.0", "legacy", ""),
		`{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"Before output"}}`,
		`{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"FunctionCallOutput","id":"fictional-output","name":"fictional-tool","output":"Fictional output"}}}`,
	})
	result := New().Events(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if result.Status != contract.StatusPartial || len(result.Events) != 2 || !hasOmission(result.Omissions, "tool_outcome_unknown") {
		t.Fatalf("Events() = %#v", result)
	}
	event := result.Events[1]
	output := event.ToolResult
	if output == nil || output.Content == nil || output.Content.Text != "Fictional output" ||
		output.Outcome != "unknown" || output.CallID != "" || output.CorrelationState != "unmatched" || event.RecordedAt != "2026-09-03T10:00:01Z" ||
		len(event.Metadata) != 1 || event.Metadata[0].Name != "tool_name" || event.Metadata[0].Value != "fictional-tool" {
		t.Fatalf("Expected standalone recorded tool output: %#v", event)
	}
}

func TestFutureCodexVersionUsesStructurallyCompatibleTimeProfile(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		withOrdinal(0, header(testThreadID, "0.155.1", "paginated", "")),
		withOrdinal(1, `{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"fictional-user","content":[{"type":"text","text":"Start"}]}}}`),
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
	if events := adapter.Events(context.Background(), source, fingerprint); len(events.Events) != 4 || hasOmission(events.Omissions, "unsupported_format") {
		t.Fatalf("Events() = %#v", events)
	}
	if evidence := adapter.Evidence(context.Background(), source, fingerprint); evidence.Status != contract.StatusComplete {
		t.Fatalf("Evidence() = %#v", evidence)
	}
}

func TestUnlistedCodexVersionReturnsOrderedMessagesAndVerification(t *testing.T) {
	for _, tc := range []struct {
		mode, version, first, last string
	}{
		{
			mode: "legacy", version: "0.151.0",
			first: `{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"First"}}`,
			last:  `{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"agent_message","message":"Last"}}`,
		},
		{
			mode: "paginated", version: "0.156.1",
			first: `{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"fictional-user","content":[{"type":"text","text":"First"}]}}}`,
			last:  `{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"fictional-agent","content":[{"type":"Text","text":"Last"}]}}}`,
		},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			home := t.TempDir()
			path := rolloutPath(home, "sessions", testThreadID)
			meta := header(testThreadID, tc.version, tc.mode, "")
			runner := cli.Runner{Version: "test", Registry: provider.NewRegistry(New())}
			var cleanVersion contract.VerifiedVersionID
			for _, unknown := range []bool{false, true} {
				rows := []string{meta, tc.first}
				if unknown {
					rows = append(rows, `{"type":"event_msg","payload":{"type":"future_message","message":"Unreadable"}}`)
				}
				rows = append(rows, tc.last)
				if tc.mode == "paginated" {
					for i := range rows {
						rows[i] = withOrdinal(i, rows[i])
					}
				}
				writeRollout(t, path, rows)
				listed := runCLI(t, runner, "list", "--provider", "codex", "--root", home)
				if listed.Data == nil || listed.Data.Sources == nil || len(*listed.Data.Sources) != 1 {
					t.Fatalf("list = %#v", listed)
				}
				ref := (*listed.Data.Sources)[0].Identity.SourceRef
				before := runCLI(t, runner, "verify", "--root", home, ref)
				events := runCLI(t, runner, "events", "--root", home, ref)
				after := runCLI(t, runner, "verify", "--root", home, ref)
				status := contract.StatusComplete
				if unknown {
					status = contract.StatusPartial
				}
				if events.Status != status || hasOmission(events.Omissions, "unknown_format") != unknown || events.Data == nil || events.Data.Events == nil {
					t.Fatalf("events = %#v", events)
				}
				messages := *events.Data.Events
				if len(messages) != 2 || messages[0].Message == nil || messages[0].Message.Text != "First" || messages[1].Message == nil || messages[1].Message.Text != "Last" {
					t.Fatalf("messages = %#v", messages)
				}
				if before.Status != status || after.Status != status || hasOmission(after.Omissions, "unknown_format") != unknown || before.Data == nil || after.Data == nil || before.Data.VerifiedVersion == nil || after.Data.VerifiedVersion == nil || *before.Data.VerifiedVersion != *after.Data.VerifiedVersion {
					t.Fatalf("verify before = %#v, after = %#v", before, after)
				}
				if !unknown {
					cleanVersion = *before.Data.VerifiedVersion
				} else if *before.Data.VerifiedVersion == cleanVersion {
					t.Fatal("the unrecognized row did not change the verified version")
				}
			}
		})
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

func TestUnlistedCodexVersionsUseInheritedHistory(t *testing.T) {
	home := t.TempDir()
	base := []string{
		withOrdinal(0, header(testThreadID, "0.156.1", "paginated", "")),
		withOrdinal(1, `{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"fictional-user","content":[{"type":"text","text":"Inherited"}]}}}`),
	}
	cutoff := len(base[0]) + len(base[1]) + 2
	base = append(base, withOrdinal(2, completedMessage("2026-09-03T12:00:00Z", "AgentMessage")))
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), base)
	extra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, testThreadID, cutoff)
	child := []string{
		withOrdinal(2, header(secondThreadID, "0.155.1", "paginated", extra)),
		withOrdinal(3, completedMessage("2026-09-03T11:00:00Z", "AgentMessage")),
	}
	childPath := rolloutPath(home, "sessions", secondThreadID)
	writeRollout(t, childPath, child)
	adapter := New()
	source := testSource(home)
	fingerprint := contract.SourceFingerprint(secondThreadID)
	shown := adapter.Show(context.Background(), source, fingerprint)
	if shown.Status != contract.StatusComplete || len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt == nil || *shown.Sources[0].LastInteractionAt != "2026-09-03T11:00:00Z" || shown.Sources[0].VersionHint == nil || shown.Sources[0].VersionHint.Kind != "content_hash" {
		t.Fatalf("Show() = %#v", shown)
	}
	events := adapter.Events(context.Background(), source, fingerprint)
	if events.Status != contract.StatusComplete || len(events.Events) != 2 || events.Events[0].RecordedAt != "2026-09-03T10:00:00Z" || events.Events[1].RecordedAt != "2026-09-03T11:00:00Z" {
		t.Fatalf("Events() = %#v", events)
	}
	evidence := adapter.Evidence(context.Background(), source, fingerprint)
	if evidence.Status != contract.StatusComplete || evidence.VerifiedVersion == nil {
		t.Fatalf("Evidence() = %#v", evidence)
	}

	badBoundary := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":3,"end_byte_offset":%d}`, testThreadID, cutoff)
	child[0] = withOrdinal(2, header(secondThreadID, "0.155.1", "paginated", badBoundary))
	writeRollout(t, childPath, child)
	events = adapter.Events(context.Background(), source, fingerprint)
	if events.Status != contract.StatusUnsupported || len(events.Events) != 0 || !hasOmission(events.Omissions, "unsupported_format") {
		t.Fatalf("Events() with invalid boundary = %#v", events)
	}
	evidence = adapter.Evidence(context.Background(), source, fingerprint)
	if evidence.Status != contract.StatusUnsupported || evidence.VerifiedVersion != nil || len(evidence.Chunks) != 0 || !hasOmission(evidence.Omissions, "unsupported_format") {
		t.Fatalf("Evidence() with invalid boundary = %#v", evidence)
	}
}

func TestUnlistedPaginatedRequiresContiguousOrdinals(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		withOrdinal(0, header(testThreadID, "0.156.1", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "AgentMessage")),
		withOrdinal(3, completedMessage("2026-09-03T11:00:00Z", "AgentMessage")),
	})
	adapter := New()
	source := testSource(home)
	fingerprint := contract.SourceFingerprint(testThreadID)
	events := adapter.Events(context.Background(), source, fingerprint)
	if events.Status != contract.StatusUnsupported || len(events.Events) != 0 || !hasOmission(events.Omissions, "unsupported_format") {
		t.Fatalf("Events() = %#v", events)
	}
	evidence := adapter.Evidence(context.Background(), source, fingerprint)
	if evidence.Status != contract.StatusUnsupported || evidence.VerifiedVersion != nil || len(evidence.Chunks) != 0 || !hasOmission(evidence.Omissions, "unsupported_format") {
		t.Fatalf("Evidence() = %#v", evidence)
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
