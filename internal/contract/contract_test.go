package contract

import (
	"bytes"
	"strings"
	"testing"
)

func TestSourceRefIsDeterministicAndDoesNotExposeNativeID(t *testing.T) {
	ref1 := NewSourceRef("codex", "codex-default", "native-secret-id")
	ref2 := NewSourceRef("codex", "codex-default", "native-secret-id")
	if ref1 != ref2 {
		t.Fatalf("source refs differ: %q != %q", ref1, ref2)
	}
	if strings.Contains(ref1, "native-secret-id") {
		t.Fatalf("source ref exposes native ID: %q", ref1)
	}
	parsed, err := ParseSourceRef(ref1)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Provider != "codex" || parsed.SourceInstance != "codex-default" {
		t.Fatalf("unexpected parsed ref: %#v", parsed)
	}
}

func TestVerifiedVersionIsDeterministicAcrossChunkOrder(t *testing.T) {
	a, err := VerifiedVersion([]EvidenceChunk{{Name: "b", Content: []byte("two")}, {Name: "a", Content: []byte("one")}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := VerifiedVersion([]EvidenceChunk{{Name: "a", Content: []byte("one")}, {Name: "b", Content: []byte("two")}})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("verified versions differ: %q != %q", a, b)
	}
	if _, err := VerifiedVersion([]EvidenceChunk{{Name: "same", Content: nil}, {Name: "same", Content: nil}}); err == nil {
		t.Fatal("duplicate evidence name was accepted")
	}
	if _, err := VerifiedVersion(nil); err == nil {
		t.Fatal("empty evidence was accepted")
	}
}

func TestVerifiedVersionReaderMatchesChunkEncoding(t *testing.T) {
	content := []byte("synthetic archive bytes")
	want, err := VerifiedVersion([]EvidenceChunk{{Name: "archive/export.zip", Content: content}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifiedVersionReader("archive/export.zip", uint64(len(content)), bytes.NewReader(content), uint64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("streamed version = %#v, want %#v", got, want)
	}
	if !ValidVerifiedVersion(got) {
		t.Fatalf("streamed version is invalid: %#v", got)
	}
	if _, err := VerifiedVersionReader("archive/export.zip", uint64(len(content)+1), bytes.NewReader(content), uint64(len(content)+1)); err == nil {
		t.Fatal("short evidence stream was accepted")
	}
}

func TestFinalizeRedactsEveryDynamicStringPosition(t *testing.T) {
	unsafe := "token=sk-fictional-secret /fictional host.example.invalid 192.0.2.10"
	nativeID := "00000000-0000-4000-8000-000000000002"
	ref := NewSourceRef("codex", "codex-default", nativeID)
	sources := []Source{{
		Identity: SourceIdentity{Provider: "codex", SourceInstance: "codex-default", ProviderNativeSourceID: nativeID, ProviderSourceFingerprint: SourceFingerprint(nativeID), SourceRef: ref},
		Kind:     "session", VersionHint: &VersionHint{Kind: "provider_metadata", Value: unsafe},
		Relationships: []Relationship{}, Metadata: []Metadata{{Name: "note", Value: unsafe}},
	}}
	events := []Event{
		{Index: 0, Kind: EventMessage, Message: &MessageEvent{Role: "user", Text: unsafe}, Metadata: []Metadata{{Name: "note", Value: unsafe}}},
		{Index: 1, Kind: EventError, Error: &ErrorEvent{Category: "provider", Message: unsafe}, Metadata: []Metadata{}},
	}
	envelope := Envelope{
		SchemaVersion:          SchemaVersion,
		CLIVersion:             unsafe,
		RedactionPolicyVersion: RedactionPolicyVersion,
		Operation:              "events",
		Status:                 StatusComplete,
		Data:                   &Data{Sources: &sources, Events: &events},
		Omissions:              []Omission{{Code: "unknown_shape", Scope: "source", Message: unsafe}},
	}
	Finalize(&envelope)
	encoded := string(mustJSON(t, envelope))
	for _, forbidden := range []string{"sk-fictional-secret", "/fictional", "host.example.invalid", "192.0.2.10"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("output contains %q: %s", forbidden, encoded)
		}
	}
	if envelope.Status != StatusPartial || len(envelope.Omissions) == 0 {
		t.Fatalf("redaction did not reduce completeness: %#v", envelope)
	}

	errorEnvelope := NewEnvelope("list", "test", StatusError)
	errorEnvelope.Error = &PublicError{Code: "provider_failure", Category: "provider", Message: unsafe, Details: []ErrorDetail{{Name: "diagnostic", Value: unsafe}, {Name: "raw_path", Value: "single-label-host"}}}
	Finalize(&errorEnvelope)
	if len(errorEnvelope.Omissions) == 0 || errorEnvelope.Omissions[len(errorEnvelope.Omissions)-1].Code != "output_redacted" {
		t.Fatalf("error redaction omission = %#v", errorEnvelope.Omissions)
	}
	if err := errorEnvelope.Validate(); err != nil {
		t.Fatalf("redacted error envelope is invalid: %v", err)
	}
	errorJSON := string(mustJSON(t, errorEnvelope))
	for _, forbidden := range []string{"sk-fictional-secret", "/fictional", "host.example.invalid", "192.0.2.10", "single-label-host"} {
		if strings.Contains(errorJSON, forbidden) {
			t.Fatalf("structured error contains %q: %s", forbidden, errorJSON)
		}
	}
}

func TestFinalizeRedactsAdversarialCredentialForms(t *testing.T) {
	for _, value := range []string{
		"ghp_fictionalcredentialvalue",
		"github_pat_fictional_credential_value",
		"Authorization: Basic ZmljdGlvbmFsOnNlY3JldA==",
		`token: "opaque-fictional-value"`,
	} {
		t.Run(value, func(t *testing.T) {
			envelope := NewEnvelope("list", value, StatusComplete)
			ref := NewSourceRef("synthetic", "synthetic-default", value)
			source := Source{
				Identity: SourceIdentity{Provider: "synthetic", SourceInstance: "synthetic-default", ProviderNativeSourceID: value, ProviderSourceFingerprint: SourceFingerprint(value), SourceRef: ref},
				Kind:     "session", VersionHint: &VersionHint{Kind: "provider_metadata", Value: value}, Relationships: []Relationship{}, Metadata: []Metadata{{Name: "note", Value: value}},
			}
			events := []Event{
				{Index: 0, Kind: EventMessage, Message: &MessageEvent{Role: "user", Text: value}, Metadata: []Metadata{{Name: "note", Value: value}}},
				{Index: 1, Kind: EventError, Error: &ErrorEvent{Category: "provider", Message: value}, Metadata: []Metadata{}},
			}
			envelope.Data = &Data{Source: &source, Events: &events}
			envelope.Omissions = []Omission{{Code: "unknown_shape", Scope: "source", Message: value}}
			envelope.Error = &PublicError{Code: "provider_failure", Category: "provider", Message: value, Details: []ErrorDetail{{Name: "diagnostic", Value: value}}}
			Finalize(&envelope)
			encoded := string(mustJSON(t, envelope))
			if strings.Contains(encoded, value) || strings.Contains(encoded, "ZmljdGlvbmFsOnNlY3JldA==") || strings.Contains(encoded, "opaque-fictional-value") {
				t.Fatalf("credential payload remained in output: %s", encoded)
			}
			if envelope.Status != StatusPartial || len(envelope.Omissions) == 0 {
				t.Fatalf("credential redaction did not reduce completeness: %#v", envelope)
			}
		})
	}
}

func TestBoundsRejectEveryOversizedStructuralString(t *testing.T) {
	oversized := strings.Repeat("x", MaxStringBytes+1)
	for _, tc := range []struct {
		name string
		set  func(*Envelope)
	}{
		{"schema_version", func(e *Envelope) { e.SchemaVersion = oversized }},
		{"redaction_policy_version", func(e *Envelope) { e.RedactionPolicyVersion = oversized }},
		{"operation", func(e *Envelope) { e.Operation = oversized }},
		{"status", func(e *Envelope) { e.Status = Status(oversized) }},
		{"data_source_ref", func(e *Envelope) { e.Data.SourceRef = oversized }},
		{"provider", func(e *Envelope) { e.Data.Source.Identity.Provider = oversized }},
		{"source_instance", func(e *Envelope) { e.Data.Source.Identity.SourceInstance = oversized }},
		{"source_fingerprint", func(e *Envelope) { e.Data.Source.Identity.ProviderSourceFingerprint = oversized }},
		{"source_ref", func(e *Envelope) { e.Data.Source.Identity.SourceRef = oversized }},
		{"source_kind", func(e *Envelope) { e.Data.Source.Kind = oversized }},
		{"version_hint_kind", func(e *Envelope) { e.Data.Source.VersionHint.Kind = oversized }},
		{"relationship_kind", func(e *Envelope) { e.Data.Source.Relationships[0].Kind = oversized }},
		{"relationship_ref", func(e *Envelope) { e.Data.Source.Relationships[0].SourceRef = oversized }},
		{"source_metadata_name", func(e *Envelope) { e.Data.Source.Metadata[0].Name = oversized }},
		{"event_kind", func(e *Envelope) { (*e.Data.Events)[0].Kind = EventKind(oversized) }},
		{"message_role", func(e *Envelope) { (*e.Data.Events)[0].Message.Role = oversized }},
		{"tool_call_id", func(e *Envelope) { (*e.Data.Events)[1].ToolCall.CallID = oversized }},
		{"tool_category", func(e *Envelope) { (*e.Data.Events)[1].ToolCall.Category = oversized }},
		{"tool_result_id", func(e *Envelope) { (*e.Data.Events)[2].ToolResult.CallID = oversized }},
		{"error_category", func(e *Envelope) { (*e.Data.Events)[3].Error.Category = oversized }},
		{"event_metadata_name", func(e *Envelope) { (*e.Data.Events)[0].Metadata[0].Name = oversized }},
		{"verified_algorithm", func(e *Envelope) { e.Data.VerifiedVersion.Algorithm = oversized }},
		{"verified_basis", func(e *Envelope) { e.Data.VerifiedVersion.Basis = oversized }},
		{"verified_value", func(e *Envelope) { e.Data.VerifiedVersion.Value = oversized }},
		{"page_cursor", func(e *Envelope) { e.Page.NextCursor = oversized }},
		{"omission_code", func(e *Envelope) { e.Omissions[0].Code = oversized }},
		{"omission_scope", func(e *Envelope) { e.Omissions[0].Scope = oversized }},
		{"public_error_code", func(e *Envelope) { e.Error.Code = oversized }},
		{"public_error_category", func(e *Envelope) { e.Error.Category = oversized }},
		{"error_detail_name", func(e *Envelope) { e.Error.Details[0].Name = oversized }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			envelope := fullyPopulatedEnvelope()
			tc.set(&envelope)
			if err := EnforceBounds(&envelope); err == nil {
				t.Fatal("oversized structural string was accepted")
			}
		})
	}
}

func TestBoundsAndCompletenessInvariants(t *testing.T) {
	envelope := NewEnvelope("events", "test", StatusComplete)
	envelope.Data = &Data{Events: eventPointer([]Event{{Index: 0, Kind: EventMessage, Message: &MessageEvent{Role: "user", Text: strings.Repeat("x", MaxStringBytes+1)}, Metadata: []Metadata{}}})}
	EnforceBounds(&envelope)
	if envelope.Status != StatusPartial || len(envelope.Omissions) == 0 {
		t.Fatalf("bounded output remained complete: %#v", envelope)
	}
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}

	invalid := NewEnvelope("list", "test", StatusComplete)
	invalid.Omissions = []Omission{{Code: "unknown_shape", Scope: "source", Message: "An unknown shape was omitted."}}
	if err := invalid.Validate(); err == nil {
		t.Fatal("complete result with an omission was accepted")
	}
}

