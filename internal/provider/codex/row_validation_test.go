package codex

import (
	"encoding/json"
	"testing"
)

func TestForwardCompletedInteractionItemsRequireKnownStructure(t *testing.T) {
	for _, tc := range []struct {
		name, itemType, valid, invalid string
	}{
		{"user_message", "UserMessage", `{"type":"UserMessage","id":"fictional","content":[]}`, `{"type":"UserMessage","id":"fictional"}`},
		{"agent_message", "AgentMessage", `{"type":"AgentMessage","id":"fictional","content":[]}`, `{"type":"AgentMessage","content":[]}`},
		{"function_output", "FunctionCallOutput", `{"type":"FunctionCallOutput","id":"fictional","name":"tool","output":""}`, `{"type":"FunctionCallOutput","id":"fictional","name":"tool"}`},
		{"command", "CommandExecution", `{"type":"CommandExecution","id":"fictional","command":[],"cwd":"/fictional","parsed_cmd":[],"source":"agent","status":"completed"}`, `{"type":"CommandExecution","id":"fictional","command":[],"cwd":"/fictional","parsed_cmd":[],"source":"agent"}`},
		{"dynamic_tool", "DynamicToolCall", `{"type":"DynamicToolCall","id":"fictional","tool":"fictional","arguments":{},"status":"completed"}`, `{"type":"DynamicToolCall","id":"fictional","tool":"fictional","status":"completed"}`},
		{"collab_tool", "CollabAgentToolCall", `{"type":"CollabAgentToolCall","id":"fictional","tool":"wait","status":"completed","sender_thread_id":"fictional"}`, `{"type":"CollabAgentToolCall","id":"fictional","tool":"wait","status":"completed"}`},
		{"web_search", "WebSearch", `{"type":"WebSearch","id":"fictional","query":"fictional","action":{"type":"search"}}`, `{"type":"WebSearch","id":"fictional","query":"fictional","action":{}}`},
		{"file_change", "FileChange", `{"type":"FileChange","id":"fictional","changes":{}}`, `{"type":"FileChange","id":"fictional","changes":[]}`},
		{"mcp_tool", "McpToolCall", `{"type":"McpToolCall","id":"fictional","server":"fictional","tool":"fictional","arguments":{},"status":"completed"}`, `{"type":"McpToolCall","id":"fictional","server":"fictional","tool":"fictional","arguments":{}}`},
		{"extension", "Extension", `{"type":"Extension","id":"fictional","kind":"web.search"}`, `{"type":"Extension","id":"fictional"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !validForwardCompletedItem(json.RawMessage(tc.valid), tc.itemType) {
				t.Fatal("valid item was rejected")
			}
			if validForwardCompletedItem(json.RawMessage(tc.invalid), tc.itemType) {
				t.Fatal("incomplete item was accepted")
			}
		})
	}
}

func TestForwardMessageEventsRequireMessageText(t *testing.T) {
	for _, eventType := range []string{"user_message", "agent_message"} {
		t.Run(eventType, func(t *testing.T) {
			valid := rolloutLine{Type: "event_msg", Payload: json.RawMessage(`{"type":"` + eventType + `","message":""}`)}
			if !validRowForProfile(profileLegacy, "9.0.0", valid, true) {
				t.Fatal("valid message event was rejected")
			}
			invalid := rolloutLine{Type: "event_msg", Payload: json.RawMessage(`{"type":"` + eventType + `"}`)}
			if validRowForProfile(profileLegacy, "9.0.0", invalid, true) {
				t.Fatal("incomplete message event was accepted")
			}
		})
	}
}

func TestForwardResponseInteractionItemsRequireKnownStructure(t *testing.T) {
	for _, tc := range []struct {
		name, valid, invalid string
	}{
		{"message", `{"type":"message","role":"user","content":[]}`, `{"type":"message","role":"user"}`},
		{"agent_message", `{"type":"agent_message","author":"assistant","recipient":"user","content":[]}`, `{"type":"agent_message","author":"assistant","recipient":"user"}`},
		{"local_shell_call", `{"type":"local_shell_call","status":"completed","action":{"type":"exec","command":[]}}`, `{"type":"local_shell_call","status":"completed","action":{"type":"exec"}}`},
		{"function_call", `{"type":"function_call","name":"fictional","arguments":"{}","call_id":"fictional-call"}`, `{"type":"function_call"}`},
		{"function_call_output", `{"type":"function_call_output","output":"fictional"}`, `{"type":"function_call_output"}`},
		{"custom_tool_call", `{"type":"custom_tool_call","call_id":"fictional-call","name":"fictional","input":"{}"}`, `{"type":"custom_tool_call","call_id":"fictional-call","name":"fictional"}`},
		{"custom_tool_call_output", `{"type":"custom_tool_call_output","call_id":"fictional-call","output":[]}`, `{"type":"custom_tool_call_output","call_id":"fictional-call"}`},
		{"tool_search_call", `{"type":"tool_search_call","execution":"server","arguments":{}}`, `{"type":"tool_search_call","execution":"server"}`},
		{"tool_search_output", `{"type":"tool_search_output","status":"completed","execution":"server","tools":[]}`, `{"type":"tool_search_output","status":"completed","execution":"server"}`},
		{"web_search_call", `{"type":"web_search_call"}`, `{"type":"web_search_call","status":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valid := rolloutLine{Type: "response_item", Payload: json.RawMessage(tc.valid)}
			if !validRowForProfile(profileNativePaginated, "9.0.0", valid, true) {
				t.Fatal("valid response item was rejected")
			}
			invalid := rolloutLine{Type: "response_item", Payload: json.RawMessage(tc.invalid)}
			if validRowForProfile(profileNativePaginated, "9.0.0", invalid, true) {
				t.Fatal("incomplete response item was accepted")
			}
		})
	}
}
