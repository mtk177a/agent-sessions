package codex

import (
	"context"
	"os"
	"strings"
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

func TestSkippedMalformedRowMakesInteractionTimeUnavailable(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.153.4", "legacy", ""),
		`{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
		`{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":`,
	})

	result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil || !hasOmission(result.Omissions, "source_time_unavailable") {
		t.Fatalf("Show() = %#v", result)
	}
}

func TestSkippedOversizedRowMakesInteractionTimeUnavailable(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.153.4", "legacy", ""),
		`{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
		`{"timestamp":"2026-09-03T11:00:00Z","type":"future","payload":{"value":"` + strings.Repeat("x", maxHistoryRowBytes) + `"}}`,
	})

	result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil || !hasOmission(result.Omissions, "source_time_unavailable") {
		t.Fatalf("Show() = %#v", result)
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

func TestInteractionTimeOnlySupportsVerifiedOlderPaginatedVersions(t *testing.T) {
	for _, version := range []string{"0.144.2", "0.147.0", "0.148.0-alpha.9"} {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
				withOrdinal(0, header(testThreadID, version, "paginated", "")),
				withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
				withOrdinal(2, `{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"WebSearch","id":"fictional-search"}}}`),
				withOrdinal(3, `{"timestamp":"2026-09-03T12:00:00Z","type":"event_msg","payload":{"type":"token_count"}}`),
			})
			fingerprint := contract.SourceFingerprint(testThreadID)
			adapter := New()
			listed := adapter.List(context.Background(), testSource(home))
			shown := adapter.Show(context.Background(), testSource(home), fingerprint)
			for _, result := range []struct {
				name      string
				sources   []contract.Source
				omissions []contract.Omission
			}{
				{"list", listed.Sources, listed.Omissions},
				{"show", shown.Sources, shown.Omissions},
			} {
				if len(result.sources) != 1 || result.sources[0].LastInteractionAt == nil || *result.sources[0].LastInteractionAt != "2026-09-03T11:00:00Z" || hasOmission(result.omissions, "unsupported_format") || hasOmission(result.omissions, "source_time_unavailable") {
					t.Fatalf("%s = sources=%#v omissions=%#v", result.name, result.sources, result.omissions)
				}
			}
			if events := adapter.Events(context.Background(), testSource(home), fingerprint); hasOmission(events.Omissions, "unsupported_format") {
				t.Fatalf("Events() = %#v", events)
			}
			if evidence := adapter.Evidence(context.Background(), testSource(home), fingerprint); evidence.Status != contract.StatusComplete {
				t.Fatalf("Evidence() = %#v", evidence)
			}
		})
	}
}

func TestOlderInteractionTimeRejectsUnverifiedFormats(t *testing.T) {
	for _, tc := range []struct{ version, mode string }{
		{"0.98.0", "legacy"},
		{"0.117.0", "legacy"},
		{"0.144.2", "legacy"},
	} {
		t.Run(tc.version+"_"+tc.mode, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
				header(testThreadID, tc.version, tc.mode, ""),
				completedMessage("2026-09-03T10:00:00Z", "UserMessage"),
			})
			result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
			if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil || !hasOmission(result.Omissions, "unsupported_format") || !hasOmission(result.Omissions, "source_time_unavailable") {
				t.Fatalf("Show() = %#v", result)
			}
		})
	}
}

func TestMigratedOlderRolloutsExposeInteractionTimeAndVerifiedFormat(t *testing.T) {
	for _, tc := range []struct {
		version string
		rows    []string
		want    string
	}{
		{"0.98.0", []string{
			completedMessage("2026-09-03T10:00:00Z", "UserMessage"),
			`{"timestamp":"2026-09-03T11:00:00Z","type":"response_item","payload":{"type":"function_call"}}`,
			`{"timestamp":"2026-09-03T12:00:00Z","type":"response_item","payload":{"type":"function_call_output"}}`,
			`{"timestamp":"2026-09-03T12:30:00Z","type":"response_item","payload":{"type":"web_search_call"}}`,
			`{"timestamp":"2026-09-03T13:00:00Z","type":"event_msg","payload":{"type":"token_count"}}`,
		}, "2026-09-03T12:30:00Z"},
		{"0.117.0", []string{
			completedMessage("2026-09-03T10:00:00Z", "UserMessage"),
			`{"timestamp":"2026-09-03T11:00:00Z","type":"response_item","payload":{"type":"web_search_call"}}`,
			`{"timestamp":"2026-09-03T12:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"WebSearch","id":"fictional-search"}}}`,
			`{"timestamp":"2026-09-03T12:30:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"fictional-command"}}}`,
			`{"timestamp":"2026-09-03T12:45:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"FileChange","id":"fictional-change"}}}`,
			`{"timestamp":"2026-09-03T13:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"McpToolCall","id":"fictional-tool"}}}`,
			`{"timestamp":"2026-09-03T14:00:00Z","type":"compacted","payload":{}}`,
		}, "2026-09-03T13:00:00Z"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			home := t.TempDir()
			rows := []string{withOrdinal(0, header(testThreadID, tc.version, "paginated", ""))}
			for i, row := range tc.rows {
				rows = append(rows, withOrdinal(i+1, row))
			}
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), rows)
			adapter := New()
			fingerprint := contract.SourceFingerprint(testThreadID)
			listed := adapter.List(context.Background(), testSource(home))
			shown := adapter.Show(context.Background(), testSource(home), fingerprint)
			for _, result := range []struct {
				name      string
				source    contract.Source
				omissions []contract.Omission
			}{
				{"list", listed.Sources[0], listed.Omissions},
				{"show", shown.Sources[0], shown.Omissions},
			} {
				if result.source.LastInteractionAt == nil || *result.source.LastInteractionAt != tc.want || result.source.VersionHint == nil || result.source.VersionHint.Kind != "stat_hash" || hasOmission(result.omissions, "unsupported_format") {
					t.Fatalf("%s = source=%#v omissions=%#v", result.name, result.source, result.omissions)
				}
			}
			if events := adapter.Events(context.Background(), testSource(home), fingerprint); hasOmission(events.Omissions, "unsupported_format") {
				t.Fatalf("Events() = %#v", events)
			}
			if evidence := adapter.Evidence(context.Background(), testSource(home), fingerprint); evidence.Status != contract.StatusComplete {
				t.Fatalf("Evidence() = %#v", evidence)
			}
		})
	}
}

