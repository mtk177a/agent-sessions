package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/provider"
)

func TestFourOperationJSONContractAndReadOnlyBehavior(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	configJSON := `{"schema_version":"v0alpha1","sources":[{"id":"synthetic-default","provider":"synthetic","root":` + quoted(root) + `}]}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, root)

	adapter := newSyntheticAdapter()
	registry := provider.NewRegistry(adapter)
	runner := Runner{Version: "test", Registry: registry}
	ref := adapter.source.Identity.SourceRef

	tests := []struct {
		name          string
		args          []string
		wantOperation string
		wantStatus    contract.Status
		wantExit      int
	}{
		{"list", []string{"list", "--config", configPath, "--provider", "synthetic"}, "list", contract.StatusComplete, ExitOK},
		{"show", []string{"show", "--config", configPath, ref}, "show", contract.StatusComplete, ExitOK},
		{"events", []string{"events", "--config", configPath, "--limit", "1", ref}, "events", contract.StatusPartial, ExitOK},
		{"verify", []string{"verify", "--config", configPath, ref}, "verify", contract.StatusComplete, ExitOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			exit := runner.Run(context.Background(), tc.args, &output)
			if exit != tc.wantExit {
				t.Fatalf("exit = %d, want %d; output=%s", exit, tc.wantExit, output.String())
			}
			var envelope contract.Envelope
			if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
				t.Fatalf("invalid JSON: %v; output=%s", err, output.String())
			}
			if envelope.Operation != tc.wantOperation || envelope.Status != tc.wantStatus {
				t.Fatalf("unexpected envelope: %#v", envelope)
			}
			switch tc.name {
			case "list":
				if envelope.Data == nil || envelope.Data.Sources == nil || len(*envelope.Data.Sources) != 1 {
					t.Fatalf("list contract is incomplete: %#v", envelope.Data)
				}
			case "show":
				if envelope.Data == nil || envelope.Data.Source == nil || envelope.Data.Source.Identity.SourceRef != ref {
					t.Fatalf("show contract is incomplete: %#v", envelope.Data)
				}
			case "events":
				if envelope.Data == nil || envelope.Data.Events == nil || len(*envelope.Data.Events) != 1 || envelope.Page == nil || !envelope.Page.HasMore || envelope.Page.NextCursor == "" {
					t.Fatalf("events contract is incomplete: data=%#v page=%#v", envelope.Data, envelope.Page)
				}
			case "verify":
				if envelope.Data == nil || envelope.Data.VerifiedVersion == nil || envelope.Data.VerifiedVersion.Algorithm != "sha256" {
					t.Fatalf("verify contract is incomplete: %#v", envelope.Data)
				}
			}
		})
	}

	var eventOutput bytes.Buffer
	if exit := runner.Run(context.Background(), []string{"events", "--config", configPath, ref}, &eventOutput); exit != ExitOK {
		t.Fatalf("full events exit=%d output=%s", exit, eventOutput.String())
	}
	var eventEnvelope contract.Envelope
	if err := json.Unmarshal(eventOutput.Bytes(), &eventEnvelope); err != nil {
		t.Fatal(err)
	}
	events := *eventEnvelope.Data.Events
	if events[1].ToolCall.CallID != "call-1" || events[2].ToolResult.CallID != "call-1" || !events[2].ToolResult.Success || events[2].ToolResult.ExitCode == nil || *events[2].ToolResult.ExitCode != 0 {
		t.Fatalf("tool relationship evidence was not preserved: %#v", events)
	}

	after := snapshot(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("read-only commands changed persistent state\nbefore=%v\nafter=%v", before, after)
	}
}

func TestStructuredErrorsAndExitCodes(t *testing.T) {
	runner := Runner{Version: "test", Registry: provider.NewRegistry()}
	cases := []struct {
		args       []string
		wantExit   int
		wantStatus contract.Status
	}{
		{[]string{"unknown"}, ExitUsage, contract.StatusError},
		{[]string{"list", "--provider", "missing"}, ExitUnsupported, contract.StatusUnsupported},
		{[]string{"events", "not-a-ref"}, ExitUsage, contract.StatusError},
		{[]string{"list", "--limit", "101"}, ExitUsage, contract.StatusError},
	}
	for _, tc := range cases {
		var output bytes.Buffer
		exit := runner.Run(context.Background(), tc.args, &output)
		if exit != tc.wantExit {
			t.Fatalf("args=%v exit=%d want=%d output=%s", tc.args, exit, tc.wantExit, output.String())
		}
		var envelope contract.Envelope
		if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Status != tc.wantStatus {
			t.Fatalf("args=%v status=%s want=%s", tc.args, envelope.Status, tc.wantStatus)
		}
		if envelope.Status == contract.StatusError && envelope.Error == nil {
			t.Fatal("error response lacks structured error")
		}
	}
}

func TestDiagnosticDoesNotExposeExplicitConfigPath(t *testing.T) {
	runner := Runner{Version: "test", Registry: provider.NewRegistry()}
	unsafePath := filepath.Join(t.TempDir(), "missing-config.json")
	var output bytes.Buffer
	if exit := runner.Run(context.Background(), []string{"list", "--config", unsafePath}, &output); exit != ExitUsage {
		t.Fatalf("exit=%d output=%s", exit, output.String())
	}
	if bytes.Contains(output.Bytes(), []byte(unsafePath)) {
		t.Fatalf("diagnostic exposed config path: %s", output.String())
	}
}

func TestEventsRejectEventCountBeyondBound(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	configJSON := `{"schema_version":"v0alpha1","sources":[{"id":"synthetic-default","provider":"synthetic","root":` + quoted(root) + `}]}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := newSyntheticAdapter()
	adapter.events = make([]contract.Event, MaxEventCount+1)
	runner := Runner{Version: "test", Registry: provider.NewRegistry(adapter)}
	var output bytes.Buffer
	exit := runner.Run(context.Background(), []string{"events", "--config", configPath, adapter.source.Identity.SourceRef}, &output)
	if exit != ExitFailure {
		t.Fatalf("exit=%d output=%s", exit, output.String())
	}
	var envelope contract.Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error == nil || envelope.Error.Code != "event_limit_exceeded" {
		t.Fatalf("unexpected error contract: %#v", envelope)
	}
}

