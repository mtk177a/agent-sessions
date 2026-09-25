package contract

import (
	"errors"
	"strings"
	"time"
)

func SetEventTime(event *Event, raw string) {
	if raw == "" {
		event.TimeState = TimeAbsent
		return
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || at.Year() < 1 || at.Year() > 9999 {
		event.TimeState = TimeUnavailable
		return
	}
	event.RecordedAt = at.UTC().Format(time.RFC3339Nano)
	event.TimeState = TimeAvailable
}

func EventTimeOmissions(events []Event) []Omission {
	count := 0
	for _, event := range events {
		if event.TimeState == TimeAbsent || event.TimeState == TimeUnavailable || event.TimeState == TimeUnsupported {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return []Omission{{Code: "event_time_omitted", Scope: "events", Count: count, Message: "A recorded event time could not be emitted."}}
}

func ValidTimeState(state TimeState) bool {
	switch state {
	case TimeAvailable, TimeAbsent, TimeUnavailable, TimeUnsupported:
		return true
	default:
		return false
	}
}

func ValidateInteractionTime(value string) error {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || !strings.HasSuffix(value, "Z") || parsed.UTC().Format(time.RFC3339Nano) != value {
		return errors.New("invalid source interaction time")
	}
	return nil
}
