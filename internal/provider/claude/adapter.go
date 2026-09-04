package claude

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
	providerName        = "claude"
	maxDiscoveredFiles  = 100_000
	maxArtifactBytes    = int64(contract.MaxEvidenceBytes)
	maxHeaderBytes      = 1 << 20
	maxRowBytes         = 1 << 20
	maxSidecarBytes     = 1 << 20
	maxNormalizedEvents = 100_000
)

var (
	errAmbiguousArtifact = errors.New("ambiguous Claude artifact")
	sessionIDPattern     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	mainNamePattern      = regexp.MustCompile(`^([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})\.jsonl$`)
	subagentNamePattern  = regexp.MustCompile(`^agent-([a-zA-Z0-9_-]+)\.jsonl$`)
)

type Adapter struct {
	maxFiles int
}

type artifact struct {
	relative  string
	size      int64
	modTime   time.Time
	sessionID string
	version   string
}

type discovery struct {
	byFingerprint          map[string][]artifact
	omissions              []contract.Omission
	omissionsByFingerprint map[string][]contract.Omission
}

type transcriptRow struct {
	Type              string          `json:"type"`
	SessionID         string          `json:"sessionId"`
	Version           string          `json:"version"`
	Subtype           string          `json:"subtype"`
	IsMeta            bool            `json:"isMeta"`
	IsAPIErrorMessage bool            `json:"isApiErrorMessage"`
	Message           json.RawMessage `json:"message"`
	Content           json.RawMessage `json:"content"`
}

type messagePayload struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	StopReason string          `json:"stop_reason"`
}

type sidecarMeta struct {
	ToolUseID     string `json:"toolUseId"`
	ParentAgentID string `json:"parentAgentId"`
	AgentType     string `json:"agentType"`
}

type contentBlock struct {
	Type       string          `json:"type"`
	Text       string          `json:"text"`
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	ToolUseID  string          `json:"tool_use_id"`
	IsErrorRaw json.RawMessage `json:"is_error"`
}

func New() *Adapter { return &Adapter{maxFiles: maxDiscoveredFiles} }

func (*Adapter) Name() string { return providerName }

func (*Adapter) EnvironmentRoot() (string, bool) {
	root := os.Getenv("CLAUDE_CONFIG_DIR")
	return root, root != ""
}

func (*Adapter) DefaultRoot() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, ".claude"), true
}

