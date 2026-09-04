package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/provider"
)

const (
	ExitOK          = 0
	ExitUsage       = 2
	ExitUnsupported = 3
	ExitFailure     = 4

	DefaultPageLimit = 50
	MaxPageLimit     = 100
	MaxEventCount    = 100_000
	MaxResponseBytes = 8 << 20
)

type Runner struct {
	Version  string
	Registry *provider.Registry
}

func (r Runner) Run(ctx context.Context, args []string, output io.Writer) int {
	if r.Version == "" {
		r.Version = "dev"
	}
	if r.Registry == nil {
		r.Registry = provider.NewRegistry()
	}
	if len(args) == 0 {
		return r.writeError(output, "cli", ExitUsage, "missing_command", "usage", "A command is required.")
	}
	switch args[0] {
	case "list":
		return r.runList(ctx, args[1:], output)
	case "show":
		return r.runShow(ctx, args[1:], output)
	case "events":
		return r.runEvents(ctx, args[1:], output)
	case "verify":
		return r.runVerify(ctx, args[1:], output)
	default:
		return r.writeError(output, "cli", ExitUsage, "unknown_command", "usage", "The requested command is not supported.")
	}
}

func (r Runner) runList(ctx context.Context, args []string, output io.Writer) int {
	flags := newFlagSet("list")
	configPath := flags.String("config", "", "configuration file")
	providerName := flags.String("provider", "", "provider")
	instance := flags.String("source-instance", "", "source instance")
	root := flags.String("root", "", "provider root")
	limit := flags.Int("limit", DefaultPageLimit, "page size")
	cursor := flags.String("cursor", "", "page cursor")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*root != "" || *instance != "") && *providerName == "" {
		return r.writeError(output, "list", ExitUsage, "invalid_arguments", "usage", "The list arguments are invalid.")
	}
	offset, err := parsePage(*limit, *cursor)
	if err != nil {
		return r.writeError(output, "list", ExitUsage, "invalid_pagination", "usage", "The pagination arguments are invalid.")
	}
	configured, exit := r.loadConfig(*configPath, output, "list")
	if exit != ExitOK {
		return exit
	}
	var adapters []provider.Adapter
	if *providerName != "" {
		adapter, ok := r.Registry.Get(*providerName)
		if !ok {
			return r.writeUnsupported(output, "list", "provider_unavailable", "The requested provider adapter is not available.")
		}
		adapters = []provider.Adapter{adapter}
	} else {
		adapters = r.Registry.All()
	}
	sources := []contract.Source{}
	omissions := []contract.Omission{}
	status := contract.Status("")
	for _, adapter := range adapters {
		instances, err := config.ResolveAll(adapter.Name(), *root, *instance, configured, adapter)
		if err != nil {
			return r.writeError(output, "list", ExitUsage, "source_resolution_failed", "configuration", "A source instance could not be resolved.")
		}
		for _, sourceInstance := range instances {
			result := adapter.List(ctx, sourceInstance)
			if result.Err != nil {
				return r.writeProviderError(output, "list", result.Err)
			}
			if err := validateProviderResultStatus(result.Status, result.Omissions, len(result.Sources)); err != nil {
				return r.writeError(output, "list", ExitFailure, "invalid_provider_result", "provider", "The provider returned an invalid source result.")
			}
			if result.Status == contract.StatusUnsupported {
				omissions = append(omissions, result.Omissions...)
				status = mergeStatus(status, result.Status)
				continue
			}
			if err := validateSources(result.Sources, adapter.Name(), sourceInstance.ID); err != nil {
				return r.writeError(output, "list", ExitFailure, "invalid_provider_result", "provider", "The provider returned an invalid source result.")
			}
			sources = append(sources, result.Sources...)
			omissions = append(omissions, result.Omissions...)
			status = mergeStatus(status, result.Status)
		}
	}
	if status == "" {
		status = contract.StatusComplete
	}
	if status == contract.StatusUnsupported && len(sources) == 0 {
		return r.writeUnsupportedResult(output, "list", omissions)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Identity.SourceRef < sources[j].Identity.SourceRef })
	page, next, pageOmission, err := paginateSources(sources, offset, *limit)
	if err != nil {
		return r.writeError(output, "list", ExitUsage, "invalid_cursor", "usage", "The page cursor is outside the available result.")
	}
	if pageOmission != nil {
		status = contract.StatusPartial
		omissions = append(omissions, *pageOmission)
	}
	envelope := contract.NewEnvelope("list", r.Version, status)
	envelope.Data = &contract.Data{Sources: &page}
	envelope.Page = next
	envelope.Omissions = omissions
	return r.write(output, envelope, exitForStatus(status))
}

