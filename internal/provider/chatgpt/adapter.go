package chatgpt

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/provider"
	"github.com/mtk177a/agent-sessions/internal/safeio"
)

const providerName = "chatgpt"

type resourceLimits struct {
	maxArchiveBytes            uint64
	maxMembers                 int
	maxDeclaredBytes           uint64
	maxConversationMemberBytes uint64
	maxConversationTotalBytes  uint64
	maxCompressionRatio        uint64
	maxConversations           int
	maxEvents                  int
}

var defaultLimits = resourceLimits{
	maxArchiveBytes:            4 << 30,
	maxMembers:                 50_000,
	maxDeclaredBytes:           8 << 30,
	maxConversationMemberBytes: 64 << 20,
	maxConversationTotalBytes:  2 << 30,
	maxCompressionRatio:        50,
	maxConversations:           100_000,
	maxEvents:                  100_000,
}

type Adapter struct {
	limits resourceLimits
}

type archiveView struct {
	file       *os.File
	openedInfo os.FileInfo
	size       uint64
	snapshot   string
	members    []*zip.File
}

type rawConversation struct {
	ID             string                     `json:"id"`
	ConversationID string                     `json:"conversation_id"`
	CurrentNode    string                     `json:"current_node"`
	Mapping        map[string]json.RawMessage `json:"mapping"`
}

type rawNode struct {
	Parent  json.RawMessage `json:"parent"`
	Message json.RawMessage `json:"message"`
}

type rawMessage struct {
	Author struct {
		Role string `json:"role"`
	} `json:"author"`
	Content struct {
		ContentType string            `json:"content_type"`
		Parts       []json.RawMessage `json:"parts"`
	} `json:"content"`
}

type scannedArchive struct {
	view          *archiveView
	conversations []rawConversation
	byFingerprint map[string]int
}

var errUnsupportedSchema = errors.New("unsupported ChatGPT export schema")

func New() *Adapter { return &Adapter{limits: defaultLimits} }

func (*Adapter) Name() string { return providerName }

func (*Adapter) EnvironmentRoot() (string, bool) { return "", false }

func (*Adapter) DefaultRoot() (string, bool) { return "", false }

func (a *Adapter) List(ctx context.Context, source config.Source) provider.SourceResult {
	scanned, err := a.scan(ctx, source)
	if err != nil {
		if errors.Is(err, errUnsupportedSchema) {
			return unsupportedSources("The archive does not contain a supported ChatGPT conversation export shape.")
		}
		return provider.SourceResult{Err: err}
	}
	defer scanned.view.file.Close()
	sources := make([]contract.Source, 0, len(scanned.conversations))
	for _, conversation := range scanned.conversations {
		nativeID, ok := conversationIdentity(conversation)
		if !ok {
			return unsupportedSources("A conversation identity is missing or inconsistent.")
		}
		sources = append(sources, makeSource(source, nativeID, scanned.view.snapshot))
	}
	if err := scanned.view.ensureUnchanged(source.Root); err != nil {
		return provider.SourceResult{Err: err}
	}
	return provider.SourceResult{Status: contract.StatusComplete, Sources: sources, Omissions: []contract.Omission{}}
}

func (a *Adapter) Show(ctx context.Context, source config.Source, fingerprint string) provider.SourceResult {
	scanned, err := a.scan(ctx, source)
	if err != nil {
		if errors.Is(err, errUnsupportedSchema) {
			return unsupportedSources("The archive does not contain a supported ChatGPT conversation export shape.")
		}
		return provider.SourceResult{Err: err}
	}
	defer scanned.view.file.Close()
	conversation, ok := scanned.find(fingerprint)
	if !ok {
		return provider.SourceResult{Err: provider.ErrNotFound}
	}
	nativeID, _ := conversationIdentity(conversation)
	if err := scanned.view.ensureUnchanged(source.Root); err != nil {
		return provider.SourceResult{Err: err}
	}
	return provider.SourceResult{Status: contract.StatusComplete, Sources: []contract.Source{makeSource(source, nativeID, scanned.view.snapshot)}, Omissions: []contract.Omission{}}
}