type syntheticAdapter struct {
	source contract.Source
	events []contract.Event
}

func newSyntheticAdapter() *syntheticAdapter {
	nativeID := "00000000-0000-4000-8000-000000000001"
	ref := contract.NewSourceRef("synthetic", "synthetic-default", nativeID)
	exitCode := 0
	return &syntheticAdapter{
		source: contract.Source{
			Identity: contract.SourceIdentity{Provider: "synthetic", SourceInstance: "synthetic-default", ProviderNativeSourceID: nativeID, ProviderSourceFingerprint: contract.SourceFingerprint(nativeID), SourceRef: ref},
			Kind:     "session", Relationships: []contract.Relationship{}, Metadata: []contract.Metadata{{Name: "label", Value: "Fictional session"}},
			VersionHint: &contract.VersionHint{Kind: "synthetic", Value: "hint-1"},
		},
		events: []contract.Event{
			{Index: 0, Kind: contract.EventMessage, Message: &contract.MessageEvent{Role: "user", Text: "Hello"}, Metadata: []contract.Metadata{}},
			{Index: 1, Kind: contract.EventToolCall, ToolCall: &contract.ToolCallEvent{CallID: "call-1", Category: "filesystem"}, Metadata: []contract.Metadata{}},
			{Index: 2, Kind: contract.EventToolResult, ToolResult: &contract.ToolResultEvent{CallID: "call-1", Success: true, ExitCode: &exitCode}, Metadata: []contract.Metadata{}},
		},
	}
}

func (a *syntheticAdapter) Name() string                    { return "synthetic" }
func (a *syntheticAdapter) EnvironmentRoot() (string, bool) { return "", false }
func (a *syntheticAdapter) DefaultRoot() (string, bool)     { return "", false }
func (a *syntheticAdapter) List(context.Context, config.Source) provider.SourceResult {
	return provider.SourceResult{Status: contract.StatusComplete, Sources: []contract.Source{a.source}}
}
func (a *syntheticAdapter) Show(_ context.Context, _ config.Source, fingerprint string) provider.SourceResult {
	if fingerprint != a.source.Identity.ProviderSourceFingerprint {
		return provider.SourceResult{Err: provider.ErrNotFound}
	}
	return provider.SourceResult{Status: contract.StatusComplete, Sources: []contract.Source{a.source}}
}
func (a *syntheticAdapter) Events(_ context.Context, _ config.Source, fingerprint string) provider.EventResult {
	if fingerprint != a.source.Identity.ProviderSourceFingerprint {
		return provider.EventResult{Err: provider.ErrNotFound}
	}
	return provider.EventResult{Status: contract.StatusComplete, Events: a.events}
}
func (a *syntheticAdapter) Evidence(_ context.Context, _ config.Source, fingerprint string) provider.EvidenceResult {
	if fingerprint != a.source.Identity.ProviderSourceFingerprint {
		return provider.EvidenceResult{Err: provider.ErrNotFound}
	}
	return provider.EvidenceResult{Status: contract.StatusComplete, Chunks: []contract.EvidenceChunk{{Name: "session.jsonl", Content: []byte("fictional evidence")}}}
}

func quoted(value string) string {
	b, _ := json.Marshal(value)
	return string(b)
}

func snapshot(t *testing.T, root string) map[string]int64 {
	t.Helper()
	result := map[string]int64{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[rel] = info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
