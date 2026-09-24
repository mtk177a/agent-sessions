package codex

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/safeio"
)

const maxHistorySegments = 32

type historySpan struct {
	item artifact
	end  uint64
}

type historyRange struct {
	item  artifact
	start uint64
	end   uint64
}

type historyPlan struct {
	spans         []historySpan
	header        historyRange
	ranges        []historyRange
	logical       bool
	hint          string
	malformedRows int
	oversizedRows int
}

// resolveHistory describes the inherited prefixes and current rollout, oldest
// first. HistoryPosition.thread_id identifies a rollout file, not necessarily a
// logical thread.
func resolveHistory(selected artifact, byRollout map[string][]artifact) ([]historySpan, error) {
	spans := make([]historySpan, 0, 2)
	seen := map[string]bool{}
	item := selected
	if item.size < 0 {
		return nil, errors.New("negative rollout size")
	}
	end := uint64(item.size)
	for {
		if len(spans) == maxHistorySegments || seen[item.rolloutID] || item.compressed {
			return nil, errors.New("invalid rollout history chain")
		}
		seen[item.rolloutID] = true
		spans = append(spans, historySpan{item: item, end: end})
		base := item.meta.HistoryBase
		if base == nil {
			break
		}
		if item.meta.HistoryMode != "paginated" {
			return nil, errors.New("referenced history is not paginated")
		}
		id, valid := canonicalThreadID(base.ThreadID)
		if !valid || len(byRollout[id]) != 1 {
			return nil, errors.New("missing or ambiguous rollout history base")
		}
		item = byRollout[id][0]
		if item.meta.HistoryMode != "paginated" {
			return nil, errors.New("referenced history is not paginated")
		}
		end = base.EndByteOffset
	}
	slices.Reverse(spans)
	return spans, nil
}

// walkHistory validates the physical history and visits only rows belonging to
// the selected logical session. A subagent start ordinal excludes its copied
// parent prefix. The selected header remains control evidence.
func walkHistory(root string, spans []historySpan, maxBytes, maxRowBytes int64, requireOrdinals bool, visit func(artifact, rolloutLine)) (historyPlan, error) {
	if len(spans) == 0 || maxBytes <= 0 || maxRowBytes <= 0 {
		return historyPlan{}, errors.New("invalid history reader limits")
	}
	var total uint64
	selected := spans[len(spans)-1].item
	logical := len(spans) > 1 || selected.meta.SubagentHistoryStartOrdinal != nil
	for _, span := range spans {
		if span.end > uint64(maxBytes)-total {
			return historyPlan{}, errors.New("effective history exceeds size limit")
		}
		total += span.end
		requireOrdinals = requireOrdinals || logical && span.item.meta.HistoryMode == "paginated"
	}
	plan := historyPlan{spans: spans, logical: logical}
	h := sha256.New()
	if logical {
		_, _ = h.Write([]byte("agent-sessions:codex-version-hint:v2\x00"))
	}
	var nextOrdinal uint64
	for i, span := range spans {
		current := i == len(spans)-1
		if err := scanHistorySpan(root, span, maxRowBytes, requireOrdinals, current, selected, &nextOrdinal, &plan, h, visit); err != nil {
			return historyPlan{}, err
		}
		if i+1 < len(spans) && nextOrdinal != spans[i+1].item.meta.HistoryBase.EndOrdinalExclusive {
			return historyPlan{}, errors.New("history ordinal boundary does not match")
		}
	}
	if boundary := selected.meta.SubagentHistoryStartOrdinal; boundary != nil && *boundary > nextOrdinal {
		return historyPlan{}, errors.New("subagent history boundary is beyond the rollout")
	}
	if logical {
		plan.hint = "sha256:" + hex.EncodeToString(h.Sum(nil))
	}
	return plan, nil
}

