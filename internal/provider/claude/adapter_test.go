package claude

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

const testSessionID = "11111111-2222-4333-8444-555555555555"

func TestAdapterDiscoversAndNormalizesClaudeTranscript(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{
		row("user", `{"role":"user","content":"hello"}`, ""),
		row("assistant", `{"role":"assistant","content":[{"type":"text","text":"hi"},{"type":"tool_use","id":"provider-call","name":"Bash","input":{"command":"must not escape"}}]}`, ""),
		row("user", `{"role":"user","content":[{"type":"tool_result","tool_use_id":"provider-call","content":"private output","is_error":false}]}`, ""),
		row("assistant", `{"role":"assistant","content":[{"type":"text","text":"fictional provider failure"}],"stop_reason":"error"}`, `,"isApiErrorMessage":true`),
	})

	adapter := New()
	source := testSource(home)
	listed := adapter.List(context.Background(), source)
	if listed.Status != contract.StatusComplete || len(listed.Sources) != 1 {
		t.Fatalf("List() = %#v", listed)
	}
	got := listed.Sources[0]
	if got.Identity.ProviderNativeSourceID != testSessionID || got.Identity.SourceRef != contract.NewSourceRef("claude", "claude-default", testSessionID) {
		t.Fatalf("identity = %#v", got.Identity)
	}
	events := adapter.Events(context.Background(), source, got.Identity.ProviderSourceFingerprint)
	if events.Status != contract.StatusComplete || len(events.Events) != 5 {
		t.Fatalf("Events() = %#v", events)
	}
	if events.Events[0].Message == nil || events.Events[0].Message.Role != "user" || events.Events[1].Message == nil || events.Events[1].Message.Role != "assistant" {
		t.Fatalf("messages = %#v", events.Events[:2])
	}
	call, result := events.Events[2].ToolCall, events.Events[3].ToolResult
	if call == nil || result == nil || call.CallID != result.CallID || call.CallID == "provider-call" || call.Category != "shell" || !result.Success {
		t.Fatalf("tool pair = %#v", events.Events[2:4])
	}
	if events.Events[4].Error == nil || events.Events[4].Error.Category != "provider" {
		t.Fatalf("error = %#v", events.Events[4])
	}
	encoded, err := json.Marshal(events.Events)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"provider-call", "must not escape", "private output", "Bash"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("normalized events exposed %q", forbidden)
		}
	}
	evidence := adapter.Evidence(context.Background(), source, got.Identity.ProviderSourceFingerprint)
	if evidence.Status != contract.StatusComplete || len(evidence.Chunks) != 1 || evidence.Chunks[0].Name != "transcript/primary.jsonl" {
		t.Fatalf("Evidence() = %#v", evidence)
	}
}

func TestObservedClaudeVersionIsSupported(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{
		rowVersion("2.1.177", "user", `{"role":"user","content":"hello"}`, ""),
		rowVersion("2.1.177", "assistant", `{"role":"assistant","content":[{"type":"text","text":"hi"},{"type":"tool_use","id":"observed-call","name":"Read","input":{}}]}`, ""),
		rowVersion("2.1.177", "user", `{"role":"user","content":[{"type":"tool_result","tool_use_id":"observed-call","content":"fictional","is_error":true}]}`, ""),
		rowVersion("2.1.177", "assistant", `{"role":"assistant","content":[{"type":"text","text":"fictional provider failure"}],"stop_reason":"error"}`, `,"isApiErrorMessage":true`),
	})

	adapter := New()
	listed := adapter.List(context.Background(), testSource(home))
	if listed.Status != contract.StatusComplete || len(listed.Sources) != 1 {
		t.Fatalf("List() = %#v", listed)
	}
	events := adapter.Events(context.Background(), testSource(home), listed.Sources[0].Identity.ProviderSourceFingerprint)
	if events.Status != contract.StatusComplete || len(events.Events) != 5 || events.Events[2].ToolCall == nil || events.Events[3].ToolResult == nil || events.Events[3].ToolResult.Success || events.Events[4].Error == nil {
		t.Fatalf("Events() = %#v", events)
	}
}