func (a *Adapter) List(_ context.Context, source config.Source) provider.SourceResult {
	discovered, err := a.discover(source)
	if err != nil {
		return provider.SourceResult{Err: err}
	}
	omissions := append([]contract.Omission{}, discovered.omissions...)
	sources := make([]contract.Source, 0, len(discovered.byFingerprint))
	fingerprints := make([]string, 0, len(discovered.byFingerprint))
	for fingerprint := range discovered.byFingerprint {
		fingerprints = append(fingerprints, fingerprint)
	}
	sort.Strings(fingerprints)
	for _, fingerprint := range fingerprints {
		candidates := discovered.byFingerprint[fingerprint]
		omissions = append(omissions, discovered.omissionsByFingerprint[fingerprint]...)
		if len(candidates) > 1 {
			omissions = append(omissions, omission("ambiguous_artifact", "source", "Multiple Claude transcripts have the same provider-native session identity."))
		}
		if !isSupportedVersion(candidates[0].version) {
			omissions = append(omissions, omission("unsupported_format", "source", "The Claude Code transcript version has not been verified for this adapter."))
		}
		sources = append(sources, makeSource(source, candidates))
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
	omissions := append([]contract.Omission{}, discovered.omissionsByFingerprint[fingerprint]...)
	if len(candidates) > 1 {
		omissions = append(omissions, omission("ambiguous_artifact", "source", "Multiple Claude transcripts have the same provider-native session identity."))
	}
	if !isSupportedVersion(candidates[0].version) {
		omissions = append(omissions, omission("unsupported_format", "source", "The Claude Code transcript version has not been verified for this adapter."))
	}
	return provider.SourceResult{Status: statusFor(omissions), Sources: []contract.Source{makeSource(source, candidates)}, Omissions: omissions}
}

func (a *Adapter) Events(_ context.Context, source config.Source, fingerprint string) provider.EventResult {
	item, omissions, err := a.find(source, fingerprint)
	if err != nil {
		if errors.Is(err, errAmbiguousArtifact) {
			omissions = append(omissions, omission("ambiguous_artifact", "events", "Multiple Claude transcripts have the same provider-native session identity."))
			return provider.EventResult{Status: contract.StatusUnsupported, Omissions: omissions}
		}
		return provider.EventResult{Err: err}
	}
	data, err := safeio.ReadFileWithin(source.Root, item.relative, maxArtifactBytes)
	if err != nil {
		return provider.EventResult{Err: err}
	}
	events, rowOmissions := normalizeRows(item.sessionID, data)
	omissions = append(omissions, rowOmissions...)
	if len(events) == 0 && len(omissions) > 0 {
		return provider.EventResult{Status: contract.StatusUnsupported, Omissions: omissions}
	}
	return provider.EventResult{Status: statusFor(omissions), Events: events, Omissions: omissions}
}

func (a *Adapter) Evidence(_ context.Context, source config.Source, fingerprint string) provider.EvidenceResult {
	item, omissions, err := a.find(source, fingerprint)
	if err != nil {
		if errors.Is(err, errAmbiguousArtifact) {
			omissions = append(omissions, omission("ambiguous_artifact", "verification", "Multiple Claude transcripts have the same provider-native session identity."))
			return provider.EvidenceResult{Status: contract.StatusUnsupported, Omissions: omissions}
		}
		return provider.EvidenceResult{Err: err}
	}
	data, err := safeio.ReadFileWithin(source.Root, item.relative, maxArtifactBytes)
	if err != nil {
		return provider.EvidenceResult{Err: err}
	}
	_, rowOmissions := normalizeRows(item.sessionID, data)
	omissions = append(omissions, rowOmissions...)
	return provider.EvidenceResult{
		Status:    statusFor(omissions),
		Chunks:    []contract.EvidenceChunk{{Name: "transcript/primary.jsonl", Content: data}},
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
	omissions := append([]contract.Omission{}, discovered.omissionsByFingerprint[fingerprint]...)
	if len(candidates) != 1 {
		return artifact{}, omissions, errAmbiguousArtifact
	}
	if !isSupportedVersion(candidates[0].version) {
		omissions = append(omissions, omission("unsupported_format", "source", "The Claude Code transcript version has not been verified for this adapter."))
	}
	return candidates[0], omissions, nil
}

func (a *Adapter) discover(source config.Source) (discovery, error) {
	limit := a.maxFiles
	if limit < 1 {
		limit = maxDiscoveredFiles
	}
	entries, err := safeio.ListRegularFilesWithin(source.Root, "projects", limit)
	if err != nil {
		return discovery{}, err
	}
	result := discovery{
		byFingerprint:          map[string][]artifact{},
		omissions:              []contract.Omission{},
		omissionsByFingerprint: map[string][]contract.Omission{},
	}
	subagentTranscripts := map[string]safeio.FileEntry{}
	sidecars := map[string]safeio.FileEntry{}
	toolUseIDs := map[string]struct{}{}
	fingerprintByContainer := map[string]string{}
	for _, entry := range entries {
		parts := splitPath(entry.Relative)
		if len(parts) == 3 && parts[0] == "projects" {
			matches := mainNamePattern.FindStringSubmatch(parts[2])
			if matches == nil {
				if strings.HasSuffix(parts[2], ".jsonl") {
					result.omissions = append(result.omissions, omission("unknown_format", "discovery", "A transcript-like artifact had an unknown name."))
				}
				continue
			}
			sessionID, version, identityErr := readIdentity(source.Root, entry.Relative)
			if identityErr != nil {
				result.omissions = append(result.omissions, omission("malformed_record", "discovery", "A Claude transcript identity could not be decoded."))
				continue
			}
			filenameID, _ := canonicalSessionID(matches[1])
			if sessionID != filenameID {
				result.omissions = append(result.omissions, omission("unsupported_format", "discovery", "A Claude transcript identity did not match its artifact."))
				continue
			}
			item := artifact{relative: entry.Relative, size: entry.Size, modTime: entry.ModTime, sessionID: sessionID, version: version}
			fingerprint := contract.SourceFingerprint(sessionID)
			result.byFingerprint[fingerprint] = append(result.byFingerprint[fingerprint], item)
			container := filepath.Join(parts[0], parts[1], strings.TrimSuffix(parts[2], ".jsonl"))
			fingerprintByContainer[container] = fingerprint
			continue
		}
		if len(parts) == 5 && parts[0] == "projects" && parts[3] == "subagents" {
			if matches := subagentNamePattern.FindStringSubmatch(parts[4]); matches != nil {
				key := filepath.Join(parts[0], parts[1], parts[2], parts[3], "agent-"+matches[1])
				subagentTranscripts[key] = entry
				continue
			}
			if strings.HasPrefix(parts[4], "agent-") && strings.HasSuffix(parts[4], ".meta.json") {
				key := strings.TrimSuffix(entry.Relative, ".meta.json")
				sidecars[key] = entry
				continue
			}
		}
	}
	appendArtifactOmission := func(key string, item contract.Omission) {
		container := filepath.Dir(filepath.Dir(key))
		if fingerprint, ok := fingerprintByContainer[container]; ok {
			result.omissionsByFingerprint[fingerprint] = append(result.omissionsByFingerprint[fingerprint], item)
			return
		}
		result.omissions = append(result.omissions, item)
	}
	for _, key := range sortedEntryKeys(subagentTranscripts) {
		appendArtifactOmission(key, omission("unsupported_relationship", "discovery", "A Claude subagent transcript lacked a provider-owned identity that can be exposed safely."))
		sidecar, ok := sidecars[key]
		if !ok {
			appendArtifactOmission(key, omission("missing_sidecar", "discovery", "A Claude subagent transcript did not have its version-specific metadata sidecar."))
			continue
		}
		meta, err := inspectSidecar(source.Root, sidecar.Relative)
		if err != nil {
			code := "malformed_record"
			if errors.Is(err, errResourceLimit) {
				code = "resource_limit"
			}
			appendArtifactOmission(key, omission(code, "discovery", "A Claude subagent metadata sidecar could not be decoded safely."))
			continue
		}
		if meta.ToolUseID != "" {
			relationKey := filepath.Dir(filepath.Dir(key)) + "\x00" + meta.ToolUseID
			if _, duplicate := toolUseIDs[relationKey]; duplicate {
				appendArtifactOmission(key, omission("ambiguous_relationship", "discovery", "Claude subagent sidecars contained a duplicate parent tool correlation."))
			}
			toolUseIDs[relationKey] = struct{}{}
		}
	}
	for _, key := range sortedEntryKeys(sidecars) {
		if _, ok := subagentTranscripts[key]; !ok {
			appendArtifactOmission(key, omission("orphan_sidecar", "discovery", "A Claude subagent metadata sidecar had no matching transcript."))
		}
	}
	return result, nil
}

func sortedEntryKeys(entries map[string]safeio.FileEntry) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isSupportedVersion(version string) bool {
	switch version {
	case "2.1.177", "2.1.228":
		return true
	default:
		return false
	}
}

var errResourceLimit = errors.New("resource limit")

func inspectSidecar(root, relative string) (sidecarMeta, error) {
	data, err := safeio.ReadFileWithin(root, relative, maxSidecarBytes)
	if err != nil {
		if strings.Contains(err.Error(), "size limit") {
			return sidecarMeta{}, errResourceLimit
		}
		return sidecarMeta{}, err
	}
	var value map[string]json.RawMessage
	if safeio.DecodeJSON(data, contract.MaxJSONDepth, &value) != nil || value == nil {
		return sidecarMeta{}, errors.New("invalid sidecar")
	}
	for _, name := range []string{"toolUseId", "parentAgentId", "agentType"} {
		if raw, ok := value[name]; ok {
			var text string
			if json.Unmarshal(raw, &text) != nil {
				return sidecarMeta{}, errors.New("invalid sidecar field")
			}
		}
	}
	var meta sidecarMeta
	if json.Unmarshal(data, &meta) != nil {
		return sidecarMeta{}, errors.New("invalid sidecar")
	}
	return meta, nil
}

func readIdentity(root, relative string) (string, string, error) {
	file, err := safeio.OpenRegularWithin(root, relative)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxRowBytes)
	read := 0
	for scanner.Scan() {
		read += len(scanner.Bytes()) + 1
		if read > maxHeaderBytes {
			return "", "", errors.New("identity prefix exceeds limit")
		}
		var row transcriptRow
		if safeio.DecodeJSON(scanner.Bytes(), contract.MaxJSONDepth, &row) != nil {
			return "", "", errors.New("malformed identity row")
		}
		if !knownObservationRow(row.Type) {
			continue
		}
		sessionID, ok := canonicalSessionID(row.SessionID)
		if !ok || row.Version == "" {
			return "", "", errors.New("invalid identity")
		}
		return sessionID, row.Version, nil
	}
	if scanner.Err() != nil {
		return "", "", scanner.Err()
	}
	return "", "", errors.New("identity not found")
}

func makeSource(source config.Source, candidates []artifact) contract.Source {
	item := candidates[0]
	ordered := append([]artifact{}, candidates...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].size != ordered[j].size {
			return ordered[i].size < ordered[j].size
		}
		return ordered[i].modTime.Before(ordered[j].modTime)
	})
	var hint strings.Builder
	hint.WriteString(strconv.Itoa(len(ordered)))
	for _, candidate := range ordered {
		hint.WriteByte(0)
		hint.WriteString(strconv.FormatInt(candidate.size, 10))
		hint.WriteByte(0)
		hint.WriteString(strconv.FormatInt(candidate.modTime.UnixNano(), 10))
	}
	sum := sha256.Sum256([]byte("agent-sessions:claude-version-hint:v0\x00" + hint.String()))
	return contract.Source{
		Identity: contract.SourceIdentity{
			Provider:                  providerName,
			SourceInstance:            source.ID,
			ProviderNativeSourceID:    item.sessionID,
			ProviderSourceFingerprint: contract.SourceFingerprint(item.sessionID),
			SourceRef:                 contract.NewSourceRef(providerName, source.ID, item.sessionID),
		},
		Kind:          "session",
		VersionHint:   &contract.VersionHint{Kind: "stat_hash", Value: "sha256:" + hex.EncodeToString(sum[:])},
		Relationships: []contract.Relationship{},
		Metadata:      []contract.Metadata{},
	}
}

