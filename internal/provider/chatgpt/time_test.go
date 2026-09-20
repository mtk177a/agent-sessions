package chatgpt

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
)

func TestParseUnixSecondsPreservesDecimalPrecision(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  string
	}{
		{"1780000000.1234567", "2026-05-28T20:26:40.1234567Z"},
		{"1.780000000123456789e9", "2026-05-28T20:26:40.123456789Z"},
		{"-1.5", "1969-12-31T23:59:58.5Z"},
	} {
		at, ok := parseUnixSeconds(json.RawMessage(tc.value))
		if !ok || at.Format(time.RFC3339Nano) != tc.want {
			t.Fatalf("parseUnixSeconds(%s) = %s, %v", tc.value, at.Format(time.RFC3339Nano), ok)
		}
	}
	for _, value := range []string{"null", `"1780000000"`, "1780000000.0000000001", "1e999999", "1e-999999", "253402300800", "-62135596801"} {
		if _, ok := parseUnixSeconds(json.RawMessage(value)); ok {
			t.Fatalf("parseUnixSeconds(%s) accepted invalid time", value)
		}
	}
}

func TestInteractionTimeCountsToolResultsAndAggregatesUnavailableConversations(t *testing.T) {
	body := `[
		{"id":"timed","current_node":"tool","mapping":{
			"root":{"parent":null,"message":null},
			"user":{"parent":"root","message":{"author":{"role":"user"},"content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer"}]},"create_time":1780000000}},
			"tool":{"parent":"user","message":{"author":{"role":"tool"},"content":{"content_type":"text","parts":["done"]},"create_time":1780000010.5}}}},
		{"id":"untimed","current_node":"user","mapping":{"user":{"parent":null,"message":{"author":{"role":"user"},"content":{"content_type":"text","parts":["hello"]}}}}}
	]`
	archive := writeZIP(t, []zipMember{{"conversations.json", body}})
	source := config.Source{ID: "export", Provider: providerName, Root: archive}
	listed := New().List(t.Context(), source)
	if listed.Err != nil || listed.Status != contract.StatusPartial || len(listed.Sources) != 2 || len(listed.Omissions) != 1 || listed.Omissions[0].Code != "source_time_unavailable" || listed.Omissions[0].Count != 1 {
		t.Fatalf("list = %#v", listed)
	}
	if listed.Sources[0].LastInteractionAt == nil || *listed.Sources[0].LastInteractionAt != "2026-05-28T20:26:50.5Z" || listed.Sources[1].LastInteractionAt != nil {
		t.Fatalf("source times = %#v", listed.Sources)
	}
	for _, tc := range []struct {
		id     string
		status contract.Status
	}{
		{"timed", contract.StatusComplete},
		{"untimed", contract.StatusPartial},
	} {
		shown := New().Show(t.Context(), source, contract.SourceFingerprint(tc.id))
		if shown.Err != nil || shown.Status != tc.status || len(shown.Sources) != 1 || (shown.Sources[0].LastInteractionAt == nil) != (tc.status == contract.StatusPartial) {
			t.Fatalf("show %s = %#v", tc.id, shown)
		}
	}
}

func TestInteractionTimeFailsClosedOnUnsafeActiveRecords(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message string
	}{
		{"missing timestamp", `{"author":{"role":"user"},"content":{"content_type":"text","parts":["hello"]}}`},
		{"invalid timestamp", `{"author":{"role":"user"},"content":{"content_type":"text","parts":["hello"]},"create_time":"bad"}`},
		{"unknown role", `{"author":{"role":"future"},"content":{"content_type":"text","parts":["hello"]},"create_time":1780000000}`},
		{"unknown content", `{"author":{"role":"assistant"},"content":{"content_type":"future","parts":[]},"create_time":1780000000}`},
		{"missing message", ``},
		{"no interaction", `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			field := `,"message":` + tc.message
			if tc.name == "missing message" {
				field = ""
			}
			body := fmt.Sprintf(`[{"id":"c","current_node":"node","mapping":{"node":{"parent":null%s}}}]`, field)
			archive := writeZIP(t, []zipMember{{"conversations.json", body}})
			result := New().List(t.Context(), config.Source{ID: "export", Provider: providerName, Root: archive})
			if result.Err != nil || result.Status != contract.StatusPartial || len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil || !hasOmission(result.Omissions, "source_time_unavailable") {
				t.Fatalf("list = %#v", result)
			}
		})
	}
	for _, body := range []string{
		`[{"id":"c","current_node":"missing","mapping":{}}]`,
		`[{"id":"c","current_node":"a","mapping":{"a":{"parent":"b","message":null},"b":{"parent":"a","message":null}}}]`,
	} {
		archive := writeZIP(t, []zipMember{{"conversations.json", body}})
		result := New().List(t.Context(), config.Source{ID: "export", Provider: providerName, Root: archive})
		if result.Err != nil || result.Status != contract.StatusPartial || len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil {
			t.Fatalf("invalid graph time = %#v", result)
		}
	}
}

func TestInteractionTimeDoesNotFallBackToEarlierMessage(t *testing.T) {
	for _, later := range []string{
		`{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["later"]}}`,
		`{"author":{"role":"assistant"},"content":{"content_type":"future","parts":[]},"create_time":1780000020}`,
	} {
		body := fmt.Sprintf(`[{"id":"c","current_node":"later","mapping":{
			"earlier":{"parent":null,"message":{"author":{"role":"user"},"content":{"content_type":"text","parts":["earlier"]},"create_time":1780000000}},
			"later":{"parent":"earlier","message":%s}
		}}]`, later)
		archive := writeZIP(t, []zipMember{{"conversations.json", body}})
		result := New().List(t.Context(), config.Source{ID: "export", Provider: providerName, Root: archive})
		if result.Err != nil || result.Status != contract.StatusPartial || len(result.Sources) != 1 || result.Sources[0].LastInteractionAt != nil || !hasOmission(result.Omissions, "source_time_unavailable") {
			t.Fatalf("unsafe later message = %#v", result)
		}
	}
}