func (r Runner) runShow(ctx context.Context, args []string, output io.Writer) int {
	parsed, configured, root, exit := r.parseSourceCommand("show", args, output, false)
	if exit != ExitOK {
		return exit
	}
	adapter, sourceInstance, exit := r.resolveSource("show", parsed, root, configured, output)
	if exit != ExitOK {
		return exit
	}
	result := adapter.Show(ctx, sourceInstance, parsed.Fingerprint)
	if result.Err != nil {
		return r.writeProviderError(output, "show", result.Err)
	}
	if err := validateProviderResultStatus(result.Status, result.Omissions, len(result.Sources)); err != nil {
		return r.writeError(output, "show", ExitFailure, "invalid_provider_result", "provider", "The provider returned an invalid source result.")
	}
	if result.Status == contract.StatusUnsupported {
		return r.writeUnsupportedResult(output, "show", result.Omissions)
	}
	if len(result.Sources) != 1 {
		return r.writeError(output, "show", ExitFailure, "invalid_provider_result", "provider", "The provider returned an invalid source result.")
	}
	if err := validateSources(result.Sources, parsed.Provider, parsed.SourceInstance); err != nil || result.Sources[0].Identity.ProviderSourceFingerprint != parsed.Fingerprint {
		return r.writeError(output, "show", ExitFailure, "invalid_provider_result", "provider", "The provider returned an invalid source result.")
	}
	envelope := contract.NewEnvelope("show", r.Version, result.Status)
	envelope.Data = &contract.Data{Source: &result.Sources[0]}
	envelope.Omissions = append([]contract.Omission{}, result.Omissions...)
	return r.write(output, envelope, exitForStatus(result.Status))
}

func (r Runner) runEvents(ctx context.Context, args []string, output io.Writer) int {
	flags := newFlagSet("events")
	configPath := flags.String("config", "", "configuration file")
	root := flags.String("root", "", "provider root")
	limit := flags.Int("limit", DefaultPageLimit, "page size")
	cursor := flags.String("cursor", "", "page cursor")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		return r.writeError(output, "events", ExitUsage, "invalid_arguments", "usage", "The events arguments are invalid.")
	}
	parsed, err := contract.ParseSourceRef(flags.Arg(0))
	if err != nil {
		return r.writeError(output, "events", ExitUsage, "invalid_source_ref", "usage", "The source reference is invalid.")
	}
	offset, err := parsePage(*limit, *cursor)
	if err != nil {
		return r.writeError(output, "events", ExitUsage, "invalid_pagination", "usage", "The pagination arguments are invalid.")
	}
	configured, exit := r.loadConfig(*configPath, output, "events")
	if exit != ExitOK {
		return exit
	}
	adapter, sourceInstance, exit := r.resolveSource("events", parsed, *root, configured, output)
	if exit != ExitOK {
		return exit
	}
	result := adapter.Events(ctx, sourceInstance, parsed.Fingerprint)
	if result.Err != nil {
		return r.writeProviderError(output, "events", result.Err)
	}
	if err := validateProviderResultStatus(result.Status, result.Omissions, len(result.Events)); err != nil {
		return r.writeError(output, "events", ExitFailure, "invalid_provider_result", "provider", "The provider returned invalid normalized events.")
	}
	if result.Status == contract.StatusUnsupported {
		return r.writeUnsupportedResult(output, "events", result.Omissions)
	}
	if len(result.Events) > MaxEventCount {
		return r.writeError(output, "events", ExitFailure, "event_limit_exceeded", "resource", "The source exceeds the event count limit.")
	}
	if err := normalizeAndValidateEvents(result.Events); err != nil {
		return r.writeError(output, "events", ExitFailure, "invalid_provider_result", "provider", "The provider returned invalid normalized events.")
	}
	events, next, pageOmission, err := paginateEvents(result.Events, offset, *limit)
	if err != nil {
		return r.writeError(output, "events", ExitUsage, "invalid_cursor", "usage", "The page cursor is outside the available result.")
	}
	status := result.Status
	omissions := append([]contract.Omission{}, result.Omissions...)
	if pageOmission != nil {
		status = contract.StatusPartial
		omissions = append(omissions, *pageOmission)
	}
	envelope := contract.NewEnvelope("events", r.Version, status)
	envelope.Data = &contract.Data{SourceRef: flags.Arg(0), Events: &events}
	envelope.Page = next
	envelope.Omissions = omissions
	return r.write(output, envelope, exitForStatus(status))
}

