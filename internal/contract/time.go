package contract

import (
	"errors"
	"strings"
	"time"
)

func ValidateInteractionTime(value string) error {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || !strings.HasSuffix(value, "Z") || parsed.UTC().Format(time.RFC3339Nano) != value {
		return errors.New("invalid source interaction time")
	}
	return nil
}
