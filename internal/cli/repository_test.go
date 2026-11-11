package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/cli"
	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/provider"
	"github.com/mtk177a/agent-sessions/internal/provider/claude"
	"github.com/mtk177a/agent-sessions/internal/provider/codex"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Fictional", "GIT_AUTHOR_EMAIL=fictional@example.invalid", "GIT_COMMITTER_NAME=Fictional", "GIT_COMMITTER_EMAIL=fictional@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}
func repo(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, path, "init", "-q")
	git(t, path, "commit", "--allow-empty", "-qm", "Synthetic initial commit")
	return path
}
func write(t *testing.T, path string, rows ...any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	for _, r := range rows {
		if err := json.NewEncoder(&b).Encode(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}
func run(t *testing.T, args ...string) contract.Envelope {
	t.Helper()
	var out bytes.Buffer
	r := cli.Runner{Version: "test", Registry: provider.NewRegistry(codex.New(), claude.New())}
	code := r.Run(t.Context(), args, &out)
	var e contract.Envelope
	if err := json.Unmarshal(out.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit=%d: %s", code, out.String())
	}
	return e
}
func codexSession(t *testing.T, home string, n int, paths ...string) string {
	t.Helper()
	id := fmt.Sprintf("11111111-2222-4333-8444-%012d", n)
	rows := []any{map[string]any{"type": "session_meta", "payload": map[string]any{"id": id, "cli_version": "0.153.0", "history_mode": "legacy", "cwd": paths[0]}}}
	for _, p := range paths[1:] {
		rows = append(rows, map[string]any{"type": "turn_context", "payload": map[string]any{"cwd": p}})
	}
	rows = append(rows, map[string]any{"type": "event_msg", "timestamp": "2026-09-03T10:00:00Z", "payload": map[string]any{"type": "user_message", "message": "Independent synthetic conversation"}})
	write(t, filepath.Join(home, "sessions", "rollout-2026-09-03T10-00-00-"+id+".jsonl"), rows...)
	return contract.NewSourceRef("codex", "codex-default", id)
}
func TestRepositorySearchAcrossWorktrees(t *testing.T) {
	base := t.TempDir()
	target := repo(t, filepath.Join(base, "target"))
	other := repo(t, filepath.Join(base, "clone"))
	nested := repo(t, filepath.Join(target, "nested"))
	work := filepath.Join(base, "linked worktree")
	git(t, target, "worktree", "add", "-q", "--detach", work)
	git(t, target, "remote", "add", "origin", "https://example.invalid/fictional.git")
	git(t, other, "remote", "add", "origin", "https://example.invalid/fictional.git")
	sub := filepath.Join(target, "sub")
	os.MkdirAll(sub, 0700)
	home := filepath.Join(base, "records")
	want := map[string]bool{codexSession(t, home, 1, target): true, codexSession(t, home, 2, sub): true, codexSession(t, home, 3, other, work, target): true}
	codexSession(t, home, 4, other)
	codexSession(t, home, 5, nested)
	seen := map[string]bool{}
	cursor := ""
	for {
		args := []string{"list", "--provider", "codex", "--root", home, "--repository", target, "--limit", "1"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		e := run(t, args...)
		for _, s := range *e.Data.Sources {
			ref := s.Identity.SourceRef
			if !want[ref] || seen[ref] {
				t.Fatalf("unexpected/duplicate %s", ref)
			}
			seen[ref] = true
		}
		if !e.Page.HasMore {
			break
		}
		cursor = e.Page.NextCursor
	}
	if len(seen) != len(want) {
		t.Fatalf("seen=%v", seen)
	}
}
func TestRepositoryUnknownDoesNotMeanNoMatches(t *testing.T) {
	base := t.TempDir()
	target := repo(t, filepath.Join(base, "target"))
	home := filepath.Join(base, "records")
	codexSession(t, home, 1, filepath.Join(base, "deleted"))
	e := run(t, "list", "--provider", "codex", "--root", home, "--repository", target)
	if e.Status != contract.StatusPartial || len(*e.Data.Sources) != 0 {
		t.Fatalf("%#v", e)
	}
	found := false
	for _, o := range e.Omissions {
		found = found || o.Code == "repository_membership_unknown" && o.Scope == "repository" && o.Count == 1
	}
	if !found {
		t.Fatal(e.Omissions)
	}
}

func TestRepositorySearchUsesCodexCommandDirectoryURI(t *testing.T) {
	base := t.TempDir()
	target := repo(t, filepath.Join(base, "target space #%"))
	other := repo(t, filepath.Join(base, "other"))
	home := filepath.Join(base, "records")
	id := "11111111-2222-4333-8444-000000000001"
	uriPath := filepath.ToSlash(target)
	if filepath.VolumeName(target) != "" {
		uriPath = "/" + uriPath
	}
	cwd := (&url.URL{Scheme: "file", Path: uriPath}).String()
	write(t, filepath.Join(home, "sessions", "rollout-2026-09-03T10-00-00-"+id+".jsonl"),
		map[string]any{"ordinal": 0, "timestamp": "2026-09-03T10:00:00Z", "type": "session_meta",
			"payload": map[string]any{
				"id": id, "session_id": id, "cli_version": "0.155.0-alpha.9.2", "history_mode": "paginated",
				"timestamp": "2026-09-03T10:00:00Z", "cwd": other, "originator": "codex_cli_rs", "source": "cli",
			}},
		map[string]any{"ordinal": 1, "timestamp": "2026-09-03T10:00:01Z", "type": "event_msg",
			"payload": map[string]any{"type": "item_completed", "item": map[string]any{
				"type": "UserMessage", "id": "fictional-message", "content": []any{},
			}}},
		map[string]any{"ordinal": 2, "timestamp": "2026-09-03T10:00:02Z", "type": "event_msg",
			"payload": map[string]any{"type": "item_completed", "item": map[string]any{
				"type": "CommandExecution", "id": "fictional-command", "cwd": cwd,
				"command": []string{"git", "status"}, "parsed_cmd": []any{}, "source": "agent", "status": "completed", "exit_code": 0,
			}}},
	)

	e := run(t, "list", "--provider", "codex", "--root", home, "--repository", target)
	sources := *e.Data.Sources
	ref := contract.NewSourceRef("codex", "codex-default", id)
	if e.Status != contract.StatusComplete || len(sources) != 1 || sources[0].Identity.SourceRef != ref {
		t.Fatalf("expected a match from the command directory, got %d sources and omissions %v", len(sources), e.Omissions)
	}
	for _, location := range sources[0].WorkingDirectories.Locations {
		if location.Evidence == "tool" && location.Path == target &&
			location.Repository != nil && location.Repository.Basis == "current_git" &&
			location.Repository.CommonDir == e.Data.Repository.CommonDir {
			return
		}
	}
	t.Fatal("missing resolved command directory")
}

func claudeSession(t *testing.T, home string, n int, paths []string, states ...any) string {
	t.Helper()
	id := fmt.Sprintf("aaaaaaaa-bbbb-4ccc-8ddd-%012d", n)
	rows := []any{}
	for _, p := range paths {
		rows = append(rows, map[string]any{"type": "user", "sessionId": id, "version": "2.1.260", "cwd": p, "timestamp": "2026-09-03T10:00:00Z", "message": map[string]any{"role": "user", "content": "Independent synthetic conversation"}})
	}
	for _, state := range states {
		if value, ok := state.(map[string]any); ok {
			value["sessionId"] = id
		}
		rows = append(rows, map[string]any{"type": "worktree-state", "sessionId": id, "worktreeSession": state})
	}
	write(t, filepath.Join(home, "projects", "synthetic", id+".jsonl"), rows...)
	return contract.NewSourceRef("claude", "claude-default", id)
}

func TestRepositoryClaudeMappingWithMissingPaths(t *testing.T) {
	for _, tc := range []struct {
		name, original, previous, basis, reason string
		removeWorktree, pruneRegistration       bool
	}{
		{name: "live_missing_origin", basis: "current_git"},
		{name: "registered_missing_origin", removeWorktree: true, basis: "git_registration"},
		{name: "live_missing_previous", original: "target", basis: "current_git"},
		{name: "registered_missing_previous", original: "target", removeWorktree: true, basis: "git_registration"},
		{name: "no_independent_evidence", removeWorktree: true, pruneRegistration: true, reason: "mapping_unavailable"},
		{name: "conflicting_origin", original: "other", reason: "conflicting_evidence"},
		{name: "conflicting_previous", previous: "other", reason: "conflicting_evidence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			target := repo(t, filepath.Join(base, "target"))
			other := repo(t, filepath.Join(base, "other"))
			origin := filepath.Join(base, "original worktree")
			work := filepath.Join(base, "working worktree")
			git(t, target, "worktree", "add", "-q", "--detach", origin)
			git(t, target, "worktree", "add", "-q", "--detach", work)
			git(t, target, "worktree", "remove", origin)
			if tc.removeWorktree {
				if err := os.RemoveAll(work); err != nil {
					t.Fatal(err)
				}
			}
			if tc.pruneRegistration {
				git(t, target, "worktree", "prune")
			}
			paths := map[string]string{"": origin, "target": target, "other": other}
			home := filepath.Join(base, "records")
			ref := claudeSession(t, home, 1, []string{other}, map[string]any{
				"originalCwd": paths[tc.original], "preEnterOriginalCwd": paths[tc.previous], "worktreePath": work,
			})

			e := run(t, "list", "--provider", "claude", "--root", home, "--repository", target)
			sources := *e.Data.Sources
			if tc.basis != "" {
				if e.Status != contract.StatusComplete || len(sources) != 1 || sources[0].Identity.SourceRef != ref {
					t.Fatalf("expected a match from %s, got %d sources and omissions %v", tc.basis, len(sources), e.Omissions)
				}
				membership := sources[0].WorkingDirectories.Locations[1].Repository
				if membership == nil || membership.State != "resolved" || membership.Basis != tc.basis {
					t.Fatalf("unexpected membership: %+v", membership)
				}
				return
			}
			if len(sources) != 0 || e.Status != contract.StatusPartial {
				t.Fatalf("expected unresolved membership, got %d sources and status %s", len(sources), e.Status)
			}
			for _, o := range e.Omissions {
				if o.Code == "repository_membership_unknown" && o.Count == 1 && o.Reason == tc.reason {
					return
				}
			}
			t.Fatalf("missing omission for %s: %v", tc.reason, e.Omissions)
		})
	}
}
func TestRepositoryDeletedWorktreesAndReusedPaths(t *testing.T) {
	for _, mode := range []string{"registered", "pruned", "reused_clone", "reused_directory"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			target := repo(t, filepath.Join(base, "target"))
			work := filepath.Join(target, "linked")
			git(t, target, "worktree", "add", "-q", "--detach", work)
			home := filepath.Join(base, "records")
			ref := codexSession(t, home, 1, work)
			if err := os.RemoveAll(work); err != nil {
				t.Fatal(err)
			}
			if mode == "pruned" {
				git(t, target, "worktree", "prune")
			}
			if mode == "reused_clone" {
				repo(t, work)
			}
			if mode == "reused_directory" {
				os.MkdirAll(work, 0700)
			}
			e := run(t, "list", "--provider", "codex", "--root", home, "--repository", target)
			if mode == "registered" {
				if len(*e.Data.Sources) != 1 || (*e.Data.Sources)[0].Identity.SourceRef != ref || (*e.Data.Sources)[0].WorkingDirectories.Locations[0].Repository.Basis != "git_registration" {
					t.Fatalf("%#v", e)
				}
			} else if len(*e.Data.Sources) != 0 || e.Status != contract.StatusPartial {
				t.Fatalf("%#v", e)
			}
		})
	}
}
func TestRepositoryClaudeMappings(t *testing.T) {
	for _, mode := range []string{"created", "existing", "hook", "different_previous", "reused", "contradictory", "clear", "match_with_gap", "existing_live", "hook_live"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			target := repo(t, filepath.Join(base, "target"))
			other := repo(t, filepath.Join(base, "other"))
			work := filepath.Join(base, "deleted")
			home := filepath.Join(base, "records")
			state := map[string]any{"originalCwd": target, "preEnterOriginalCwd": target, "worktreePath": work}
			paths := []string{other}
			states := []any{state}
			switch mode {
			case "existing":
				state["enteredExisting"] = true
			case "existing_live", "hook_live":
				git(t, target, "worktree", "add", "-q", "--detach", work)
				state["originalCwd"] = other
				state["preEnterOriginalCwd"] = other
				if mode == "existing_live" {
					state["enteredExisting"] = true
				} else {
					state["hookBased"] = true
				}
			case "hook":
				state["hookBased"] = true
			case "different_previous":
				state["preEnterOriginalCwd"] = other
			case "reused":
				repo(t, work)
				paths = append(paths, work)
			case "contradictory":
				states = append(states, map[string]any{"originalCwd": other, "worktreePath": work})
			case "clear":
				states = append(states, nil)
			case "match_with_gap":
				paths = append(paths, target, filepath.Join(base, "unknown"))
			}
			ref := claudeSession(t, home, 1, paths, states...)
			e := run(t, "list", "--provider", "claude", "--root", home, "--repository", target)
			want := mode == "created" || mode == "clear" || mode == "match_with_gap" || mode == "existing_live" || mode == "hook_live"
			if (len(*e.Data.Sources) == 1) != want {
				t.Fatalf("sources=%#v omissions=%#v", *e.Data.Sources, e.Omissions)
			}
			if !want && e.Status != contract.StatusPartial {
				t.Fatalf("missing unknown: %#v", e)
			}
			shown := run(t, "show", "--root", home, ref)
			if shown.Data.Source.WorkingDirectories == nil {
				t.Fatal("missing locations")
			}
			if mode == "match_with_gap" && e.Status != contract.StatusPartial {
				t.Fatal("match erased incomplete evidence")
			}
		})
	}
}
func TestRepositoryKnownNonMatchesAndInvalidTarget(t *testing.T) {
	base := t.TempDir()
	target := repo(t, filepath.Join(base, "target"))
	other := repo(t, filepath.Join(base, "other"))
	home := filepath.Join(base, "records")
	codexSession(t, home, 1, other)
	e := run(t, "list", "--provider", "codex", "--root", home, "--repository", target)
	if len(*e.Data.Sources) != 0 || e.Status != contract.StatusComplete || e.Data.Repository == nil {
		t.Fatalf("%#v", e)
	}
	var out bytes.Buffer
	r := cli.Runner{Registry: provider.NewRegistry(codex.New())}
	if code := r.Run(t.Context(), []string{"list", "--provider", "codex", "--root", home, "--repository", base}, &out); code != cli.ExitUsage || !bytes.Contains(out.Bytes(), []byte(`"invalid_repository"`)) {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
}

func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Query current attributes directly; directory-enumeration attributes can
		// be stale after fixture creation on Windows.
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		entry := fmt.Sprintf("%v %d %d", info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			entry += fmt.Sprintf(" %x", sum)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry += " " + target
		}
		rel, _ := filepath.Rel(root, path)
		result[rel] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestRepositoryOperationsPreserveProviderAndGitTrees(t *testing.T) {
	base := t.TempDir()
	target := repo(t, filepath.Join(base, "target"))
	home := filepath.Join(base, "records")
	ref := codexSession(t, home, 1, target)
	claudeHome := filepath.Join(base, "claude")
	claudeRef := claudeSession(t, claudeHome, 1, []string{target})
	before := treeSnapshot(t, base)
	eventsBefore := run(t, "events", "--root", home, ref)
	versionBefore := run(t, "verify", "--root", home, ref)
	for _, p := range []struct{ name, home, ref string }{{"codex", home, ref}, {"claude", claudeHome, claudeRef}} {
		run(t, "list", "--provider", p.name, "--root", p.home, "--repository", target)
		run(t, "show", "--root", p.home, p.ref)
		run(t, "events", "--root", p.home, p.ref)
		run(t, "verify", "--root", p.home, p.ref)
	}
	eventsAfter := run(t, "events", "--root", home, ref)
	versionAfter := run(t, "verify", "--root", home, ref)
	if !reflect.DeepEqual(eventsBefore, eventsAfter) || !reflect.DeepEqual(versionBefore, versionAfter) {
		t.Fatal("search changed event indices, bodies or verification")
	}
	if after := treeSnapshot(t, base); !reflect.DeepEqual(before, after) {
		for path, entry := range after {
			if before[path] != entry {
				t.Errorf("snapshot changed %q: before=%q after=%q", path, before[path], entry)
			}
		}
		for path := range before {
			if _, exists := after[path]; !exists {
				t.Errorf("snapshot removed %q", path)
			}
		}
		t.Fatal("read operations changed provider or Git file set, mode, size, modification time or bytes")
	}
}
func TestRepositoryMembershipIgnoresGitEnvironmentOverrides(t *testing.T) {
	base := t.TempDir()
	target := repo(t, filepath.Join(base, "target"))
	other := repo(t, filepath.Join(base, "other"))
	home := filepath.Join(base, "records")
	codexSession(t, home, 1, other)
	t.Setenv("GIT_DIR", filepath.Join(target, ".git"))
	t.Setenv("GIT_WORK_TREE", target)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.bare")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	e := run(t, "list", "--provider", "codex", "--root", home, "--repository", target)
	if len(*e.Data.Sources) != 0 || e.Status != contract.StatusComplete {
		t.Fatalf("overrides contaminated resolution: %#v", e)
	}
}
func TestRepositoryUnknownCandidatesAreCountedBeforePagination(t *testing.T) {
	base := t.TempDir()
	target := repo(t, filepath.Join(base, "target"))
	home := filepath.Join(base, "records")
	codexSession(t, home, 1, target)
	codexSession(t, home, 2, target)
	codexSession(t, home, 3, filepath.Join(base, "absent-one"))
	codexSession(t, home, 4, filepath.Join(base, "absent-two"))
	e := run(t, "list", "--provider", "codex", "--root", home, "--repository", target, "--limit", "1")
	if !e.Page.HasMore {
		t.Fatal("missing next page")
	}
	var count int
	for _, o := range e.Omissions {
		if o.Code == "repository_membership_unknown" {
			count += o.Count
			if o.Reason != "directory_unavailable" {
				t.Fatal(o)
			}
		}
	}
	if count != 2 {
		t.Fatalf("count=%d", count)
	}
	last := run(t, "list", "--provider", "codex", "--root", home, "--repository", target, "--limit", "1", "--cursor", e.Page.NextCursor)
	if last.Status != contract.StatusPartial || last.Page.HasMore {
		t.Fatal("last page lost incomplete search")
	}
}

func TestRepositoryQueryAcceptsBareRepositoryWithLinkedWorktree(t *testing.T) {
	base := t.TempDir()
	seed := repo(t, filepath.Join(base, "seed"))
	bare := filepath.Join(base, "bare.git")
	git(t, base, "clone", "-q", "--bare", seed, bare)
	work := filepath.Join(base, "work")
	git(t, bare, "worktree", "add", "-q", "--detach", work)
	home := filepath.Join(base, "records")
	ref := codexSession(t, home, 1, work)
	e := run(t, "list", "--provider", "codex", "--root", home, "--repository", bare)
	if e.Status != contract.StatusComplete || len(*e.Data.Sources) != 1 || (*e.Data.Sources)[0].Identity.SourceRef != ref {
		t.Fatalf("%#v", e)
	}
}