func (r Runner) runVerify(ctx context.Context, args []string, output io.Writer) int {
	parsed, configured, root, exit := r.parseSourceCommand("verify", args, output, false)
	if exit != ExitOK {
		return exit
	}
	adapter, sourceInstance, exit := r.resolveSource("verify", parsed, root, configured, output)
	if exit != ExitOK {
		return exit
	}
	result := adapter.Evidence(ctx, sourceInstance, parsed.Fingerprint)
	if result.Err != nil {
		return r.writeProviderError(output, "verify", result.Err)
	}
	if err := validateProviderResultStatus(result.Status, result.Omissions, len(result.Chunks)); err != nil {
		return r.writeError(output, "verify", ExitFailure, "invalid_provider_result", "provider", "The provider returned invalid verification evidence.")
	}
	if result.Status == contract.StatusUnsupported {
		return r.writeUnsupportedResult(output, "verify", result.Omissions)
	}
	verified, err := contract.VerifiedVersion(result.Chunks)
	if err != nil {
		return r.writeError(output, "verify", ExitFailure, "verification_failed", "resource", "The source evidence could not be verified.")
	}
	envelope := contract.NewEnvelope("verify", r.Version, result.Status)
	envelope.Data = &contract.Data{SourceRef: "as0:" + parsed.Provider + ":" + parsed.SourceInstance + ":" + parsed.Fingerprint, VerifiedVersion: &verified}
	envelope.Omissions = append([]contract.Omission{}, result.Omissions...)
	return r.write(output, envelope, exitForStatus(result.Status))
}

func (r Runner) parseSourceCommand(operation string, args []string, output io.Writer, _ bool) (contract.ParsedSourceRef, config.Config, string, int) {
	flags := newFlagSet(operation)
	configPath := flags.String("config", "", "configuration file")
	root := flags.String("root", "", "provider root")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		return contract.ParsedSourceRef{}, config.Config{}, "", r.writeError(output, operation, ExitUsage, "invalid_arguments", "usage", "The command arguments are invalid.")
	}
	parsed, err := contract.ParseSourceRef(flags.Arg(0))
	if err != nil {
		return contract.ParsedSourceRef{}, config.Config{}, "", r.writeError(output, operation, ExitUsage, "invalid_source_ref", "usage", "The source reference is invalid.")
	}
	configured, exit := r.loadConfig(*configPath, output, operation)
	return parsed, configured, *root, exit
}

func (r Runner) resolveSource(operation string, parsed contract.ParsedSourceRef, root string, configured config.Config, output io.Writer) (provider.Adapter, config.Source, int) {
	adapter, ok := r.Registry.Get(parsed.Provider)
	if !ok {
		return nil, config.Source{}, r.writeUnsupported(output, operation, "provider_unavailable", "The requested provider adapter is not available.")
	}
	instance, err := config.ResolveOne(parsed.Provider, parsed.SourceInstance, root, configured, adapter)
	if err != nil {
		return nil, config.Source{}, r.writeError(output, operation, ExitUsage, "source_resolution_failed", "configuration", "The source instance could not be resolved.")
	}
	return adapter, instance, ExitOK
}

func (r Runner) loadConfig(path string, output io.Writer, operation string) (config.Config, int) {
	explicit := path != ""
	if path == "" {
		var err error
		path, err = config.DefaultPath()
		if err != nil {
			return config.Config{}, r.writeError(output, operation, ExitUsage, "config_path_unavailable", "configuration", "The user configuration path is unavailable.")
		}
	}
	configured, err := config.Load(path, explicit)
	if err != nil {
		return config.Config{}, r.writeError(output, operation, ExitUsage, "invalid_configuration", "configuration", "The configuration file is invalid.")
	}
	return configured, ExitOK
}

func (r Runner) writeProviderError(output io.Writer, operation string, err error) int {
	if errors.Is(err, provider.ErrNotFound) {
		return r.writeError(output, operation, ExitFailure, "source_not_found", "provider", "The requested source was not found.")
	}
	return r.writeError(output, operation, ExitFailure, "provider_failure", "provider", "The provider operation failed.")
}

func (r Runner) writeUnsupported(output io.Writer, operation, code, message string) int {
	return r.writeUnsupportedResult(output, operation, []contract.Omission{{Code: code, Scope: "operation", Message: message}})
}

func (r Runner) writeUnsupportedResult(output io.Writer, operation string, omissions []contract.Omission) int {
	envelope := contract.NewEnvelope(operation, r.Version, contract.StatusUnsupported)
	envelope.Omissions = append([]contract.Omission{}, omissions...)
	return r.write(output, envelope, ExitUnsupported)
}

