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
		if forward && !validForwardEvent(line.Payload, event.Type) {
			return false
		}
		if event.Type == "item_completed" {
			var item struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(event.Item, &item) != nil || !knownTurnItemType(item.Type) {
				return false
			}
			return !forward || validForwardCompletedItem(event.Item, item.Type)
		}
	}
	if line.Type == "response_item" {
		var response struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line.Payload, &response) != nil || !knownResponseType(response.Type) {
			return false
		}
		return !forward || validForwardResponseItem(line.Payload, response.Type)
	}
	return true
}

func validForwardEvent(raw json.RawMessage, eventType string) bool {
	fields, ok := jsonObjectFields(raw)
	if !ok {
		return false
	}
	switch eventType {
	case "user_message", "agent_message":
		return hasRequiredFields(fields, "message") && jsonString(fields["message"])
	default:
		return true
	}
}

func validForwardCompletedItem(raw json.RawMessage, itemType string) bool {
	fields, ok := jsonObjectFields(raw)
	if !ok {
		return false
	}
	required := func(names ...string) bool { return hasRequiredFields(fields, names...) }
	id := func() bool { return required("id") && jsonNonEmptyString(fields["id"]) }
	switch itemType {
	case "UserMessage", "AgentMessage":
		return id() && required("content") && jsonArray(fields["content"])
	case "FunctionCallOutput":
		return id() && required("name", "output") && jsonString(fields["name"]) && jsonStringOrArray(fields["output"])
	case "CommandExecution":
		return id() && required("command", "cwd", "parsed_cmd", "source", "status") &&
			jsonArray(fields["command"]) && jsonString(fields["cwd"]) && jsonArray(fields["parsed_cmd"]) &&
			jsonStringIn(fields["source"], "agent", "user_shell", "unified_exec_startup", "unified_exec_interaction") &&
			jsonStringIn(fields["status"], "in_progress", "completed", "failed", "declined")
	case "DynamicToolCall":
		return id() && required("tool", "arguments", "status") && jsonString(fields["tool"]) &&
			jsonStringIn(fields["status"], "in_progress", "completed", "failed")
	case "CollabAgentToolCall":
		return id() && required("tool", "status", "sender_thread_id") &&
			jsonStringIn(fields["tool"], "spawn_agent", "send_input", "resume_agent", "wait", "close_agent", "send_message", "followup_task", "interrupt_agent", "list_agents") &&
			jsonStringIn(fields["status"], "in_progress", "completed", "failed", "interrupted") && jsonString(fields["sender_thread_id"])
	case "WebSearch":
		return id() && required("query", "action") && jsonString(fields["query"]) && jsonObjectWithStringField(fields["action"], "type")
	case "FileChange":
		return id() && required("changes") && jsonObject(fields["changes"])
	case "McpToolCall":
		return id() && required("server", "tool", "arguments", "status") && jsonString(fields["server"]) && jsonString(fields["tool"]) &&
			jsonStringIn(fields["status"], "inProgress", "completed", "failed")
	case "Extension":
		return id() && required("kind") && jsonString(fields["kind"])
	default:
		return true
	}
}

func validForwardResponseItem(raw json.RawMessage, itemType string) bool {
	fields, ok := jsonObjectFields(raw)
	if !ok {
		return false
	}
	required := func(names ...string) bool { return hasRequiredFields(fields, names...) }
	switch itemType {
	case "message":
		return required("role", "content") && jsonString(fields["role"]) && jsonArray(fields["content"])
	case "agent_message":
		return required("author", "recipient", "content") && jsonString(fields["author"]) &&
			jsonString(fields["recipient"]) && jsonArray(fields["content"])
	case "local_shell_call":
		return required("status", "action") &&
			jsonStringIn(fields["status"], "completed", "in_progress", "incomplete") &&
			validForwardLocalShellAction(fields["action"])
	case "function_call":
		return required("name", "arguments", "call_id") && jsonString(fields["name"]) &&
			jsonString(fields["arguments"]) && jsonString(fields["call_id"])
	case "function_call_output":
		return required("output") && jsonStringOrArray(fields["output"])
	case "custom_tool_call":
		return required("call_id", "name", "input") && jsonString(fields["call_id"]) &&
			jsonString(fields["name"]) && jsonString(fields["input"])
	case "custom_tool_call_output":
		return required("call_id", "output") && jsonString(fields["call_id"]) &&
			jsonStringOrArray(fields["output"])
	case "tool_search_call":
		return required("execution", "arguments") && jsonString(fields["execution"])
	case "tool_search_output":
		return required("status", "execution", "tools") && jsonString(fields["status"]) &&
			jsonString(fields["execution"]) && jsonArray(fields["tools"])
	case "web_search_call":
		return jsonOptionalString(fields, "status") && jsonOptionalObjectWithStringField(fields, "action", "type")
	default:
		return true
	}
}

func validForwardLocalShellAction(raw json.RawMessage) bool {
	fields, ok := jsonObjectFields(raw)
	return ok && hasRequiredFields(fields, "type", "command") &&
		jsonStringIn(fields["type"], "exec") && jsonArray(fields["command"])
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

func jsonString(raw json.RawMessage) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil
}

func jsonStringIn(raw json.RawMessage, allowed ...string) bool {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func jsonArray(raw json.RawMessage) bool {
	var value []json.RawMessage
	return json.Unmarshal(raw, &value) == nil && value != nil
}

func jsonStringOrArray(raw json.RawMessage) bool {
	return jsonString(raw) || jsonArray(raw)
}

func jsonOptionalString(fields map[string]json.RawMessage, name string) bool {
	raw, ok := fields[name]
	return !ok || jsonNull(raw) || jsonString(raw)
}

func jsonOptionalObjectWithStringField(fields map[string]json.RawMessage, name, field string) bool {
	raw, ok := fields[name]
	return !ok || jsonNull(raw) || jsonObjectWithStringField(raw, field)
}

func jsonNull(raw json.RawMessage) bool {
	var value any
	return json.Unmarshal(raw, &value) == nil && value == nil
}

func jsonObjectWithStringField(raw json.RawMessage, name string) bool {
	fields, ok := jsonObjectFields(raw)
	return ok && hasRequiredFields(fields, name) && jsonString(fields[name])
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
