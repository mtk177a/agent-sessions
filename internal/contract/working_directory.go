package contract

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxWorkingDirectories = 256

func NewWorkingDirectories() *WorkingDirectories {
	return &WorkingDirectories{State: "complete", Locations: []WorkingDirectory{}, Reasons: []string{}}
}
func (w *WorkingDirectories) Incomplete(reason string) {
	w.State = "partial"
	for _, r := range w.Reasons {
		if r == reason {
			return
		}
	}
	w.Reasons = append(w.Reasons, reason)
}
func ValidDirectoryPath(path string) bool {
	return path != "" && filepath.IsAbs(path) && len(path) <= MaxStringBytes && utf8.ValidString(path) && !strings.ContainsAny(path, "\x00\r\n")
}
func (w *WorkingDirectories) Add(location WorkingDirectory) {
	if !ValidDirectoryPath(location.Path) {
		w.Incomplete("invalid_directory")
		return
	}
	for _, old := range w.Locations {
		if old.Path == location.Path && old.Evidence == location.Evidence && (old.Mapping == nil && location.Mapping == nil || old.Mapping != nil && location.Mapping != nil && *old.Mapping == *location.Mapping) {
			return
		}
	}
	if len(w.Locations) == MaxWorkingDirectories {
		w.Incomplete("directory_limit")
		return
	}
	w.Locations = append(w.Locations, location)
}

func ValidateWorkingDirectories(w *WorkingDirectories) error {
	if w == nil {
		return nil
	}
	if w.Locations == nil || w.Reasons == nil || len(w.Locations) > MaxWorkingDirectories {
		return errors.New("invalid working directories")
	}
	switch w.State {
	case "complete":
		if len(w.Reasons) != 0 {
			return errors.New("complete directories have reasons")
		}
	case "partial", "unsupported":
		if len(w.Reasons) == 0 {
			return errors.New("incomplete directories need reasons")
		}
	default:
		return errors.New("invalid working directory state")
	}
	for _, reason := range w.Reasons {
		if !ValidToken(reason) {
			return errors.New("invalid working directory reason")
		}
	}
	for _, location := range w.Locations {
		if !ValidDirectoryPath(location.Path) {
			return errors.New("invalid working directory path")
		}
		switch location.Evidence {
		case "session", "turn", "tool", "interaction", "worktree":
		default:
			return errors.New("invalid directory evidence")
		}
		if m := location.Mapping; m != nil {
			if location.Evidence != "worktree" || !ValidDirectoryPath(m.OriginalPath) || m.PreviousPath != "" && !ValidDirectoryPath(m.PreviousPath) {
				return errors.New("invalid worktree mapping")
			}
			switch m.Kind {
			case "git_created", "existing", "hook", "unknown":
			default:
				return errors.New("invalid mapping kind")
			}
		}
		if r := location.Repository; r != nil {
			switch r.State {
			case "resolved":
				if !ValidDirectoryPath(r.CommonDir) || r.Reason != "" {
					return errors.New("invalid resolved repository")
				}
				switch r.Basis {
				case "current_git", "git_registration", "provider_mapping":
				default:
					return errors.New("invalid repository basis")
				}
			case "unknown":
				if !ValidToken(r.Reason) || r.CommonDir != "" || r.Basis != "" {
					return errors.New("invalid unknown repository")
				}
			default:
				return errors.New("invalid repository state")
			}
		}
	}
	return nil
}
