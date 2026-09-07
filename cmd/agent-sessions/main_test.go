package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/cli"
	"github.com/mtk177a/agent-sessions/internal/contract"
)

func TestProductionRegistryListsOneAvailableDefaultProvider(t *testing.T) {
	for _, providerName := range []string{"claude", "codex"} {
		t.Run(providerName, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("CLAUDE_CONFIG_DIR", "")
			t.Setenv("CODEX_HOME", "")
			configPath := filepath.Join(home, "config.json")
			if err := os.WriteFile(configPath, []byte(`{"schema_version":"v1","sources":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}

			switch providerName {
			case "claude":
				writeClaudeTranscript(t, filepath.Join(home, ".claude"))
			case "codex":
				writeCodexTranscript(t, filepath.Join(home, ".codex"))
			}

			var output bytes.Buffer
			exit := newRunner().Run(context.Background(), []string{"list", "--config", configPath}, &output)
			if exit != cli.ExitOK {
				t.Fatalf("exit = %d, output = %s", exit, output.String())
			}
			var envelope contract.Envelope
			if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Status != contract.StatusComplete || envelope.Data == nil || envelope.Data.Sources == nil || len(*envelope.Data.Sources) != 1 || (*envelope.Data.Sources)[0].Identity.Provider != providerName {
				t.Fatalf("list = %#v", envelope)
			}
		})
	}
}

func TestProductionRegistrySupportsBothProvidersEndToEnd(t *testing.T) {
	root := t.TempDir()
	claudeRoot := filepath.Join(root, "claude")
	codexRoot := filepath.Join(root, "codex")
	writeClaudeTranscript(t, claudeRoot)
	writeCodexTranscript(t, codexRoot)

	configPath := filepath.Join(root, "config.json")
	configured := fmt.Sprintf(`{"schema_version":"v1","sources":[{"id":"claude-one","provider":"claude","root":%q},{"id":"codex-one","provider":"codex","root":%q}]}`, claudeRoot, codexRoot)
	if err := os.WriteFile(configPath, []byte(configured), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := newRunner()
	listed := runProductionCLI(t, runner, "list", "--config", configPath)
	if listed.Data == nil || listed.Data.Sources == nil || len(*listed.Data.Sources) != 2 {
		t.Fatalf("list data = %#v", listed.Data)
	}

	for _, source := range *listed.Data.Sources {
		t.Run(source.Identity.Provider, func(t *testing.T) {
			providerName := source.Identity.Provider
			wantInstance := providerName + "-one"
			if source.Identity.SourceInstance != wantInstance || source.Identity.SourceRef != contract.NewSourceRef(providerName, wantInstance, source.Identity.ProviderNativeSourceID) {
				t.Fatalf("identity = %#v", source.Identity)
			}
			if source.VersionHint == nil {
				t.Fatalf("version hint is missing: %#v", source)
			}

			shown := runProductionCLI(t, runner, "show", "--config", configPath, source.Identity.SourceRef)
			if shown.Data == nil || shown.Data.Source == nil || shown.Data.Source.Identity != source.Identity {
				t.Fatalf("show data = %#v", shown.Data)
			}

			eventEnvelope := runProductionCLI(t, runner, "events", "--config", configPath, source.Identity.SourceRef)
			if eventEnvelope.Data == nil || eventEnvelope.Data.Events == nil || eventEnvelope.Data.SourceRef != source.Identity.SourceRef {
				t.Fatalf("events data = %#v", eventEnvelope.Data)
			}
			assertCommonEvents(t, *eventEnvelope.Data.Events)

			verified := runProductionCLI(t, runner, "verify", "--config", configPath, source.Identity.SourceRef)
			if verified.Data == nil || verified.Data.SourceRef != source.Identity.SourceRef || verified.Data.VerifiedVersion == nil {
				t.Fatalf("verify data = %#v", verified.Data)
			}
			if verified.Data.VerifiedVersion.Basis != "provider-content-v0" || verified.Data.VerifiedVersion.Value == source.VersionHint.Value {
				t.Fatalf("version contracts were conflated: hint=%#v verified=%#v", source.VersionHint, verified.Data.VerifiedVersion)
			}
		})
	}
}

func TestProductionRegistrySupportsChatGPTDataExport(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	exportPath := filepath.Join(root, "chatgpt-export.zip")
	writeChatGPTExport(t, exportPath)
	configPath := filepath.Join(root, "config.json")
	configured := fmt.Sprintf(`{"schema_version":"v1","sources":[{"id":"chatgpt-one","provider":"chatgpt","root":%q}]}`, exportPath)
	if err := os.WriteFile(configPath, []byte(configured), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := newRunner()
	listed := runProductionCLI(t, runner, "list", "--config", configPath)
	if listed.Data == nil || listed.Data.Sources == nil || len(*listed.Data.Sources) != 1 {
		t.Fatalf("list data = %#v", listed.Data)
	}
	source := (*listed.Data.Sources)[0]
	if source.Identity.Provider != "chatgpt" || source.Identity.SourceInstance != "chatgpt-one" || source.Kind != "conversation" || source.VersionHint == nil || source.VersionHint.Kind != "snapshot_hash" {
		t.Fatalf("source = %#v", source)
	}
	shown := runProductionCLI(t, runner, "show", "--config", configPath, source.Identity.SourceRef)
	if shown.Data == nil || shown.Data.Source == nil || shown.Data.Source.Identity != source.Identity {
		t.Fatalf("show data = %#v", shown.Data)
	}
	events := runProductionCLI(t, runner, "events", "--config", configPath, source.Identity.SourceRef)
	if events.Data == nil || events.Data.Events == nil || len(*events.Data.Events) != 2 {
		t.Fatalf("events data = %#v", events.Data)
	}
	verified := runProductionCLI(t, runner, "verify", "--config", configPath, source.Identity.SourceRef)
	if verified.Data == nil || verified.Data.VerifiedVersion == nil || verified.Data.VerifiedVersion.Basis != "provider-content-v0" || verified.Data.VerifiedVersion.Value == source.VersionHint.Value {
		t.Fatalf("verify data = %#v", verified.Data)
	}
}

func TestChatGPTEventsApplyFinalRedactionAndRetainPolicyVersion(t *testing.T) {
	root := t.TempDir()
	exportPath := filepath.Join(root, "chatgpt-export.zip")
	content := `[{"id":"fictional-conversation","conversation_id":"fictional-conversation","current_node":"user","mapping":{"user":{"parent":null,"message":{"author":{"role":"user"},"content":{"content_type":"text","parts":["token=sk-fictional-secret"]}}}}}]`
	writeChatGPTExportContent(t, exportPath, content)
	configPath := filepath.Join(root, "config.json")
	configured := fmt.Sprintf(`{"schema_version":"v1","sources":[{"id":"chatgpt-one","provider":"chatgpt","root":%q}]}`, exportPath)
	if err := os.WriteFile(configPath, []byte(configured), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := contract.NewSourceRef("chatgpt", "chatgpt-one", "fictional-conversation")
	var output bytes.Buffer
	if exit := newRunner().Run(context.Background(), []string{"events", "--config", configPath, ref}, &output); exit != cli.ExitOK {
		t.Fatalf("exit = %d, output = %s", exit, output.String())
	}
	if bytes.Contains(output.Bytes(), []byte("sk-fictional-secret")) {
		t.Fatalf("redaction leaked source content: %s", output.String())
	}
	var envelope contract.Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != contract.StatusPartial || envelope.RedactionPolicyVersion != "v1" || !hasOmissionCode(envelope.Omissions, "output_redacted") {
		t.Fatalf("redaction contract = %#v", envelope)
	}
}

func TestProductionRegistryDoesNotSkipExplicitOrConfiguredRoots(t *testing.T) {
	home := t.TempDir()
	missing := filepath.Join(home, "missing")
	configuredPath := filepath.Join(home, "configured.json")
	configured := fmt.Sprintf(`{"schema_version":"v1","sources":[{"id":"claude-default","provider":"claude","root":%q}]}`, missing)
	if err := os.WriteFile(configuredPath, []byte(configured), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"list", "--provider", "claude", "--root", missing},
		{"list", "--provider", "claude", "--config", configuredPath},
	} {
		var output bytes.Buffer
		if exit := newRunner().Run(context.Background(), args, &output); exit != cli.ExitFailure {
			t.Fatalf("Run(%v) exit = %d, output = %s", args, exit, output.String())
		}
		var envelope contract.Envelope
		if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Error == nil || envelope.Error.Code != "provider_failure" {
			t.Fatalf("Run(%v) = %#v", args, envelope)
		}
	}
}

func writeClaudeTranscript(t *testing.T, root string) {
	t.Helper()
	sessionID := "11111111-2222-4333-8444-555555555555"
	path := filepath.Join(root, "projects", "fictional-project", sessionID+".jsonl")
	content := fmt.Sprintf(`{"type":"user","sessionId":%q,"uuid":"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee","version":"2.1.228","message":{"role":"user","content":"hello"}}`, sessionID) + "\n" +
		fmt.Sprintf(`{"type":"assistant","sessionId":%q,"uuid":"bbbbbbbb-cccc-4ddd-8eee-ffffffffffff","version":"2.1.228","message":{"role":"assistant","content":[{"type":"text","text":"hi"},{"type":"tool_use","id":"provider-call","name":"Bash","input":{"command":"must not escape"}}]}}`, sessionID) + "\n" +
		fmt.Sprintf(`{"type":"user","sessionId":%q,"uuid":"cccccccc-dddd-4eee-8fff-000000000000","version":"2.1.228","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"provider-call","content":"private output","is_error":false}]}}`, sessionID) + "\n" +
		fmt.Sprintf(`{"type":"assistant","sessionId":%q,"uuid":"dddddddd-eeee-4fff-8000-111111111111","version":"2.1.228","isApiErrorMessage":true,"message":{"role":"assistant","content":[{"type":"text","text":"fictional provider failure"}],"stop_reason":"error"}}`, sessionID) + "\n"
	writeFixture(t, path, content)
}

func writeCodexTranscript(t *testing.T, root string) {
	t.Helper()
	threadID := "018f47e2-7b0a-7d31-8a13-7dc76c914abc"
	path := filepath.Join(root, "sessions", "2026", "09", "04", "rollout-2026-09-04T10-00-00-"+threadID+".jsonl")
	header := fmt.Sprintf(`{"type":"session_meta","payload":{"session_id":%q,"id":%q,"cli_version":"0.149.1","history_mode":"paginated"}}`, threadID, threadID)
	content := header + "\n" +
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"user-1","content":[{"type":"text","text":"hello"}]}}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"agent-1","content":[{"type":"Text","text":"hi"}]}}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"command-1","status":"completed","exit_code":0,"command":"must not escape"}}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"error","message":"fictional provider failure"}}` + "\n"
	writeFixture(t, path, content)
}

func writeChatGPTExport(t *testing.T, filename string) {
	t.Helper()
	content := `[{"id":"fictional-conversation","conversation_id":"fictional-conversation","current_node":"assistant","mapping":{"root":{"parent":null,"message":null},"user":{"parent":"root","message":{"author":{"role":"user"},"content":{"content_type":"text","parts":["hello"]}}},"assistant":{"parent":"user","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["hi"]}}}}}]`
	writeChatGPTExportContent(t, filename, content)
}

func writeChatGPTExportContent(t *testing.T, filename, content string) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	member, err := writer.Create("conversations.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func hasOmissionCode(omissions []contract.Omission, code string) bool {
	for _, omission := range omissions {
		if omission.Code == code {
			return true
		}
	}
	return false
}

func runProductionCLI(t *testing.T, runner cli.Runner, args ...string) contract.Envelope {
	t.Helper()
	var output bytes.Buffer
	if exit := runner.Run(context.Background(), args, &output); exit != cli.ExitOK {
		t.Fatalf("Run(%v) exit = %d, output = %s", args, exit, output.String())
	}
	var envelope contract.Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "v1" || envelope.RedactionPolicyVersion != "v1" || envelope.Status != contract.StatusComplete || envelope.Omissions == nil {
		t.Fatalf("unstable envelope for %v: %#v", args, envelope)
	}
	return envelope
}

func assertCommonEvents(t *testing.T, events []contract.Event) {
	t.Helper()
	var user, assistant, providerError bool
	var callID, resultID string
	for _, event := range events {
		switch event.Kind {
		case contract.EventMessage:
			user = user || event.Message != nil && event.Message.Role == "user"
			assistant = assistant || event.Message != nil && event.Message.Role == "assistant"
		case contract.EventToolCall:
			if event.ToolCall != nil {
				callID = event.ToolCall.CallID
			}
		case contract.EventToolResult:
			if event.ToolResult != nil && event.ToolResult.Success {
				resultID = event.ToolResult.CallID
			}
		case contract.EventError:
			providerError = event.Error != nil && event.Error.Category == "provider"
		}
	}
	if !user || !assistant || !providerError || callID == "" || callID != resultID {
		t.Fatalf("common event contract is incomplete: %#v", events)
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
