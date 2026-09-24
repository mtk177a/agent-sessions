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

func isOlderTimeVersion(version string) bool {
	return compatibilityProfile(version, "paginated") == profileCanonicalizedLegacyPaginated
}

func isMigratedLegacyTimeVersion(version string) bool {
	return compatibilityProfile(version, "paginated") == profileCanonicalizedLegacyPaginated
}

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
		requireOrdinals = requireOrdinals || isMigratedLegacyTimeVersion(span.item.meta.CLIVersion)
	}
	plan, err := walkHistory(root, spans, maxTimeHistoryBytes, maxTimeRowBytes, requireOrdinals, func(origin artifact, line rolloutLine) {
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
	if !safeTime || latest.IsZero() {
		return "", hint, false
	}
	return latest.UTC().Format(time.RFC3339Nano), hint, true
}

func codexInteractionRow(historyMode, version string, line rolloutLine) (bool, bool) {
	if isMigratedLegacyTimeVersion(version) && (historyMode != "paginated" || !knownMigratedTimeRow(version, line)) {
		return false, false
	}
	switch line.Type {
	case "session_meta", "inter_agent_communication_metadata", "compacted", "turn_context", "world_state":
		return false, true
	case "security_risk_score":
		return false, !isOlderTimeVersion(version)
	case "token_usage_record":
		return false, isMigratedLegacyTimeVersion(version) || supportsTokenUsageRecord(version)
	case "inter_agent_communication", "realtime_item":
		return false, false
	case "event_msg":
		var event struct {
			Type string          `json:"type"`
			Item json.RawMessage `json:"item"`
		}
		if json.Unmarshal(line.Payload, &event) != nil || !knownEventType(event.Type) || isOlderTimeVersion(version) && !knownOlderTimeEventType(event.Type) {
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
			if !knownTurnItemType(completed.Type) || isOlderTimeVersion(version) && !knownOlderTimeItemType(completed.Type) {
				return false, false
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
		if json.Unmarshal(line.Payload, &response) != nil || !knownResponseType(response.Type) || isOlderTimeVersion(version) && !knownOlderTimeResponseType(response.Type) {
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

// These headers retain the old CLI version after Codex rewrites the rollout.
// Only the observed migrated row vocabulary is accepted for interaction time.
func knownMigratedTimeRow(version string, line rolloutLine) bool {
	switch line.Type {
	case "session_meta", "turn_context", "compacted":
		return true
	case "world_state", "inter_agent_communication_metadata", "token_usage_record":
		return knownMigratedBookkeepingRow(version, line)
	case "event_msg":
		var event struct {
			Type string          `json:"type"`
			Item json.RawMessage `json:"item"`
		}
		if json.Unmarshal(line.Payload, &event) != nil {
			return false
		}
		if !knownOlderTimeEventType(event.Type) {
			return false
		}
		if event.Type == "item_completed" {
			var item struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(event.Item, &item) != nil {
				return false
			}
			return knownOlderTimeItemType(item.Type) || item.Type == "Sleep" && version == "0.144.2"
		}
		return true
	case "response_item":
		var response struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line.Payload, &response) != nil {
			return false
		}
		return knownOlderTimeResponseType(response.Type)
	}
	return false
}

func knownMigratedBookkeepingRow(version string, line rolloutLine) bool {
	fields, ok := jsonObjectFields(line.Payload)
	if !ok {
		return false
	}
	switch line.Type {
	case "world_state":
		if version != "0.139.0" && version != "0.142.5" && version != "0.144.2" || !hasExactFields(fields, "full", "state") {
			return false
		}
		return jsonBool(fields["full"]) && jsonObject(fields["state"])
	case "inter_agent_communication_metadata":
		return version == "0.144.2" && hasExactFields(fields, "trigger_turn") && jsonBool(fields["trigger_turn"])
	case "token_usage_record":
		if version != "0.142.5" || !hasExactFields(fields, "thread_id", "turn_id", "session_id", "root_turn_id", "response_id", "usage", "turn_token_usage", "thread_token_usage") {
			return false
		}
		for _, name := range []string{"thread_id", "turn_id", "session_id", "root_turn_id", "response_id"} {
			if !jsonNonEmptyString(fields[name]) {
				return false
			}
		}
		return jsonObject(fields["usage"]) && jsonObject(fields["turn_token_usage"]) && jsonObject(fields["thread_token_usage"])
	default:
		return false
	}
}

func jsonObjectFields(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, false
	}
	return fields, true
}

func hasExactFields(fields map[string]json.RawMessage, names ...string) bool {
	if len(fields) != len(names) {
		return false
	}
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			return false
		}
	}
	return true
}

func jsonBool(raw json.RawMessage) bool {
	var value bool
	return json.Unmarshal(raw, &value) == nil
}

func jsonNonEmptyString(raw json.RawMessage) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && value != ""
}

func jsonObject(raw json.RawMessage) bool {
	_, ok := jsonObjectFields(raw)
	return ok
}

func knownOlderTimeEventType(value string) bool {
	switch value {
	case "item_completed", "task_started", "task_complete", "token_count", "turn_aborted", "thread_settings_applied", "turn_started", "turn_complete", "thread_rolled_back":
		return true
	default:
		return false
	}
}

func knownOlderTimeItemType(value string) bool {
	switch value {
	case "UserMessage", "HookPrompt", "AgentMessage", "Plan", "Reasoning", "CommandExecution", "DynamicToolCall", "CollabAgentToolCall", "SubAgentActivity", "WebSearch", "ImageView", "Extension", "ImageGeneration", "EnteredReviewMode", "ExitedReviewMode", "FileChange", "McpToolCall", "ContextCompaction":
		return true
	default:
		return false
	}
}

func knownOlderTimeResponseType(value string) bool {
	switch value {
	case "message", "agent_message", "reasoning", "local_shell_call", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "tool_search_call", "tool_search_output", "web_search_call", "image_generation_call", "compaction", "context_compaction":
		return true
	default:
		return false
	}
}
