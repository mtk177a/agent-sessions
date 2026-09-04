package codex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/cli"
	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/provider"
)

const secondThreadID = "018f47e2-7b0a-7d31-8a13-7dc76c914abd"

func TestPaginatedHistoryUsesCompletedItemsAsCanonicalRows(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.149.1", "paginated", ""),
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"duplicate"}]}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"secret\"}","call_id":"raw-call"}}`,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"user-1","content":[{"type":"text","text":"canonical user"}]}}}`,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"agent-1","content":[{"type":"Text","text":"canonical assistant"}]}}}`,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"command-1","status":"completed","exit_code":0,"command":"must not escape"}}}`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusComplete || len(result.Events) != 4 {
		t.Fatalf("Events() = %#v", result)
	}
	if result.Events[0].Message == nil || result.Events[0].Message.Text != "canonical user" || result.Events[1].Message == nil || result.Events[1].Message.Text != "canonical assistant" {
		t.Fatalf("canonical messages = %#v", result.Events[:2])
	}
	call, toolResult := result.Events[2].ToolCall, result.Events[3].ToolResult
	if call == nil || toolResult == nil || call.CallID != toolResult.CallID || call.CallID == "command-1" || !contract.ValidIdentifier(call.CallID) {
		t.Fatalf("normalized tool pair = %#v", result.Events[2:])
	}
	encoded, err := json.Marshal(result.Events)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"raw-call", "must not escape", "secret", "exec_command"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("normalized events exposed %q", forbidden)
		}
	}
}