func (a *Adapter) Events(ctx context.Context, source config.Source, fingerprint string) provider.EventResult {
	scanned, err := a.scan(ctx, source)
	if err != nil {
		if errors.Is(err, errUnsupportedSchema) {
			return unsupportedEvents("The archive does not contain a supported ChatGPT conversation export shape.")
		}
		return provider.EventResult{Err: err}
	}
	defer scanned.view.file.Close()
	conversation, ok := scanned.find(fingerprint)
	if !ok {
		return provider.EventResult{Err: provider.ErrNotFound}
	}
	events, omissions, supported, err := a.normalize(conversation)
	if err != nil {
		return provider.EventResult{Err: err}
	}
	if !supported {
		return provider.EventResult{Status: contract.StatusUnsupported, Omissions: omissions}
	}
	if err := scanned.view.ensureUnchanged(source.Root); err != nil {
		return provider.EventResult{Err: err}
	}
	return provider.EventResult{Status: statusFor(omissions), Events: events, Omissions: omissions}
}

func (a *Adapter) Evidence(ctx context.Context, source config.Source, fingerprint string) provider.EvidenceResult {
	scanned, err := a.scan(ctx, source)
	if err != nil {
		if errors.Is(err, errUnsupportedSchema) {
			return unsupportedEvidence("The archive does not contain a supported ChatGPT conversation export shape.")
		}
		return provider.EvidenceResult{Err: err}
	}
	defer scanned.view.file.Close()
	conversation, ok := scanned.find(fingerprint)
	if !ok {
		return provider.EvidenceResult{Err: provider.ErrNotFound}
	}
	_, omissions, supported, err := a.normalize(conversation)
	if err != nil {
		return provider.EvidenceResult{Err: err}
	}
	if !supported {
		return provider.EvidenceResult{Status: contract.StatusUnsupported, Omissions: omissions}
	}
	if _, err := scanned.view.file.Seek(0, io.SeekStart); err != nil {
		return provider.EvidenceResult{Err: fmt.Errorf("%w: seek archive", provider.ErrInvalidArchive)}
	}
	verified, err := contract.VerifiedVersionReader("archive/export.zip", scanned.view.size, scanned.view.file, a.limits.maxArchiveBytes)
	if err != nil {
		return provider.EvidenceResult{Err: fmt.Errorf("%w: verify archive", provider.ErrInvalidArchive)}
	}
	if err := scanned.view.ensureUnchanged(source.Root); err != nil {
		return provider.EvidenceResult{Err: err}
	}
	return provider.EvidenceResult{Status: statusFor(omissions), VerifiedVersion: &verified, Omissions: omissions}
}