func TestContentAndCorrelationFailuresDegradeCompleteness(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{
		row("user", `{"role":"user","content":[{"type":"text","text":"usable"},{"type":"image","source":{"data":"fictional"}}]}`, ""),
		row("assistant", `{"role":"assistant","content":[{"type":"tool_use","id":"same","name":"Read","input":{"file_path":"/fictional/private"}},{"type":"tool_use","id":"same","name":"Read","input":{}}]}`, ""),
		row("user", `{"role":"user","content":[{"type":"tool_result","tool_use_id":"missing","content":"x"}]}`, ""),
		row("assistant", `{"role":"assistant","content":[{"type":"future_block"}]}`, ""),
		`{"type":"future_row","sessionId":"` + testSessionID + `","version":"2.1.228"}`,
		`{broken`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 2 || result.Events[0].Message == nil || result.Events[1].ToolCall == nil {
		t.Fatalf("Events() = %#v", result)
	}
	for _, code := range []string{"unsupported_content", "duplicate_call_id", "correlation_omitted", "unknown_format", "malformed_record"} {
		if !hasOmission(result.Omissions, code) {
			t.Fatalf("missing %q in %#v", code, result.Omissions)
		}
	}
}

func TestToolFailureAndDuplicateResultUseExplicitProviderSemantics(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{
		row("assistant", `{"role":"assistant","content":[{"type":"tool_use","id":"call-failure","name":"Edit","input":{}}]}`, ""),
		row("user", `{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-failure","content":"fictional","is_error":true},{"type":"tool_result","tool_use_id":"call-failure","content":"duplicate","is_error":false}]}`, ""),
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 2 || result.Events[1].ToolResult == nil || result.Events[1].ToolResult.Success || !hasOmission(result.Omissions, "duplicate_result") {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestNullToolErrorMarkerIsNotGuessedAsSuccess(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{
		row("assistant", `{"role":"assistant","content":[{"type":"tool_use","id":"call-null","name":"Read","input":{}}]}`, ""),
		row("user", `{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-null","content":"fictional","is_error":null}]}`, ""),
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || result.Events[0].ToolCall == nil || !hasOmission(result.Omissions, "malformed_record") {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestUnsupportedVersionAndIdentityMismatchAreNotDecoded(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "unsupported", testSessionID, []string{
		strings.Replace(row("user", `{"role":"user","content":"not decoded"}`, ""), `"2.1.228"`, `"9.9.9"`, 1),
	})
	writeTranscript(t, home, "mismatch", "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", []string{
		row("user", `{"role":"user","content":"not discovered"}`, ""),
	})
	listed := New().List(context.Background(), testSource(home))
	if listed.Status != contract.StatusPartial || len(listed.Sources) != 1 || !hasOmission(listed.Omissions, "unsupported_format") {
		t.Fatalf("List() = %#v", listed)
	}
	result := New().Events(context.Background(), testSource(home), listed.Sources[0].Identity.ProviderSourceFingerprint)
	if result.Status != contract.StatusUnsupported || len(result.Events) != 0 {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestDiscoveryDoesNotTakeIdentityFromUnknownOrMalformedRows(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first string
	}{
		{name: "unknown", first: `{"type":"future_row","sessionId":"` + testSessionID + `","version":"2.1.228"}`},
		{name: "malformed", first: `{broken`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeTranscript(t, home, "fictional-project", testSessionID, []string{tc.first})
			listed := New().List(context.Background(), testSource(home))
			if listed.Status != contract.StatusUnsupported || len(listed.Sources) != 0 || !hasOmission(listed.Omissions, "malformed_record") {
				t.Fatalf("List() = %#v", listed)
			}
		})
	}
}

func TestDeepIdentityRowIsRejected(t *testing.T) {
	home := t.TempDir()
	deep := strings.Repeat(`{"x":`, contract.MaxJSONDepth) + `0` + strings.Repeat(`}`, contract.MaxJSONDepth)
	writeTranscript(t, home, "fictional-project", testSessionID, []string{
		row("user", `{"role":"user","content":"not decoded"}`, `,"deep":`+deep),
	})

	listed := New().List(context.Background(), testSource(home))
	if listed.Status != contract.StatusUnsupported || len(listed.Sources) != 0 || !hasOmission(listed.Omissions, "malformed_record") {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestDeepEventRowDegradesCompleteness(t *testing.T) {
	home := t.TempDir()
	deep := strings.Repeat(`{"x":`, contract.MaxJSONDepth) + `0` + strings.Repeat(`}`, contract.MaxJSONDepth)
	writeTranscript(t, home, "fictional-project", testSessionID, []string{
		row("user", `{"role":"user","content":"usable"}`, ""),
		row("assistant", `{"role":"assistant","content":"not decoded"}`, `,"deep":`+deep),
	})

	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || !hasOmission(result.Omissions, "malformed_record") {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestBookkeepingRowsCannotHideIdentityOrVersionMismatch(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{
		row("user", `{"role":"user","content":"usable"}`, ""),
		`{"type":"last-prompt","sessionId":"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee","version":"9.9.9"}`,
	})
	result := eventsForOnlySource(t, home)
	if result.Status != contract.StatusPartial || len(result.Events) != 1 || !hasOmission(result.Omissions, "malformed_record") || !hasOmission(result.Omissions, "unsupported_format") {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestDuplicateLocatorsAreAmbiguousAndIdentityIsPathIndependent(t *testing.T) {
	home := t.TempDir()
	lines := []string{row("user", `{"role":"user","content":"x"}`, "")}
	writeTranscript(t, home, "one", testSessionID, lines)
	writeTranscript(t, home, "two", testSessionID, lines)
	adapter := New()
	listed := adapter.List(context.Background(), testSource(home))
	if listed.Status != contract.StatusPartial || len(listed.Sources) != 1 || !hasOmission(listed.Omissions, "ambiguous_artifact") {
		t.Fatalf("List() = %#v", listed)
	}
	result := adapter.Events(context.Background(), testSource(home), listed.Sources[0].Identity.ProviderSourceFingerprint)
	if result.Status != contract.StatusUnsupported || len(result.Events) != 0 {
		t.Fatalf("Events() = %#v", result)
	}
}

func TestSubagentArtifactsRemainExplicitlyUnsupported(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{row("user", `{"role":"user","content":"main"}`, "")})
	subagents := filepath.Join(home, "projects", "fictional-project", testSessionID, "subagents")
	if err := os.MkdirAll(subagents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subagents, "agent-fictional.jsonl"), []byte(row("assistant", `{"role":"assistant","content":[{"type":"text","text":"subagent"}]}`, "")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	listed := New().List(context.Background(), testSource(home))
	if listed.Status != contract.StatusPartial || len(listed.Sources) != 1 || !hasOmission(listed.Omissions, "missing_sidecar") || !hasOmission(listed.Omissions, "unsupported_relationship") {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestSubagentOmissionsFollowOnlyTheirMainSource(t *testing.T) {
	home := t.TempDir()
	otherSessionID := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	writeTranscript(t, home, "affected", testSessionID, []string{row("user", `{"role":"user","content":"affected"}`, "")})
	writeTranscript(t, home, "unaffected", otherSessionID, []string{
		strings.Replace(row("user", `{"role":"user","content":"unaffected"}`, ""), testSessionID, otherSessionID, 1),
	})
	subagents := filepath.Join(home, "projects", "affected", testSessionID, "subagents")
	if err := os.MkdirAll(subagents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subagents, "agent-fictional.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	adapter := New()
	listed := adapter.List(context.Background(), testSource(home))
	if listed.Status != contract.StatusPartial || len(listed.Sources) != 2 {
		t.Fatalf("List() = %#v", listed)
	}

	for _, source := range listed.Sources {
		fingerprint := source.Identity.ProviderSourceFingerprint
		show := adapter.Show(context.Background(), testSource(home), fingerprint)
		events := adapter.Events(context.Background(), testSource(home), fingerprint)
		evidence := adapter.Evidence(context.Background(), testSource(home), fingerprint)
		if source.Identity.ProviderNativeSourceID == testSessionID {
			for operation, status := range map[string]contract.Status{"show": show.Status, "events": events.Status, "verify": evidence.Status} {
				if status != contract.StatusPartial {
					t.Fatalf("affected %s status = %s", operation, status)
				}
			}
			for operation, omissions := range map[string][]contract.Omission{"show": show.Omissions, "events": events.Omissions, "verify": evidence.Omissions} {
				if !hasOmission(omissions, "unsupported_relationship") || !hasOmission(omissions, "missing_sidecar") {
					t.Fatalf("affected %s omissions = %#v", operation, omissions)
				}
			}
			continue
		}
		if show.Status != contract.StatusComplete || events.Status != contract.StatusComplete || evidence.Status != contract.StatusComplete {
			t.Fatalf("unaffected operations = %#v %#v %#v", show, events, evidence)
		}
	}
}

func TestMalformedAndOversizedSidecarsAreReported(t *testing.T) {
	deep := []byte(strings.Repeat(`{"x":`, contract.MaxJSONDepth+1) + `0` + strings.Repeat(`}`, contract.MaxJSONDepth+1))
	for _, tc := range []struct {
		name string
		data []byte
		code string
	}{
		{name: "malformed", data: []byte(`{broken`), code: "malformed_record"},
		{name: "deep", data: deep, code: "malformed_record"},
		{name: "oversized", data: bytes.Repeat([]byte("x"), maxSidecarBytes+1), code: "resource_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			writeTranscript(t, home, "fictional-project", testSessionID, []string{row("user", `{"role":"user","content":"main"}`, "")})
			subagents := filepath.Join(home, "projects", "fictional-project", testSessionID, "subagents")
			if err := os.MkdirAll(subagents, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(subagents, "agent-fictional.jsonl"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(subagents, "agent-fictional.meta.json"), tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			listed := New().List(context.Background(), testSource(home))
			if !hasOmission(listed.Omissions, tc.code) {
				t.Fatalf("List() = %#v", listed)
			}
		})
	}
}

func TestDuplicateSubagentCorrelationIsAmbiguous(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{row("user", `{"role":"user","content":"main"}`, "")})
	subagents := filepath.Join(home, "projects", "fictional-project", testSessionID, "subagents")
	if err := os.MkdirAll(subagents, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(subagents, "agent-"+id+".jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subagents, "agent-"+id+".meta.json"), []byte(`{"toolUseId":"same-parent-call","parentAgentId":"fictional-parent","agentType":"fictional"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	listed := New().List(context.Background(), testSource(home))
	if !hasOmission(listed.Omissions, "ambiguous_relationship") {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestOversizedRowAndArtifactAreBounded(t *testing.T) {
	home := t.TempDir()
	path := writeTranscript(t, home, "fictional-project", testSessionID, []string{
		row("user", `{"role":"user","content":"usable"}`, ""),
		`{"type":"future","value":"` + strings.Repeat("x", maxRowBytes) + `"}`,
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

func TestExcessiveDiscoveredFilesFailTheOperation(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{row("user", `{"role":"user","content":"x"}`, "")})
	path := filepath.Join(home, "projects", "fictional-project", "unrelated.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if listed := (&Adapter{maxFiles: 1}).List(context.Background(), testSource(home)); listed.Err == nil {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestProviderRootSymlinkIsRejected(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	writeTranscript(t, outside, "fictional-project", testSessionID, []string{row("user", `{"role":"user","content":"outside"}`, "")})
	if err := os.Symlink(filepath.Join(outside, "projects"), filepath.Join(home, "projects")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if listed := New().List(context.Background(), testSource(home)); listed.Err == nil {
		t.Fatalf("List() = %#v", listed)
	}
}

func TestEventLimitProducesPartialResult(t *testing.T) {
	var data strings.Builder
	for i := 0; i <= maxNormalizedEvents; i++ {
		data.WriteString(row("user", `{"role":"user","content":"x"}`, ""))
		data.WriteByte('\n')
	}
	events, omissions := normalizeRows(testSessionID, []byte(data.String()))
	if len(events) != maxNormalizedEvents || !hasOmission(omissions, "resource_limit") {
		t.Fatalf("events = %d, omissions = %#v", len(events), omissions)
	}
}

func TestProviderTreeIsUnchangedByReadOnlyCLI(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{row("user", `{"role":"user","content":"hello"}`, "")})
	before := snapshotTree(t, home)
	runner := cli.Runner{Version: "test", Registry: provider.NewRegistry(New())}
	listed := runCLI(t, runner, "list", "--provider", "claude", "--root", home)
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
	if after := snapshotTree(t, home); !reflect.DeepEqual(before, after) {
		t.Fatalf("provider tree changed: before=%#v after=%#v", before, after)
	}
}

func TestCLIOutputRedactsProviderText(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, "fictional-project", testSessionID, []string{
		row("user", `{"role":"user","content":"token=sk-fictional-secret /fictional/private"}`, ""),
	})
	runner := cli.Runner{Version: "test", Registry: provider.NewRegistry(New())}
	listed := runCLI(t, runner, "list", "--provider", "claude", "--root", home)
	ref := (*listed.Data.Sources)[0].Identity.SourceRef
	var output bytes.Buffer
	if exit := runner.Run(context.Background(), []string{"events", "--root", home, ref}, &output); exit != cli.ExitOK {
		t.Fatalf("events exit = %d, output = %s", exit, output.String())
	}
	for _, forbidden := range []string{"sk-fictional-secret", "/fictional/private"} {
		if bytes.Contains(output.Bytes(), []byte(forbidden)) {
			t.Fatalf("CLI output exposed %q: %s", forbidden, output.String())
		}
	}
}

func TestDefaults(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/fictional/claude")
	if root, ok := New().EnvironmentRoot(); !ok || root != "/fictional/claude" {
		t.Fatalf("EnvironmentRoot() = %q, %v", root, ok)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if root, ok := New().DefaultRoot(); !ok || root != filepath.Join(home, ".claude") {
		t.Fatalf("DefaultRoot() = %q, %v", root, ok)
	}
}

func row(kind, message, extra string) string {
	return rowVersion("2.1.228", kind, message, extra)
}

func rowVersion(version, kind, message, extra string) string {
	return `{"type":"` + kind + `","sessionId":"` + testSessionID + `","uuid":"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee","version":"` + version + `","message":` + message + extra + `}`
}

func writeTranscript(t *testing.T, root, project, filenameID string, lines []string) string {
	t.Helper()
	path := filepath.Join(root, "projects", project, filenameID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testSource(root string) config.Source {
	return config.Source{ID: "claude-default", Provider: "claude", Root: root}
}

func eventsForOnlySource(t *testing.T, root string) provider.EventResult {
	t.Helper()
	adapter := New()
	listed := adapter.List(context.Background(), testSource(root))
	if len(listed.Sources) != 1 {
		t.Fatalf("List() = %#v", listed)
	}
	return adapter.Events(context.Background(), testSource(root), listed.Sources[0].Identity.ProviderSourceFingerprint)
}

func hasOmission(omissions []contract.Omission, code string) bool {
	for _, item := range omissions {
		if item.Code == code {
			return true
		}
	}
	return false
}

type treeEntry struct {
	Mode    os.FileMode
	Size    int64
	ModTime int64
	Hash    string
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
			entry.Hash = hex.EncodeToString(sum[:])
		}
		result[relative] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func runCLI(t *testing.T, runner cli.Runner, args ...string) contract.Envelope {
	t.Helper()
	var output bytes.Buffer
	if exit := runner.Run(context.Background(), args, &output); exit != cli.ExitOK {
		t.Fatalf("%v exit = %d, output = %s", args, exit, output.String())
	}
	var envelope contract.Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope
}
