package codex

import (
	"encoding/json"
	"time"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

const (
	maxTimeHistoryBytes = 128 << 20
	maxTimeRowBytes     = 4 << 20
)

// readLastInteractionAt uses only recognized conversation rows. An unfamiliar row
// may contain a later interaction, so it makes the timestamp unavailable.
func readLastInteractionAt(root string, item artifact, byRollout map[string][]artifact) (string, *contract.VersionHint, bool) {
	spans, err := resolveHistory(item, byRollout)
	if err != nil {
		return "", nil, false
	}
	var latest time.Time
	safeTime := true
	for _, span := range spans {
		if !isSupportedVersion(span.item.meta.CLIVersion) {
			return "", nil, false
		}
	}
	hintValue, err := walkHistory(root, spans, maxTimeHistoryBytes, maxTimeRowBytes, func(origin artifact, line rolloutLine) {
		activity, safe := codexInteractionRow(origin.meta.HistoryMode, origin.meta.CLIVersion, line)
		if !safe {
			safeTime = false
			return
		}
		if !activity {
			return
		}
		at, err := time.Parse(time.RFC3339Nano, line.Timestamp)
		if err != nil {
			safeTime = false
			return
		}
		if latest.IsZero() || at.After(latest) {
			latest = at
		}
	})
	if err != nil {
		return "", nil, false
	}
	var hint *contract.VersionHint
	if item.meta.HistoryBase != nil {
		hint = &contract.VersionHint{Kind: "content_hash", Value: hintValue}
	}
	if !safeTime || latest.IsZero() {
		return "", hint, false
	}
	return latest.UTC().Format(time.RFC3339Nano), hint, true
}

func codexInteractionRow(historyMode, version string, line rolloutLine) (bool, bool) {
	switch line.Type {
	case "session_meta", "inter_agent_communication_metadata", "compacted", "turn_context", "world_state", "security_risk_score":
		return false, true
	case "token_usage_record":
		return false, supportsTokenUsageRecord(version)
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
				Kind string `json:"kind"`
				ID   string `json:"id"`
			}
			if json.Unmarshal(event.Item, &completed) != nil || !knownTurnItemType(completed.Type) {
				return false, false
			}
			switch completed.Type {
			case "UserMessage", "AgentMessage", "CommandExecution", "McpToolCall", "DynamicToolCall", "CollabAgentToolCall", "FileChange", "FunctionCallOutput":
				return true, true
			case "Extension":
				known := completed.Kind == "web.search" || completed.Kind == "clock.sleep"
				return known && completed.ID != "", known && completed.ID != ""
			case "Reasoning", "Plan", "ContextCompaction", "HookPrompt", "EnteredReviewMode", "ExitedReviewMode", "SubAgentActivity":
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
