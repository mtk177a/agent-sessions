package contract

import "unicode/utf8"

const (
	MaxStringBytes   = 64 << 10
	MaxMetadata      = 64
	MaxRelationships = 64
)

func EnforceBounds(envelope *Envelope) {
	if envelope.Data == nil {
		return
	}
	changed := false
	if envelope.Data.Sources != nil {
		for i := range *envelope.Data.Sources {
			if boundSource(&(*envelope.Data.Sources)[i]) {
				changed = true
			}
		}
	}
	if envelope.Data.Source != nil && boundSource(envelope.Data.Source) {
		changed = true
	}
	if envelope.Data.Events != nil {
		for i := range *envelope.Data.Events {
			if boundEvent(&(*envelope.Data.Events)[i]) {
				changed = true
			}
		}
	}
	if changed {
		envelope.Omissions = append(envelope.Omissions, Omission{Code: "resource_truncation", Scope: "output", Message: "One or more values exceeded the public output bounds."})
		if envelope.Status == StatusComplete {
			envelope.Status = StatusPartial
		}
	}
}

func boundSource(source *Source) bool {
	changed := false
	if len(source.Metadata) > MaxMetadata {
		source.Metadata = source.Metadata[:MaxMetadata]
		changed = true
	}
	if len(source.Relationships) > MaxRelationships {
		source.Relationships = source.Relationships[:MaxRelationships]
		changed = true
	}
	for i := range source.Metadata {
		if boundMetadata(&source.Metadata[i]) {
			changed = true
		}
	}
	if source.VersionHint != nil {
		changed = boundString(&source.VersionHint.Value) || changed
	}
	return changed
}

func boundEvent(event *Event) bool {
	changed := false
	if len(event.Metadata) > MaxMetadata {
		event.Metadata = event.Metadata[:MaxMetadata]
		changed = true
	}
	for i := range event.Metadata {
		if boundMetadata(&event.Metadata[i]) {
			changed = true
		}
	}
	if event.Message != nil {
		changed = boundString(&event.Message.Text) || changed
	}
	if event.Error != nil {
		changed = boundString(&event.Error.Message) || changed
	}
	return changed
}

func boundMetadata(metadata *Metadata) bool {
	changed := boundString(&metadata.Name)
	return boundString(&metadata.Value) || changed
}

func boundString(value *string) bool {
	if len(*value) <= MaxStringBytes && utf8.ValidString(*value) {
		return false
	}
	valid := []byte(string([]rune(*value)))
	if len(valid) > MaxStringBytes {
		valid = valid[:MaxStringBytes]
		for !utf8.Valid(valid) {
			valid = valid[:len(valid)-1]
		}
	}
	*value = string(valid)
	return true
}
