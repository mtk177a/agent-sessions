package codex

import (
	"encoding/json"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

type eventNormalizer struct {
	threadID       string
	events         []contract.Event
	omissions      []contract.Omission
	calls          map[string]string
	completedItems map[string]struct{}
}

func newEventNormalizer(threadID string) *eventNormalizer {
	return &eventNormalizer{threadID: threadID, calls: map[string]string{}, completedItems: map[string]struct{}{}}
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
			n.completed(profile, item)
		}
	case "response_item":
		if profile == profileNativePaginated {
			return
		}
		n.legacyResponse(line.Payload)
	}
}

func (n *eventNormalizer) completed(profile storageProfile, item completedItem) {
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
	case "CommandExecution", "McpToolCall", "DynamicToolCall":
		if profile == profileCanonicalizedLegacyPaginated {
			n.omissions = append(n.omissions, omission("unsupported_event", "events", "A migrated completed tool item is not emitted because its legacy response rows are the canonical correlation evidence."))
			return
		}
		if item.ID == "" {
			n.omissions = append(n.omissions, omission("correlation_omitted", "events", "A completed tool item lacked a correlation identifier."))
			return
		}
		if _, duplicate := n.completedItems[item.ID]; duplicate {
			n.omissions = append(n.omissions, omission("duplicate_call_id", "events", "A completed provider tool identifier was duplicated."))
			return
		}
		n.completedItems[item.ID] = struct{}{}
		category := map[string]string{"CommandExecution": "shell", "McpToolCall": "mcp", "DynamicToolCall": "tool"}[item.Type]
		action := "invoke"
		if item.Type == "CommandExecution" {
			action = "execute"
		}
		callID := normalizedCallID(n.threadID, item.ID)
		n.events = append(n.events, contract.Event{Kind: contract.EventToolCall, ToolCall: &contract.ToolCallEvent{CallID: callID, Category: category, Action: action, EvidenceState: contract.EvidenceAvailable}, Metadata: []contract.Metadata{}})
		result := codexToolResult(item, callID, item.Status == "completed" || item.ExitCode != nil && *item.ExitCode == 0)
		n.events = append(n.events, contract.Event{Kind: contract.EventToolResult, ToolResult: &result, Metadata: []contract.Metadata{}})
		n.omissions = append(n.omissions, contract.ToolResultOmissions(result)...)
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
		if item.CallID == "" {
			n.omissions = append(n.omissions, omission("correlation_omitted", "events", "A tool call lacked a correlation identifier."))
			return
		}
		if _, duplicate := n.calls[item.CallID]; duplicate {
			n.omissions = append(n.omissions, omission("duplicate_call_id", "events", "A provider tool call identifier was duplicated."))
			n.calls[item.CallID] = ""
			return
		}
		callID := normalizedCallID(n.threadID, item.CallID)
		n.calls[item.CallID] = callID
		category, action := "tool", "invoke"
		if item.Type == "local_shell_call" || item.Name == "exec_command" || item.Name == "shell" {
			category, action = "shell", "execute"
		} else if item.Name == "apply_patch" {
			category, action = "file_change", "edit"
		}
		n.events = append(n.events, contract.Event{Kind: contract.EventToolCall, ToolCall: &contract.ToolCallEvent{CallID: callID, Category: category, Action: action, EvidenceState: contract.EvidenceAvailable}, Metadata: []contract.Metadata{}})
	case "function_call_output", "custom_tool_call_output":
		if normalized, ok := n.calls[item.CallID]; ok && normalized != "" {
			n.omissions = append(n.omissions, omission("unsupported_tool_result", "events", "A correlated tool result did not expose a safe success value."))
			delete(n.calls, item.CallID)
		} else {
			n.omissions = append(n.omissions, omission("correlation_omitted", "events", "A tool result did not reference an earlier unique call."))
		}
	}
}

func (n *eventNormalizer) finish() ([]contract.Event, []contract.Omission) {
	for _, normalized := range n.calls {
		if normalized != "" {
			n.omissions = append(n.omissions, omission("correlation_omitted", "events", "A tool call did not have a safely correlated persisted result."))
		}
	}
	if len(n.events) > maxNormalizedEvents {
		n.events = n.events[:maxNormalizedEvents]
		n.omissions = append(n.omissions, omission("resource_limit", "events", "The normalized event count exceeded the input limit."))
	}
	return n.events, n.omissions
}
