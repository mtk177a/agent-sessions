package contract

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	SchemaVersion          = "v0alpha1"
	RedactionPolicyVersion = "v0alpha1"
)

type Status string

const (
	StatusComplete    Status = "complete"
	StatusPartial     Status = "partial"
	StatusUnsupported Status = "unsupported"
	StatusError       Status = "error"
)

type Envelope struct {
	SchemaVersion          string       `json:"schema_version"`
	CLIVersion             string       `json:"cli_version"`
	RedactionPolicyVersion string       `json:"redaction_policy_version"`
	Operation              string       `json:"operation"`
	Status                 Status       `json:"status"`
	Data                   *Data        `json:"data,omitempty"`
	Page                   *Page        `json:"page,omitempty"`
	Omissions              []Omission   `json:"omissions"`
	Error                  *PublicError `json:"error,omitempty"`
}

type Data struct {
	Sources         *[]Source          `json:"sources,omitempty"`
	Source          *Source            `json:"source,omitempty"`
	SourceRef       string             `json:"source_ref,omitempty"`
	Events          *[]Event           `json:"events,omitempty"`
	VerifiedVersion *VerifiedVersionID `json:"verified_version,omitempty"`
}

type Source struct {
	Identity      SourceIdentity `json:"identity"`
	Kind          string         `json:"kind"`
	VersionHint   *VersionHint   `json:"version_hint,omitempty"`
	Relationships []Relationship `json:"relationships"`
	Metadata      []Metadata     `json:"metadata"`
}

type SourceIdentity struct {
	Provider                  string `json:"provider"`
	SourceInstance            string `json:"source_instance"`
	ProviderNativeSourceID    string `json:"provider_native_source_id,omitempty"`
	ProviderSourceFingerprint string `json:"provider_source_fingerprint"`
	SourceRef                 string `json:"source_ref"`
}

type VersionHint struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type VerifiedVersionID struct {
	Algorithm string `json:"algorithm"`
	Basis     string `json:"basis"`
	Value     string `json:"value"`
}

type Relationship struct {
	Kind      string `json:"kind"`
	SourceRef string `json:"source_ref"`
}

type Metadata struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type EventKind string

const (
	EventMessage    EventKind = "message"
	EventToolCall   EventKind = "tool_call"
	EventToolResult EventKind = "tool_result"
	EventError      EventKind = "error"
)

type Event struct {
	Index      uint64           `json:"index"`
	Kind       EventKind        `json:"kind"`
	Message    *MessageEvent    `json:"message,omitempty"`
	ToolCall   *ToolCallEvent   `json:"tool_call,omitempty"`
	ToolResult *ToolResultEvent `json:"tool_result,omitempty"`
	Error      *ErrorEvent      `json:"error,omitempty"`
	Metadata   []Metadata       `json:"metadata"`
}

type MessageEvent struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type ToolCallEvent struct {
	CallID   string `json:"call_id"`
	Category string `json:"category"`
}

type ToolResultEvent struct {
	CallID   string `json:"call_id"`
	Success  bool   `json:"success"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

type ErrorEvent struct {
	Category string `json:"category"`
	Message  string `json:"message"`
}

type Page struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type Omission struct {
	Code    string `json:"code"`
	Scope   string `json:"scope"`
	Count   int    `json:"count,omitempty"`
	Message string `json:"message"`
}

type ErrorDetail struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type PublicError struct {
	Code      string        `json:"code"`
	Category  string        `json:"category"`
	Message   string        `json:"message"`
	Retryable bool          `json:"retryable"`
	Details   []ErrorDetail `json:"details"`
}

func NewEnvelope(operation, cliVersion string, status Status) Envelope {
	return Envelope{
		SchemaVersion:          SchemaVersion,
		CLIVersion:             cliVersion,
		RedactionPolicyVersion: RedactionPolicyVersion,
		Operation:              operation,
		Status:                 status,
		Omissions:              []Omission{},
	}
}

func (e Envelope) Validate() error {
	if e.SchemaVersion != SchemaVersion || e.RedactionPolicyVersion != RedactionPolicyVersion {
		return errors.New("invalid contract version")
	}
	for _, omission := range e.Omissions {
		if !ValidToken(omission.Code) || !ValidToken(omission.Scope) {
			return errors.New("invalid omission identifier")
		}
	}
	if e.Error != nil {
		if !ValidToken(e.Error.Code) || !ValidToken(e.Error.Category) {
			return errors.New("invalid structured error identifier")
		}
		for _, detail := range e.Error.Details {
			if !ValidToken(detail.Name) {
				return errors.New("invalid structured error detail name")
			}
		}
	}
	switch e.Status {
	case StatusComplete:
		if len(e.Omissions) != 0 || e.Error != nil {
			return errors.New("complete results cannot contain omissions or an error")
		}
	case StatusPartial:
		if len(e.Omissions) == 0 || e.Error != nil {
			return errors.New("partial results require omissions and cannot contain an error")
		}
	case StatusUnsupported:
		if len(e.Omissions) == 0 || e.Error != nil {
			return errors.New("unsupported results require an omission and cannot contain an error")
		}
	case StatusError:
		if e.Error == nil {
			return errors.New("error results require a structured error")
		}
	default:
		return fmt.Errorf("unknown status %q", e.Status)
	}
	return nil
}

func Marshal(value any) ([]byte, error) {
	return json.Marshal(value)
}
