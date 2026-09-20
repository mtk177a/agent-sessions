package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"time"

	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/safeio"
)

// readLastInteractionAt uses only recognized conversation rows. An unfamiliar row
// may contain a later interaction, so it makes the timestamp unavailable.
func readLastInteractionAt(root string, item artifact) (string, bool) {
	if item.meta.HistoryBase != nil || item.compressed {
		return "", false
	}
	data, err := safeio.ReadFileWithin(root, item.relative, maxArtifactBytes)
	if err != nil {
		return "", false
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), maxRowBytes)
	var latest time.Time
	for scanner.Scan() {
		var line rolloutLine
		if safeio.DecodeJSON(scanner.Bytes(), contract.MaxJSONDepth, &line) != nil {
			return "", false
		}
		activity, safe := codexInteractionRow(item.meta.HistoryMode, item.meta.CLIVersion, line)
		if !safe {
			return "", false
		}
		if !activity {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, line.Timestamp)
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

func codexInteractionRow(historyMode, version string, line rolloutLine) (bool, bool) {
	switch line.Type {
	case "session_meta", "inter_agent_communication_metadata", "compacted", "turn_context", "world_state", "security_risk_score":
		return false, true
	case "token_usage_record":
		return false, version == "0.153.0"
	case "inter_agent_communication", "realtime_item":
		return false, false
	case "event_msg":
		var event struct {
			Type string          `json:"type"`
			Item json.RawMessage `json:"item"`
		}
		if json.Unmarshal(line.Payload, &event) != nil || !knownEventType(event.Type) {
			return false, false
		}
		if historyMode == "legacy" && (event.Type == "user_message" || event.Type == "agent_message") {
			return true, true
		}
		if historyMode == "paginated" && event.Type == "item_completed" {
			var completed struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(event.Item, &completed) != nil || !knownTurnItemType(completed.Type) {
				return false, false
			}
			switch completed.Type {
			case "UserMessage", "AgentMessage", "CommandExecution", "McpToolCall", "DynamicToolCall":
				return true, true
			case "Reasoning", "Plan", "ContextCompaction", "HookPrompt", "EnteredReviewMode", "ExitedReviewMode":
				return false, true
			default:
				return false, false
			}
		}
		if historyMode == "legacy" && event.Type == "item_completed" {
			return false, false
		}
		return false, true
	case "response_item":
		var response struct {
			Type string `json:"type"`
			Role string `json:"role"`
		}
		if json.Unmarshal(line.Payload, &response) != nil || !knownResponseType(response.Type) {
			return false, false
		}
		switch response.Type {
		case "function_call", "custom_tool_call", "local_shell_call", "function_call_output", "custom_tool_call_output":
			return true, true
		case "message":
			switch response.Role {
			case "user", "assistant":
				return true, true
			case "system", "developer":
				return false, true
			default:
				return false, false
			}
		case "agent_message":
			return true, true
		case "reasoning", "compaction", "compaction_summary", "context_compaction", "configuration_update":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}
