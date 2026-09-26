package codex

import (
	"encoding/json"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

type eventNormalizer struct {
	threadID  string
	events    []contract.Event
	omissions []contract.Omission
}

func newEventNormalizer(threadID string) *eventNormalizer {
	return &eventNormalizer{threadID: threadID}
}

func (n *eventNormalizer) consume(origin artifact, line rolloutLine) {
	profile := compatibilityProfile(origin.meta.CLIVersion, origin.meta.HistoryMode)
	if profile == profileUnsupported {
		n.omissions = append(n.omissions, omission("unsupported_format", "events", "The Codex artifact version and history mode have not been verified."))
		return
	}
	if !validRowForProfile(profile, origin.meta.CLIVersion, line, false) {
		n.omissions = append(n.omissions, omission("unknown_format", "events", "A Codex JSONL row or payload type was not recognized for its stored format."))
		return
	}
	before := len(n.events)
	switch line.Type {
	case "event_msg":
		var event struct {
			Type    string          `json:"type"`
			Message string          `json:"message"`
			Item    json.RawMessage `json:"item"`
		}
		if json.Unmarshal(line.Payload, &event) != nil {
			n.omissions = append(n.omissions, omission("malformed_record", "events", "A Codex event row could not be decoded."))
			return
		}
		switch event.Type {
		case "user_message", "agent_message":
			if profile == profileLegacy {
				role := "user"
				if event.Type == "agent_message" {
					role = "assistant"
				}
				n.events = append(n.events, messageEvent(role, event.Message))
			}
		case "error":
			n.events = append(n.events, contract.Event{Kind: contract.EventError, Error: &contract.ErrorEvent{Category: "provider", Message: event.Message}, Metadata: []contract.Metadata{}})
		case "item_completed":
			if profile == profileLegacy {
				return
			}
			var item completedItem
			if json.Unmarshal(event.Item, &item) != nil {
				n.omissions = append(n.omissions, omission("malformed_record", "events", "A completed Codex item could not be decoded."))
				return
			}
			n.completed(profile, item, event.Item)
		}
	case "response_item":
		if profile == profileNativePaginated {
			return
		}
		n.legacyResponse(line.Payload)
	}
	for i := before; i < len(n.events); i++ {
		contract.SetEventTime(&n.events[i], line.Timestamp)
	}
}

func (n *eventNormalizer) completed(profile storageProfile, item completedItem, raw json.RawMessage) {
	switch item.Type {
	case "UserMessage":
		text, hasText, contentOmission := normalizeUserContent(item.Content)
		if hasText {
			n.events = append(n.events, messageEvent("user", text))
		}
		if contentOmission != "" {
			message := "A message contained content that is not represented by the public text model."
			if contentOmission == "unknown_format" {
				message = "A message content type was not recognized."
			}
			n.omissions = append(n.omissions, omission(contentOmission, "events", message))
		}
	case "AgentMessage":
		text, valid := normalizeAgentContent(item.Content)
		if valid {
			n.events = append(n.events, messageEvent("assistant", text))
		} else {
			n.omissions = append(n.omissions, omission("unknown_format", "events", "An assistant message content type was not recognized."))
		}
	case "FunctionCallOutput":
		if profile == profileCanonicalizedLegacyPaginated {
			n.omissions = append(n.omissions, omission("unsupported_event", "events", "A migrated completed output is not emitted because its legacy response row is canonical."))
			return
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		body, state := legacyToolContent(fields["output"])
		result := contract.ToolResultEvent{Outcome: "unknown", Content: body, ContentState: state}
		metadata := []contract.Metadata{}
		var name string
		if json.Unmarshal(fields["name"], &name) == nil {
			metadata = append(metadata, contract.Metadata{Name: "tool_name", Value: name})
		}
		n.events = append(n.events, contract.Event{Kind: contract.EventToolResult, ToolResult: &result, Metadata: metadata})
	case "CommandExecution", "McpToolCall", "DynamicToolCall":
		if profile == profileCanonicalizedLegacyPaginated {
			n.omissions = append(n.omissions, omission("unsupported_event", "events", "A migrated completed tool item is not emitted because its legacy response rows are the canonical correlation evidence."))
			return
		}
		category := map[string]string{"CommandExecution": "shell", "McpToolCall": "mcp", "DynamicToolCall": "tool"}[item.Type]
		action := "invoke"
		if item.Type == "CommandExecution" {
			action = "execute"
		}
		callID := toolCorrelationID(n.threadID, item.ID)
		n.events = append(n.events, contract.Event{Kind: contract.EventToolCall, ToolCall: completedToolCall(n.threadID, item, raw, category, action), Metadata: codexToolMetadata(raw)})
		result := codexToolResult(item, callID)
		n.events = append(n.events, contract.Event{Kind: contract.EventToolResult, ToolResult: &result, Metadata: []contract.Metadata{}})
	default:
		if knownTurnItemType(item.Type) || item.Type == "Sleep" {
			n.omissions = append(n.omissions, omission("unsupported_event", "events", "A recognized Codex item is not represented by the public event model."))
		} else {
			n.omissions = append(n.omissions, omission("unknown_format", "events", "A completed Codex item type was not recognized."))
		}
	}
}

func (n *eventNormalizer) legacyResponse(raw json.RawMessage) {
	var item struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		CallID string `json:"call_id"`
	}
	if json.Unmarshal(raw, &item) != nil {
		n.omissions = append(n.omissions, omission("malformed_record", "events", "A Codex response row could not be decoded."))
		return
	}
	switch item.Type {
	case "function_call", "custom_tool_call", "local_shell_call":
		callID := toolCorrelationID(n.threadID, item.CallID)
		category, action := "tool", "invoke"
		if item.Type == "local_shell_call" || item.Name == "exec_command" || item.Name == "shell" {
			category, action = "shell", "execute"
		} else if item.Name == "apply_patch" {
			category, action = "file_change", "edit"
		}
		n.events = append(n.events, contract.Event{Kind: contract.EventToolCall, ToolCall: legacyToolCall(item.Type, item.Name, callID, raw, category, action), Metadata: codexToolMetadata(raw)})
	case "function_call_output", "custom_tool_call_output":
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		body, state := legacyToolContent(fields["output"])
		result := contract.ToolResultEvent{CallID: toolCorrelationID(n.threadID, item.CallID), Outcome: "unknown", Content: body, ContentState: state}
		n.events = append(n.events, contract.Event{Kind: contract.EventToolResult, ToolResult: &result, Metadata: []contract.Metadata{}})
	}

}

func (n *eventNormalizer) finish() ([]contract.Event, []contract.Omission) {
	if len(n.events) > maxNormalizedEvents {
		n.events = n.events[:maxNormalizedEvents]
		n.omissions = append(n.omissions, omission("resource_limit", "events", "The normalized event count exceeded the input limit."))
	}
	n.omissions = append(n.omissions, contract.ResolveToolCorrelations(n.events)...)
	n.omissions = append(n.omissions, contract.EventTimeOmissions(n.events)...)
	n.omissions = append(n.omissions, contract.ToolInputOmissions(n.events)...)
	return n.events, n.omissions
}

func toolCorrelationID(threadID, id string) string {
	if id == "" {
		return ""
	}
	return normalizedCallID(threadID, id)
}
func completedToolCall(threadID string, item completedItem, raw json.RawMessage, category, action string) *contract.ToolCallEvent {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	field := "arguments"
	name := ""
	_ = json.Unmarshal(fields["tool"], &name)
	if item.Type == "CommandExecution" {
		field = "command"
		name = item.Type
	}
	content, state := contract.RecordedContent(fields[field])
	if field == "arguments" && state == contract.EvidenceUnsupported && json.Valid(fields[field]) {
		// Arguments are JSON values in the verified completed-item format.
		content = contract.JSONContent(json.RawMessage(fields[field]))
		state = contract.EvidenceAvailable
	}
	inputState := contract.InputState(state)
	if state == contract.EvidenceAbsent {
		inputState = contract.InputUnavailable
	}
	if item.Type == "CommandExecution" && content != nil && (hasJSONValue(fields["cwd"]) || hasJSONValue(fields["interaction_input"])) {
		input := map[string]json.RawMessage{"command": fields[field]}
		for _, key := range []string{"cwd", "interaction_input"} {
			if hasJSONValue(fields[key]) {
				input[key] = fields[key]
			}
		}
		content = contract.JSONContent(input)
	}
	call := &contract.ToolCallEvent{CallID: toolCorrelationID(threadID, item.ID), Name: name, Category: category, Action: action, Input: content, InputState: inputState}
	return call
}
func legacyToolCall(itemType, name, id string, raw json.RawMessage, category, action string) *contract.ToolCallEvent {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	field := "arguments"
	if itemType == "custom_tool_call" {
		field = "input"
	}
	if itemType == "local_shell_call" {
		field = "action"
		name = itemType
	}
	content, state := contract.RecordedContent(fields[field])
	if itemType == "function_call" && content != nil && content.Format == "text" {
		var original string
		if json.Unmarshal(fields[field], &original) == nil && json.Valid([]byte(original)) {
			content.Format = "json"
		}
	}
	inputState := contract.InputState(state)
	if state == contract.EvidenceAbsent {
		inputState = contract.InputUnavailable
	}
	return &contract.ToolCallEvent{CallID: id, Name: name, Category: category, Action: action, Input: content, InputState: inputState}
}

// Legacy output text or verified text blocks do not establish an outcome.
func legacyToolContent(raw json.RawMessage) (*contract.Content, contract.EvidenceState) {
	if len(raw) > 0 && raw[0] == '[' {
		return recordedTextBlocks(raw, "input_text")
	}
	return contract.RecordedContent(raw)
}

func codexToolMetadata(raw json.RawMessage) []contract.Metadata {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	metadata := []contract.Metadata{}
	for _, key := range []string{"server", "namespace"} {
		var value string
		if json.Unmarshal(fields[key], &value) == nil {
			metadata = append(metadata, contract.Metadata{Name: key, Value: value})
		}
	}
	return metadata
}
