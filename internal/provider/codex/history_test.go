package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mtk177a/agent-sessions/internal/contract"
)

func TestInheritedHistoryTimeAndHintIgnoreExcludedSuffix(t *testing.T) {
	home := t.TempDir()
	basePath := rolloutPath(home, "sessions", testThreadID)
	base := []string{
		withOrdinal(0, header(testThreadID, "0.153.4", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
		withOrdinal(2, completedMessage("2026-09-03T23:00:00Z", "AgentMessage")),
	}
	writeRollout(t, basePath, base)
	cutoff := len(base[0]) + 1 + len(base[1]) + 1
	extra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, testThreadID, cutoff)
	childPath := rolloutPath(home, "sessions", secondThreadID)
	writeRollout(t, childPath, []string{
		withOrdinal(2, header(secondThreadID, "0.153.4", "paginated", extra)),
		withOrdinal(3, completedMessage("2026-09-03T11:00:00Z", "AgentMessage")),
	})
	read := func() (string, string) {
		t.Helper()
		result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(secondThreadID))
		if result.Status != contract.StatusComplete || len(result.Sources) != 1 || result.Sources[0].LastInteractionAt == nil || result.Sources[0].VersionHint == nil {
			t.Fatalf("Show() = %#v", result)
		}
		return *result.Sources[0].LastInteractionAt, result.Sources[0].VersionHint.Value
	}
	gotTime, hint := read()
	if gotTime != "2026-09-03T11:00:00Z" {
		t.Fatalf("inherited time = %q", gotTime)
	}
	file, err := os.OpenFile(basePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(withOrdinal(3, completedMessage("2026-09-04T00:00:00Z", "AgentMessage")) + "\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	gotTime, laterHint := read()
	if gotTime != "2026-09-03T11:00:00Z" || laterHint != hint {
		t.Fatalf("excluded suffix changed source: time=%q hint=%q", gotTime, laterHint)
	}
	updated := strings.Replace(base[1], "10:00:00", "10:00:01", 1)
	base[1] = updated
	writeRollout(t, basePath, base)
	_, changedHint := read()
	if changedHint == hint {
		t.Fatal("inherited prefix changed without changing version hint")
	}
}

func TestInheritedHistoryReadsVerifiedOlderTimeFormat(t *testing.T) {
	home := t.TempDir()
	base := []string{
		withOrdinal(0, header(testThreadID, "0.147.0", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
	}
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), base)
	cutoff := len(base[0]) + 1 + len(base[1]) + 1
	extra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, testThreadID, cutoff)
	writeRollout(t, rolloutPath(home, "sessions", secondThreadID), []string{
		withOrdinal(2, header(secondThreadID, "0.153.4", "paginated", extra)),
		withOrdinal(3, `{"timestamp":"2026-09-03T11:00:00Z","type":"event_msg","payload":{"type":"token_count"}}`),
	})
	result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(secondThreadID))
	if len(result.Sources) != 1 || result.Sources[0].LastInteractionAt == nil || *result.Sources[0].LastInteractionAt != "2026-09-03T10:00:00Z" || result.Sources[0].VersionHint == nil {
		t.Fatalf("Show() = %#v", result)
	}
}

func TestInheritedHistoryReadsMigratedOlderPrefixes(t *testing.T) {
	home := t.TempDir()
	basePath := rolloutPath(home, "sessions", testThreadID)
	base := []string{
		withOrdinal(0, header(testThreadID, "0.98.0", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
		withOrdinal(2, completedMessage("2026-09-03T23:00:00Z", "AgentMessage")),
	}
	writeRollout(t, basePath, base)
	cutoff := len(base[0]) + 1 + len(base[1]) + 1
	extra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, testThreadID, cutoff)
	writeRollout(t, rolloutPath(home, "sessions", secondThreadID), []string{
		withOrdinal(2, header(secondThreadID, "0.117.0", "paginated", extra)),
		withOrdinal(3, `{"timestamp":"2026-09-03T11:00:00Z","type":"response_item","payload":{"type":"web_search_call"}}`),
	})
	read := func() (string, string) {
		t.Helper()
		result := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(secondThreadID))
		if result.Status != contract.StatusComplete || len(result.Sources) != 1 || result.Sources[0].LastInteractionAt == nil || result.Sources[0].VersionHint == nil || result.Sources[0].VersionHint.Kind != "content_hash" {
			t.Fatalf("Show() = %#v", result)
		}
		return *result.Sources[0].LastInteractionAt, result.Sources[0].VersionHint.Value
	}
	gotTime, hint := read()
	if gotTime != "2026-09-03T11:00:00Z" {
		t.Fatalf("time = %q", gotTime)
	}
	base = append(base, withOrdinal(3, completedMessage("2026-09-04T00:00:00Z", "AgentMessage")))
	writeRollout(t, basePath, base)
	gotTime, laterHint := read()
	if gotTime != "2026-09-03T11:00:00Z" || laterHint != hint {
		t.Fatalf("excluded suffix affected history: time=%q hint=%q", gotTime, laterHint)
	}
	base[1] = withOrdinal(1, completedMessage("2026-09-03T10:00:01Z", "UserMessage"))
	writeRollout(t, basePath, base)
	_, changedHint := read()
	if changedHint == hint {
		t.Fatal("included prefix changed without changing version hint")
	}
}

