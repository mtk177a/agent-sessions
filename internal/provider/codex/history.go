package codex

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
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

// resolveHistory describes the effective prefix of each rollout, oldest first.
// HistoryPosition.thread_id identifies a rollout file, not necessarily a thread.
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

// walkHistory validates ordinal and byte boundaries while streaming effective rows.
// Its limits belong to the caller rather than the shared history representation.
func walkHistory(root string, spans []historySpan, maxBytes, maxRowBytes int64, visit func(artifact, rolloutLine)) (string, error) {
	if len(spans) == 0 || maxBytes <= 0 || maxRowBytes <= 0 {
		return "", errors.New("invalid history reader limits")
	}
	var total uint64
	for _, span := range spans {
		if span.end > uint64(maxBytes)-total {
			return "", errors.New("effective history exceeds size limit")
		}
		total += span.end
	}
	h := sha256.New()
	_, _ = h.Write([]byte("agent-sessions:codex-version-hint:v1\x00"))
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(spans)))
	_, _ = h.Write(count[:])
	requireOrdinals := len(spans) > 1
	var nextOrdinal uint64
	for i, span := range spans {
		if err := walkHistorySpan(root, span, maxRowBytes, requireOrdinals, i == len(spans)-1, &nextOrdinal, h, visit); err != nil {
			return "", err
		}
		if i+1 < len(spans) && nextOrdinal != spans[i+1].item.meta.HistoryBase.EndOrdinalExclusive {
			return "", errors.New("history ordinal boundary does not match")
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func walkHistorySpan(root string, span historySpan, maxRowBytes int64, requireOrdinals, current bool, nextOrdinal *uint64, h hash.Hash, visit func(artifact, rolloutLine)) error {
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
		// A current rollout is always read to the size observed during discovery.
		return errors.New("current rollout changed during discovery")
	}
	if span.end > 0 && !current {
		var boundary [1]byte
		if _, err := file.ReadAt(boundary[:], int64(span.end-1)); err != nil || boundary[0] != '\n' {
			return errors.New("history byte boundary is not a JSONL boundary")
		}
	}
	_, _ = h.Write([]byte(span.item.rolloutID))
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], span.end)
	_, _ = h.Write(length[:])
	limited := &io.LimitedReader{R: file, N: int64(span.end)}
	scanner := bufio.NewScanner(io.TeeReader(limited, h))
	initialBuffer := int(maxRowBytes)
	if initialBuffer > 64<<10 {
		initialBuffer = 64 << 10
	}
	scanner.Buffer(make([]byte, initialBuffer), int(maxRowBytes))
	first := true
	for scanner.Scan() {
		var line rolloutLine
		if err := safeio.DecodeJSON(scanner.Bytes(), contract.MaxJSONDepth, &line); err != nil {
			return err
		}
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
		visit(span.item, line)
	}
	if scanner.Err() != nil || limited.N != 0 || (span.end != 0 && first) {
		return errors.New("rollout history could not be read within limits")
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("rollout history changed while reading")
	}
	return nil
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
