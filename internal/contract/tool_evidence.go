package contract

import (
	"regexp"
	"strings"
)

const MaxToolExcerptBytes = 512

func ValidInputState(state InputState) bool {
	switch state {
	case InputAbsent, InputWithheld, InputUnavailable, InputUnsupported:
		return true
	default:
		return false
	}
}

func ToolInputOmissions(events []Event) []Omission {
	count := 0
	for _, event := range events {
		if event.ToolCall != nil && (event.ToolCall.InputState == InputUnavailable || event.ToolCall.InputState == InputUnsupported) {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return []Omission{{Code: "tool_input_unavailable", Scope: "tool_call", Count: count, Message: "A tool call input could not be classified safely."}}
}

var safeCountLine = regexp.MustCompile(`^([0-9]{1,12}) (tests?|checks?|assertions?|errors?|failures?|warnings?) (passed|failed|skipped|found)$`)

// SafeToolExcerpt constructs a bounded excerpt from a deliberately narrow line grammar.
// Arbitrary source text is never copied to the public result.
func SafeToolExcerpt(body string, present bool) (string, EvidenceState, bool, bool) {
	if !present || body == "" {
		return "", EvidenceAbsent, false, false
	}
	var result strings.Builder
	redacted, truncated := false, false
	for len(body) > 0 {
		line := body
		if before, after, found := strings.Cut(body, "\n"); found {
			line, body = before, after
		} else {
			body = ""
		}
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		if len(line) > 64 {
			redacted = true
			continue
		}
		safe := ""
		switch line {
		case "PASS", "FAIL", "OK", "SUCCESS":
			safe = line
		default:
			if parts := safeCountLine.FindStringSubmatch(line); parts != nil {
				safe = parts[1] + " " + parts[2] + " " + parts[3]
			}
		}
		if safe == "" {
			redacted = true
			continue
		}
		extra := len(safe)
		if result.Len() > 0 {
			extra++
		}
		if result.Len()+extra > MaxToolExcerptBytes {
			truncated = true
			continue
		}
		if result.Len() > 0 {
			result.WriteByte('\n')
		}
		result.WriteString(safe)
	}
	if result.Len() == 0 {
		return "", EvidenceUnavailable, true, truncated
	}
	return result.String(), EvidenceAvailable, redacted, truncated
}

func ToolResultOmissions(result ToolResultEvent) []Omission {
	omissions := []Omission{}
	if result.EvidenceState == EvidenceUnavailable {
		omissions = append(omissions, Omission{Code: "tool_excerpt_unavailable", Scope: "tool_result", Message: "A tool result body had no safe excerpt."})
	}
	if result.EvidenceState == EvidenceUnsupported {
		omissions = append(omissions, Omission{Code: "tool_excerpt_unsupported", Scope: "tool_result", Message: "A tool result body format was not supported."})
	}
	if result.Redacted {
		omissions = append(omissions, Omission{Code: "tool_excerpt_redacted", Scope: "tool_result", Message: "Unsafe tool result content was omitted."})
	}
	if result.Truncated {
		omissions = append(omissions, Omission{Code: "tool_excerpt_truncated", Scope: "tool_result", Message: "A safe tool result excerpt exceeded its byte limit."})
	}
	return omissions
}

func ValidToolAction(action string) bool {
	switch action {
	case "execute", "read", "search", "write", "edit", "invoke":
		return true
	default:
		return false
	}
}

func ValidEvidenceState(state EvidenceState) bool {
	switch state {
	case EvidenceAvailable, EvidenceAbsent, EvidenceUnavailable, EvidenceUnsupported:
		return true
	default:
		return false
	}
}

func ValidSafeToolExcerpt(excerpt string) bool {
	if excerpt == "" || len(excerpt) > MaxToolExcerptBytes {
		return false
	}
	canonical, state, redacted, truncated := SafeToolExcerpt(excerpt, true)
	return state == EvidenceAvailable && canonical == excerpt && !redacted && !truncated
}
