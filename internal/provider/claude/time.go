package claude

import (
	"bufio"
	"bytes"
	"time"

	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/safeio"
)

func readLastInteractionAt(root string, item artifact) (string, bool) {
	data, err := safeio.ReadFileWithin(root, item.relative, maxArtifactBytes)
	if err != nil {
		return "", false
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), maxRowBytes)
	var latest time.Time
	for scanner.Scan() {
		var row transcriptRow
		if safeio.DecodeJSON(scanner.Bytes(), contract.MaxJSONDepth, &row) != nil || row.Type == "" {
			return "", false
		}
		if row.SessionID != "" {
			id, ok := canonicalSessionID(row.SessionID)
			if !ok || id != item.sessionID {
				return "", false
			}
		}
		if row.Version != "" && !supportsInteractionTimeVersion(row.Version) {
			return "", false
		}
		activity, safe := claudeInteractionRow(row)
		if !safe {
			return "", false
		}
		if !activity {
			continue
		}
		if row.SessionID == "" || row.Version == "" {
			return "", false
		}
		at, err := time.Parse(time.RFC3339Nano, row.Timestamp)
		if err != nil {
			return "", false
		}
		if latest.IsZero() || at.After(latest) {
			latest = at
		}
	}
	if scanner.Err() != nil || latest.IsZero() {
		return "", false
	}
	return latest.UTC().Format(time.RFC3339Nano), true
}

func claudeInteractionRow(row transcriptRow) (bool, bool) {
	if knownBookkeepingRow(row.Type) {
		return false, true
	}
	if !knownObservationRow(row.Type) {
		return false, false
	}
	switch row.Type {
	case "system":
		if row.Subtype == "turn_duration" || row.Subtype == "compact_boundary" {
			return false, true
		}
		return false, row.Subtype == "away_summary" || row.Subtype == "informational"
	case "attachment":
		var attachment struct {
			Type string `json:"type"`
		}
		if len(row.Attachment) == 0 || safeio.DecodeJSON(row.Attachment, contract.MaxJSONDepth, &attachment) != nil {
			return false, false
		}
		switch attachment.Type {
		case "agent_listing_delta", "command_permissions", "deferred_tools_delta", "diagnostics", "edited_text_file", "opened_file_in_ide", "plan_mode", "plan_mode_exit", "selected_lines_in_ide", "skill_listing", "task_reminder":
			return false, true
		default:
			return false, false
		}
	case "user":
		if row.IsMeta {
			return false, true
		}
		payload, ok := decodeMessage(row.Message, "user")
		if !ok {
			return false, false
		}
		if text, ok := decodeStringContent(payload.Content); ok {
			return text != "", true
		}
		blocks, ok := decodeBlocks(payload.Content)
		if !ok {
			return false, false
		}
		activity := false
		for _, block := range blocks {
			switch block.Type {
			case "text", "tool_result":
				activity = true
			case "image", "document", "tool_reference":
				return false, false
			default:
				return false, false
			}
		}
		return activity, true
	case "assistant":
		payload, ok := decodeMessage(row.Message, "assistant")
		if !ok {
			return false, false
		}
		if row.IsAPIErrorMessage && payload.StopReason != "refusal" {
			return false, true
		}
		blocks, ok := decodeBlocks(payload.Content)
		if !ok {
			return false, false
		}
		activity := false
		for _, block := range blocks {
			switch block.Type {
			case "text", "tool_use":
				activity = true
			case "thinking", "redacted_thinking":
			default:
				return false, false
			}
		}
		return activity, true
	default:
		return false, false
	}
}
