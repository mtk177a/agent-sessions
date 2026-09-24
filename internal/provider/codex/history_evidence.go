package codex

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mtk177a/agent-sessions/internal/contract"
	"github.com/mtk177a/agent-sessions/internal/safeio"
)

type openedHistoryRange struct {
	file   *os.File
	before os.FileInfo
	item   artifact
}

func verifiedArtifact(root string, item artifact) (contract.VerifiedVersionID, error) {
	file, err := safeio.OpenRegularWithin(root, item.relative)
	if err != nil {
		return contract.VerifiedVersionID{}, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || before.Size() != item.size || !before.ModTime().Equal(item.modTime) {
		return contract.VerifiedVersionID{}, errors.New("rollout changed before verification")
	}
	verified, err := contract.VerifiedVersionReader("rollout/primary.jsonl", uint64(item.size), file, uint64(maxTimeHistoryBytes))
	if err != nil {
		return contract.VerifiedVersionID{}, err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return contract.VerifiedVersionID{}, errors.New("rollout changed while verifying")
	}
	return verified, nil
}

func verifiedHistory(root string, plan historyPlan) (contract.VerifiedVersionID, error) {
	ranges := make([]historyRange, 0, 1+len(plan.ranges))
	if plan.header.end <= plan.header.start {
		return contract.VerifiedVersionID{}, errors.New("selected rollout header is unavailable")
	}
	ranges = append(ranges, plan.header)
	ranges = append(ranges, plan.ranges...)
	readers := make([]contract.EvidenceReader, 0, len(ranges))
	opened := make([]openedHistoryRange, 0, len(ranges))
	defer func() {
		for _, entry := range opened {
			_ = entry.file.Close()
		}
	}()
	for i, span := range ranges {
		file, err := safeio.OpenRegularWithin(root, span.item.relative)
		if err != nil {
			return contract.VerifiedVersionID{}, err
		}
		before, err := file.Stat()
		if err != nil || before.Size() != span.item.size || !before.ModTime().Equal(span.item.modTime) || span.end > uint64(before.Size()) {
			_ = file.Close()
			return contract.VerifiedVersionID{}, errors.New("rollout history changed before verification")
		}
		opened = append(opened, openedHistoryRange{file: file, before: before, item: span.item})
		kind := "history"
		if i == 0 {
			kind = "header"
		}
		name := fmt.Sprintf("rollout/%03d-%s.jsonl", i, kind)
		readers = append(readers, contract.EvidenceReader{Name: name, Size: span.end - span.start, Reader: io.NewSectionReader(file, int64(span.start), int64(span.end-span.start))})
	}
	verified, err := contract.VerifiedVersionReaders(readers, uint64(maxTimeHistoryBytes))
	if err != nil {
		return contract.VerifiedVersionID{}, err
	}
	for _, entry := range opened {
		after, err := entry.file.Stat()
		if err != nil || !os.SameFile(entry.before, after) || entry.before.Size() != after.Size() || !entry.before.ModTime().Equal(after.ModTime()) {
			return contract.VerifiedVersionID{}, errors.New("rollout history changed while verifying")
		}
	}
	return verified, nil
}