func TestMigratedBookkeepingRowsDoNotAdvanceInteractionTime(t *testing.T) {
	for _, tc := range []struct {
		name, version, row string
	}{
		{"world_state", "0.139.0", `{"timestamp":"2026-09-03T11:00:00Z","type":"world_state","payload":{"full":true,"state":{"model":"fictional"}}}`},
		{"token_usage_record", "0.142.5", `{"timestamp":"2026-09-03T11:00:00Z","type":"token_usage_record","payload":{"thread_id":"fictional-thread","turn_id":"fictional-turn","session_id":"fictional-session","root_turn_id":"fictional-root-turn","response_id":"fictional-response","usage":{},"turn_token_usage":{},"thread_token_usage":{}}}`},
		{"inter_agent_communication_metadata", "0.144.2", `{"timestamp":"2026-09-03T11:00:00Z","type":"inter_agent_communication_metadata","payload":{"trigger_turn":false}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
				withOrdinal(0, header(testThreadID, tc.version, "paginated", "")),
				withOrdinal(1, `{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"fictional-message","content":[{"type":"text","text":"hello"}]}}}`),
				withOrdinal(2, tc.row),
			})
			result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
			if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt == nil || *result.Sources[0].LastInteractionAt != "2026-09-03T10:00:00Z" || hasOmission(result.Omissions, "source_time_unavailable") {
				t.Fatalf("Show() = %#v", result)
			}
			fingerprint := contract.SourceFingerprint(testThreadID)
			if events := New().Events(context.Background(), testSource(home), fingerprint); events.Status != contract.StatusComplete || len(events.Events) != 1 || events.Events[0].Message == nil {
				t.Fatalf("Events() = %#v", events)
			}
			if evidence := New().Evidence(context.Background(), testSource(home), fingerprint); evidence.Status != contract.StatusComplete {
				t.Fatalf("Evidence() = %#v", evidence)
			}
		})
	}
}

func TestMigratedBookkeepingRowsRejectUnverifiedVersionsAndShapes(t *testing.T) {
	validTokenUsage := `{"thread_id":"fictional-thread","turn_id":"fictional-turn","session_id":"fictional-session","root_turn_id":"fictional-root-turn","response_id":"fictional-response","usage":{},"turn_token_usage":{},"thread_token_usage":{}}`
	for _, tc := range []struct {
		name, version, row string
	}{
		{"world_state_unverified_version", "0.117.0", `{"type":"world_state","payload":{"full":true,"state":{}}}`},
		{"world_state_missing_full", "0.139.0", `{"type":"world_state","payload":{"state":{}}}`},
		{"world_state_non_object_state", "0.142.5", `{"type":"world_state","payload":{"full":true,"state":[]}}`},
		{"world_state_extra_field", "0.144.2", `{"type":"world_state","payload":{"full":true,"state":{},"future":true}}`},
		{"metadata_unverified_version", "0.142.5", `{"type":"inter_agent_communication_metadata","payload":{"trigger_turn":false}}`},
		{"metadata_missing_trigger", "0.144.2", `{"type":"inter_agent_communication_metadata","payload":{}}`},
		{"metadata_extra_field", "0.144.2", `{"type":"inter_agent_communication_metadata","payload":{"trigger_turn":false,"future":true}}`},
		{"token_usage_unverified_version", "0.139.0", `{"type":"token_usage_record","payload":` + validTokenUsage + `}`},
		{"token_usage_empty_id", "0.142.5", `{"type":"token_usage_record","payload":{"thread_id":"","turn_id":"fictional-turn","session_id":"fictional-session","root_turn_id":"fictional-root-turn","response_id":"fictional-response","usage":{},"turn_token_usage":{},"thread_token_usage":{}}}`},
		{"token_usage_non_object_usage", "0.142.5", `{"type":"token_usage_record","payload":{"thread_id":"fictional-thread","turn_id":"fictional-turn","session_id":"fictional-session","root_turn_id":"fictional-root-turn","response_id":"fictional-response","usage":[],"turn_token_usage":{},"thread_token_usage":{}}}`},
		{"token_usage_extra_field", "0.142.5", `{"type":"token_usage_record","payload":{"thread_id":"fictional-thread","turn_id":"fictional-turn","session_id":"fictional-session","root_turn_id":"fictional-root-turn","response_id":"fictional-response","usage":{},"turn_token_usage":{},"thread_token_usage":{},"future":true}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
				withOrdinal(0, header(testThreadID, tc.version, "paginated", "")),
				withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
				withOrdinal(2, tc.row),
			})
			result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
			if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil || !hasOmission(result.Omissions, "source_time_unavailable") {
				t.Fatalf("Show() = %#v", result)
			}
			fingerprint := contract.SourceFingerprint(testThreadID)
			if events := New().Events(context.Background(), testSource(home), fingerprint); events.Status == contract.StatusComplete || !hasOmission(events.Omissions, "unknown_format") {
				t.Fatalf("Events() = %#v", events)
			}
			if evidence := New().Evidence(context.Background(), testSource(home), fingerprint); evidence.Status != contract.StatusPartial || !hasOmission(evidence.Omissions, "unknown_format") {
				t.Fatalf("Evidence() = %#v", evidence)
			}
		})
	}
}

