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
	SessionID                   string           `json:"session_id"`
	ID                          string           `json:"id"`
	ForkedFromID                string           `json:"forked_from_id"`
	ParentThreadID              string           `json:"parent_thread_id"`
	CLIVersion                  string           `json:"cli_version"`
	HistoryMode                 string           `json:"history_mode"`
	HistoryBase                 *historyPosition `json:"history_base"`
	SubagentHistoryStartOrdinal *uint64          `json:"subagent_history_start_ordinal"`
}

type historyPosition struct {
	ThreadID            string `json:"thread_id"`
	EndOrdinalExclusive uint64 `json:"end_ordinal_exclusive"`
	EndByteOffset       uint64 `json:"end_byte_offset"`
}

type rolloutLine struct {
	Ordinal   *uint64         `json:"ordinal"`
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type completedItem struct {
	Type             string                        `json:"type"`
	ID               string                        `json:"id"`
	Content          []struct{ Type, Text string } `json:"content"`
	Status           string                        `json:"status"`
	ExitCode         *int                          `json:"exit_code"`
	Stdout           *string                       `json:"stdout"`
	Stderr           *string                       `json:"stderr"`
	AggregatedOutput *string                       `json:"aggregated_output"`
	FormattedOutput  *string                       `json:"formatted_output"`
	Result           json.RawMessage               `json:"result"`
	ContentItems     json.RawMessage               `json:"content_items"`
	Error            json.RawMessage               `json:"error"`
}

type discovery struct {
	byFingerprint map[string][]artifact
	byRollout     map[string][]artifact
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
	missingTimes := 0
	for _, candidates := range discovered.byFingerprint {
		selected, ambiguous := selectArtifact(candidates)
		itemSource, sourceOmissions := observeArtifact(source, selected, ambiguous, discovered.byRollout)
		if itemSource.LastInteractionAt == nil {
			missingTimes++
		}
		sources = append(sources, itemSource)
		omissions = append(omissions, sourceOmissions...)
	}
	if missingTimes > 0 {
		omissions = append(omissions, contract.Omission{Code: "source_time_unavailable", Scope: "source", Count: missingTimes, Message: "A Codex source interaction time could not be established safely."})
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
	itemSource, omissions := observeArtifact(source, selected, ambiguous, discovered.byRollout)
	if itemSource.LastInteractionAt == nil {
		omissions = append(omissions, omission("source_time_unavailable", "source", "A Codex source interaction time could not be established safely."))
	}
	return provider.SourceResult{Status: statusFor(omissions), Sources: []contract.Source{itemSource}, Omissions: omissions}
}

func observeArtifact(source config.Source, selected artifact, ambiguous bool, byRollout map[string][]artifact) (contract.Source, []contract.Omission) {
	itemSource, omissions := makeSource(source, selected)
	logical := selected.meta.HistoryBase != nil || selected.meta.SubagentHistoryStartOrdinal != nil
	if logical {
		itemSource.VersionHint = nil
	}
	if ambiguous {
		omissions = append([]contract.Omission{omission("ambiguous_artifact", "source", "Multiple current artifacts could not be distinguished safely.")}, omissions...)
	}
	supported := supportsInteractionTime(selected.meta.CLIVersion, selected.meta.HistoryMode)
	if !supported {
		omissions = append(omissions, omission("unsupported_format", "source", "The Codex artifact format has not been verified for interaction time."))
	}
	if ambiguous || !supported {
		return itemSource, omissions
	}
	value, hint, ok := readLastInteractionAt(source.Root, selected, byRollout)
	if logical {
		itemSource.VersionHint = hint
	}
	if ok {
		itemSource.LastInteractionAt = &value
	}
	return itemSource, omissions
}

func (a *Adapter) Events(_ context.Context, source config.Source, fingerprint string) provider.EventResult {
	selected, discovered, omissions, err := a.findHistory(source, fingerprint)
	if err != nil {
		if errors.Is(err, errAmbiguousArtifact) {
			return provider.EventResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("ambiguous_artifact", "events", "Multiple current artifacts could not be distinguished safely.")}}
		}
		return provider.EventResult{Err: err}
	}
	if selected.compressed {
		return provider.EventResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("unsupported_compression", "events", "Compressed Codex rollout artifacts are not supported.")}}
	}
	selectedProfile := compatibilityProfile(selected.meta.CLIVersion, selected.meta.HistoryMode)
	spans, err := resolveHistory(selected, discovered.byRollout)
	if err != nil {
		return provider.EventResult{Status: contract.StatusUnsupported, Omissions: append(omissions, omission("unsupported_format", "events", "The effective Codex history could not be resolved safely."))}
	}
	normalizer := newEventNormalizer(selected.threadID)
	plan, err := walkHistory(source.Root, spans, maxEffectiveHistoryBytes, maxHistoryRowBytes, selectedProfile == profileCanonicalizedLegacyPaginated, normalizer.consume)
	if err != nil {
		return provider.EventResult{Status: contract.StatusUnsupported, Omissions: append(omissions, omission("unsupported_format", "events", "The effective Codex history could not be read safely."))}
	}
	events, rowOmissions := normalizer.finish()
	omissions = append(omissions, rowOmissions...)
	if plan.malformedRows > 0 {
		omissions = append(omissions, omission("malformed_record", "events", "A Codex JSONL row could not be decoded."))
	}
	if plan.oversizedRows > 0 {
		omissions = append(omissions, omission("resource_limit", "events", "A Codex JSONL row exceeded the input limit."))
	}
	if len(events) == 0 && len(omissions) > 0 {
		return provider.EventResult{Status: contract.StatusUnsupported, Omissions: omissions}
	}
	return provider.EventResult{Status: statusFor(omissions), Events: events, Omissions: omissions}
}

