package claude

import (
	"bufio"
	"bytes"
	"encoding/json"

	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/safeio"
)

func readWorkingDirectories(root string, candidates []artifact) *contract.WorkingDirectories {
	w := contract.NewWorkingDirectories()
	if len(candidates) != 1 {
		w.Incomplete("ambiguous_artifact")
		return w
	}
	item := candidates[0]
	data, err := safeio.ReadFileWithin(root, item.relative, maxArtifactBytes)
	if err != nil {
		w.Incomplete("history_unavailable")
		return w
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), maxRowBytes)
	currentVersion := item.version
	for scanner.Scan() {
		var row transcriptRow
		if safeio.DecodeJSON(scanner.Bytes(), contract.MaxJSONDepth, &row) != nil || row.Type == "" {
			w.Incomplete("invalid_directory_record")
			continue
		}
		if row.SessionID != "" {
			id, ok := canonicalSessionID(row.SessionID)
			if !ok || id != item.sessionID {
				w.Incomplete("invalid_directory_record")
				continue
			}
		}
		version := row.Version
		if version == "" {
			version = currentVersion
		}
		if row.Version != "" {
			currentVersion = row.Version
		}
		// Directory decoding has its own compatibility boundary. Event/time forward
		// compatibility must not silently validate newly added mapping semantics.
		if !supportsInteractionTimeVersion(version) {
			w.Incomplete("unsupported_directory_record")
			continue
		}
		var raw map[string]json.RawMessage
		if json.Unmarshal(scanner.Bytes(), &raw) != nil {
			w.Incomplete("invalid_directory_record")
			continue
		}
		if row.Type == "worktree-state" {
			if version != "2.1.228" && version != "2.1.260" && version != "2.1.322" {
				w.Incomplete("unsupported_directory_record")
				continue
			}
			if row.SessionID == "" {
				w.Incomplete("invalid_directory_record")
				continue
			}
			state, exists := raw["worktreeSession"]
			if !exists {
				w.Incomplete("invalid_directory_record")
				continue
			}
			if bytes.Equal(bytes.TrimSpace(state), []byte("null")) {
				continue
			}
			var value struct {
				Original  string          `json:"originalCwd"`
				Previous  string          `json:"preEnterOriginalCwd"`
				Path      string          `json:"worktreePath"`
				SessionID string          `json:"sessionId"`
				Hook      bool            `json:"hookBased"`
				Existing  bool            `json:"enteredExisting"`
				Elsewhere json.RawMessage `json:"madeElsewhere"`
			}
			if json.Unmarshal(state, &value) != nil || !contract.ValidDirectoryPath(value.Original) || (value.Previous != "" && !contract.ValidDirectoryPath(value.Previous)) || !mappingSessionMatches(value.SessionID, item.sessionID) {
				w.Incomplete("invalid_directory_record")
				continue
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(state, &fields) != nil {
				w.Incomplete("invalid_directory_record")
				continue
			}
			if _, present := fields["preEnterOriginalCwd"]; present && !contract.ValidDirectoryPath(value.Previous) {
				w.Incomplete("invalid_directory_record")
				continue
			}
			validFlags := true
			for _, name := range []string{"hookBased", "enteredExisting"} {
				if value, present := fields[name]; present && !bytes.Equal(bytes.TrimSpace(value), []byte("true")) && !bytes.Equal(bytes.TrimSpace(value), []byte("false")) {
					validFlags = false
				}
			}
			if !validFlags {
				w.Incomplete("invalid_directory_record")
				continue
			}
			kind := "git_created"
			if value.Hook {
				kind = "hook"
			} else if value.Existing {
				kind = "existing"
			} else if len(value.Elsewhere) > 0 && !bytes.Equal(value.Elsewhere, []byte("null")) {
				kind = "unknown"
				w.Incomplete("unsupported_directory_record")
			}
			w.Add(contract.WorkingDirectory{Path: value.Path, Evidence: "worktree", Mapping: &contract.WorktreeMapping{OriginalPath: value.Original, PreviousPath: value.Previous, Kind: kind}})
			continue
		}
		if !knownBookkeepingRow(row.Type) && !knownObservationRow(row.Type) {
			w.Incomplete("unsupported_directory_record")
			continue
		}
		if knownObservationRow(row.Type) {
			if row.SessionID == "" || row.Version == "" {
				w.Incomplete("invalid_directory_record")
				continue
			}
			if !isVerifiedVersion(version) {
				if _, safe := claudeInteractionRow(row); !safe {
					w.Incomplete("unsupported_directory_record")
					continue
				}
			}
			var path string
			if json.Unmarshal(raw["cwd"], &path) != nil {
				w.Incomplete("directory_unavailable")
				continue
			}
			w.Add(contract.WorkingDirectory{Path: path, Evidence: "interaction"})
		}
	}
	if scanner.Err() != nil {
		w.Incomplete("invalid_directory_record")
	}
	if len(w.Locations) == 0 {
		w.Incomplete("directory_unavailable")
	}
	return w
}

func mappingSessionMatches(value, sessionID string) bool {
	id, ok := canonicalSessionID(value)
	return ok && id == sessionID
}