func withOrdinal(ordinal int, row string) string {
	return fmt.Sprintf(`{"ordinal":%d,`, ordinal) + row[1:]
}

func completedMessage(timestamp, itemType string) string {
	return fmt.Sprintf(`{"timestamp":"%s","type":"event_msg","payload":{"type":"item_completed","item":{"type":"%s","id":"fictional-item","content":[]}}}`, timestamp, itemType)
}

func rolloutPathWithID(home, timestamp, threadID, rolloutID string) string {
	return filepath.Join(home, "sessions", "rollout-"+timestamp+"-"+threadID+"_"+rolloutID+".jsonl")
}

func TestRevertedRolloutChainUsesRolloutIDsAndOriginVersions(t *testing.T) {
	const finalRolloutID = "018f47e2-7b0a-7d31-8a13-7dc76c914abe"
	home := t.TempDir()
	original := []string{
		withOrdinal(0, header(testThreadID, "0.153.3", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
	}
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), original)
	originalEnd := len(original[0]) + 1 + len(original[1]) + 1
	middleExtra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, testThreadID, originalEnd)
	middle := []string{
		withOrdinal(2, header(testThreadID, "0.153.4", "paginated", middleExtra)),
		withOrdinal(3, completedExtension("2026-09-03T12:00:00Z", "clock.sleep")),
	}
	middlePath := rolloutPathWithID(home, "2026-09-04T10-00-00", testThreadID, secondThreadID)
	writeRollout(t, middlePath, middle)
	middleEnd := len(middle[0]) + 1 + len(middle[1]) + 1
	finalExtra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":4,"end_byte_offset":%d}`, secondThreadID, middleEnd)
	writeRollout(t, rolloutPathWithID(home, "2026-09-05T10-00-00", testThreadID, finalRolloutID), []string{
		withOrdinal(4, header(testThreadID, "0.155.0-alpha.9.2", "paginated", finalExtra)),
		withOrdinal(5, completedMessage("2026-09-03T13:00:00Z", "AgentMessage")),
		withOrdinal(6, completedExtension("2026-09-03T14:00:00Z", "web.search")),
		withOrdinal(7, `{"timestamp":"2026-09-03T15:00:00Z","type":"token_usage_record","payload":{}}`),
	})
	source := testSource(home)
	fingerprint := contract.SourceFingerprint(testThreadID)
	shown := New().Show(context.Background(), source, fingerprint)
	if shown.Status != contract.StatusComplete || shown.Sources[0].LastInteractionAt == nil || *shown.Sources[0].LastInteractionAt != "2026-09-03T14:00:00Z" || shown.Sources[0].VersionHint == nil || shown.Sources[0].VersionHint.Kind != "content_hash" {
		t.Fatalf("Show() = %#v", shown)
	}
	if events := New().Events(context.Background(), source, fingerprint); events.Status != contract.StatusPartial || hasOmission(events.Omissions, "unsupported_format") || len(events.Events) != 1 {
		t.Fatalf("Events() = %#v", events)
	}
	if evidence := New().Evidence(context.Background(), source, fingerprint); evidence.Status != contract.StatusComplete || evidence.VerifiedVersion == nil {
		t.Fatalf("Evidence() = %#v", evidence)
	}
}

func completedExtension(timestamp, kind string) string {
	return fmt.Sprintf(`{"timestamp":"%s","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Extension","kind":"%s","id":"fictional-item"}}}`, timestamp, kind)
}

