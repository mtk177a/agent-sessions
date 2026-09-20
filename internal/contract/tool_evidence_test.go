package contract

import (
	"strings"
	"testing"
)

func TestSafeToolExcerptSelectsOnlyWholeSafeLines(t *testing.T) {
	excerpt, state, redacted, truncated := SafeToolExcerpt("PASS\n3 tests passed\nsecret-token: fictional\n/fictional/private\nPASS; token=fake\n", true)
	if excerpt != "PASS\n3 tests passed" || state != EvidenceAvailable || !redacted || truncated {
		t.Fatalf("excerpt = %q, %q, %v, %v", excerpt, state, redacted, truncated)
	}
	if excerpt, state, redacted, truncated = SafeToolExcerpt("", false); excerpt != "" || state != EvidenceAbsent || redacted || truncated {
		t.Fatalf("absent excerpt = %q, %q, %v, %v", excerpt, state, redacted, truncated)
	}
	if excerpt, state, redacted, truncated = SafeToolExcerpt("private output", true); excerpt != "" || state != EvidenceUnavailable || !redacted || truncated {
		t.Fatalf("unsafe excerpt = %q, %q, %v, %v", excerpt, state, redacted, truncated)
	}
	input := strings.Repeat("3 tests passed\n", 60)
	excerpt, state, redacted, truncated = SafeToolExcerpt(input, true)
	if len(excerpt) > MaxToolExcerptBytes || state != EvidenceAvailable || redacted || !truncated {
		t.Fatalf("bounded excerpt = %d, %q, %v, %v", len(excerpt), state, redacted, truncated)
	}
}

func TestSafeToolExcerptRejectsAdversarialLines(t *testing.T) {
	for _, body := range []string{
		"PASS token=fake", "PASS /fictional/private", "PASS host.example.invalid", "$ PASS", "PASS\x1b[0m", "4 tests passed; secret=fake", "9 tests passed by Alice", "9223372036854775807 tests passed",
	} {
		excerpt, state, redacted, _ := SafeToolExcerpt(body, true)
		if excerpt != "" || state != EvidenceUnavailable || !redacted {
			t.Fatalf("unsafe body %q produced %q, %q, %v", body, excerpt, state, redacted)
		}
	}
}