func (a *Adapter) scan(ctx context.Context, source config.Source) (scannedArchive, error) {
	view, err := a.openArchive(source.Root)
	if err != nil {
		return scannedArchive{}, err
	}
	failed := true
	defer func() {
		if failed {
			_ = view.file.Close()
		}
	}()
	result := scannedArchive{view: view, byFingerprint: map[string]int{}}
	var actualConversationBytes uint64
	for _, member := range view.members {
		if err := ctx.Err(); err != nil {
			return scannedArchive{}, err
		}
		data, err := readZIPMember(member, a.limits.maxConversationMemberBytes)
		if err != nil {
			return scannedArchive{}, err
		}
		var overflow bool
		actualConversationBytes, overflow = addBounded(actualConversationBytes, uint64(len(data)), a.limits.maxConversationTotalBytes)
		if overflow {
			return scannedArchive{}, fmt.Errorf("%w: actual conversation bytes", provider.ErrResourceLimit)
		}
		var conversations []json.RawMessage
		if !json.Valid(data) {
			return scannedArchive{}, fmt.Errorf("%w: conversation member", provider.ErrInvalidJSON)
		}
		if err := safeio.DecodeJSON(data, contract.MaxJSONDepth, &conversations); err != nil {
			var typeError *json.UnmarshalTypeError
			if errors.As(err, &typeError) {
				return scannedArchive{}, errUnsupportedSchema
			}
			if strings.Contains(err.Error(), "nesting exceeds") {
				return scannedArchive{}, fmt.Errorf("%w: JSON depth", provider.ErrResourceLimit)
			}
			return scannedArchive{}, fmt.Errorf("%w: conversation member", provider.ErrInvalidJSON)
		}
		for _, raw := range conversations {
			if err := ctx.Err(); err != nil {
				return scannedArchive{}, err
			}
			if len(result.conversations) >= a.limits.maxConversations {
				return scannedArchive{}, fmt.Errorf("%w: conversations", provider.ErrResourceLimit)
			}
			var conversation rawConversation
			if err := json.Unmarshal(raw, &conversation); err != nil {
				return scannedArchive{}, errUnsupportedSchema
			}
			nativeID, ok := conversationIdentity(conversation)
			if !ok {
				return scannedArchive{}, errUnsupportedSchema
			}
			fingerprint := contract.SourceFingerprint(nativeID)
			if _, duplicate := result.byFingerprint[fingerprint]; duplicate {
				return scannedArchive{}, errUnsupportedSchema
			}
			result.byFingerprint[fingerprint] = len(result.conversations)
			result.conversations = append(result.conversations, conversation)
		}
	}
	failed = false
	return result, nil
}