func TestUnsafeInheritedHistoryOmitsTimeAndHint(t *testing.T) {
	for _, tc := range []struct {
		name         string
		baseID       string
		ordinal      int
		adjustOffset int
		duplicate    bool
		baseVersion  string
		itemKind     string
	}{
		{name: "missing", baseID: "018f47e2-7b0a-7d31-8a13-7dc76c914abe", ordinal: 2},
		{name: "cycle", baseID: secondThreadID, ordinal: 2},
		{name: "duplicate", baseID: testThreadID, ordinal: 2, duplicate: true},
		{name: "wrong_ordinal", baseID: testThreadID, ordinal: 3},
		{name: "mid_row_offset", baseID: testThreadID, ordinal: 2, adjustOffset: -1},
		{name: "unverified_base", baseID: testThreadID, ordinal: 2, baseVersion: "9.9.9"},
		{name: "unknown_extension", baseID: testThreadID, ordinal: 2, itemKind: "future.tool"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			version := tc.baseVersion
			if version == "" {
				version = "0.153.4"
			}
			base := []string{
				withOrdinal(0, header(testThreadID, version, "paginated", "")),
				withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
			}
			if tc.itemKind != "" {
				base[1] = withOrdinal(1, completedExtension("2026-09-03T10:00:00Z", tc.itemKind))
			}
			writeRollout(t, rolloutPath(home, "sessions", testThreadID), base)
			if tc.duplicate {
				writeRollout(t, rolloutPath(home, "archived_sessions", testThreadID), base)
			}
			offset := len(base[0]) + 1 + len(base[1]) + 1 + tc.adjustOffset
			extra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d}`, tc.baseID, offset)
			writeRollout(t, rolloutPath(home, "sessions", secondThreadID), []string{
				withOrdinal(tc.ordinal, header(secondThreadID, "0.153.4", "paginated", extra)),
				withOrdinal(tc.ordinal+1, completedMessage("2026-09-03T11:00:00Z", "AgentMessage")),
			})
			shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(secondThreadID))
			if shown.Status != contract.StatusPartial || len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt != nil || !hasOmission(shown.Omissions, "source_time_unavailable") {
				t.Fatalf("Show() = %#v", shown)
			}
			expectHint := tc.itemKind != "" || tc.baseVersion != ""
			if (shown.Sources[0].VersionHint != nil) != expectHint {
				t.Fatalf("unexpected hint for %s: %#v", tc.name, shown.Sources[0].VersionHint)
			}
		})
	}
}

func TestInteractionTimeAcceptsBoundedLargeToolRow(t *testing.T) {
	home := t.TempDir()
	large := strings.Repeat("x", 1<<20)
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		header(testThreadID, "0.153.4", "paginated", ""),
		`{"timestamp":"2026-09-03T10:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"fictional-tool","aggregated_output":"` + large + `"}}}`,
	})
	shown := New().Show(context.Background(), testSource(home), contract.SourceFingerprint(testThreadID))
	if shown.Status != contract.StatusComplete || shown.Sources[0].LastInteractionAt == nil || *shown.Sources[0].LastInteractionAt != "2026-09-03T10:00:00Z" {
		t.Fatalf("Show() = %#v", shown)
	}
}

func TestHistoryReaderUsesCallerLimits(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		withOrdinal(0, header(testThreadID, "0.153.4", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
	})
	discovered, err := New().discover(testSource(home))
	if err != nil {
		t.Fatal(err)
	}
	spans, err := resolveHistory(discovered.byFingerprint[contract.SourceFingerprint(testThreadID)][0], discovered.byRollout)
	if err != nil {
		t.Fatal(err)
	}
	for _, limits := range [][2]int64{{1, maxTimeRowBytes}, {maxTimeHistoryBytes, 64}} {
		if _, err := walkHistory(home, spans, limits[0], limits[1], false, func(artifact, rolloutLine) {}); err == nil {
			t.Fatalf("walkHistory succeeded with limits %v", limits)
		}
	}
}

func TestSubagentHistoryUsesOnlyChildOwnedRows(t *testing.T) {
	home := t.TempDir()
	extra := `,"subagent_history_start_ordinal":3`
	path := rolloutPath(home, "sessions", testThreadID)
	rows := []string{
		withOrdinal(0, header(testThreadID, "0.153.4", "paginated", extra)),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
		withOrdinal(2, completedMessage("2026-09-03T11:00:00Z", "AgentMessage")),
		withOrdinal(3, completedMessage("2026-09-03T12:00:00Z", "AgentMessage")),
	}
	writeRollout(t, path, rows)
	adapter := New()
	source := testSource(home)
	fingerprint := contract.SourceFingerprint(testThreadID)
	shown := adapter.Show(context.Background(), source, fingerprint)
	if shown.Status != contract.StatusComplete || shown.Sources[0].LastInteractionAt == nil || *shown.Sources[0].LastInteractionAt != "2026-09-03T12:00:00Z" || shown.Sources[0].VersionHint == nil || shown.Sources[0].VersionHint.Kind != "content_hash" {
		t.Fatalf("Show() = %#v", shown)
	}
	events := adapter.Events(context.Background(), source, fingerprint)
	if events.Status != contract.StatusComplete || len(events.Events) != 1 || events.Events[0].Message == nil || events.Events[0].Message.Role != "assistant" {
		t.Fatalf("Events() = %#v", events)
	}
	evidence := adapter.Evidence(context.Background(), source, fingerprint)
	if evidence.Status != contract.StatusComplete || evidence.VerifiedVersion == nil || len(evidence.Chunks) != 0 {
		t.Fatalf("Evidence() = %#v", evidence)
	}
	hint, verified := shown.Sources[0].VersionHint.Value, evidence.VerifiedVersion.Value
	rows[1] = strings.Replace(rows[1], "10:00:00", "10:00:01", 1)
	writeRollout(t, path, rows)
	shown = adapter.Show(context.Background(), source, fingerprint)
	evidence = adapter.Evidence(context.Background(), source, fingerprint)
	if shown.Sources[0].VersionHint == nil || shown.Sources[0].VersionHint.Value != hint || evidence.VerifiedVersion == nil || evidence.VerifiedVersion.Value != verified {
		t.Fatalf("excluded parent prefix changed child identity: show=%#v evidence=%#v", shown, evidence)
	}
}

