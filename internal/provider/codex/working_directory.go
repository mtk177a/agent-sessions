package codex

import (
	"encoding/json"

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
		switch line.Type {
		case "session_meta":
			kind = "session"
		case "turn_context":
			kind = "turn"
		case "event_msg":
			if fields.Type == "item_completed" && fields.Item.Type == "CommandExecution" {
				kind = "tool"
				path = fields.Item.Cwd
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
