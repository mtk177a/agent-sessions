// Package repository resolves recorded paths against local Git administration.
// It never executes recorded commands or asks Git to repair or refresh metadata.
package repository

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

type Resolver struct {
	ctx           context.Context
	target        string
	registrations map[string]bool
	cache         map[string]contract.RepositoryMembership
	roots         map[string]string
}

func New(ctx context.Context, target string) (*Resolver, error) {
	r := &Resolver{ctx: ctx, roots: map[string]string{}, cache: map[string]contract.RepositoryMembership{}, registrations: map[string]bool{}}
	if target == "" {
		return r, nil
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	m := r.current(absolute)
	if m.State != "resolved" {
		return nil, errors.New("invalid repository")
	}
	r.target = m.CommonDir
	// Porcelain -z preserves paths with spaces and does not prune stale entries.
	data, err := gitRead(ctx, absolute, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	for _, field := range strings.Split(string(data), "\x00") {
		if strings.HasPrefix(field, "worktree ") {
			p := strings.TrimPrefix(field, "worktree ")
			if contract.ValidDirectoryPath(p) {
				r.registrations[canonicalLocator(p)] = true
			}
		}
	}
	return r, nil
}
func unknown(reason string) contract.RepositoryMembership {
	return contract.RepositoryMembership{State: "unknown", Reason: reason}
}
func (r *Resolver) current(path string) contract.RepositoryMembership {
	if m, ok := r.cache[path]; ok {
		return m
	}
	if len(r.cache) >= 4096 {
		return unknown("git_lookup_limit")
	}
	m := unknown("git_unavailable")
	if !contract.ValidDirectoryPath(path) {
		return unknown("invalid_directory")
	}
	info, err := os.Stat(path)
	if err != nil {
		m = unknown("directory_unavailable")
	} else if !info.IsDir() {
		m = unknown("invalid_directory")
	} else {
		data, err := gitRead(r.ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err == nil {
			parts := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			if len(parts) == 1 && contract.ValidDirectoryPath(parts[0]) {
				common, e := filepath.EvalSymlinks(parts[0])
				if e == nil {
					m = contract.RepositoryMembership{State: "resolved", CommonDir: filepath.Clean(common), Basis: "current_git"}
				}
			}
		}
	}
	r.cache[path] = m
	return m
}
func (r *Resolver) resolve(location contract.WorkingDirectory) contract.RepositoryMembership {
	m := r.current(location.Path)
	registered := r.registrations[canonicalLocator(location.Path)]
	canonicalPath, _ := filepath.EvalSymlinks(location.Path)
	if registered && m.State == "resolved" {
		if m.CommonDir != r.target {
			return unknown("conflicting_evidence")
		}
		if canonicalPath != m.CommonDir {
			root, cached := r.roots[location.Path]
			if !cached {
				top, err := gitRead(r.ctx, location.Path, "rev-parse", "--show-toplevel")
				if err == nil {
					root, _ = filepath.EvalSymlinks(strings.TrimSuffix(string(top), "\n"))
				}
				r.roots[location.Path] = root
			}
			if root != canonicalPath {
				return unknown("conflicting_evidence")
			}
		}
	}
	if registered && m.State != "resolved" {
		// Use a stale registration only when the worktree root itself is absent.
		if _, err := os.Lstat(location.Path); os.IsNotExist(err) {
			m = contract.RepositoryMembership{State: "resolved", CommonDir: r.target, Basis: "git_registration"}
		}
	}
	mapping := location.Mapping
	if mapping == nil || mapping.Kind != "git_created" {
		return m
	}
	original := r.current(mapping.OriginalPath)
	if original.State != "resolved" {
		return unknown("mapping_unavailable")
	}
	if mapping.PreviousPath != "" {
		previous := r.current(mapping.PreviousPath)
		if previous.State != "resolved" || previous.CommonDir != original.CommonDir {
			return unknown("conflicting_evidence")
		}
	}
	if m.State == "resolved" {
		if m.CommonDir != original.CommonDir {
			return unknown("conflicting_evidence")
		}
		return m
	}
	// A provider mapping can recover an absent worktree, never an existing path
	// that no longer has the recorded Git relationship.
	if _, err := os.Lstat(location.Path); !os.IsNotExist(err) {
		return unknown("conflicting_evidence")
	}
	return contract.RepositoryMembership{State: "resolved", CommonDir: original.CommonDir, Basis: "provider_mapping"}
}

// Observe enriches one source and returns match, non_match, or unknown.
// A confirmed match takes precedence over incomplete observations.
func (r *Resolver) Observe(source *contract.Source) string {
	w := source.WorkingDirectories
	if w == nil {
		return "unknown"
	}
	incomplete := w.State != "complete" || len(w.Locations) == 0
	match := false
	associations := map[string]string{}
	conflicts := map[string]bool{}
	for i := range w.Locations {
		location := &w.Locations[i]
		m := r.resolve(*location)
		location.Repository = &m
		key := canonicalLocator(location.Path)
		if m.Reason == "conflicting_evidence" {
			conflicts[key] = true
		}
		if m.State == "resolved" {
			if prior := associations[key]; prior != "" && prior != m.CommonDir {
				conflicts[key] = true
			}
			associations[key] = m.CommonDir
		}
	}
	for i := range w.Locations {
		location := &w.Locations[i]
		key := canonicalLocator(location.Path)
		if conflicts[key] {
			m := unknown("conflicting_evidence")
			location.Repository = &m
		}
		m := location.Repository
		if m.State != "resolved" {
			incomplete = true
		} else if r.target != "" && m.CommonDir == r.target {
			match = true
		}
	}

	if match {
		return "match"
	}
	if incomplete {
		return "unknown"
	}
	return "non_match"
}

// Bound output even when Git reads corrupt or adversarial administration files.
type boundedBuffer struct{ data bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > (1<<20)-b.data.Len() {
		return 0, errors.New("git output limit")
	}
	return b.data.Write(p)
}
func gitRead(ctx context.Context, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-C", path}, args...)...)
	// Ignore inherited Git repository/config overrides, including GIT_DIR,
	// GIT_WORK_TREE, config injection and external object directories.
	for _, e := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(e), "GIT_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	var out boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	return out.data.Bytes(), err
}

func (r *Resolver) CommonDir() string { return r.target }

// Uncertainty returns one reason per candidate; counts therefore do not double
// count candidates that have several incomplete observations.
func Uncertainty(source contract.Source) string {
	w := source.WorkingDirectories
	if w == nil {
		return "unsupported_provider"
	}
	for _, location := range w.Locations {
		if location.Repository != nil && location.Repository.State == "unknown" {
			return location.Repository.Reason
		}
	}
	if len(w.Reasons) > 0 {
		return w.Reasons[0]
	}
	if w.State != "complete" || len(w.Locations) == 0 {
		return "directory_unavailable"
	}
	return ""
}

// Resolve existing ancestors of an absent path without assuming that an absent
// descendant belonged to an ancestor repository.
func canonicalLocator(path string) string {
	path = filepath.Clean(path)
	suffix := []string{}
	for {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		suffix = append(suffix, filepath.Base(path))
		path = parent
	}
}