func TestInactiveSubagentRemainsListedWithoutInteractionTime(t *testing.T) {
	home := t.TempDir()
	extra := `,"subagent_history_start_ordinal":3`
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		withOrdinal(0, header(testThreadID, "0.153.4", "paginated", extra)),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
		withOrdinal(2, completedMessage("2026-09-03T11:00:00Z", "AgentMessage")),
	})
	adapter := New()
	source := testSource(home)
	fingerprint := contract.SourceFingerprint(testThreadID)
	shown := adapter.Show(context.Background(), source, fingerprint)
	if len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt != nil || !hasOmission(shown.Omissions, "source_time_unavailable") {
		t.Fatalf("Show() = %#v", shown)
	}
	events := adapter.Events(context.Background(), source, fingerprint)
	if events.Status != contract.StatusComplete || len(events.Events) != 0 {
		t.Fatalf("Events() = %#v", events)
	}
	evidence := adapter.Evidence(context.Background(), source, fingerprint)
	if evidence.Status != contract.StatusComplete || evidence.VerifiedVersion == nil {
		t.Fatalf("Evidence() = %#v", evidence)
	}
}

func TestExcludedUnsupportedParentDoesNotPoisonSubagent(t *testing.T) {
	home := t.TempDir()
	base := []string{
		withOrdinal(0, header(testThreadID, "9.9.9", "paginated", "")),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "UserMessage")),
	}
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), base)
	cutoff := len(base[0]) + len(base[1]) + 2
	extra := fmt.Sprintf(`,"history_base":{"thread_id":"%s","end_ordinal_exclusive":2,"end_byte_offset":%d},"subagent_history_start_ordinal":3`, testThreadID, cutoff)
	writeRollout(t, rolloutPath(home, "sessions", secondThreadID), []string{
		withOrdinal(2, header(secondThreadID, "0.153.4", "paginated", extra)),
		withOrdinal(3, completedMessage("2026-09-03T12:00:00Z", "AgentMessage")),
	})
	adapter := New()
	source := testSource(home)
	fingerprint := contract.SourceFingerprint(secondThreadID)
	shown := adapter.Show(context.Background(), source, fingerprint)
	if shown.Status != contract.StatusComplete || shown.Sources[0].LastInteractionAt == nil || *shown.Sources[0].LastInteractionAt != "2026-09-03T12:00:00Z" {
		t.Fatalf("Show() = %#v", shown)
	}
	if events := adapter.Events(context.Background(), source, fingerprint); events.Status != contract.StatusComplete || len(events.Events) != 1 {
		t.Fatalf("Events() = %#v", events)
	}
	if evidence := adapter.Evidence(context.Background(), source, fingerprint); evidence.Status != contract.StatusComplete || evidence.VerifiedVersion == nil {
		t.Fatalf("Evidence() = %#v", evidence)
	}
}

func TestInvalidSubagentBoundaryFailsClosed(t *testing.T) {
	home := t.TempDir()
	writeRollout(t, rolloutPath(home, "sessions", testThreadID), []string{
		withOrdinal(0, header(testThreadID, "0.153.4", "paginated", `,"subagent_history_start_ordinal":9`)),
		withOrdinal(1, completedMessage("2026-09-03T10:00:00Z", "AgentMessage")),
	})
	adapter := New()
	source := testSource(home)
	fingerprint := contract.SourceFingerprint(testThreadID)
	shown := adapter.Show(context.Background(), source, fingerprint)
	if len(shown.Sources) != 1 || shown.Sources[0].LastInteractionAt != nil || shown.Sources[0].VersionHint != nil || !hasOmission(shown.Omissions, "source_time_unavailable") {
		t.Fatalf("Show() = %#v", shown)
	}
	if events := adapter.Events(context.Background(), source, fingerprint); events.Status != contract.StatusUnsupported || !hasOmission(events.Omissions, "unsupported_format") {
		t.Fatalf("Events() = %#v", events)
	}
	if evidence := adapter.Evidence(context.Background(), source, fingerprint); evidence.Status != contract.StatusUnsupported || !hasOmission(evidence.Omissions, "unsupported_format") {
		t.Fatalf("Evidence() = %#v", evidence)
	}
}