func (a *Adapter) openArchive(filename string) (*archiveView, error) {
	file, _, err := safeio.OpenRegular(filename)
	if err != nil {
		return nil, err
	}
	closeError := func(err error) (*archiveView, error) {
		_ = file.Close()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return closeError(err)
	}
	if info.Size() < 0 || uint64(info.Size()) > a.limits.maxArchiveBytes {
		return closeError(fmt.Errorf("%w: archive bytes", provider.ErrResourceLimit))
	}
	snapshot, hashedSize, err := hashFile(file, a.limits.maxArchiveBytes)
	if err != nil {
		if errors.Is(err, provider.ErrResourceLimit) {
			return closeError(err)
		}
		return closeError(fmt.Errorf("%w: hash archive", provider.ErrInvalidArchive))
	}
	if hashedSize != uint64(info.Size()) {
		return closeError(fmt.Errorf("%w: archive size", provider.ErrSourceChanged))
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return closeError(fmt.Errorf("%w: open ZIP", provider.ErrInvalidArchive))
	}
	if len(reader.File) > a.limits.maxMembers {
		return closeError(fmt.Errorf("%w: archive members", provider.ErrResourceLimit))
	}
	seen := map[string]struct{}{}
	var declared, conversations uint64
	var members []*zip.File
	for _, member := range reader.File {
		if !safeArchiveName(member.Name) || member.Flags&0x1 != 0 || member.Mode()&os.ModeType != 0 && !member.FileInfo().IsDir() {
			return closeError(fmt.Errorf("%w: ZIP member", provider.ErrUnsafeArchiveMember))
		}
		if _, ok := seen[member.Name]; ok {
			return closeError(fmt.Errorf("%w: ZIP member", provider.ErrDuplicateArchiveMember))
		}
		seen[member.Name] = struct{}{}
		var overflow bool
		declared, overflow = addBounded(declared, member.UncompressedSize64, a.limits.maxDeclaredBytes)
		if overflow {
			return closeError(fmt.Errorf("%w: declared archive bytes", provider.ErrResourceLimit))
		}
		if !conversationMember(member.Name) || member.FileInfo().IsDir() {
			continue
		}
		if member.UncompressedSize64 > a.limits.maxConversationMemberBytes || compressionRatioExceeded(member, a.limits.maxCompressionRatio) {
			return closeError(fmt.Errorf("%w: conversation member", provider.ErrResourceLimit))
		}
		conversations, overflow = addBounded(conversations, member.UncompressedSize64, a.limits.maxConversationTotalBytes)
		if overflow {
			return closeError(fmt.Errorf("%w: conversation bytes", provider.ErrResourceLimit))
		}
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	if len(members) == 0 {
		return closeError(errUnsupportedSchema)
	}
	return &archiveView{file: file, openedInfo: info, size: uint64(info.Size()), snapshot: "sha256:" + snapshot, members: members}, nil
}

func (a *Adapter) normalize(conversation rawConversation) ([]contract.Event, []contract.Omission, bool, error) {
	if conversation.CurrentNode == "" || conversation.Mapping == nil {
		return nil, unsupportedFormat(), false, nil
	}
	active := map[string]struct{}{}
	var reversed []rawNode
	nodeID := conversation.CurrentNode
	for nodeID != "" {
		if len(reversed) >= a.limits.maxEvents {
			return nil, nil, false, fmt.Errorf("%w: active branch nodes", provider.ErrResourceLimit)
		}
		if _, cycle := active[nodeID]; cycle {
			return nil, unsupportedFormat(), false, nil
		}
		raw, ok := conversation.Mapping[nodeID]
		if !ok {
			return nil, unsupportedFormat(), false, nil
		}
		active[nodeID] = struct{}{}
		var node rawNode
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, unsupportedFormat(), false, nil
		}
		reversed = append(reversed, node)
		parent, ok := parentID(node.Parent)
		if !ok {
			return nil, unsupportedFormat(), false, nil
		}
		nodeID = parent
	}
	slicesReverse(reversed)
	omissions := []contract.Omission{}
	if count := len(conversation.Mapping) - len(active); count > 0 {
		omissions = append(omissions, contract.Omission{Code: "non_active_branch", Scope: "conversation", Count: count, Message: "Nodes outside the selected active branch were omitted."})
	}
	events := make([]contract.Event, 0, len(reversed))
	for _, node := range reversed {
		if len(node.Message) == 0 || string(node.Message) == "null" {
			continue
		}
		var message rawMessage
		if err := json.Unmarshal(node.Message, &message); err != nil {
			omissions = append(omissions, omission("unknown_node", "event", "An unrecognized active-branch node was omitted."))
			continue
		}
		switch message.Author.Role {
		case "tool":
			omissions = append(omissions, omission("correlation_omitted", "event", "A tool result without a stable provider-neutral correlation was omitted."))
			continue
		case "user", "assistant":
		default:
			omissions = append(omissions, omission("unknown_node", "event", "An unrecognized active-branch node was omitted."))
			continue
		}
		text, complete := messageText(message)
		if !complete {
			omissions = append(omissions, omission("unknown_node", "event", "Unsupported message content was omitted."))
			if text == "" {
				continue
			}
		}
		events = append(events, contract.Event{Kind: contract.EventMessage, Message: &contract.MessageEvent{Role: message.Author.Role, Text: text}, Metadata: []contract.Metadata{}})
	}
	if len(events) == 0 && len(omissions) > 0 {
		return nil, omissions, false, nil
	}
	return events, omissions, true, nil
}

func (s scannedArchive) find(fingerprint string) (rawConversation, bool) {
	index, ok := s.byFingerprint[fingerprint]
	if !ok {
		return rawConversation{}, false
	}
	return s.conversations[index], true
}

func (v *archiveView) ensureUnchanged(filename string) error {
	current, err := os.Stat(filename)
	if err != nil || !os.SameFile(v.openedInfo, current) {
		return fmt.Errorf("%w: archive path", provider.ErrSourceChanged)
	}
	again, size, err := hashFile(v.file, v.size)
	if err != nil || size != v.size || "sha256:"+again != v.snapshot {
		return fmt.Errorf("%w: archive content", provider.ErrSourceChanged)
	}
	return nil
}

func makeSource(source config.Source, nativeID, snapshot string) contract.Source {
	return contract.Source{
		Identity: contract.SourceIdentity{
			Provider:                  providerName,
			SourceInstance:            source.ID,
			ProviderNativeSourceID:    nativeID,
			ProviderSourceFingerprint: contract.SourceFingerprint(nativeID),
			SourceRef:                 contract.NewSourceRef(providerName, source.ID, nativeID),
		},
		Kind:          "conversation",
		VersionHint:   &contract.VersionHint{Kind: "snapshot_hash", Value: snapshot},
		Relationships: []contract.Relationship{},
		Metadata:      []contract.Metadata{},
	}
}