func TestMigratedOlderRolloutsRejectUnverifiedShapeAndRows(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		rows          []string
	}{
		{"missing_ordinal", "0.98.0", []string{withOrdinal(0, header(testThreadID, "0.98.0", "paginated", "")), completedMessage("2026-09-03T10:00:00Z", "UserMessage")}},
		{"duplicate_ordinal", "0.117.0", []string{withOrdinal(0, header(testThreadID, "0.117.0", "paginated", "")), withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")), withOrdinal(1, completedMessage("2026-09-03T11:00:00Z", "AgentMessage"))}},
		{"invalid_time", "0.117.0", []string{withOrdinal(0, header(testThreadID, "0.117.0", "paginated", "")), withOrdinal(1, completedMessage("not-a-time", "UserMessage"))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), tc.rows)
			result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
			if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil || !hasOmission(result.Omissions, "source_time_unavailable") {
				t.Fatalf("Show() = %#v", result)
			}
		})
	}
}

func TestMigratedOlderChildTimeTracksInteraction(t *testing.T) {
	for _, tc := range []struct {
		name, row, wantTime string
	}{
		{"no_interaction", `{"timestamp":"2026-09-03T12:00:00Z","type":"event_msg","payload":{"type":"token_count"}}`, ""},
		{"worked", completedMessage("2026-09-03T12:00:00Z", "AgentMessage"), "2026-09-03T12:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
				withOrdinal(0, header(testThreadID, "0.117.0", "paginated", `,"parent_thread_id":"`+secondThreadID+`"`)),
				withOrdinal(1, tc.row),
			})
			result := New().List(context.Background(), testSource(home))
			if len(result.Sources) != 1 || len(result.Sources[0].Relationships) == 0 {
				t.Fatalf("List() = %#v", result)
			}
			got := result.Sources[0].LastInteractionAt
			if tc.wantTime == "" && (got != nil || !hasOmission(result.Omissions, "source_time_unavailable")) || tc.wantTime != "" && (got == nil || *got != tc.wantTime) {
				t.Fatalf("List() = %#v", result)
			}
		})
	}
}

