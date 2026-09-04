package contract

import (
	"errors"
	"unicode/utf8"
)

const (
	MaxStringBytes   = 64 << 10
	MaxMetadata      = 64
	MaxRelationships = 64
	MaxJSONDepth     = 64
)

var ErrStructuralStringBound = errors.New("structural output string exceeds bound")

func EnforceBounds(envelope *Envelope) error {
	changed := false
	for _, value := range []string{envelope.SchemaVersion, envelope.RedactionPolicyVersion, envelope.Operation, string(envelope.Status)} {
		if err := requireBoundedString(value); err != nil {
			return err
		}
	}
	changed = boundString(&envelope.CLIVersion) || changed

	if envelope.Data != nil {
		if envelope.Data.Sources != nil {
			for i := range *envelope.Data.Sources {
				bounded, err := boundSource(&(*envelope.Data.Sources)[i])
				if err != nil {
					return err
				}
				changed = bounded || changed
			}
		}
		if envelope.Data.Source != nil {
			bounded, err := boundSource(envelope.Data.Source)
			if err != nil {
				return err
			}
			changed = bounded || changed
		}
		if err := requireBoundedString(envelope.Data.SourceRef); err != nil {
			return err
		}
		if envelope.Data.Events != nil {
			for i := range *envelope.Data.Events {
				bounded, err := boundEvent(&(*envelope.Data.Events)[i])
				if err != nil {
					return err
				}
				changed = bounded || changed
			}
		}
		if envelope.Data.VerifiedVersion != nil {
			for _, value := range []string{envelope.Data.VerifiedVersion.Algorithm, envelope.Data.VerifiedVersion.Basis, envelope.Data.VerifiedVersion.Value} {
				if err := requireBoundedString(value); err != nil {
					return err
				}
			}
		}
	}

	if envelope.Page != nil {
		if err := requireBoundedString(envelope.Page.NextCursor); err != nil {
			return err
		}
	}
	for i := range envelope.Omissions {
		if err := requireBoundedString(envelope.Omissions[i].Code); err != nil {
			return err
		}
		if err := requireBoundedString(envelope.Omissions[i].Scope); err != nil {
			return err
		}
		changed = boundString(&envelope.Omissions[i].Message) || changed
	}
	if envelope.Error != nil {
		if err := requireBoundedString(envelope.Error.Code); err != nil {
			return err
		}
		if err := requireBoundedString(envelope.Error.Category); err != nil {
			return err
		}
		changed = boundString(&envelope.Error.Message) || changed
		for i := range envelope.Error.Details {
			if err := requireBoundedString(envelope.Error.Details[i].Name); err != nil {
				return err
			}
			changed = boundString(&envelope.Error.Details[i].Value) || changed
		}
	}

	if changed {
		envelope.Omissions = append(envelope.Omissions, Omission{Code: "resource_truncation", Scope: "output", Message: "One or more values exceeded the public output bounds."})
		if envelope.Status == StatusComplete {
			envelope.Status = StatusPartial
		}
	}
	return nil
}

func boundSource(source *Source) (bool, error) {
	changed := false
	if len(source.Metadata) > MaxMetadata {
		source.Metadata = source.Metadata[:MaxMetadata]
		changed = true
	}
	if len(source.Relationships) > MaxRelationships {
		source.Relationships = source.Relationships[:MaxRelationships]
		changed = true
	}
	identity := &source.Identity
	for _, value := range []string{identity.Provider, identity.SourceInstance, identity.ProviderSourceFingerprint, identity.SourceRef, source.Kind} {
		if err := requireBoundedString(value); err != nil {
			return false, err
		}
	}
	if !boundedString(identity.ProviderNativeSourceID) {
		identity.ProviderNativeSourceID = ""
		changed = true
	}
	if source.VersionHint != nil {
		if err := requireBoundedString(source.VersionHint.Kind); err != nil {
			return false, err
		}
		changed = boundString(&source.VersionHint.Value) || changed
	}
	for i := range source.Relationships {
		if err := requireBoundedString(source.Relationships[i].Kind); err != nil {
			return false, err
		}
		if err := requireBoundedString(source.Relationships[i].SourceRef); err != nil {
			return false, err
		}
	}
	for i := range source.Metadata {
		bounded, err := boundMetadata(&source.Metadata[i])
		if err != nil {
			return false, err
		}
		changed = bounded || changed
	}
	return changed, nil
}

func boundEvent(event *Event) (bool, error) {
	changed := false
	if len(event.Metadata) > MaxMetadata {
		event.Metadata = event.Metadata[:MaxMetadata]
		changed = true
	}
	if err := requireBoundedString(string(event.Kind)); err != nil {
		return false, err
	}
	for i := range event.Metadata {
		bounded, err := boundMetadata(&event.Metadata[i])
		if err != nil {
			return false, err
		}
		changed = bounded || changed
	}
	if event.Message != nil {
		if err := requireBoundedString(event.Message.Role); err != nil {
			return false, err
		}
		changed = boundString(&event.Message.Text) || changed
	}
	if event.ToolCall != nil {
		if err := requireBoundedString(event.ToolCall.CallID); err != nil {
			return false, err
		}
		if err := requireBoundedString(event.ToolCall.Category); err != nil {
			return false, err
		}
	}
	if event.ToolResult != nil {
		if err := requireBoundedString(event.ToolResult.CallID); err != nil {
			return false, err
		}
	}
	if event.Error != nil {
		if err := requireBoundedString(event.Error.Category); err != nil {
			return false, err
		}
		changed = boundString(&event.Error.Message) || changed
	}
	return changed, nil
}

func boundMetadata(metadata *Metadata) (bool, error) {
	if err := requireBoundedString(metadata.Name); err != nil {
		return false, err
	}
	return boundString(&metadata.Value), nil
}

func requireBoundedString(value string) error {
	if !boundedString(value) {
		return ErrStructuralStringBound
	}
	return nil
}

func boundedString(value string) bool {
	return len(value) <= MaxStringBytes && utf8.ValidString(value)
}

func boundString(value *string) bool {
	if boundedString(*value) {
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
