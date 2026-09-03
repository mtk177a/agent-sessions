package codex

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/provider"
	"github.com/mtk177a/agent-sessions/internal/safeio"
)

const (
	providerName        = "codex"
	supportedVersion    = "0.149.1"
	maxDiscoveredFiles  = 100_000
	maxArtifactBytes    = int64(contract.MaxEvidenceBytes)
	maxHeaderBytes      = 1 << 20
	maxRowBytes         = 1 << 20
	maxNormalizedEvents = 100_000
)

var errAmbiguousArtifact = errors.New("ambiguous Codex artifact")

var rolloutNamePattern = regexp.MustCompile(`^rollout-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})-([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:_([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}))?\.jsonl(?:\.zst)?$`)
var threadIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Adapter struct{}

type artifact struct {
	relative   string
	collection string
	size       int64
	modTime    time.Time
	threadID   string
	rolloutID  string
	timestamp  time.Time
	meta       sessionMeta
	compressed bool
}

type sessionMeta struct {
	SessionID      string           `json:"session_id"`
	ID             string           `json:"id"`
	ForkedFromID   string           `json:"forked_from_id"`
	ParentThreadID string           `json:"parent_thread_id"`
	CLIVersion     string           `json:"cli_version"`
	HistoryMode    string           `json:"history_mode"`
	HistoryBase    *historyPosition `json:"history_base"`
}

type historyPosition struct {
	ThreadID string `json:"thread_id"`
}

type rolloutLine struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type discovery struct {
	byFingerprint map[string][]artifact
	omissions     []contract.Omission
}

func New() *Adapter { return &Adapter{} }

func (*Adapter) Name() string { return providerName }

func (*Adapter) EnvironmentRoot() (string, bool) {
	root := os.Getenv("CODEX_HOME")
	return root, root != ""
}

func (*Adapter) DefaultRoot() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, ".codex"), true
}

func (a *Adapter) List(_ context.Context, source config.Source) provider.SourceResult {
	discovered, err := a.discover(source)
	if err != nil {
		return provider.SourceResult{Err: err}
	}
	sources := make([]contract.Source, 0, len(discovered.byFingerprint))
	omissions := append([]contract.Omission{}, discovered.omissions...)
	for _, candidates := range discovered.byFingerprint {
		selected, ambiguous := selectArtifact(candidates)
		if ambiguous {
			omissions = append(omissions, omission("ambiguous_artifact", "source", "Multiple current artifacts could not be distinguished safely."))
		}
		itemSource, relationshipOmissions := makeSource(source, selected)
		sources = append(sources, itemSource)
		omissions = append(omissions, relationshipOmissions...)
		if selected.meta.CLIVersion != supportedVersion {
			omissions = append(omissions, omission("unsupported_format", "source", "The Codex artifact version has not been verified for this adapter."))
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Identity.SourceRef < sources[j].Identity.SourceRef })
	status := statusFor(omissions)
	if len(sources) == 0 && len(omissions) > 0 {
		status = contract.StatusUnsupported
	}
	return provider.SourceResult{Status: status, Sources: sources, Omissions: omissions}
}

func (a *Adapter) Show(_ context.Context, source config.Source, fingerprint string) provider.SourceResult {
	discovered, err := a.discover(source)
	if err != nil {
		return provider.SourceResult{Err: err}
	}
	candidates, ok := discovered.byFingerprint[fingerprint]
	if !ok {
		return provider.SourceResult{Err: provider.ErrNotFound}
	}
	selected, ambiguous := selectArtifact(candidates)
	omissions := []contract.Omission{}
	if ambiguous {
		omissions = append(omissions, omission("ambiguous_artifact", "source", "Multiple current artifacts could not be distinguished safely."))
	}
	if selected.meta.CLIVersion != supportedVersion {
		omissions = append(omissions, omission("unsupported_format", "source", "The Codex artifact version has not been verified for this adapter."))
	}
	itemSource, relationshipOmissions := makeSource(source, selected)
	omissions = append(omissions, relationshipOmissions...)
	return provider.SourceResult{Status: statusFor(omissions), Sources: []contract.Source{itemSource}, Omissions: omissions}
}

