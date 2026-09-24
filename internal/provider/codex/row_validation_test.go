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
