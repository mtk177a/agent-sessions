package contract

import (
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
	errorJSON := string(mustJSON(t, errorEnvelope))
	for _, forbidden := range []string{"sk-fictional-secret", "/fictional", "host.example.invalid", "192.0.2.10", "single-label-host"} {
		if strings.Contains(errorJSON, forbidden) {
			t.Fatalf("structured error contains %q: %s", forbidden, errorJSON)
		}
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

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