func (a *Adapter) Events(_ context.Context, source config.Source, fingerprint string) provider.EventResult {
	selected, omissions, err := a.find(source, fingerprint)
	if err != nil {
		if errors.Is(err, errAmbiguousArtifact) {
			return provider.EventResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("ambiguous_artifact", "events", "Multiple current artifacts could not be distinguished safely.")}}
		}
		return provider.EventResult{Err: err}
	}
	if selected.compressed {
		return provider.EventResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("unsupported_compression", "events", "Compressed Codex rollout artifacts are not supported.")}}
	}
	data, err := safeio.ReadFileWithin(source.Root, selected.relative, maxArtifactBytes)
	if err != nil {
		return provider.EventResult{Err: err}
	}
	events, rowOmissions := normalizeRows(selected.threadID, selected.meta.HistoryMode, data)
	omissions = append(omissions, rowOmissions...)
	if selected.meta.HistoryBase != nil {
		omissions = append(omissions, omission("unsupported_format", "events", "Referenced rollout history is not included in this observation."))
	}
	if len(events) == 0 && len(omissions) > 0 {
		return provider.EventResult{Status: contract.StatusUnsupported, Omissions: omissions}
	}
	return provider.EventResult{Status: statusFor(omissions), Events: events, Omissions: omissions}
}

func (a *Adapter) Evidence(_ context.Context, source config.Source, fingerprint string) provider.EvidenceResult {
	selected, omissions, err := a.find(source, fingerprint)
	if err != nil {
		if errors.Is(err, errAmbiguousArtifact) {
			return provider.EvidenceResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("ambiguous_artifact", "verification", "Multiple current artifacts could not be distinguished safely.")}}
		}
		return provider.EvidenceResult{Err: err}
	}
	if selected.compressed {
		return provider.EvidenceResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("unsupported_compression", "verification", "Compressed Codex rollout artifacts are not supported.")}}
	}
	data, err := safeio.ReadFileWithin(source.Root, selected.relative, maxArtifactBytes)
	if err != nil {
		return provider.EvidenceResult{Err: err}
	}
	_, formatOmissions := inspectRows(selected.meta.HistoryMode, data)
	omissions = append(omissions, formatOmissions...)
	if selected.meta.HistoryBase != nil {
		omissions = append(omissions, omission("unsupported_format", "verification", "Referenced rollout history is not included in this verification."))
	}
	return provider.EvidenceResult{
		Status:    statusFor(omissions),
		Chunks:    []contract.EvidenceChunk{{Name: "rollout/primary.jsonl", Content: data}},
		Omissions: omissions,
	}
}

func (a *Adapter) find(source config.Source, fingerprint string) (artifact, []contract.Omission, error) {
	discovered, err := a.discover(source)
	if err != nil {
		return artifact{}, nil, err
	}
	candidates, ok := discovered.byFingerprint[fingerprint]
	if !ok {
		return artifact{}, nil, provider.ErrNotFound
	}
	selected, ambiguous := selectArtifact(candidates)
	omissions := []contract.Omission{}
	if ambiguous {
		return artifact{}, nil, errAmbiguousArtifact
	}
	if selected.meta.CLIVersion != supportedVersion {
		omissions = append(omissions, omission("unsupported_format", "source", "The Codex artifact version has not been verified for this adapter."))
	}
	return selected, omissions, nil
}