func (a *Adapter) Evidence(_ context.Context, source config.Source, fingerprint string) provider.EvidenceResult {
	selected, discovered, omissions, err := a.findHistory(source, fingerprint)
	if err != nil {
		if errors.Is(err, errAmbiguousArtifact) {
			return provider.EvidenceResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("ambiguous_artifact", "verification", "Multiple current artifacts could not be distinguished safely.")}}
		}
		return provider.EvidenceResult{Err: err}
	}
	if selected.compressed {
		return provider.EvidenceResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("unsupported_compression", "verification", "Compressed Codex rollout artifacts are not supported.")}}
	}
	spans, err := resolveHistory(selected, discovered.byRollout)
	if err != nil {
		return provider.EvidenceResult{Status: contract.StatusUnsupported, Omissions: append(omissions, omission("unsupported_format", "verification", "The effective Codex history could not be resolved safely."))}
	}
	formatOmissions := []contract.Omission{}
	plan, err := walkHistory(source.Root, spans, maxEffectiveHistoryBytes, maxHistoryRowBytes, compatibilityProfile(selected.meta.CLIVersion, selected.meta.HistoryMode) == profileCanonicalizedLegacyPaginated, func(origin artifact, line rolloutLine) {
		profile := compatibilityProfile(origin.meta.CLIVersion, origin.meta.HistoryMode)
		if profile == profileUnsupported || !validRowForProfile(profile, origin.meta.CLIVersion, line) {
			formatOmissions = append(formatOmissions, omission("unknown_format", "verification", "A Codex row was not recognized for its stored format."))
		}
	})
	if err != nil {
		return provider.EvidenceResult{Status: contract.StatusUnsupported, Omissions: append(omissions, omission("unsupported_format", "verification", "The effective Codex history could not be read safely."))}
	}
	omissions = append(omissions, formatOmissions...)
	if plan.malformedRows > 0 {
		omissions = append(omissions, omission("malformed_record", "verification", "A Codex JSONL row could not be decoded."))
	}
	if plan.oversizedRows > 0 {
		omissions = append(omissions, omission("resource_limit", "verification", "A Codex JSONL row exceeded the input limit."))
	}
	if plan.logical {
		verified, err := verifiedHistory(source.Root, plan)
		if err != nil {
			return provider.EvidenceResult{Err: err}
		}
		return provider.EvidenceResult{Status: statusFor(omissions), VerifiedVersion: &verified, Omissions: omissions}
	}
	if selected.size > maxArtifactBytes {
		verified, err := verifiedArtifact(source.Root, selected)
		if err != nil {
			return provider.EvidenceResult{Err: err}
		}
		return provider.EvidenceResult{Status: statusFor(omissions), VerifiedVersion: &verified, Omissions: omissions}
	}
	data, err := safeio.ReadFileWithin(source.Root, selected.relative, maxArtifactBytes)
	if err != nil {
		return provider.EvidenceResult{Err: err}
	}
	return provider.EvidenceResult{
		Status:    statusFor(omissions),
		Chunks:    []contract.EvidenceChunk{{Name: "rollout/primary.jsonl", Content: data}},
		Omissions: omissions,
	}
}

