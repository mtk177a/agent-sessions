package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
)

const testThreadID = "018f47e2-7b0a-7d31-8a13-7dc76c914abc"

func TestAdapterDiscoversAndNormalizesCodexRollout(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "2026", "09", "03", "rollout-2026-09-03T10-00-00-"+testThreadID+".jsonl")
	writeRollout(t, path, []string{
		`{"timestamp":"2026-09-03T10:00:00Z","type":"session_meta","payload":{"session_id":"018f47e2-7b0a-7d31-8a13-7dc76c914abc","id":"018f47e2-7b0a-7d31-8a13-7dc76c914abc","timestamp":"2026-09-03T10:00:00Z","cwd":"/fictional/work","originator":"codex_cli_rs","cli_version":"0.149.1","source":"cli","history_mode":"legacy"}}`,
		`{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
		`{"timestamp":"2026-09-03T10:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"hi"}}`,
		`{"timestamp":"2026-09-03T10:00:03Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"do not expose\"}","call_id":"provider-call-1"}}`,
		`{"timestamp":"2026-09-03T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"provider-call-1","output":"fictional output"}}`,
		`{"timestamp":"2026-09-03T10:00:05Z","type":"event_msg","payload":{"type":"error","message":"fictional failure"}}`,
	})

	adapter := New()
	source := config.Source{ID: "codex-default", Provider: "codex", Root: home}
	listed := adapter.List(context.Background(), source)
	if listed.Status != contract.StatusComplete || len(listed.Sources) != 1 {
		t.Fatalf("List() = %#v", listed)
	}
	got := listed.Sources[0]
	if got.Identity.ProviderNativeSourceID != testThreadID || got.Identity.SourceRef != contract.NewSourceRef("codex", "codex-default", testThreadID) {
		t.Fatalf("identity = %#v", got.Identity)
	}

	events := adapter.Events(context.Background(), source, got.Identity.ProviderSourceFingerprint)
	if events.Status != contract.StatusPartial || len(events.Events) != 4 || !hasOmission(events.Omissions, "unsupported_tool_result") {
		t.Fatalf("Events() = %#v", events)
	}
	if events.Events[0].Message == nil || events.Events[0].Message.Role != "user" || events.Events[1].Message == nil || events.Events[1].Message.Role != "assistant" {
		t.Fatalf("messages = %#v", events.Events[:2])
	}
	call := events.Events[2].ToolCall
	if call == nil || call.CallID == "provider-call-1" || call.Category != "shell" {
		t.Fatalf("tool call = %#v", events.Events[2])
	}
	if events.Events[3].Error == nil || events.Events[3].Error.Category != "provider" {
		t.Fatalf("error = %#v", events.Events[3])
	}
	evidence := adapter.Evidence(context.Background(), source, got.Identity.ProviderSourceFingerprint)
	if evidence.Status != contract.StatusComplete || len(evidence.Chunks) != 1 || evidence.Chunks[0].Name != "rollout/primary.jsonl" {
		t.Fatalf("Evidence() = %#v", evidence)
	}
}

func TestUnknownAndMalformedRowsDegradeCompleteness(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "archived_sessions", "rollout-2026-09-03T10-00-00-"+testThreadID+".jsonl")
	writeRollout(t, path, []string{
		`{"timestamp":"2026-09-03T10:00:00Z","type":"session_meta","payload":{"session_id":"018f47e2-7b0a-7d31-8a13-7dc76c914abc","id":"018f47e2-7b0a-7d31-8a13-7dc76c914abc","timestamp":"2026-09-03T10:00:00Z","cwd":"/fictional/work","originator":"codex_cli_rs","cli_version":"0.149.1","source":"cli","history_mode":"legacy"}}`,
		`{"timestamp":"2026-09-03T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"usable"}}`,
		`{"timestamp":"2026-09-03T10:00:01Z","type":"future_row","payload":{}}`,
		`{broken`,
	})
	adapter := New()
	source := config.Source{ID: "archive", Provider: "codex", Root: home}
	listed := adapter.List(context.Background(), source)
	if listed.Status != contract.StatusComplete || len(listed.Sources) != 1 {
		t.Fatalf("List() = %#v", listed)
	}
	events := adapter.Events(context.Background(), source, listed.Sources[0].Identity.ProviderSourceFingerprint)
	if events.Status != contract.StatusPartial || len(events.Omissions) != 2 {
		t.Fatalf("Events() = %#v", events)
	}
}

func TestIdentityDoesNotDependOnActiveOrArchivedLocation(t *testing.T) {
	adapter := New()
	refs := make([]string, 0, 2)
	for _, collection := range []string{"sessions/2026/09/03", "archived_sessions"} {
		home := t.TempDir()
		path := filepath.Join(home, filepath.FromSlash(collection), "rollout-2026-09-03T10-00-00-"+testThreadID+".jsonl")
		writeRollout(t, path, []string{`{"timestamp":"2026-09-03T10:00:00Z","type":"session_meta","payload":{"session_id":"018f47e2-7b0a-7d31-8a13-7dc76c914abc","id":"018f47e2-7b0a-7d31-8a13-7dc76c914abc","timestamp":"2026-09-03T10:00:00Z","cwd":"/fictional/work","originator":"codex_cli_rs","cli_version":"0.149.1","source":"cli","history_mode":"legacy"}}`})
		listed := adapter.List(context.Background(), config.Source{ID: "same", Provider: "codex", Root: home})
		if len(listed.Sources) != 1 {
			t.Fatalf("List() = %#v", listed)
		}
		refs = append(refs, listed.Sources[0].Identity.SourceRef)
	}
	if refs[0] != refs[1] {
		t.Fatalf("refs differ: %q != %q", refs[0], refs[1])
	}
}

func TestDefaults(t *testing.T) {
	t.Setenv("CODEX_HOME", "/fictional/codex-home")
	adapter := New()
	if root, ok := adapter.EnvironmentRoot(); !ok || root != "/fictional/codex-home" {
		t.Fatalf("EnvironmentRoot() = %q, %v", root, ok)
	}
	t.Setenv("CODEX_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if root, ok := adapter.DefaultRoot(); !ok || root != filepath.Join(home, ".codex") {
		t.Fatalf("DefaultRoot() = %q, %v", root, ok)
	}
}

func writeRollout(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte{}
	for _, line := range lines {
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
