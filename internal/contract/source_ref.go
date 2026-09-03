package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var tokenPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

type ParsedSourceRef struct {
	Provider       string
	SourceInstance string
	Fingerprint    string
}

func ValidIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

func ValidToken(value string) bool {
	return tokenPattern.MatchString(value)
}

func SourceFingerprint(nativeID string) string {
	sum := sha256.Sum256([]byte("agent-sessions:source-id:v0\x00" + nativeID))
	return hex.EncodeToString(sum[:])
}

func NewSourceRef(provider, instance, nativeID string) string {
	return "as0:" + provider + ":" + instance + ":" + SourceFingerprint(nativeID)
}

func ParseSourceRef(value string) (ParsedSourceRef, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 4 || parts[0] != "as0" || !ValidIdentifier(parts[1]) || !ValidIdentifier(parts[2]) {
		return ParsedSourceRef{}, errors.New("invalid source reference")
	}
	if len(parts[3]) != 64 {
		return ParsedSourceRef{}, errors.New("invalid source fingerprint")
	}
	if _, err := hex.DecodeString(parts[3]); err != nil {
		return ParsedSourceRef{}, errors.New("invalid source fingerprint")
	}
	return ParsedSourceRef{Provider: parts[1], SourceInstance: parts[2], Fingerprint: parts[3]}, nil
}
