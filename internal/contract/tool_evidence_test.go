package contract

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRecordedContentDistinguishesAbsentEmptyAndUnsupported(t *testing.T) {
	for _, tc := range []struct {
		raw          string
		state        EvidenceState
		text, format string
	}{
		{"", EvidenceAbsent, "", ""}, {`""`, EvidenceAvailable, "", "text"}, {`{}`, EvidenceAvailable, `{}`, "json"}, {`[]`, EvidenceAvailable, `[]`, "json"}, {`null`, EvidenceUnsupported, "", ""},
		{`"PASS /fictional/private token=fictional"`, EvidenceAvailable, "PASS /fictional/private token=fictional", "text"},
	} {
		content, state := RecordedContent(json.RawMessage(tc.raw))
		if state != tc.state {
			t.Fatalf("%s: state %s", tc.raw, state)
		}
		if state == EvidenceAvailable && (content.Text != tc.text || content.Format != tc.format || !ValidContent(content, true)) {
			t.Fatalf("%s: %#v", tc.raw, content)
		}
	}
}
func TestContentBoundsPreserveUTF8AndReportIncompleteJSON(t *testing.T) {
	original := strings.Repeat("日", MaxStringBytes)
	content := TextContent(original)
	if !content.Truncated || len(content.Text) > MaxStringBytes || !utf8.ValidString(content.Text) || !strings.HasPrefix(original, content.Text) {
		t.Fatal("invalid text prefix")
	}
	content = JSONContent([]string{original, "second argument"})
	if !content.Truncated || !utf8.ValidString(content.Text) || json.Valid([]byte(content.Text)) || !ValidContent(content, true) {
		t.Fatal("truncated JSON not reported")
	}
}
func TestCorrelationsRetainAmbiguousUnmatchedAndOutOfOrderResults(t *testing.T) {
	call := func(id string) Event { return Event{ToolCall: &ToolCallEvent{CallID: id}} }
	result := func(id string) Event {
		return Event{ToolResult: &ToolResultEvent{CallID: id, Outcome: "unknown", ContentState: EvidenceAvailable, Content: TextContent("body")}}
	}
	events := []Event{result("later"), call("later"), call("duplicate"), call("duplicate"), result("duplicate"), result("missing"), result(""), call("twice"), result("twice"), result("twice")}
	omissions := ResolveToolCorrelations(events)
	if len(omissions) == 0 || events[0].ToolResult.CorrelationState != "matched" || events[4].ToolResult.CorrelationState != "ambiguous" || events[5].ToolResult.CorrelationState != "unmatched" || events[8].ToolResult.CorrelationState != "ambiguous" {
		t.Fatal("incorrect correlation")
	}
	for _, e := range events {
		if e.ToolResult != nil && (e.ToolResult.Content.Text != "body" || e.ToolResult.CorrelationState != "matched" && e.ToolResult.CallID != "") {
			t.Fatal("result was lost or falsely correlated")
		}
	}
}
