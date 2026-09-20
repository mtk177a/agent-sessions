package contract

import "testing"

func TestValidateInteractionTime(t *testing.T) {
	for _, value := range []string{"2026-09-03T10:00:00Z", "2026-09-03T10:00:00.123456789Z"} {
		if err := ValidateInteractionTime(value); err != nil {
			t.Fatalf("valid %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "2026-09-03", "2026-09-03T10:00:00+00:00", "2026-09-03T10:00:00.000Z", "2026-02-30T10:00:00Z"} {
		if err := ValidateInteractionTime(value); err == nil {
			t.Fatalf("accepted invalid time %q", value)
		}
	}
}

func TestListWithMissingInteractionTimeRequiresOmission(t *testing.T) {
	sources := []Source{{Kind: "session", Relationships: []Relationship{}, Metadata: []Metadata{}}}
	envelope := NewEnvelope("list", "test", StatusComplete)
	envelope.Data = &Data{Sources: &sources}
	if err := envelope.Validate(); err == nil {
		t.Fatal("complete list accepted a source without interaction time")
	}
	envelope.Status = StatusPartial
	envelope.Omissions = []Omission{{Code: "source_time_unavailable", Scope: "source", Message: "Time unavailable."}}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("partial list rejected: %v", err)
	}
}