func (a *Adapter) discover(source config.Source) (discovery, error) {
	result := discovery{byFingerprint: map[string][]artifact{}, omissions: []contract.Omission{}}
	discoveredFiles := 0
	for _, collection := range []string{"sessions", "archived_sessions"} {
		remaining := maxDiscoveredFiles - discoveredFiles
		if remaining < 1 {
			remaining = 1
		}
		entries, err := safeio.ListRegularFilesWithin(source.Root, collection, remaining)
		if err != nil {
			return discovery{}, err
		}
		discoveredFiles += len(entries)
		if discoveredFiles > maxDiscoveredFiles {
			return discovery{}, errors.New("file count exceeds limit")
		}
		for _, entry := range entries {
			name := filepath.Base(entry.Relative)
			matches := rolloutNamePattern.FindStringSubmatch(name)
			if matches == nil {
				if strings.HasPrefix(name, "rollout-") {
					result.omissions = append(result.omissions, omission("unknown_format", "discovery", "A rollout-like artifact had an unknown name."))
				}
				continue
			}
			compressed := strings.HasSuffix(name, ".zst")
			if compressed {
				result.omissions = append(result.omissions, omission("unsupported_compression", "discovery", "A compressed Codex rollout artifact could not be inspected."))
				continue
			}
			meta, err := readSessionMeta(source.Root, entry.Relative)
			if err != nil {
				result.omissions = append(result.omissions, omission("malformed_record", "discovery", "A Codex rollout header could not be decoded."))
				continue
			}
			if meta.ID == "" || !strings.EqualFold(meta.ID, matches[2]) {
				result.omissions = append(result.omissions, omission("unsupported_format", "discovery", "A Codex rollout identity did not match its artifact."))
				continue
			}
			timestamp, err := time.Parse("2006-01-02T15-04-05", matches[1])
			if err != nil {
				result.omissions = append(result.omissions, omission("unknown_format", "discovery", "A Codex rollout timestamp was not recognized."))
				continue
			}
			rolloutID := matches[3]
			if rolloutID == "" {
				rolloutID = matches[2]
			}
			item := artifact{relative: entry.Relative, collection: collection, size: entry.Size, modTime: entry.ModTime, threadID: strings.ToLower(meta.ID), rolloutID: strings.ToLower(rolloutID), timestamp: timestamp, meta: meta}
			fingerprint := contract.SourceFingerprint(item.threadID)
			result.byFingerprint[fingerprint] = append(result.byFingerprint[fingerprint], item)
		}
	}
	return result, nil
}

func readSessionMeta(root, relative string) (sessionMeta, error) {
	file, err := safeio.OpenRegularWithin(root, relative)
	if err != nil {
		return sessionMeta{}, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, maxHeaderBytes)
	line, err := reader.ReadSlice('\n')
	if err != nil && len(line) == 0 {
		return sessionMeta{}, err
	}
	if len(line) > maxHeaderBytes {
		return sessionMeta{}, errors.New("header exceeds limit")
	}
	var outer rolloutLine
	if err := json.Unmarshal(bytes.TrimSpace(line), &outer); err != nil || outer.Type != "session_meta" {
		return sessionMeta{}, errors.New("invalid session metadata")
	}
	var meta sessionMeta
	if err := json.Unmarshal(outer.Payload, &meta); err != nil {
		return sessionMeta{}, err
	}
	if meta.HistoryMode == "" {
		meta.HistoryMode = "legacy"
	}
	if meta.HistoryMode != "legacy" && meta.HistoryMode != "paginated" {
		return sessionMeta{}, errors.New("unsupported history mode")
	}
	return meta, nil
}

func selectArtifact(candidates []artifact) (artifact, bool) {
	ordered := append([]artifact{}, candidates...)
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].timestamp.Equal(ordered[j].timestamp) {
			return ordered[i].timestamp.Before(ordered[j].timestamp)
		}
		if ordered[i].rolloutID != ordered[j].rolloutID {
			return ordered[i].rolloutID < ordered[j].rolloutID
		}
		return ordered[i].relative < ordered[j].relative
	})
	selected := ordered[len(ordered)-1]
	ambiguous := false
	if len(ordered) > 1 {
		previous := ordered[len(ordered)-2]
		ambiguous = previous.timestamp.Equal(selected.timestamp) && previous.rolloutID == selected.rolloutID && previous.relative != selected.relative
	}
	return selected, ambiguous
}