func (r Runner) writeError(output io.Writer, operation string, exit int, code, category, message string) int {
	envelope := contract.NewEnvelope(operation, r.Version, contract.StatusError)
	envelope.Error = &contract.PublicError{Code: code, Category: category, Message: message, Details: []contract.ErrorDetail{}}
	return r.write(output, envelope, exit)
}

func (r Runner) write(output io.Writer, envelope contract.Envelope, exit int) int {
	if envelope.Status == contract.StatusComplete && len(envelope.Omissions) > 0 {
		envelope.Status = contract.StatusPartial
	}
	if err := prepareEnvelope(&envelope); err != nil {
		code := "invalid_result"
		category := "internal"
		message := "The operation produced an invalid result."
		if errors.Is(err, contract.ErrStructuralStringBound) {
			code = "output_bound_exceeded"
			category = "resource"
			message = "The operation produced a structural value outside the output bounds."
		}
		envelope = r.fallbackEnvelope(envelope.Operation, code, category, message)
		exit = ExitFailure
	}
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > MaxResponseBytes {
		envelope = r.fallbackEnvelope(envelope.Operation, "response_limit_exceeded", "resource", "The response could not be encoded within the output limit.")
		encoded, _ = json.Marshal(envelope)
		exit = ExitFailure
	}
	encoded = append(encoded, '\n')
	if _, err := output.Write(encoded); err != nil {
		return ExitFailure
	}
	return exit
}

func prepareEnvelope(envelope *contract.Envelope) error {
	if err := contract.EnforceBounds(envelope); err != nil {
		return err
	}
	contract.Finalize(envelope)
	return envelope.Validate()
}

func (r Runner) fallbackEnvelope(operation, code, category, message string) contract.Envelope {
	envelope := contract.NewEnvelope(operation, r.Version, contract.StatusError)
	envelope.Error = &contract.PublicError{Code: code, Category: category, Message: message, Details: []contract.ErrorDetail{}}
	if err := prepareEnvelope(&envelope); err == nil {
		return envelope
	}
	envelope = contract.NewEnvelope("cli", "unknown", contract.StatusError)
	envelope.Error = &contract.PublicError{Code: "invalid_result", Category: "internal", Message: "The operation produced an invalid result.", Details: []contract.ErrorDetail{}}
	_ = prepareEnvelope(&envelope)
	return envelope
}

func newFlagSet(name string) *flag.FlagSet {
	result := flag.NewFlagSet(name, flag.ContinueOnError)
	result.SetOutput(io.Discard)
	return result
}

func parsePage(limit int, cursor string) (int, error) {
	if limit < 1 || limit > MaxPageLimit {
		return 0, errors.New("invalid page limit")
	}
	if cursor == "" {
		return 0, nil
	}
	if !strings.HasPrefix(cursor, "v0:") {
		return 0, errors.New("invalid cursor")
	}
	offset, err := strconv.Atoi(strings.TrimPrefix(cursor, "v0:"))
	if err != nil || offset < 0 {
		return 0, errors.New("invalid cursor")
	}
	return offset, nil
}

func paginateSources(values []contract.Source, offset, limit int) ([]contract.Source, *contract.Page, *contract.Omission, error) {
	if offset > len(values) {
		return nil, nil, nil, errors.New("offset outside result")
	}
	end := min(offset+limit, len(values))
	hasMore := end < len(values)
	page := &contract.Page{Limit: limit, HasMore: hasMore}
	var omission *contract.Omission
	if hasMore {
		page.NextCursor = fmt.Sprintf("v0:%d", end)
		omission = &contract.Omission{Code: "pagination", Scope: "sources", Count: len(values) - end, Message: "Additional sources are available on the next page."}
	}
	return values[offset:end], page, omission, nil
}

func paginateEvents(values []contract.Event, offset, limit int) ([]contract.Event, *contract.Page, *contract.Omission, error) {
	if offset > len(values) {
		return nil, nil, nil, errors.New("offset outside result")
	}
	end := min(offset+limit, len(values))
	hasMore := end < len(values)
	page := &contract.Page{Limit: limit, HasMore: hasMore}
	var omission *contract.Omission
	if hasMore {
		page.NextCursor = fmt.Sprintf("v0:%d", end)
		omission = &contract.Omission{Code: "pagination", Scope: "events", Count: len(values) - end, Message: "Additional events are available on the next page."}
	}
	return values[offset:end], page, omission, nil
}