func conversationIdentity(conversation rawConversation) (string, bool) {
	if conversation.ID != "" && conversation.ConversationID != "" && conversation.ID != conversation.ConversationID {
		return "", false
	}
	if conversation.ConversationID != "" {
		return conversation.ConversationID, true
	}
	return conversation.ID, conversation.ID != ""
}

func messageText(message rawMessage) (string, bool) {
	if message.Content.ContentType != "text" && message.Content.ContentType != "multimodal_text" {
		return "", false
	}
	parts := make([]string, 0, len(message.Content.Parts))
	complete := true
	for _, raw := range message.Content.Parts {
		var part string
		if err := json.Unmarshal(raw, &part); err != nil {
			complete = false
			continue
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "\n"), complete
}

func parentID(raw json.RawMessage) (string, bool) {
	if string(raw) == "null" {
		return "", true
	}
	if len(raw) == 0 {
		return "", false
	}
	var parent string
	if err := json.Unmarshal(raw, &parent); err != nil || parent == "" {
		return "", false
	}
	return parent, true
}

func readZIPMember(member *zip.File, maxBytes uint64) ([]byte, error) {
	reader, err := member.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: open ZIP member", provider.ErrInvalidArchive)
	}
	defer reader.Close()
	limited := io.LimitReader(reader, int64(maxBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: read ZIP member", provider.ErrInvalidArchive)
	}
	if uint64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: conversation member", provider.ErrResourceLimit)
	}
	return data, nil
}

func hashFile(file *os.File, maxBytes uint64) (string, uint64, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	h := sha256.New()
	read, err := io.Copy(h, io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return "", 0, err
	}
	if uint64(read) > maxBytes {
		return "", uint64(read), fmt.Errorf("%w: archive bytes", provider.ErrResourceLimit)
	}
	return hex.EncodeToString(h.Sum(nil)), uint64(read), nil
}

func safeArchiveName(name string) bool {
	if name == "" || strings.IndexByte(name, 0) >= 0 || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || len(name) >= 2 && name[1] == ':' {
		return false
	}
	trimmed := strings.TrimSuffix(name, "/")
	if trimmed == "" || path.Clean(trimmed) != trimmed {
		return false
	}
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func conversationMember(name string) bool {
	base := path.Base(name)
	if base == "conversations.json" {
		return true
	}
	if !strings.HasPrefix(base, "conversations-") || !strings.HasSuffix(base, ".json") {
		return false
	}
	number := strings.TrimSuffix(strings.TrimPrefix(base, "conversations-"), ".json")
	if number == "" {
		return false
	}
	for _, character := range number {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func compressionRatioExceeded(member *zip.File, max uint64) bool {
	if member.UncompressedSize64 == 0 {
		return false
	}
	if member.CompressedSize64 == 0 {
		return true
	}
	if max == 0 || member.CompressedSize64 > ^uint64(0)/max {
		return false
	}
	return member.UncompressedSize64 > member.CompressedSize64*max
}

func addBounded(current, value, max uint64) (uint64, bool) {
	if value > max || current > max-value {
		return current, true
	}
	return current + value, false
}

func slicesReverse[T any](values []T) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
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

func unsupportedFormat() []contract.Omission {
	return []contract.Omission{omission("unsupported_format", "conversation", "The conversation graph cannot be normalized safely.")}
}

func unsupportedSources(message string) provider.SourceResult {
	return provider.SourceResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("unsupported_format", "archive", message)}}
}

func unsupportedEvents(message string) provider.EventResult {
	return provider.EventResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("unsupported_format", "archive", message)}}
}

func unsupportedEvidence(message string) provider.EvidenceResult {
	return provider.EvidenceResult{Status: contract.StatusUnsupported, Omissions: []contract.Omission{omission("unsupported_format", "archive", message)}}
}