func normalizeRows(sessionID string, data []byte) ([]contract.Event, []contract.Omission) {
	events := []contract.Event{}
	omissions := []contract.Omission{}
	calls := map[string]string{}
	results := map[string]struct{}{}
	invalidCalls := map[string]struct{}{}
	appendEvent := func(event contract.Event) {
		if len(events) < maxNormalizedEvents {
			events = append(events, event)
		} else if !hasOmissionCode(omissions, "resource_limit") {
			omissions = append(omissions, omission("resource_limit", "events", "The normalized event count exceeded the input limit."))
		}
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), maxRowBytes)
	for scanner.Scan() {
		var row transcriptRow
		if safeio.DecodeJSON(scanner.Bytes(), contract.MaxJSONDepth, &row) != nil || row.Type == "" {
			omissions = append(omissions, omission("malformed_record", "events", "A Claude JSONL row could not be decoded."))
			continue
		}
		if knownBookkeepingRow(row.Type) {
			if row.SessionID != "" {
				canonical, ok := canonicalSessionID(row.SessionID)
				if !ok || canonical != sessionID {
					omissions = append(omissions, omission("malformed_record", "events", "A Claude bookkeeping row had an invalid or mismatched session identity."))
				}
			}
			if row.Version != "" && !isSupportedVersion(row.Version) {
				omissions = append(omissions, omission("unsupported_format", "events", "A Claude bookkeeping row version has not been verified for this adapter."))
			}
			continue
		}
		if !knownObservationRow(row.Type) {
			omissions = append(omissions, omission("unknown_format", "events", "A Claude JSONL row type was not recognized."))
			continue
		}
		canonical, ok := canonicalSessionID(row.SessionID)
		if !ok || canonical != sessionID {
			omissions = append(omissions, omission("malformed_record", "events", "A Claude row had an invalid or mismatched session identity."))
			continue
		}
		if !isSupportedVersion(row.Version) {
			omissions = append(omissions, omission("unsupported_format", "events", "A Claude row version has not been verified for this adapter."))
			continue
		}

		switch row.Type {
		case "user":
			if row.IsMeta {
				omissions = append(omissions, omission("unsupported_event", "events", "A Claude internal user metadata message is not represented by the public event model."))
				continue
			}
			payload, ok := decodeMessage(row.Message, "user")
			if !ok {
				omissions = append(omissions, omission("malformed_record", "events", "A Claude user message could not be decoded."))
				continue
			}
			normalizeUserPayload(payload, sessionID, &events, &omissions, calls, results, invalidCalls, appendEvent)
		case "assistant":
			payload, ok := decodeMessage(row.Message, "assistant")
			if !ok {
				omissions = append(omissions, omission("malformed_record", "events", "A Claude assistant message could not be decoded."))
				continue
			}
			normalizeAssistantPayload(payload, row.IsAPIErrorMessage, sessionID, &omissions, calls, invalidCalls, appendEvent)
		case "system":
			if row.Subtype != "turn_duration" && row.Subtype != "compact_boundary" {
				omissions = append(omissions, omission("unsupported_event", "events", "A Claude system observation is not represented by the public event model."))
			}
		case "attachment":
			omissions = append(omissions, omission("unsupported_event", "events", "A Claude attachment is not represented by the public event model."))
		}
	}
	if scanner.Err() != nil {
		omissions = append(omissions, omission("resource_limit", "events", "A Claude JSONL row exceeded the input limit."))
	}
	for providerID, callID := range calls {
		if _, invalid := invalidCalls[providerID]; invalid {
			continue
		}
		if _, done := results[providerID]; !done && callID != "" {
			omissions = append(omissions, omission("correlation_omitted", "events", "A Claude tool call did not have a uniquely correlated persisted result."))
		}
	}
	return events, omissions
}

