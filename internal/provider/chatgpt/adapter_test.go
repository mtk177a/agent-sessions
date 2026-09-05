package chatgpt

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/config"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/provider"
)

const twoConversations = `[
  {
    "id":"conversation-a","conversation_id":"conversation-a","current_node":"assistant-a",
    "mapping":{
      "root":{"id":"root","parent":null,"message":null},
      "user-a":{"id":"user-a","parent":"root","message":{"id":"message-user","author":{"role":"user"},"content":{"content_type":"text","parts":["token=sk-fictional-secret"]}}},
      "assistant-a":{"id":"assistant-a","parent":"user-a","message":{"id":"message-assistant","author":{"role":"assistant"},"content":{"content_type":"text","parts":["safe answer"]}}},
      "branch-a":{"id":"branch-a","parent":"user-a","message":{"id":"message-branch","author":{"role":"assistant"},"content":{"content_type":"text","parts":["inactive answer"]}}}
    }
  },
  {
    "id":"conversation-b","conversation_id":"conversation-b","current_node":"tool-b",
    "mapping":{
      "root":{"id":"root","parent":null,"message":null},
      "user-b":{"id":"user-b","parent":"root","message":{"id":"message-b","author":{"role":"user"},"content":{"content_type":"text","parts":["run it"]}}},
      "tool-b":{"id":"tool-b","parent":"user-b","message":{"id":"tool-message","author":{"role":"tool"},"content":{"content_type":"text","parts":["tool output"]}}}
    }
  }
]`

func TestDiscoverySeparatesArchiveSnapshotFromConversationIdentity(t *testing.T) {
	path := writeZIP(t, []zipMember{{"conversations.json", twoConversations}})
	source := config.Source{ID: "chatgpt-personal", Provider: providerName, Root: path}
	result := New().List(t.Context(), source)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Status != contract.StatusComplete || len(result.Sources) != 2 {
		t.Fatalf("unexpected discovery: %#v", result)
	}
	if result.Sources[0].Identity.ProviderNativeSourceID == result.Sources[1].Identity.ProviderNativeSourceID {
		t.Fatal("conversation identities collapsed")
	}
	if result.Sources[0].VersionHint == nil || result.Sources[1].VersionHint == nil || result.Sources[0].VersionHint.Value != result.Sources[1].VersionHint.Value {
		t.Fatalf("archive snapshot identities differ: %#v", result.Sources)
	}
	if result.Sources[0].Identity.ProviderSourceFingerprint == result.Sources[0].VersionHint.Value {
		t.Fatal("conversation and archive identities were conflated")
	}
	changedPath := writeZIP(t, []zipMember{{"conversations.json", twoConversations}, {"chat.html", "different snapshot"}})
	changed := New().List(t.Context(), config.Source{ID: "chatgpt-personal", Provider: providerName, Root: changedPath})
	if changed.Err != nil || changed.Sources[0].Identity.ProviderSourceFingerprint != result.Sources[0].Identity.ProviderSourceFingerprint || changed.Sources[0].VersionHint.Value == result.Sources[0].VersionHint.Value {
		t.Fatalf("snapshot change altered conversation identity or preserved archive identity: %#v", changed)
	}
}

func TestVerificationStreamsTheWholeArchiveWithCommonVersionSemantics(t *testing.T) {
	archivePath := writeZIP(t, []zipMember{{"conversations.json", twoConversations}, {"chat.html", "synthetic companion"}})
	result := New().Evidence(t.Context(), config.Source{ID: "export", Provider: providerName, Root: archivePath}, contract.SourceFingerprint("conversation-a"))
	if result.Err != nil || result.VerifiedVersion == nil || len(result.Chunks) != 0 {
		t.Fatalf("evidence = %#v", result)
	}
	content, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	want, err := contract.VerifiedVersion([]contract.EvidenceChunk{{Name: "archive/export.zip", Content: content}})
	if err != nil {
		t.Fatal(err)
	}
	if *result.VerifiedVersion != want {
		t.Fatalf("streamed version = %#v, want %#v", *result.VerifiedVersion, want)
	}
}

