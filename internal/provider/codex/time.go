package codex

import (
	"encoding/json"
	"time"

	"github.com/mtk177a/agent-sessions/internal/contract"
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
	requireOrdinals := false
	for _, span := range spans {
		requireOrdinals = requireOrdinals || compatibilityProfile(span.item.meta.CLIVersion, span.item.meta.HistoryMode) == profileCanonicalizedLegacyPaginated
	}
	plan, err := walkHistory(root, spans, maxEffectiveHistoryBytes, maxHistoryRowBytes, requireOrdinals, func(origin artifact, line rolloutLine) {
		if !supportsInteractionTime(origin.meta.CLIVersion, origin.meta.HistoryMode) {
			safeTime = false
			return
		}
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
	if plan.logical {
		hint = &contract.VersionHint{Kind: "content_hash", Value: plan.hint}
	}
	if plan.malformedRows > 0 || plan.oversizedRows > 0 {
		safeTime = false
	}
	if !safeTime || latest.IsZero() {
		return "", hint, false
	}
	return latest.UTC().Format(time.RFC3339Nano), hint, true
}

func codexInteractionRow(historyMode, version string, line rolloutLine) (bool, bool) {
	profile := compatibilityProfile(version, historyMode)
	if !validRowForProfile(profile, version, line) {
		return false, false
	}
	switch line.Type {
	case "session_meta", "inter_agent_communication_metadata", "compacted", "turn_context", "world_state":
		return false, true
	case "security_risk_score":
		return false, true
	case "token_usage_record":
		return false, true
	case "inter_agent_communication", "realtime_item":
		return false, false
	case "event_msg":
		var event struct {
			Type string          `json:"type"`
			Item json.RawMessage `json:"item"`
		}
		if json.Unmarshal(line.Payload, &event) != nil {
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
			if json.Unmarshal(event.Item, &completed) != nil {
				return false, false
			}
			if completed.Type == "Sleep" && version == "0.144.2" {
				return true, true
			}
			switch completed.Type {
			case "UserMessage", "AgentMessage", "CommandExecution", "McpToolCall", "DynamicToolCall", "CollabAgentToolCall", "FileChange", "FunctionCallOutput", "WebSearch":
				return true, true
			case "Extension":
				known := completed.Kind == "web.search" || (completed.Kind == "clock.sleep" && version != "0.144.2")
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
		if json.Unmarshal(line.Payload, &response) != nil {
			return false, false
		}
		switch response.Type {
		case "function_call", "custom_tool_call", "local_shell_call", "function_call_output", "custom_tool_call_output", "web_search_call", "tool_search_call", "tool_search_output":
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
