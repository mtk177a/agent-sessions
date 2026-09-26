package contract

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"
)

func ValidInputState(state InputState) bool {
	return state == InputAvailable || state == InputAbsent || state == InputUnavailable || state == InputUnsupported
}
func ValidEvidenceState(state EvidenceState) bool {
	return state == EvidenceAvailable || state == EvidenceAbsent || state == EvidenceUnavailable || state == EvidenceUnsupported
}
func ValidOutcome(value string) bool {
	return value == "success" || value == "failure" || value == "unknown"
}
func ValidToolAction(action string) bool {
	switch action {
	case "execute", "read", "search", "write", "edit", "invoke":
		return true
	}
	return false
}
func ValidContent(content *Content, available bool) bool {
	if content == nil {
		return !available
	}
	if !available || content.Format != "text" && content.Format != "json" || len(content.Text) > MaxStringBytes || !utf8.ValidString(content.Text) {
		return false
	}
	return content.Format != "json" || content.Truncated || json.Valid([]byte(content.Text))
}
func TextContent(text string) *Content {
	content := &Content{Format: "text", Text: text}
	boundContent(content)
	return content
}

// JSONContent encodes an already decoded, adapter-selected value, not a raw record.
func JSONContent(value any) *Content {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	content := &Content{Format: "json", Text: string(raw)}
	boundContent(content)
	return content
}

// RecordedContent retains strings as text and objects/arrays as JSON.
func RecordedContent(raw json.RawMessage) (*Content, EvidenceState) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, EvidenceAbsent
	}
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return nil, EvidenceUnsupported
		}
		return TextContent(text), EvidenceAvailable
	}
	if raw[0] != '{' && raw[0] != '[' || !json.Valid(raw) {
		return nil, EvidenceUnsupported
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		return nil, EvidenceUnsupported
	}
	content := &Content{Format: "json", Text: compact.String()}
	boundContent(content)
	return content, EvidenceAvailable
}
func ToolInputOmissions(events []Event) []Omission {
	count, truncated, names := 0, 0, 0
	for _, event := range events {
		if event.ToolCall == nil {
			continue
		}
		call := event.ToolCall
		if call.InputState == InputUnavailable || call.InputState == InputUnsupported {
			count++
		}
		if call.Input != nil && call.Input.Truncated {
			truncated++
		}
		if call.Name == "" {
			names++
		}
	}
	omissions := []Omission{}
	if count > 0 {
		omissions = append(omissions, Omission{Code: "tool_input_unavailable", Scope: "tool_call", Count: count, Message: "Recorded tool inputs could not be fully read."})
	}
	if truncated > 0 {
		omissions = append(omissions, Omission{Code: "tool_input_truncated", Scope: "tool_call", Count: truncated, Message: "Recorded tool inputs exceeded the content bound."})
	}
	if names > 0 {
		omissions = append(omissions, Omission{Code: "tool_name_unavailable", Scope: "tool_call", Count: names, Message: "Recorded tool names were unavailable."})
	}
	return omissions
}
func ToolResultOmissions(result ToolResultEvent) []Omission {
	omissions := []Omission{}
	add := func(code, message string) {
		omissions = append(omissions, Omission{Code: code, Scope: "tool_result", Message: message})
	}
	if result.Outcome == "unknown" {
		add("tool_outcome_unknown", "The recorded tool outcome could not be established.")
	}
	if result.CorrelationState != "matched" {
		add("correlation_omitted", "The tool result could not be uniquely related to a recorded call.")
	}
	if result.ContentState == EvidenceUnavailable || result.ContentState == EvidenceUnsupported {
		add("tool_content_unavailable", "The recorded tool result body could not be fully read.")
	}
	if result.Content != nil {
		if result.Content.Truncated {
			add("tool_content_truncated", "The recorded tool result exceeded the content bound.")
		}
		if result.Content.Omitted {
			add("unsupported_content", "Non-text or unsupported tool content was omitted.")
		}
	}
	return omissions
}

// ResolveToolCorrelations uses only explicit adapter-owned IDs across the full
// selected history, before pagination. Ambiguous observations retain their bodies.
func ResolveToolCorrelations(events []Event) []Omission {
	calls, results := map[string]int{}, map[string]int{}
	for _, e := range events {
		if e.ToolCall != nil && e.ToolCall.CallID != "" {
			calls[e.ToolCall.CallID]++
		}
		if e.ToolResult != nil && e.ToolResult.CallID != "" {
			results[e.ToolResult.CallID]++
		}
	}
	omissions := []Omission{}
	for i := range events {
		e := &events[i]
		if e.ToolCall != nil {
			id := e.ToolCall.CallID
			if id == "" || calls[id] > 1 {
				e.ToolCall.CallID = ""
				omissions = append(omissions, Omission{Code: "correlation_omitted", Scope: "tool_call", Message: "A tool call lacked a unique correlation identifier."})
			} else if results[id] == 0 {
				omissions = append(omissions, Omission{Code: "correlation_omitted", Scope: "tool_call", Message: "A tool call had no recorded result."})
			}
		}
		if e.ToolResult != nil {
			r := e.ToolResult
			id := r.CallID
			r.CorrelationState = "unmatched"
			if id != "" && (calls[id] > 1 || results[id] > 1) {
				r.CorrelationState = "ambiguous"
			} else if id != "" && calls[id] == 1 {
				r.CorrelationState = "matched"
			}
			if r.CorrelationState != "matched" {
				r.CallID = ""
			}
			omissions = append(omissions, ToolResultOmissions(*r)...)
		}
	}
	return omissions
}