func makeSource(source config.Source, item artifact) (contract.Source, []contract.Omission) {
	identity := contract.SourceIdentity{
		Provider:                  providerName,
		SourceInstance:            source.ID,
		ProviderNativeSourceID:    item.threadID,
		ProviderSourceFingerprint: contract.SourceFingerprint(item.threadID),
		SourceRef:                 contract.NewSourceRef(providerName, source.ID, item.threadID),
	}
	relationships := []contract.Relationship{}
	omissions := []contract.Omission{}
	if item.meta.ParentThreadID != "" {
		if parentID, ok := canonicalThreadID(item.meta.ParentThreadID); ok {
			relationships = append(relationships, contract.Relationship{Kind: "parent", SourceRef: contract.NewSourceRef(providerName, source.ID, parentID)})
		} else {
			omissions = append(omissions, omission("malformed_record", "relationship", "A Codex relationship identifier was invalid."))
		}
	}
	if item.meta.ForkedFromID != "" {
		if forkedFromID, ok := canonicalThreadID(item.meta.ForkedFromID); ok {
			relationships = append(relationships, contract.Relationship{Kind: "forked_from", SourceRef: contract.NewSourceRef(providerName, source.ID, forkedFromID)})
		} else {
			omissions = append(omissions, omission("malformed_record", "relationship", "A Codex relationship identifier was invalid."))
		}
	}
	hintInput := strconv.FormatInt(item.size, 10) + "\x00" + strconv.FormatInt(item.modTime.UnixNano(), 10)
	hintHash := sha256.Sum256([]byte("agent-sessions:codex-version-hint:v0\x00" + hintInput))
	return contract.Source{
		Identity:      identity,
		Kind:          "session",
		VersionHint:   &contract.VersionHint{Kind: "stat_hash", Value: "sha256:" + hex.EncodeToString(hintHash[:])},
		Relationships: relationships,
		Metadata:      []contract.Metadata{},
	}, omissions
}

func canonicalThreadID(value string) (string, bool) {
	if !threadIDPattern.MatchString(value) {
		return "", false
	}
	return strings.ToLower(value), true
}