func scanHistorySpan(root string, span historySpan, maxRowBytes int64, requireOrdinals, current bool, selected artifact, nextOrdinal *uint64, plan *historyPlan, hint io.Writer, visit func(artifact, rolloutLine)) error {
	file, err := safeio.OpenRegularWithin(root, span.item.relative)
	if err != nil {
		return err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || before.Size() < 0 || span.end > uint64(before.Size()) {
		return errors.New("rollout history is shorter than its boundary")
	}
	if current && uint64(before.Size()) != span.end {
		return errors.New("current rollout changed during discovery")
	}
	if span.end > 0 && !current {
		var boundary [1]byte
		if _, err := file.ReadAt(boundary[:], int64(span.end-1)); err != nil || boundary[0] != '\n' {
			return errors.New("history byte boundary is not a JSONL boundary")
		}
	}

	limited := &io.LimitedReader{R: file, N: int64(span.end)}
	reader := bufio.NewReaderSize(limited, 64<<10)
	first := true
	var offset uint64
	for limited.N > 0 || reader.Buffered() > 0 {
		raw, oversized, readErr := readBoundedJSONLRow(reader, maxRowBytes)
		if len(raw) == 0 && readErr == io.EOF && !oversized {
			break
		}
		if oversized {
			if first || plan.logical || requireOrdinals {
				return errors.New("rollout row exceeds size limit")
			}
			plan.oversizedRows++
			if readErr == io.EOF {
				break
			}
			continue
		}
		var line rolloutLine
		if err := safeio.DecodeJSON(bytes.TrimSuffix(raw, []byte{'\n'}), contract.MaxJSONDepth, &line); err != nil {
			if first || plan.logical || requireOrdinals {
				return err
			}
			plan.malformedRows++
			offset += uint64(len(raw))
			if readErr == io.EOF {
				break
			}
			continue
		}
		start, end := offset, offset+uint64(len(raw))
		offset = end
		if first {
			if line.Type != "session_meta" {
				return errors.New("rollout header missing")
			}
			meta, err := decodeHistoryMeta(line)
			if err != nil || !strings.EqualFold(meta.ID, span.item.threadID) || !reflect.DeepEqual(meta, span.item.meta) {
				return errors.New("rollout header changed after discovery")
			}
			first = false
		}
		if requireOrdinals {
			if line.Ordinal == nil || *line.Ordinal != *nextOrdinal {
				return errors.New("rollout ordinal is missing or discontinuous")
			}
			*nextOrdinal++
		}

		selectedHeader := span.item.relative == selected.relative && line.Type == "session_meta"
		included := line.Type != "session_meta"
		if boundary := selected.meta.SubagentHistoryStartOrdinal; boundary != nil {
			included = included && line.Ordinal != nil && *line.Ordinal >= *boundary
		}
		if selectedHeader {
			plan.header = historyRange{item: span.item, start: start, end: end}
			if plan.logical {
				writeHintPart(hint, "header", raw)
			}
			visit(span.item, line)
		} else if included {
			appendHistoryRange(plan, span.item, start, end)
			if plan.logical {
				writeHintPart(hint, "row", raw)
			}
			visit(span.item, line)
		} else if !plan.logical {
			visit(span.item, line)
		}
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if readErr == io.EOF {
			break
		}
	}
	if limited.N != 0 || (span.end != 0 && first) {
		return errors.New("rollout history could not be read within limits")
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("rollout history changed while reading")
	}
	return nil
}

func readBoundedJSONLRow(reader *bufio.Reader, maxBytes int64) ([]byte, bool, error) {
	var raw []byte
	var size int64
	oversized := false
	for {
		fragment, err := reader.ReadSlice('\n')
		size += int64(len(fragment))
		if !oversized && size <= maxBytes {
			raw = append(raw, fragment...)
		} else {
			oversized = true
			raw = nil
		}
		switch err {
		case nil:
			return raw, oversized, nil
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			return raw, oversized, io.EOF
		default:
			return nil, oversized, err
		}
	}
}

func appendHistoryRange(plan *historyPlan, item artifact, start, end uint64) {
	if len(plan.ranges) > 0 {
		last := &plan.ranges[len(plan.ranges)-1]
		if last.item.relative == item.relative && last.end == start {
			last.end = end
			return
		}
	}
	plan.ranges = append(plan.ranges, historyRange{item: item, start: start, end: end})
}

func writeHintPart(w io.Writer, kind string, raw []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(kind)))
	_, _ = w.Write(length[:])
	_, _ = w.Write([]byte(kind))
	binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
	_, _ = w.Write(length[:])
	_, _ = w.Write(raw)
}

func decodeHistoryMeta(line rolloutLine) (sessionMeta, error) {
	var meta sessionMeta
	if err := safeio.DecodeJSON(line.Payload, contract.MaxJSONDepth, &meta); err != nil {
		return sessionMeta{}, err
	}
	if meta.HistoryMode == "" {
		meta.HistoryMode = "legacy"
	}
	return meta, nil
}