func normalizeUserPayload(payload messagePayload, sessionID string, events *[]contract.Event, omissions *[]contract.Omission, calls map[string]string, results map[string]struct{}, invalidCalls map[string]struct{}, appendEvent func(contract.Event)) {
	before := len(*events)
	if text, ok := decodeStringContent(payload.Content); ok {
		if text != "" {
			appendEvent(messageEvent("user", text))
		} else {
			*omissions = append(*omissions, omission("unsupported_event", "events", "A Claude user message had no public text content."))
		}
		return
	}
	blocks, ok := decodeBlocks(payload.Content)
	if !ok {
		*omissions = append(*omissions, omission("malformed_record", "events", "A Claude user message content value could not be decoded."))
		return
	}
	texts := []string{}
	flushText := func() {
		if len(texts) > 0 {
			appendEvent(messageEvent("user", strings.Join(texts, "\n")))
			texts = nil
		}
	}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case "tool_result":
			flushText()
			if block.ToolUseID == "" {
				*omissions = append(*omissions, omission("correlation_omitted", "events", "A Claude tool result lacked a provider correlation identifier."))
				continue
			}
			callID, exists := calls[block.ToolUseID]
			_, invalid := invalidCalls[block.ToolUseID]
			if !exists || invalid {
				*omissions = append(*omissions, omission("correlation_omitted", "events", "A Claude tool result did not reference an earlier unique call."))
				continue
			}
			if _, duplicate := results[block.ToolUseID]; duplicate {
				*omissions = append(*omissions, omission("duplicate_result", "events", "A Claude provider tool result was duplicated."))
				continue
			}
			success, valid := toolResultSuccess(block.IsErrorRaw)
			if !valid {
				*omissions = append(*omissions, omission("malformed_record", "events", "A Claude tool result had an invalid error marker."))
				continue
			}
			results[block.ToolUseID] = struct{}{}
			appendEvent(contract.Event{Kind: contract.EventToolResult, ToolResult: &contract.ToolResultEvent{CallID: callID, Success: success}, Metadata: []contract.Metadata{}})
		case "image", "document", "tool_reference":
			flushText()
			*omissions = append(*omissions, omission("unsupported_content", "events", "A Claude user message contained content that is not represented by the public text model."))
		default:
			flushText()
			*omissions = append(*omissions, omission("unknown_format", "events", "A Claude user content block type was not recognized."))
		}
	}
	flushText()
	if len(*events) == before {
		*omissions = append(*omissions, omission("unsupported_event", "events", "A Claude user message had no public text or correlated result."))
	}
}