func TestOlderInteractionTimeIncludesSearchRowsWithoutCountingStatus(t *testing.T) {
	for _, tc := range []struct {
		name, version, row string
	}{
		{"raw_web_search", "0.147.0", `{"timestamp":"2026-09-03T11:00:00Z","type":"response_item","payload":{"type":"web_search_call","status":"completed"}}`},
		{"raw_tool_search", "0.148.0-alpha.9", `{"timestamp":"2026-09-03T11:00:00Z","type":"response_item","payload":{"type":"tool_search_output","execution":"server","status":"completed","tools":[]}}`},
		{"completed_sleep", "0.144.2", `{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Sleep","id":"fictional-sleep"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			rows := []string{
				header(testThreadID, tc.version, "paginated", ""),
				completedMessage("2026-09-03T10:00:00Z", "UserMessage"),
				tc.row,
				`{"timestamp":"2026-09-03T12:00:00Z","type":"event_msg","payload":{"type":"token_count"}}`,
			}
			if tc.version == "0.144.2" {
				for i := range rows {
					rows[i] = withOrdinal(i, rows[i])
				}
			}
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), rows)
			result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
			if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt == nil || *result.Sources[0].LastInteractionAt != "2026-09-03T11:00:00Z" {
				t.Fatalf("Show() = %#v", result)
			}
		})
	}
}

func TestOlderInteractionTimeUsesLatestSearchRecord(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.148.0-alpha.9", "paginated", ""),
		`{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"WebSearch","id":"fictional-search"}}}`,
		`{"timestamp":"2026-09-03T11:00:01Z","type":"response_item","payload":{"type":"web_search_call","status":"completed"}}`,
	})
	result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt == nil || *result.Sources[0].LastInteractionAt != "2026-09-03T11:00:01Z" {
		t.Fatalf("Show() = %#v", result)
	}
}

func TestOlderChildWithoutInteractionRemainsListed(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.147.0", "paginated", `,"parent_thread_id":"`+secondThreadID+`"`),
		`{"timestamp":"2026-09-03T12:00:00Z","type":"event_msg","payload":{"type":"token_count"}}`,
	})
	result := New().List(context.Background(), testSource(home))
	if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil || len(result.Sources[0].Relationships) == 0 || !hasOmission(result.Omissions, "source_time_unavailable") {
		t.Fatalf("List() = %#v", result)
	}
}

func TestOlderInteractionTimeFailsClosedForUncertainRows(t *testing.T) {
	for _, tc := range []struct{ version, row string }{
		{"0.148.0-alpha.9", `{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"FutureTool","id":"fictional-tool"}}}`},
		{"0.148.0-alpha.9", `{"timestamp":"invalid","type":"event_msg","payload":{"type":"item_completed","item":{"type":"WebSearch","id":"fictional-search"}}}`},
		{"0.148.0-alpha.9", `{"timestamp":"2026-09-03T11:00:00Z","type":"response_item","payload":{"type":"future_tool_call"}}`},
		{"0.144.2", completedExtension("2026-09-03T11:00:00Z", "clock.sleep")},
	} {
		home := t.TempDir()
		writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
			header(testThreadID, tc.version, "paginated", ""),
			completedMessage("2026-09-03T10:00:00Z", "UserMessage"),
			tc.row,
		})
		result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
		if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil || !hasOmission(result.Omissions, "source_time_unavailable") {
			t.Fatalf("Show() = %#v", result)
		}
	}
}
