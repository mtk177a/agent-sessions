package main

import (
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
			if err := os.WriteFile(configPath, []byte(`{"schema_version":"v0alpha1","sources":[]}`), 0o600); err != nil {
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

func TestProductionRegistryDoesNotSkipExplicitOrConfiguredRoots(t *testing.T) {
	home := t.TempDir()
	missing := filepath.Join(home, "missing")
	configuredPath := filepath.Join(home, "configured.json")
	configured := fmt.Sprintf(`{"schema_version":"v0alpha1","sources":[{"id":"claude-default","provider":"claude","root":%q}]}`, missing)
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
	writeFixture(t, path, fmt.Sprintf(`{"type":"user","sessionId":%q,"uuid":"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee","version":"2.1.228","message":{"role":"user","content":"hello"}}`+"\n", sessionID))
}

func writeCodexTranscript(t *testing.T, root string) {
	t.Helper()
	threadID := "018f47e2-7b0a-7d31-8a13-7dc76c914abc"
	path := filepath.Join(root, "sessions", "2026", "09", "04", "rollout-2026-09-04T10-00-00-"+threadID+".jsonl")
	header := fmt.Sprintf(`{"type":"session_meta","payload":{"session_id":%q,"id":%q,"cli_version":"0.149.1","history_mode":"legacy"}}`, threadID, threadID)
	writeFixture(t, path, header+"\n")
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