func normalizeAssistantPayload(payload messagePayload, apiError bool, sessionID string, omissions *[]contract.Omission, calls map[string]string, invalidCalls map[string]struct{}, appendEvent func(contract.Event)) {
	blocks, ok := decodeBlocks(payload.Content)
	if !ok {
		*omissions = append(*omissions, omission("malformed_record", "events", "A Claude assistant message content value could not be decoded."))
		return
	}
	if apiError && payload.StopReason != "refusal" {
		texts := []string{}
		for _, block := range blocks {
			if block.Type == "text" {
				if block.Text != "" {
					texts = append(texts, block.Text)
				}
			} else {
				*omissions = append(*omissions, omission("unsupported_content", "events", "A Claude API error contained a non-text content block."))
			}
		}
		message := strings.Join(texts, "\n")
		if message == "" {
			message = "Claude Code recorded a provider API error."
		}
		appendEvent(errorEvent(message))
		return
	}
	texts := []string{}
	emittedText := false
	flushText := func() {
		if len(texts) > 0 {
			appendEvent(messageEvent("assistant", strings.Join(texts, "\n")))
			texts = nil
			emittedText = true
		}
	}
	emittedTool := false
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			flushText()
			if block.ID == "" || block.Name == "" {
				*omissions = append(*omissions, omission("correlation_omitted", "events", "A Claude tool call lacked a correlation identifier or name."))
				continue
			}
			if _, duplicate := calls[block.ID]; duplicate {
				invalidCalls[block.ID] = struct{}{}
				*omissions = append(*omissions, omission("duplicate_call_id", "events", "A Claude provider tool call identifier was duplicated."))
				continue
			}
			callID := normalizedCallID(sessionID, block.ID)
			calls[block.ID] = callID
			appendEvent(contract.Event{Kind: contract.EventToolCall, ToolCall: &contract.ToolCallEvent{CallID: callID, Category: toolCategory(block.Name)}, Metadata: []contract.Metadata{}})
			emittedTool = true
		case "thinking", "redacted_thinking":
			flushText()
			*omissions = append(*omissions, omission("unsupported_content", "events", "Claude reasoning content is not represented by the public event model."))
		default:
			flushText()
			*omissions = append(*omissions, omission("unknown_format", "events", "A Claude assistant content block type was not recognized."))
		}
	}
	flushText()
	if !emittedText && !emittedTool {
		*omissions = append(*omissions, omission("unsupported_event", "events", "A Claude assistant message had no public text or tool call."))
	}
}