func (a *Adapter) findHistory(source config.Source, fingerprint string) (artifact, discovery, []contract.Omission, error) {
	discovered, err := a.discover(source)
	if err != nil {
		return artifact{}, discovery{}, nil, err
	}
	candidates, ok := discovered.byFingerprint[fingerprint]
	if !ok {
		return artifact{}, discovery{}, nil, provider.ErrNotFound
	}
	selected, ambiguous := selectArtifact(candidates)
	omissions := []contract.Omission{}
	if ambiguous {
		return artifact{}, discovery{}, nil, errAmbiguousArtifact
	}
	if compatibilityProfile(selected.meta.CLIVersion, selected.meta.HistoryMode) == profileUnsupported {
		omissions = append(omissions, omission("unsupported_format", "source", "The Codex artifact version has not been verified for this adapter."))
	}
	return selected, discovered, omissions, nil
}

func (a *Adapter) discover(source config.Source) (discovery, error) {
	result := discovery{byFingerprint: map[string][]artifact{}, byRollout: map[string][]artifact{}, omissions: []contract.Omission{}}
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
			result.byRollout[item.rolloutID] = append(result.byRollout[item.rolloutID], item)
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
	if err := safeio.DecodeJSON(bytes.TrimSpace(line), contract.MaxJSONDepth, &outer); err != nil || outer.Type != "session_meta" {
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
	if meta.SubagentHistoryStartOrdinal != nil && meta.HistoryMode != "paginated" {
		return sessionMeta{}, errors.New("subagent history boundary requires paginated history")
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

func codexToolResult(item completedItem, callID string, success bool) contract.ToolResultEvent {
	result := contract.ToolResultEvent{CallID: callID, Success: success, ExitCode: item.ExitCode}
	var body string
	present, omitted, unsupported := false, false, false
	switch item.Type {
	case "CommandExecution":
		if item.AggregatedOutput != nil && *item.AggregatedOutput != "" {
			body, present = *item.AggregatedOutput, true
		} else {
			if item.Stdout != nil {
				body, present = *item.Stdout, true
			}
			if item.Stderr != nil {
				if body != "" && *item.Stderr != "" {
					body += "\n"
				}
				body += *item.Stderr
				present = true
			}
			if body == "" && item.FormattedOutput != nil {
				body, present = *item.FormattedOutput, true
			}
		}
	case "McpToolCall":
		if hasJSONValue(item.Result) {
			var payload struct {
				Content           json.RawMessage `json:"content"`
				StructuredContent json.RawMessage `json:"structuredContent"`
			}
			if json.Unmarshal(item.Result, &payload) != nil {
				unsupported = true
			} else {
				body, present, omitted, unsupported = codexTextBlocks(payload.Content, "text")
				omitted = omitted || hasJSONValue(payload.StructuredContent)
			}
		}
		if hasJSONValue(item.Error) {
			var payload struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(item.Error, &payload) != nil {
				unsupported = true
			} else {
				if body != "" && payload.Message != "" {
					body += "\n"
				}
				body += payload.Message
				present = true
			}
		}
	case "DynamicToolCall":
		if hasJSONValue(item.ContentItems) {
			body, present, omitted, unsupported = codexTextBlocks(item.ContentItems, "inputText")
		}
		if hasJSONValue(item.Error) {
			var errorText string
			if json.Unmarshal(item.Error, &errorText) != nil {
				unsupported = true
			} else {
				if body != "" && errorText != "" {
					body += "\n"
				}
				body += errorText
				present = true
			}
		}
	}
	result.Excerpt, result.EvidenceState, result.Redacted, result.Truncated = contract.SafeToolExcerpt(body, present)
	result.Redacted = result.Redacted || omitted
	if omitted && result.EvidenceState == contract.EvidenceAbsent {
		result.EvidenceState = contract.EvidenceUnavailable
	}
	if unsupported {
		if result.EvidenceState != contract.EvidenceAvailable {
			result.EvidenceState = contract.EvidenceUnsupported
		} else {
			result.Redacted = true
		}
	}
	return result
}

func hasJSONValue(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func codexTextBlocks(raw json.RawMessage, textType string) (string, bool, bool, bool) {
	if !hasJSONValue(raw) {
		return "", false, false, false
	}
	var blocks []json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false, false, true
	}
	texts := []string{}
	omitted := false
	for _, rawBlock := range blocks {
		var block struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		if json.Unmarshal(rawBlock, &block) != nil || block.Type == "" {
			omitted = true
			continue
		}
		if block.Type != textType {
			omitted = true
			continue
		}
		if block.Text == nil {
			omitted = true
			continue
		}
		texts = append(texts, *block.Text)
	}
	return strings.Join(texts, "\n"), len(texts) > 0, omitted, false
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
