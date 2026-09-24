package codex

import "encoding/json"

// validRowForProfile owns the known row vocabulary and shape checks for a
// stored Codex format. Consumers still decide how a valid row affects their
// operation.
func validRowForProfile(profile storageProfile, version string, line rolloutLine, forward bool) bool {
	if profile == profileUnsupported {
		return false
	}
	if profile == profileCanonicalizedLegacyPaginated {
		return validCanonicalizedLegacyRow(version, line)
	}
	if line.Type == "token_usage_record" {
		return forward && validForwardBookkeepingRow(line) || !forward && supportsTokenUsageRecord(version)
	}
	if !knownTopLevel(line.Type) {
		return false
	}
	if forward && isForwardBookkeepingType(line.Type) && !validForwardBookkeepingRow(line) {
		return false
	}
	if line.Type == "event_msg" {
		var event struct {
			Type string          `json:"type"`
			Item json.RawMessage `json:"item"`
		}
		if json.Unmarshal(line.Payload, &event) != nil || !knownEventType(event.Type) {
			return false
		}
		if event.Type == "item_completed" {
			var item struct {
				Type string `json:"type"`
			}
			return json.Unmarshal(event.Item, &item) == nil && knownTurnItemType(item.Type)
		}
	}
	if line.Type == "response_item" {
		var response struct {
			Type string `json:"type"`
		}
		return json.Unmarshal(line.Payload, &response) == nil && knownResponseType(response.Type)
	}
	return true
}

func isForwardBookkeepingType(value string) bool {
	switch value {
	case "session_meta", "turn_context", "compacted", "world_state", "inter_agent_communication_metadata", "security_risk_score":
		return true
	default:
		return false
	}
}

func validForwardBookkeepingRow(line rolloutLine) bool {
	fields, ok := jsonObjectFields(line.Payload)
	if !ok {
		return false
	}
	switch line.Type {
	case "session_meta", "turn_context", "compacted", "security_risk_score":
		return true
	case "world_state":
		return hasRequiredFields(fields, "full", "state") && jsonBool(fields["full"]) && jsonObject(fields["state"])
	case "inter_agent_communication_metadata":
		return hasRequiredFields(fields, "trigger_turn") && jsonBool(fields["trigger_turn"])
	case "token_usage_record":
		if !hasRequiredFields(fields, "thread_id", "turn_id", "session_id", "root_turn_id", "response_id", "usage", "turn_token_usage", "thread_token_usage") {
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

// Canonicalized rollouts retain their old CLI version after Codex rewrites
// their rows. Only the observed migrated vocabulary is accepted.
func validCanonicalizedLegacyRow(version string, line rolloutLine) bool {
	switch line.Type {
	case "session_meta", "turn_context", "compacted":
		return true
	case "world_state", "inter_agent_communication_metadata", "token_usage_record":
		return validCanonicalizedBookkeepingRow(version, line)
	case "event_msg":
		var event struct {
			Type string          `json:"type"`
			Item json.RawMessage `json:"item"`
		}
		if json.Unmarshal(line.Payload, &event) != nil || !knownCanonicalizedEventType(event.Type) {
			return false
		}
		if event.Type == "item_completed" {
			var item struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(event.Item, &item) != nil {
				return false
			}
			return knownCanonicalizedItemType(item.Type) || item.Type == "Sleep" && version == "0.144.2"
		}
		return true
	case "response_item":
		var response struct {
			Type string `json:"type"`
		}
		return json.Unmarshal(line.Payload, &response) == nil && knownCanonicalizedResponseType(response.Type)
	default:
		return false
	}
}

func validCanonicalizedBookkeepingRow(version string, line rolloutLine) bool {
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

func hasRequiredFields(fields map[string]json.RawMessage, names ...string) bool {
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

func knownTopLevel(value string) bool {
	switch value {
	case "session_meta", "response_item", "event_msg", "inter_agent_communication", "inter_agent_communication_metadata", "compacted", "turn_context", "world_state", "security_risk_score":
		return true
	default:
		return false
	}
}

func knownEventType(value string) bool {
	switch value {
	case "user_message", "agent_message", "error", "item_completed", "task_started", "turn_started", "task_complete", "turn_complete", "turn_aborted", "token_count", "thread_rolled_back", "thread_settings_applied", "context_compacted", "agent_reasoning", "agent_reasoning_raw_content", "mcp_tool_call_end", "web_search_end", "image_generation_end", "entered_review_mode", "exited_review_mode", "sub_agent_activity":
		return true
	default:
		return false
	}
}

func knownTurnItemType(value string) bool {
	switch value {
	case "UserMessage", "FunctionCallOutput", "HookPrompt", "AgentMessage", "Plan", "Reasoning", "CommandExecution", "DynamicToolCall", "CollabAgentToolCall", "SubAgentActivity", "WebSearch", "ImageView", "Extension", "ImageGeneration", "EnteredReviewMode", "ExitedReviewMode", "FileChange", "McpToolCall", "ContextCompaction":
		return true
	default:
		return false
	}
}

func knownResponseType(value string) bool {
	switch value {
	case "message", "agent_message", "reasoning", "local_shell_call", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "tool_search_call", "tool_search_output", "web_search_call", "image_generation_call", "configuration_update", "compaction", "compaction_summary", "context_compaction":
		return true
	default:
		return false
	}
}

func knownCanonicalizedEventType(value string) bool {
	switch value {
	case "item_completed", "task_started", "task_complete", "token_count", "turn_aborted", "thread_settings_applied", "turn_started", "turn_complete", "thread_rolled_back":
		return true
	default:
		return false
	}
}

func knownCanonicalizedItemType(value string) bool {
	switch value {
	case "UserMessage", "HookPrompt", "AgentMessage", "Plan", "Reasoning", "CommandExecution", "DynamicToolCall", "CollabAgentToolCall", "SubAgentActivity", "WebSearch", "ImageView", "Extension", "ImageGeneration", "EnteredReviewMode", "ExitedReviewMode", "FileChange", "McpToolCall", "ContextCompaction":
		return true
	default:
		return false
	}
}

func knownCanonicalizedResponseType(value string) bool {
	switch value {
	case "message", "agent_message", "reasoning", "local_shell_call", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "tool_search_call", "tool_search_output", "web_search_call", "image_generation_call", "compaction", "context_compaction":
		return true
	default:
		return false
	}
}
