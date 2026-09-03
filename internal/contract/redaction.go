package contract

import (
	"reflect"
	"regexp"
	"strings"
)

var (
	credentialPattern  = regexp.MustCompile(`(?i)(?:authorization\s*[:=]\s*(?:bearer\s+)?|(?:api[_-]?key|token|password|secret)\s*[:=]\s*)[^\s,;"']+|\b(?:sk|ghp|github_pat)-[A-Za-z0-9_-]{8,}\b`)
	fileURIPattern     = regexp.MustCompile(`(?i)file://[^\s"']+`)
	windowsPathPattern = regexp.MustCompile(`(?i)(?:[a-z]:\\|\\\\)[^\s"']*`)
	unixPathPattern    = regexp.MustCompile(`(?:^|[\s("'])/(?:[^\s/"']+/)*[^\s/)"']+`)
	URLHostPattern     = regexp.MustCompile(`(?i)(https?://)(?:[a-z0-9-]+\.)+[a-z]{2,63}`)
	hostPattern        = regexp.MustCompile(`(?i)\blocalhost\b|\b(?:\d{1,3}\.){3}\d{1,3}\b|\b(?:[a-z0-9-]+\.)+(?:example\.invalid|invalid|test|[a-z]{2,63})\b`)
	commandPattern     = regexp.MustCompile(`(?m)(?:^|\n)\s*(?:\$|>)\s+[^\n]+|` + "`{1,3}" + `[^` + "`" + `\n]+` + "`{1,3}")
)

func Finalize(envelope *Envelope) {
	changed := redactTypedSensitiveFields(envelope)
	changed = redactStrings(reflect.ValueOf(envelope)) || changed
	if !changed {
		return
	}
	envelope.Omissions = append(envelope.Omissions, Omission{
		Code: "output_redacted", Scope: "output", Message: "One or more unsafe output values were redacted.",
	})
	if envelope.Status == StatusComplete {
		envelope.Status = StatusPartial
	}
}

func redactTypedSensitiveFields(envelope *Envelope) bool {
	if envelope.Data == nil {
		if envelope.Error == nil {
			return false
		}
	}
	changed := false
	redactNamedValue := func(name string, value *string) {
		name = strings.ToLower(name)
		if strings.Contains(name, "host") || strings.Contains(name, "path") || strings.Contains(name, "command") || strings.Contains(name, "credential") || strings.Contains(name, "token") {
			if *value != "<redacted:unsafe-field>" {
				*value = "<redacted:unsafe-field>"
				changed = true
			}
		}
	}
	redactMetadata := func(entries []Metadata) {
		for i := range entries {
			redactNamedValue(entries[i].Name, &entries[i].Value)
		}
	}
	if envelope.Data != nil && envelope.Data.Sources != nil {
		for i := range *envelope.Data.Sources {
			redactMetadata((*envelope.Data.Sources)[i].Metadata)
			if (*envelope.Data.Sources)[i].VersionHint != nil {
				redactNamedValue((*envelope.Data.Sources)[i].VersionHint.Kind, &(*envelope.Data.Sources)[i].VersionHint.Value)
			}
		}
	}
	if envelope.Data != nil && envelope.Data.Source != nil {
		redactMetadata(envelope.Data.Source.Metadata)
		if envelope.Data.Source.VersionHint != nil {
			redactNamedValue(envelope.Data.Source.VersionHint.Kind, &envelope.Data.Source.VersionHint.Value)
		}
	}
	if envelope.Data != nil && envelope.Data.Events != nil {
		for i := range *envelope.Data.Events {
			redactMetadata((*envelope.Data.Events)[i].Metadata)
		}
	}
	if envelope.Error != nil {
		for i := range envelope.Error.Details {
			redactNamedValue(envelope.Error.Details[i].Name, &envelope.Error.Details[i].Value)
		}
	}
	return changed
}

func redactStrings(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		return redactStrings(value.Elem())
	}
	changed := false
	switch value.Kind() {
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).CanSet() && redactStrings(value.Field(i)) {
				changed = true
			}
		}
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			if redactStrings(value.Index(i)) {
				changed = true
			}
		}
	case reflect.String:
		original := value.String()
		redacted := Redact(original)
		if redacted != original {
			value.SetString(redacted)
			changed = true
		}
	}
	return changed
}

func Redact(value string) string {
	value = strings.ToValidUTF8(value, "<redacted:invalid-utf8>")
	value = credentialPattern.ReplaceAllString(value, "<redacted:credential>")
	value = fileURIPattern.ReplaceAllString(value, "<redacted:path>")
	value = windowsPathPattern.ReplaceAllString(value, "<redacted:path>")
	value = unixPathPattern.ReplaceAllStringFunc(value, func(match string) string {
		prefix := ""
		if len(match) > 0 && strings.ContainsAny(match[:1], " \t\n(\"'") {
			prefix, match = match[:1], match[1:]
		}
		return prefix + "<redacted:path>"
	})
	value = URLHostPattern.ReplaceAllString(value, `${1}<redacted:host>`)
	value = hostPattern.ReplaceAllString(value, "<redacted:host>")
	value = commandPattern.ReplaceAllString(value, "<redacted:command>")
	return value
}