func TestEnvelopeRejectsUnsupportedSchemaVersions(t *testing.T) {
	for _, version := range []string{"v0alpha1", "v2"} {
		t.Run(version, func(t *testing.T) {
			envelope := NewEnvelope("list", "test", StatusComplete)
			envelope.SchemaVersion = version
			if err := envelope.Validate(); err == nil {
				t.Fatalf("schema version %q was accepted", version)
			}
		})
	}
}

func TestBoundsCoverEveryPublicFreeFormString(t *testing.T) {
	oversized := strings.Repeat("x", MaxStringBytes+1)
	ref := NewSourceRef("synthetic", "synthetic-default", oversized)
	source := Source{
		Identity: SourceIdentity{
			Provider:                  "synthetic",
			SourceInstance:            "synthetic-default",
			ProviderNativeSourceID:    oversized,
			ProviderSourceFingerprint: SourceFingerprint(oversized),
			SourceRef:                 ref,
		},
		Kind:          "session",
		VersionHint:   &VersionHint{Kind: "provider_metadata", Value: oversized},
		Relationships: []Relationship{},
		Metadata:      []Metadata{{Name: "note", Value: oversized}},
	}
	events := []Event{
		{Index: 0, Kind: EventMessage, Message: &MessageEvent{Role: "user", Text: oversized}, Metadata: []Metadata{}},
		{Index: 1, Kind: EventError, Error: &ErrorEvent{Category: "provider", Message: oversized}, Metadata: []Metadata{}},
	}
	envelope := NewEnvelope("events", oversized, StatusComplete)
	envelope.Data = &Data{Source: &source, Events: &events}
	envelope.Omissions = []Omission{{Code: "unknown_shape", Scope: "source", Message: oversized}}
	EnforceBounds(&envelope)

	values := map[string]string{
		"cli_version":      envelope.CLIVersion,
		"version_hint":     source.VersionHint.Value,
		"metadata":         source.Metadata[0].Value,
		"message":          events[0].Message.Text,
		"event_error":      events[1].Error.Message,
		"omission_message": envelope.Omissions[0].Message,
		"native_source_id": source.Identity.ProviderNativeSourceID,
	}
	for name, value := range values {
		if len(value) > MaxStringBytes {
			t.Fatalf("%s remained oversized: %d bytes", name, len(value))
		}
	}
	if source.Identity.ProviderNativeSourceID != "" {
		t.Fatalf("oversized native source ID was truncated instead of omitted: %d bytes", len(source.Identity.ProviderNativeSourceID))
	}
	if envelope.Status != StatusPartial {
		t.Fatalf("bounded envelope status = %s, want partial", envelope.Status)
	}

	errorEnvelope := NewEnvelope("list", oversized, StatusError)
	errorEnvelope.Error = &PublicError{Code: "provider_failure", Category: "provider", Message: oversized, Details: []ErrorDetail{{Name: "diagnostic", Value: oversized}}}
	EnforceBounds(&errorEnvelope)
	if len(errorEnvelope.CLIVersion) > MaxStringBytes || len(errorEnvelope.Error.Message) > MaxStringBytes || len(errorEnvelope.Error.Details[0].Value) > MaxStringBytes {
		t.Fatalf("data-less structured error remained oversized: %#v", errorEnvelope)
	}
}