func TestLegacyPersistedToolOutputDoesNotGuessSuccess(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.149.1", "legacy", ""),
		`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"provider-call"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"provider-call","output":"fictional output"}}`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || result.Events[0].ToolCall == nil || !hasOmission(result.Omissions, "unsupported_tool_result") {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestLegacyToolCallWithoutPersistedOutputIsPartial(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.149.1", "legacy", ""),
		`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"provider-call"}}`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || result.Events[0].ToolCall == nil || !hasOmission(result.Omissions, "correlation_omitted") {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestPaginatedUnsupportedKnownItemsDegradeCompleteness(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.149.1", "paginated", ""),
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"user-1","content":[{"type":"text","text":"usable"}]}}}`,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"FileChange","id":"change-1"}}}`,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CollabAgentToolCall","id":"agent-1"}}}`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || countOmissions(result.Omissions, "unsupported_event") != 2 {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestPaginatedUserMessageReportsUnsupportedContent(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.149.1", "paginated", ""),
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"user-1","content":[{"type":"text","text":"usable"},{"type":"image","image_url":"data:image/png;base64,fictional"}]}}}`,
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"user-2","content":[{"type":"audio","audio_url":"data:audio/wav;base64,fictional"}]}}}`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || result.Events[0].Message == nil || result.Events[0].Message.Text != "usable" || countOmissions(result.Omissions, "unsupported_content") != 2 {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestUnverifiedTopLevelRowsDegradeCompleteness(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.149.1", "legacy", ""),
		`{"type":"event_msg","payload":{"type":"user_message","message":"usable"}}`,
		`{"type":"token_usage_record","payload":{}}`,
		`{"type":"retained_context","payload":{}}`,
		`{"type":"realtime_item","payload":{}}`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || countOmissions(result.Omissions, "unknown_format") != 3 {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestDeepHeaderIsRejected(t *testing.T) {
	home := t.TempDir()
	deep := strings.Repeat(`{"x":`, contract.MaxJSONDepth) + `0` + strings.Repeat(`}`, contract.MaxJSONDepth)
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{header(testThreadID, "0.149.1", "legacy", `,"deep":`+deep)})

	listed := New().List(context.Background(), testSource(home))
	if listed.Status != contract.StatusUnsupported || len(listed.Sources) != 0 || !hasOmission(listed.Omissions, "malformed_record") {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestDeepEventRowDegradesCompleteness(t *testing.T) {
	home := t.TempDir()
	deep := strings.Repeat(`{"x":`, contract.MaxJSONDepth) + `0` + strings.Repeat(`}`, contract.MaxJSONDepth)
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.149.1", "legacy", ""),
		`{"type":"event_msg","payload":{"type":"user_message","message":"usable"},"deep":` + deep + `}`,
	})

	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusUnsupported || len(result.Events) != 0 || !hasOmission(result.Omissions, "malformed_record") {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestCorrelationFailuresArePartialAndNotGuessed(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.149.1", "legacy", ""),
		`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"same"}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"same"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"orphan","output":"fictional output"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"orphan","output":"fictional output"}}`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || result.Events[0].ToolCall == nil || len(result.Omissions) != 3 {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestRelationshipsRequireExplicitProviderFields(t *testing.T) {
	home := t.TempDir()
	extra := `,"parent_thread_id":"` + secondThreadID + `","forked_from_id":"018f47e2-7b0a-7d31-8a13-7dc76c914abe"`
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{header(testThreadID, "0.149.1", "legacy", extra)})
	listed := New().List(context.Background(), testSource(home))
	if listed.Status != contract.StatusComplete || len(listed.Sources) != 1 || len(listed.Sources[0].Relationships) != 2 {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestMalformedRelationshipIDsAreOmitted(t *testing.T) {
	home := t.TempDir()
	extra := `,"parent_thread_id":"not-a-thread-id","forked_from_id":"also-invalid"`
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{header(testThreadID, "0.149.1", "legacy", extra)})
	listed := New().List(context.Background(), testSource(home))
	if listed.Status != contract.StatusPartial || len(listed.Sources) != 1 || len(listed.Sources[0].Relationships) != 0 || countOmissions(listed.Omissions, "malformed_record") != 2 {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestResumeKeepsIdentityAndChangesVersionHint(t *testing.T) {
	home := t.TempDir()
	path := rolloutPath(home, "sessions", testThreadID)
	writeRollout(t, path, []string{header(testThreadID, "0.149.1", "legacy", "")})
	first := New().List(context.Background(), testSource(home)).Sources[0]
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"type":"event_msg","payload":{"type":"user_message","message":"resumed"}}` + "\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	second := New().List(context.Background(), testSource(home)).Sources[0]
	if first.Identity.SourceRef != second.Identity.SourceRef || first.VersionHint == nil || second.VersionHint == nil || first.VersionHint.Value == second.VersionHint.Value {
		t.Fatalf("before = %#v, after = %#v", first, second)
	}
}

func TestAmbiguousActiveAndArchivedArtifactsAreNotSilentlySelected(t *testing.T) {
	home := t.TempDir()
	lines := []string{header(testThreadID, "0.149.1", "legacy", "")}
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), lines)
	writeRollout(t, rolloutPath(home, "archived_sessions", testThreadID), lines)
	adapter := New()
	listed := adapter.List(context.Background(), testSource(home))
	if listed.Status != contract.StatusPartial || len(listed.Sources) != 1 {
		t.Fatalf("List() = %#v", listed)
	}
	result := adapter.Events(context.Background(), testSource(home), listed.Sources[0].Identity.ProviderSourceFingerprint)
	if result.Status != contract.StatusUnsupported || len(result.Events) != 0 {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestUnsupportedVersionAndMalformedHeaderDegradeDiscovery(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{header(testThreadID, "9.9.9", "legacy", "")})
	malformed := rolloutPath(home, "archived_sessions", secondThreadID)
	writeRollout(t, malformed, []string{`{"type":"future_header","payload":{}}`})
	listed := New().List(context.Background(), testSource(home))
	if listed.Status != contract.StatusPartial || len(listed.Sources) != 1 || len(listed.Omissions) != 2 {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestMalformedOnlyDiscoveryIsUnsupported(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{`{"type":"future_header","payload":{}}`})
	listed := New().List(context.Background(), testSource(home))
	if listed.Status != contract.StatusUnsupported || len(listed.Sources) != 0 || !hasOmission(listed.Omissions, "malformed_record") {
		t.Fatalf("List() = %#v", listed)
	}
	var output bytes.Buffer
	exit := (cli.Runner{Version: "test", Registry: provider.NewRegistry(New())}).Run(context.Background(), []string{"list", "--provider", "codex", "--root", home}, &output)
	if exit != cli.ExitUnsupported {
		t.Fatalf("list exit = %d, output = %s", exit, output.String())
	}
	var envelope contract.Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != contract.StatusUnsupported || envelope.Data != nil {
		t.Fatalf("list envelope = %#v", envelope)
	}
}

func TestOversizedRowDegradesEventsAndOversizedArtifactFails(t *testing.T) {
	home := t.TempDir()
	path := rolloutPath(home, "sessions", testThreadID)
	writeRollout(t, path, []string{
		header(testThreadID, "0.149.1", "legacy", ""),
		`{"type":"event_msg","payload":{"type":"user_message","message":"usable"}}`,
		`{"type":"future","payload":{"value":"` + strings.Repeat("x", maxRowBytes) + `"}}`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || !hasOmission(result.Omissions, "resource_limit") {
		t.Fatalf("Events() = %#v", result)
	}
	if err := os.Truncate(path, maxArtifactBytes+1); err != nil {
		t.Fatal(err)
	}
	listed := New().List(context.Background(), testSource(home))
	evidence := New().Evidence(context.Background(), testSource(home), listed.Sources[0].Identity.ProviderSourceFingerprint)
	if evidence.Err == nil {
		t.Fatalf("Evidence() = %#v", evidence)
	}
}

func TestAdapterRejectsProviderRootEscape(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	writeRollout(t, rolloutPath(outside, "sessions", testThreadID), []string{header(testThreadID, "0.149.1", "legacy", "")})
	if err := os.Symlink(filepath.Join(outside, "sessions"), filepath.Join(home, "sessions")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	listed := New().List(context.Background(), testSource(home))
	if listed.Err == nil {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestEventLimitProducesPartialResult(t *testing.T) {
	var data bytes.Buffer
	data.WriteString(header(testThreadID, "0.149.1", "legacy", "") + "\n")
	for i := 0; i <= maxNormalizedEvents; i++ {
		data.WriteString(`{"type":"event_msg","payload":{"type":"user_message","message":"x"}}` + "\n")
	}
	events, omissions := normalizeRows(testThreadID, "legacy", data.Bytes())
	if len(events) != maxNormalizedEvents || !hasOmission(omissions, "resource_limit") {
		t.Fatalf("events = %d, omissions = %#v", len(events), omissions)
	}
}

func TestCLIIntegrationPreservesProviderContentAndSemanticMetadata(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.149.1", "legacy", ""),
		`{"type":"event_msg","payload":{"type":"user_message","message":"hello"}}`,
	})
	before := snapshotTree(t, home)
	runner := cli.Runner{Version: "test", Registry: provider.NewRegistry(New())}
	listed := runCLI(t, runner, "list", "--provider", "codex", "--root", home)
	if listed.Status != contract.StatusComplete || listed.Data == nil || listed.Data.Sources == nil || len(*listed.Data.Sources) != 1 {
		t.Fatalf("list = %#v", listed)
	}
	ref := (*listed.Data.Sources)[0].Identity.SourceRef
	for _, args := range [][]string{{"show", "--root", home, ref}, {"events", "--root", home, ref}, {"verify", "--root", home, ref}} {
		first := runCLI(t, runner, args...)
		second := runCLI(t, runner, args...)
		if first.Status != contract.StatusComplete || !reflect.DeepEqual(first, second) {
			t.Fatalf("%v results differ or are incomplete: %#v %#v", args, first, second)
		}
	}
	after := snapshotTree(t, home)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("provider tree changed: before=%#v after=%#v", before, after)
	}
}

func TestCLIUsesMultipleConfiguredCodexHomes(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	writeRollout(t, rolloutPath(first, "sessions", testThreadID), []string{header(testThreadID, "0.149.1", "legacy", "")})
	writeRollout(t, rolloutPath(second, "sessions", secondThreadID), []string{header(secondThreadID, "0.149.1", "legacy", "")})
	configPath := filepath.Join(t.TempDir(), "config.json")
	configJSON := fmt.Sprintf(`{"schema_version":"v1","sources":[{"id":"codex-one","provider":"codex","root":%q},{"id":"codex-two","provider":"codex","root":%q}]}`, first, second)
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, cli.Runner{Version: "test", Registry: provider.NewRegistry(New())}, "list", "--provider", "codex", "--config", configPath)
	if result.Status != contract.StatusComplete || result.Data == nil || result.Data.Sources == nil || len(*result.Data.Sources) != 2 {
		t.Fatalf("list = %#v", result)
	}
}

func TestCLIResolvesEnvironmentAndStandardCodexHomes(t *testing.T) {
	for _, test := range []struct {
		name     string
		prepare  func(*testing.T, string) string
		listArgs []string
	}{
		{
			name: "CODEX_HOME",
			prepare: func(t *testing.T, parent string) string {
				home := filepath.Join(parent, "environment-codex")
				t.Setenv("CODEX_HOME", home)
				return home
			},
			listArgs: []string{"list", "--provider", "codex"},
		},
		{
			name: "standard home",
			prepare: func(t *testing.T, parent string) string {
				t.Setenv("CODEX_HOME", "")
				t.Setenv("HOME", parent)
				t.Setenv("USERPROFILE", parent)
				return filepath.Join(parent, ".codex")
			},
			listArgs: []string{"list", "--provider", "codex"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := test.prepare(t, t.TempDir())
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{header(testThreadID, "0.149.1", "legacy", "")})
			result := runCLI(t, cli.Runner{Version: "test", Registry: provider.NewRegistry(New())}, test.listArgs...)
			if result.Status != contract.StatusComplete || result.Data == nil || result.Data.Sources == nil || len(*result.Data.Sources) != 1 {
				t.Fatalf("list = %#v", result)
			}
		})
	}
}

func eventsForOnlySource(t *testing.T, home string) provider.EventResult {
	t.Helper()
	adapter := New()
	listed := adapter.List(context.Background(), testSource(home))
	if len(listed.Sources) != 1 {
		t.Fatalf("List() = %#v", listed)
	}
	return adapter.Events(context.Background(), testSource(home), listed.Sources[0].Identity.ProviderSourceFingerprint)
}

func testSource(home string) config.Source {
	return config.Source{ID: "codex-default", Provider: "codex", Root: home}
}

func rolloutPath(home, collection, threadID string) string {
	return filepath.Join(home, collection, "2026", "09", "03", "rollout-2026-09-03T10-00-00-"+threadID+".jsonl")
}

func header(threadID, version, historyMode, extra string) string {
	return fmt.Sprintf(`{"type":"session_meta","payload":{"session_id":%q,"id":%q,"cli_version":%q,"history_mode":%q%s}}`, threadID, threadID, version, historyMode, extra)
}

func hasOmission(omissions []contract.Omission, code string) bool {
	for _, item := range omissions {
		if item.Code == code {
			return true
		}
	}
	return false
}

func countOmissions(omissions []contract.Omission, code string) int {
	count := 0
	for _, item := range omissions {
		if item.Code == code {
			count++
		}
	}
	return count
}

func runCLI(t *testing.T, runner cli.Runner, args ...string) contract.Envelope {
	t.Helper()
	var output bytes.Buffer
	exit := runner.Run(context.Background(), args, &output)
	if exit != cli.ExitOK {
		t.Fatalf("Run(%v) exit = %d, output = %s", args, exit, output.String())
	}
	var envelope contract.Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope
}

type treeEntry struct {
	Mode    os.FileMode
	Size    int64
	ModTime int64
	Digest  string
}

func snapshotTree(t *testing.T, root string) map[string]treeEntry {
	t.Helper()
	result := map[string]treeEntry{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := treeEntry{Mode: info.Mode(), Size: info.Size(), ModTime: info.ModTime().UnixNano()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			entry.Digest = hex.EncodeToString(sum[:])
		}
		result[relative] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