func decodeMessage(raw json.RawMessage, role string) (messagePayload, bool) {
	var payload messagePayload
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil || payload.Role != role || len(payload.Content) == 0 {
		return messagePayload{}, false
	}
	return payload, true
}

func decodeStringContent(raw json.RawMessage) (string, bool) {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return "", false
	}
	return text, true
}

func decodeBlocks(raw json.RawMessage) ([]contentBlock, bool) {
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil || blocks == nil {
		return nil, false
	}
	return blocks, true
}

func toolResultSuccess(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 {
		return true, true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false, false
	}
	var isError bool
	if json.Unmarshal(raw, &isError) != nil {
		return false, false
	}
	return !isError, true
}

func toolCategory(name string) string {
	switch name {
	case "Bash", "Shell":
		return "shell"
	case "Read", "Glob", "Grep", "LS":
		return "filesystem"
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		return "file_change"
	}
	if strings.HasPrefix(name, "mcp__") {
		return "mcp"
	}
	return "tool"
}

func knownObservationRow(value string) bool {
	switch value {
	case "user", "assistant", "system", "attachment":
		return true
	default:
		return false
	}
}

func knownBookkeepingRow(value string) bool {
	switch value {
	case "file-history-snapshot", "file-history-delta", "last-prompt", "mode", "permission-mode", "ai-title", "custom-title", "agent-name", "agent-color", "agent-setting", "attribution-snapshot", "content-replacement", "fork-context-ref", "worktree-state", "pr-link", "bridge-session", "ended-by-model", "relocated", "history-suppression", "isolation-latch":
		return true
	default:
		return false
	}
}

func canonicalSessionID(value string) (string, bool) {
	if !sessionIDPattern.MatchString(value) {
		return "", false
	}
	return strings.ToLower(value), true
}

func normalizedCallID(sessionID, providerCallID string) string {
	sum := sha256.Sum256([]byte("agent-sessions:claude-call-id:v0\x00" + sessionID + "\x00" + providerCallID))
	return "call-" + hex.EncodeToString(sum[:])[:56]
}

func messageEvent(role, text string) contract.Event {
	return contract.Event{Kind: contract.EventMessage, Message: &contract.MessageEvent{Role: role, Text: text}, Metadata: []contract.Metadata{}}
}

func errorEvent(message string) contract.Event {
	return contract.Event{Kind: contract.EventError, Error: &contract.ErrorEvent{Category: "provider", Message: message}, Metadata: []contract.Metadata{}}
}

func splitPath(path string) []string {
	return strings.Split(filepath.ToSlash(path), "/")
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

func hasOmissionCode(omissions []contract.Omission, code string) bool {
	for _, item := range omissions {
		if item.Code == code {
			return true
		}
	}
	return false
}

var _ provider.Adapter = (*Adapter)(nil)