func mergeStatus(current, next contract.Status) contract.Status {
	if current == "" {
		return next
	}
	if next == "" || current == next {
		return current
	}
	if current == contract.StatusPartial || next == contract.StatusPartial {
		return contract.StatusPartial
	}
	return contract.StatusPartial
}

func exitForStatus(status contract.Status) int {
	if status == contract.StatusUnsupported {
		return ExitUnsupported
	}
	if status == contract.StatusError {
		return ExitFailure
	}
	return ExitOK
}

func validateProviderResultStatus(status contract.Status, omissions []contract.Omission, payloadCount int) error {
	switch status {
	case contract.StatusComplete:
		if len(omissions) != 0 {
			return errors.New("complete provider result contains omissions")
		}
	case contract.StatusPartial:
		if len(omissions) == 0 {
			return errors.New("partial provider result lacks omissions")
		}
	case contract.StatusUnsupported:
		if len(omissions) == 0 || payloadCount != 0 {
			return errors.New("unsupported provider result is invalid")
		}
	default:
		return errors.New("unknown provider result status")
	}
	return nil
}

func validateSources(sources []contract.Source, providerName, instance string) error {
	seen := map[string]struct{}{}
	for _, source := range sources {
		identity := source.Identity
		if identity.Provider != providerName || identity.SourceInstance != instance {
			return errors.New("source identity does not match its instance")
		}
		if !contract.ValidToken(source.Kind) {
			return errors.New("invalid source kind")
		}
		if source.VersionHint != nil && !contract.ValidToken(source.VersionHint.Kind) {
			return errors.New("invalid version hint kind")
		}
		for _, metadata := range source.Metadata {
			if !contract.ValidToken(metadata.Name) {
				return errors.New("invalid source metadata name")
			}
		}
		parsed, err := contract.ParseSourceRef(identity.SourceRef)
		if err != nil || parsed.Provider != providerName || parsed.SourceInstance != instance || parsed.Fingerprint != identity.ProviderSourceFingerprint {
			return errors.New("source reference does not match its identity")
		}
		if identity.ProviderNativeSourceID != "" && contract.SourceFingerprint(identity.ProviderNativeSourceID) != identity.ProviderSourceFingerprint {
			return errors.New("native source ID does not match its fingerprint")
		}
		if _, ok := seen[identity.SourceRef]; ok {
			return errors.New("duplicate logical source")
		}
		seen[identity.SourceRef] = struct{}{}
		for _, relationship := range source.Relationships {
			if !contract.ValidToken(relationship.Kind) {
				return errors.New("invalid source relationship kind")
			}
			if _, err := contract.ParseSourceRef(relationship.SourceRef); err != nil {
				return errors.New("invalid source relationship")
			}
		}
	}
	return nil
}

func normalizeAndValidateEvents(events []contract.Event) error {
	calls := map[string]struct{}{}
	results := map[string]struct{}{}
	for i := range events {
		event := &events[i]
		event.Index = uint64(i)
		payloads := 0
		if event.Message != nil {
			payloads++
		}
		if event.ToolCall != nil {
			payloads++
		}
		if event.ToolResult != nil {
			payloads++
		}
		if event.Error != nil {
			payloads++
		}
		if payloads != 1 {
			return errors.New("event must contain one typed payload")
		}
		switch event.Kind {
		case contract.EventMessage:
			if event.Message == nil || event.Message.Role != "user" && event.Message.Role != "assistant" {
				return errors.New("invalid message event")
			}
		case contract.EventToolCall:
			if event.ToolCall == nil || !contract.ValidIdentifier(event.ToolCall.CallID) || !contract.ValidToken(event.ToolCall.Category) {
				return errors.New("invalid tool call event")
			}
			if _, exists := calls[event.ToolCall.CallID]; exists {
				return errors.New("duplicate tool call ID")
			}
			calls[event.ToolCall.CallID] = struct{}{}
		case contract.EventToolResult:
			if event.ToolResult == nil {
				return errors.New("invalid tool result event")
			}
			if _, exists := calls[event.ToolResult.CallID]; !exists {
				return errors.New("tool result does not reference an earlier call")
			}
			if _, exists := results[event.ToolResult.CallID]; exists {
				return errors.New("duplicate tool result")
			}
			results[event.ToolResult.CallID] = struct{}{}
		case contract.EventError:
			if event.Error == nil || !contract.ValidToken(event.Error.Category) {
				return errors.New("invalid error event")
			}
		default:
			return errors.New("unknown event kind")
		}
		for _, metadata := range event.Metadata {
			if !contract.ValidToken(metadata.Name) {
				return errors.New("invalid event metadata name")
			}
		}
	}
	return nil
}