func normalizeRows(threadID, historyMode string, data []byte) ([]contract.Event, []contract.Omission) {
	lines, omissions := inspectRows(historyMode, data)
	events := []contract.Event{}
	calls := map[string]string{}
	completedItems := map[string]struct{}{}
	for _, line := range lines {
		switch line.Type {
		case "event_msg":
			var event struct {
				Type    string          `json:"type"`
				Message string          `json:"message"`
				Item    json.RawMessage `json:"item"`
			}
			if json.Unmarshal(line.Payload, &event) != nil {
				continue
			}
			switch event.Type {
			case "user_message":
				if historyMode == "legacy" {
					events = append(events, messageEvent("user", event.Message))
				}
			case "agent_message":
				if historyMode == "legacy" {
					events = append(events, messageEvent("assistant", event.Message))
				}
			case "error":
				events = append(events, contract.Event{Kind: contract.EventError, Error: &contract.ErrorEvent{Category: "provider", Message: event.Message}, Metadata: []contract.Metadata{}})
			case "item_completed":
				if historyMode == "paginated" {
					var item struct {
						Type     string                        `json:"type"`
						ID       string                        `json:"id"`
						Content  []struct{ Type, Text string } `json:"content"`
						Status   string                        `json:"status"`
						ExitCode *int                          `json:"exit_code"`
					}
					if json.Unmarshal(event.Item, &item) != nil {
						omissions = append(omissions, omission("malformed_record", "events", "A completed Codex item could not be decoded."))
						continue
					}
					switch item.Type {
					case "UserMessage":
						text, hasText, contentOmission := normalizeUserContent(item.Content)
						if hasText {
							events = append(events, messageEvent("user", text))
						}
						if contentOmission != "" {
							message := "A message contained content that is not represented by the public text model."
							if contentOmission == "unknown_format" {
								message = "A message content type was not recognized."
							}
							omissions = append(omissions, omission(contentOmission, "events", message))
						}
					case "AgentMessage":
						text, validContent := normalizeAgentContent(item.Content)
						if validContent {
							events = append(events, messageEvent("assistant", text))
						} else {
							omissions = append(omissions, omission("unknown_format", "events", "An assistant message content type was not recognized."))
						}
					case "CommandExecution", "McpToolCall", "DynamicToolCall":
						if item.ID == "" {
							omissions = append(omissions, omission("correlation_omitted", "events", "A completed tool item lacked a correlation identifier."))
							continue
						}
						if _, duplicate := completedItems[item.ID]; duplicate {
							omissions = append(omissions, omission("duplicate_call_id", "events", "A completed provider tool identifier was duplicated."))
							continue
						}
						completedItems[item.ID] = struct{}{}
						category := map[string]string{"CommandExecution": "shell", "McpToolCall": "mcp", "DynamicToolCall": "tool"}[item.Type]
						callID := normalizedCallID(threadID, item.ID)
						events = append(events, contract.Event{Kind: contract.EventToolCall, ToolCall: &contract.ToolCallEvent{CallID: callID, Category: category}, Metadata: []contract.Metadata{}})
						success := item.Status == "completed" || item.ExitCode != nil && *item.ExitCode == 0
						events = append(events, contract.Event{Kind: contract.EventToolResult, ToolResult: &contract.ToolResultEvent{CallID: callID, Success: success, ExitCode: item.ExitCode}, Metadata: []contract.Metadata{}})
					default:
						if knownTurnItemType(item.Type) {
							omissions = append(omissions, omission("unsupported_event", "events", "A recognized Codex item is not represented by the public event model."))
						} else {
							omissions = append(omissions, omission("unknown_format", "events", "A completed Codex item type was not recognized."))
						}
					}
				}
			}
		case "response_item":
			if historyMode != "legacy" {
				continue
			}
			var item struct {
				Type   string `json:"type"`
				Name   string `json:"name"`
				CallID string `json:"call_id"`
			}
			if json.Unmarshal(line.Payload, &item) != nil {
				continue
			}
			switch item.Type {
			case "function_call", "custom_tool_call", "local_shell_call":
				if item.CallID == "" {
					omissions = append(omissions, omission("correlation_omitted", "events", "A tool call lacked a correlation identifier."))
					continue
				}
				if _, duplicate := calls[item.CallID]; duplicate {
					omissions = append(omissions, omission("duplicate_call_id", "events", "A provider tool call identifier was duplicated."))
					calls[item.CallID] = ""
					continue
				}
				callID := normalizedCallID(threadID, item.CallID)
				calls[item.CallID] = callID
				category := "tool"
				if item.Type == "local_shell_call" || item.Name == "exec_command" || item.Name == "shell" {
					category = "shell"
				} else if item.Name == "apply_patch" {
					category = "file_change"
				}
				events = append(events, contract.Event{Kind: contract.EventToolCall, ToolCall: &contract.ToolCallEvent{CallID: callID, Category: category}, Metadata: []contract.Metadata{}})
			case "function_call_output", "custom_tool_call_output":
				if normalized, ok := calls[item.CallID]; ok && normalized != "" {
					omissions = append(omissions, omission("unsupported_tool_result", "events", "A correlated tool result did not expose a safe success value."))
					delete(calls, item.CallID)
				} else {
					omissions = append(omissions, omission("correlation_omitted", "events", "A tool result did not reference an earlier unique call."))
				}
			}
		}
	}
	for _, normalized := range calls {
		if normalized != "" {
			omissions = append(omissions, omission("correlation_omitted", "events", "A tool call did not have a safely correlated persisted result."))
		}
	}
	if len(events) > maxNormalizedEvents {
		events = events[:maxNormalizedEvents]
		omissions = append(omissions, omission("resource_limit", "events", "The normalized event count exceeded the input limit."))
	}
	return events, omissions
}