func TestSensitiveMetadataFieldCannotRemainComplete(t *testing.T) {
	sources := []Source{{Metadata: []Metadata{{Name: "raw_path", Value: "relative-looking-value"}}, Relationships: []Relationship{}}}
	envelope := NewEnvelope("list", "test", StatusComplete)
	envelope.Data = &Data{Sources: &sources}
	Finalize(&envelope)
	if envelope.Status != StatusPartial || sources[0].Metadata[0].Value != "<redacted:unsafe-field>" {
		t.Fatalf("sensitive metadata was not safely omitted: %#v", envelope)
	}
}

func eventPointer(events []Event) *[]Event { return &events }

func fullyPopulatedEnvelope() Envelope {
	nativeID := "fictional-source"
	ref := NewSourceRef("synthetic", "synthetic-default", nativeID)
	source := Source{
		Identity: SourceIdentity{Provider: "synthetic", SourceInstance: "synthetic-default", ProviderNativeSourceID: nativeID, ProviderSourceFingerprint: SourceFingerprint(nativeID), SourceRef: ref},
		Kind:     "session", VersionHint: &VersionHint{Kind: "provider_metadata", Value: "hint"}, Relationships: []Relationship{{Kind: "related", SourceRef: ref}}, Metadata: []Metadata{{Name: "note", Value: "value"}},
	}
	exitCode := 0
	events := []Event{
		{Index: 0, Kind: EventMessage, Message: &MessageEvent{Role: "user", Text: "message"}, Metadata: []Metadata{{Name: "note", Value: "value"}}},
		{Index: 1, Kind: EventToolCall, ToolCall: &ToolCallEvent{CallID: "call-1", Category: "filesystem"}, Metadata: []Metadata{}},
		{Index: 2, Kind: EventToolResult, ToolResult: &ToolResultEvent{CallID: "call-1", Success: true, ExitCode: &exitCode}, Metadata: []Metadata{}},
		{Index: 3, Kind: EventError, Error: &ErrorEvent{Category: "provider", Message: "error"}, Metadata: []Metadata{}},
	}
	envelope := NewEnvelope("events", "test", StatusError)
	envelope.Data = &Data{Source: &source, SourceRef: ref, Events: &events, VerifiedVersion: &VerifiedVersionID{Algorithm: "sha256", Basis: "provider-content-v0", Value: "sha256:value"}}
	envelope.Page = &Page{Limit: 50, NextCursor: "v0:50", HasMore: true}
	envelope.Omissions = []Omission{{Code: "unknown_shape", Scope: "source", Message: "omitted"}}
	envelope.Error = &PublicError{Code: "provider_failure", Category: "provider", Message: "failed", Details: []ErrorDetail{{Name: "diagnostic", Value: "value"}}}
	return envelope
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