func TestDiscoverySupportsNumberedConversationMembers(t *testing.T) {
	first := `[{"id":"first","conversation_id":"first","current_node":"root","mapping":{"root":{"parent":null,"message":null}}}]`
	second := `[{"id":"second","conversation_id":"second","current_node":"root","mapping":{"root":{"parent":null,"message":null}}}]`
	path := writeZIP(t, []zipMember{{"nested/conversations-2.json", second}, {"conversations-1.json", first}})
	result := New().List(t.Context(), config.Source{ID: "export", Provider: providerName, Root: path})
	if result.Err != nil || result.Status != contract.StatusComplete || len(result.Sources) != 2 {
		t.Fatalf("numbered discovery = %#v", result)
	}
}

func TestEventsFollowActiveBranchAndDescribeOmissions(t *testing.T) {
	path := writeZIP(t, []zipMember{{"conversations.json", twoConversations}})
	source := config.Source{ID: "chatgpt-personal", Provider: providerName, Root: path}
	adapter := New()
	ref := contract.SourceFingerprint("conversation-a")
	result := adapter.Events(t.Context(), source, ref)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Status != contract.StatusPartial || len(result.Events) != 2 {
		t.Fatalf("unexpected events: %#v", result)
	}
	if result.Events[0].Message.Role != "user" || result.Events[1].Message.Role != "assistant" || result.Events[1].Message.Text != "safe answer" {
		t.Fatalf("active branch order is wrong: %#v", result.Events)
	}
	if !hasOmission(result.Omissions, "non_active_branch") {
		t.Fatalf("non-active branch omission missing: %#v", result.Omissions)
	}

	tool := adapter.Events(t.Context(), source, contract.SourceFingerprint("conversation-b"))
	if tool.Status != contract.StatusPartial || len(tool.Events) != 1 || !hasOmission(tool.Omissions, "correlation_omitted") {
		t.Fatalf("tool result handling is not explicit: %#v", tool)
	}
}

func TestMissingNodeAndCycleAreUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"missing", `[ {"id":"c","conversation_id":"c","current_node":"missing","mapping":{}} ]`},
		{"cycle", `[ {"id":"c","conversation_id":"c","current_node":"a","mapping":{"a":{"parent":"b","message":null},"b":{"parent":"a","message":null}}} ]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeZIP(t, []zipMember{{"conversations.json", tc.body}})
			result := New().Events(t.Context(), config.Source{ID: "export", Provider: providerName, Root: path}, contract.SourceFingerprint("c"))
			if result.Err != nil || result.Status != contract.StatusUnsupported || !hasOmission(result.Omissions, "unsupported_format") {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestArchiveAndJSONFailuresAreDistinct(t *testing.T) {
	dir := t.TempDir()
	badZIP := filepath.Join(dir, "bad.zip")
	if err := os.WriteFile(badZIP, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	badJSON := writeZIP(t, []zipMember{{"conversations.json", `[{`}})
	duplicate := writeZIP(t, []zipMember{{"conversations.json", `[]`}, {"conversations.json", `[]`}})
	unsafe := writeZIP(t, []zipMember{{"../conversations.json", `[]`}})
	windowsUnsafe := writeZIP(t, []zipMember{{`folder\conversations.json`, `[]`}})

	for _, tc := range []struct {
		name string
		path string
		want error
	}{
		{"malformed_zip", badZIP, provider.ErrInvalidArchive},
		{"malformed_json", badJSON, provider.ErrInvalidJSON},
		{"duplicate", duplicate, provider.ErrDuplicateArchiveMember},
		{"unsafe", unsafe, provider.ErrUnsafeArchiveMember},
		{"windows_unsafe", windowsUnsafe, provider.ErrUnsafeArchiveMember},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := New().List(t.Context(), config.Source{ID: "export", Provider: providerName, Root: tc.path})
			if !errors.Is(result.Err, tc.want) {
				t.Fatalf("error = %v, want %v", result.Err, tc.want)
			}
		})
	}
}

func TestUnsupportedInputAndResourceLimitAreDistinct(t *testing.T) {
	unsupported := writeZIP(t, []zipMember{{"chat.html", "synthetic"}})
	result := New().List(t.Context(), config.Source{ID: "export", Provider: providerName, Root: unsupported})
	if result.Err != nil || result.Status != contract.StatusUnsupported {
		t.Fatalf("unsupported input = %#v", result)
	}

	limited := New()
	limited.limits.maxConversationMemberBytes = 4
	over := writeZIP(t, []zipMember{{"conversations.json", `[]   `}})
	result = limited.List(t.Context(), config.Source{ID: "export", Provider: providerName, Root: over})
	if !errors.Is(result.Err, provider.ErrResourceLimit) {
		t.Fatalf("resource error = %v", result.Err)
	}

	ratioLimited := New()
	ratioLimited.limits.maxCompressionRatio = 1
	compressed := writeZIP(t, []zipMember{{"conversations.json", strings.Repeat(" ", 1024) + `[]`}})
	result = ratioLimited.List(t.Context(), config.Source{ID: "export", Provider: providerName, Root: compressed})
	if !errors.Is(result.Err, provider.ErrResourceLimit) {
		t.Fatalf("compression-ratio error = %v", result.Err)
	}
}

func TestActiveBranchResourceLimitIsAnError(t *testing.T) {
	body := `[{"id":"c","conversation_id":"c","current_node":"b","mapping":{"a":{"parent":null,"message":null},"b":{"parent":"a","message":null}}}]`
	path := writeZIP(t, []zipMember{{"conversations.json", body}})
	limited := New()
	limited.limits.maxEvents = 1
	result := limited.Events(t.Context(), config.Source{ID: "export", Provider: providerName, Root: path}, contract.SourceFingerprint("c"))
	if !errors.Is(result.Err, provider.ErrResourceLimit) {
		t.Fatalf("active-branch resource error = %v", result.Err)
	}
}

func TestUnknownActiveNodeIsIncomplete(t *testing.T) {
	body := `[ {"id":"c","conversation_id":"c","current_node":"m","mapping":{"m":{"parent":null,"message":{"id":"m","author":{"role":"future-role"},"content":{"content_type":"future","parts":[]}}}}} ]`
	path := writeZIP(t, []zipMember{{"conversations.json", body}})
	result := New().Events(t.Context(), config.Source{ID: "export", Provider: providerName, Root: path}, contract.SourceFingerprint("c"))
	if result.Err != nil || result.Status != contract.StatusUnsupported || !hasOmission(result.Omissions, "unknown_node") {
		t.Fatalf("unknown node result = %#v", result)
	}
}

func TestAdapterDoesNotWriteOutsideArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.zip")
	writeZIPAt(t, path, []zipMember{{"conversations.json", twoConversations}})
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = New().List(t.Context(), config.Source{ID: "export", Provider: providerName, Root: path})
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) || before[0].Name() != after[0].Name() {
		t.Fatalf("adapter wrote alongside input: before=%v after=%v", before, after)
	}
}

func hasOmission(values []contract.Omission, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}

type zipMember struct {
	name string
	body string
}

func writeZIP(t *testing.T, members []zipMember) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.zip")
	writeZIPAt(t, path, members)
	return path
}

func writeZIPAt(t *testing.T, path string, members []zipMember) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, member := range members {
		entry, err := writer.Create(member.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(member.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSyntheticFixturesDoNotContainMachinePaths(t *testing.T) {
	if strings.Contains(twoConversations, string(filepath.Separator)+"Users"+string(filepath.Separator)) {
		t.Fatal("synthetic fixture contains a machine path")
	}
}
