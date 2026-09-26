package claude

import (
	"encoding/json"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"testing"
)

func TestV2ResultBodiesDistinguishEmptyAbsentAndUnreadable(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		state contract.EvidenceState
		text  string
	}{
		{"", contract.EvidenceAbsent, ""},
		{`""`, contract.EvidenceAvailable, ""},
		{`[]`, contract.EvidenceAvailable, "[]"},
		{`null`, contract.EvidenceUnsupported, ""},
		{`[{"type":"text"}]`, contract.EvidenceUnavailable, ""},
		{`[{"type":"text","text":""}]`, contract.EvidenceAvailable, ""},
		{`[{"type":"text","text":"first"},{"type":"text","text":"second"}]`, contract.EvidenceAvailable, `["first","second"]`},
	} {
		body, state := claudeResultContent(json.RawMessage(tc.raw))
		if state != tc.state || body != nil && body.Text != tc.text {
			t.Fatalf("%s: %#v, %s", tc.raw, body, state)
		}
	}
}

func TestV2ResultsBeforeCallsUseExplicitUniqueIDs(t *testing.T) {
	data := row("user", `{"role":"user","content":[{"type":"tool_result","tool_use_id":"later","content":"recorded first","is_error":true}]}`, "") + "\n" +
		row("assistant", `{"role":"assistant","content":[{"type":"tool_use","id":"later","name":"Read","input":{"file_path":"/fictional/source","token":"fictional-only"}}]}`, "")
	events, omissions := normalizeRows(testSessionID, []byte(data))
	if len(events) != 2 || len(omissions) != 0 || events[0].ToolResult.CorrelationState != "matched" || events[0].ToolResult.CallID != events[1].ToolCall.CallID || events[0].ToolResult.Outcome != "failure" {
		t.Fatalf("events %#v omissions %#v", events, omissions)
	}
	if events[1].ToolCall.Name != "Read" || events[1].ToolCall.Input.Text != `{"file_path":"/fictional/source","token":"fictional-only"}` {
		t.Fatal("recorded input changed")
	}
}