func inspectRows(historyMode string, data []byte) ([]rolloutLine, []contract.Omission) {
	lines := []rolloutLine{}
	omissions := []contract.Omission{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), maxRowBytes)
	for scanner.Scan() {
		var line rolloutLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			omissions = append(omissions, omission("malformed_record", "events", "A Codex JSONL row could not be decoded."))
			continue
		}
		if !knownTopLevel(line.Type) {
			omissions = append(omissions, omission("unknown_format", "events", "A Codex JSONL row type was not recognized."))
			continue
		}
		if line.Type == "event_msg" {
			var payload struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(line.Payload, &payload) != nil || payload.Type == "" {
				omissions = append(omissions, omission("malformed_record", "events", "A Codex event row could not be decoded."))
				continue
			}
			if !knownEventType(payload.Type) {
				omissions = append(omissions, omission("unknown_format", "events", "A Codex event type was not recognized."))
				continue
			}
		}
		if line.Type == "response_item" {
			var payload struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(line.Payload, &payload) != nil || payload.Type == "" {
				omissions = append(omissions, omission("malformed_record", "events", "A Codex response row could not be decoded."))
				continue
			}
			if !knownResponseType(payload.Type) {
				omissions = append(omissions, omission("unknown_format", "events", "A Codex response type was not recognized."))
				continue
			}
		}
		lines = append(lines, line)
	}
	if scanner.Err() != nil {
		omissions = append(omissions, omission("resource_limit", "events", "A Codex JSONL row exceeded the input limit."))
	}
	return lines, omissions
}

func knownResponseType(value string) bool {
	switch value {
	case "message", "agent_message", "reasoning", "local_shell_call", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "tool_search_call", "tool_search_output", "web_search_call", "image_generation_call", "configuration_update", "compaction", "compaction_summary", "context_compaction":
		return true
	default:
		return false
	}
}

func knownTurnItemType(value string) bool {
	switch value {
	case "UserMessage", "HookPrompt", "AgentMessage", "Plan", "Reasoning", "CommandExecution", "DynamicToolCall", "CollabAgentToolCall", "SubAgentActivity", "WebSearch", "ImageView", "Extension", "ImageGeneration", "EnteredReviewMode", "ExitedReviewMode", "FileChange", "McpToolCall", "ContextCompaction":
		return true
	default:
		return false
	}
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

func messageEvent(role, text string) contract.Event {
	return contract.Event{Kind: contract.EventMessage, Message: &contract.MessageEvent{Role: role, Text: text}, Metadata: []contract.Metadata{}}
}

func normalizeAgentContent(content []struct{ Type, Text string }) (string, bool) {
	parts := []string{}
	for _, item := range content {
		if item.Type != "Text" {
			return "", false
		}
		if item.Text != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n"), true
}

func normalizeUserContent(content []struct{ Type, Text string }) (string, bool, string) {
	parts := []string{}
	hasText := false
	omissionCode := ""
	for _, item := range content {
		switch item.Type {
		case "text":
			hasText = true
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		case "image", "local_image", "audio", "local_audio", "skill", "mention":
			if omissionCode == "" {
				omissionCode = "unsupported_content"
			}
		default:
			omissionCode = "unknown_format"
		}
	}
	return strings.Join(parts, "\n"), hasText, omissionCode
}

func normalizedCallID(threadID, providerCallID string) string {
	sum := sha256.Sum256([]byte("agent-sessions:codex-call-id:v0\x00" + threadID + "\x00" + providerCallID))
	return "call-" + hex.EncodeToString(sum[:])[:56]
}

func statusFor(omissions []contract.Omission) contract.Status {
	if len(omissions) > 0 {
		return contract.StatusPartial
	}
	return contract.StatusComplete
}

func omission(code, scope, message string) contract.Omission {
	return contract.Omission{Code: code, Scope: scope, Message: message}
}

var _ provider.Adapter = (*Adapter)(nil)
