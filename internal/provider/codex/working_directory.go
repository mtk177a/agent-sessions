package codex

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

func readWorkingDirectories(root string, item artifact, byRollout map[string][]artifact, ambiguous bool) *contract.WorkingDirectories {
	w := contract.NewWorkingDirectories()
	if ambiguous {
		w.Incomplete("ambiguous_artifact")
		return w
	}
	spans, err := resolveHistory(item, byRollout)
	if err != nil {
		w.Incomplete("history_unavailable")
		return w
	}
	requireOrdinals := false
	for _, span := range spans {
		requireOrdinals = requireOrdinals || interactionTimeCompatibility(span.item.meta.CLIVersion, span.item.meta.HistoryMode).requireOrdinals
	}
	plan, err := walkHistory(root, spans, maxEffectiveHistoryBytes, maxHistoryRowBytes, requireOrdinals, func(origin artifact, line rolloutLine) {
		c := interactionTimeCompatibility(origin.meta.CLIVersion, origin.meta.HistoryMode)
		if !supportsInteractionTime(origin.meta.CLIVersion, origin.meta.HistoryMode) || !validRowForProfile(c.profile, origin.meta.CLIVersion, line, c.forward) {
			w.Incomplete("unsupported_directory_record")
			return
		}
		var fields struct {
			Cwd  json.RawMessage `json:"cwd"`
			Type string          `json:"type"`
			Item struct {
				Type string          `json:"type"`
				Cwd  json.RawMessage `json:"cwd"`
			} `json:"item"`
			Action struct {
				Cwd json.RawMessage `json:"cwd"`
			} `json:"action"`
		}
		if json.Unmarshal(line.Payload, &fields) != nil {
			w.Incomplete("invalid_directory_record")
			return
		}
		path := fields.Cwd
		kind := ""
		commandDirectory := false
		switch line.Type {
		case "session_meta":
			kind = "session"
		case "turn_context":
			kind = "turn"
		case "event_msg":
			if fields.Type == "item_completed" && fields.Item.Type == "CommandExecution" {
				kind = "tool"
				path = fields.Item.Cwd
				commandDirectory = true
			}
		case "response_item":
			if fields.Type == "local_shell_call" {
				kind = "tool"
				path = fields.Action.Cwd
			}
		}
		if kind != "" {
			var p string
			if json.Unmarshal(path, &p) != nil {
				w.Incomplete("directory_unavailable")
				return
			}
			if commandDirectory {
				p = commandDirectoryPath(p)
			}
			w.Add(contract.WorkingDirectory{Path: p, Evidence: kind})
		}
	})
	if err != nil { // Failed boundaries can invalidate already visited rows.
		w.Locations = []contract.WorkingDirectory{}
		w.Incomplete("history_unavailable")
	} else if plan.malformedRows > 0 || plan.oversizedRows > 0 {
		w.Incomplete("invalid_directory_record")
	}
	if len(w.Locations) == 0 {
		w.Incomplete("directory_unavailable")
	}
	return w
}

// commandDirectoryPath decodes Codex's local PathUri form and accepts paths
// recorded as absolute directory strings.
func commandDirectoryPath(value string) string {
	scheme, _, _ := strings.Cut(value, ":")
	if !strings.EqualFold(scheme, "file") {
		return value
	}
	uri, err := url.Parse(value)
	if err != nil || uri.Opaque != "" || uri.User != nil ||
		uri.Host != "" && !strings.EqualFold(uri.Host, "localhost") ||
		strings.ContainsAny(value, "?#") {
		return ""
	}
	path := uri.Path
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") ||
		strings.Contains(strings.ToLower(uri.EscapedPath()), "%2f") {
		return ""
	}
	first, _, _ := strings.Cut(path[1:], "/")
	hasDrive := len(first) == 2 && first[1] == ':' &&
		strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", rune(first[0]))
	if filepath.Separator == '\\' {
		if !hasDrive || strings.ContainsRune(path, '\\') {
			return ""
		}
		if len(path) == 3 {
			path += "/"
		}
		return filepath.FromSlash(path[1:])
	}
	if hasDrive {
		return ""
	}
	return path
}
