package contract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"regexp"
	"sort"
	"strings"
)

const MaxEvidenceBytes = 64 << 20

var evidenceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,254}$`)

type EvidenceChunk struct {
	Name    string
	Content []byte
}

func VerifiedVersion(chunks []EvidenceChunk) (VerifiedVersionID, error) {
	if len(chunks) == 0 {
		return VerifiedVersionID{}, errors.New("verified evidence is empty")
	}
	ordered := append([]EvidenceChunk(nil), chunks...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	total := 0
	h := sha256.New()
	_, _ = h.Write([]byte("agent-sessions:verified-version:v0\x00"))
	for i, chunk := range ordered {
		if !evidenceNamePattern.MatchString(chunk.Name) {
			return VerifiedVersionID{}, errors.New("invalid evidence chunk name")
		}
		for _, segment := range strings.Split(chunk.Name, "/") {
			if segment == "." || segment == ".." {
				return VerifiedVersionID{}, errors.New("invalid evidence chunk name")
			}
		}
		if i > 0 && ordered[i-1].Name == chunk.Name {
			return VerifiedVersionID{}, errors.New("duplicate evidence chunk name")
		}
		total += len(chunk.Content)
		if total > MaxEvidenceBytes {
			return VerifiedVersionID{}, errors.New("verified evidence exceeds size limit")
		}
		writeLength(h, len(chunk.Name))
		_, _ = h.Write([]byte(chunk.Name))
		writeLength(h, len(chunk.Content))
		_, _ = h.Write(chunk.Content)
	}
	return VerifiedVersionID{Algorithm: "sha256", Basis: "provider-content-v0", Value: "sha256:" + hex.EncodeToString(h.Sum(nil))}, nil
}

func writeLength(h hash.Hash, length int) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(length))
	_, _ = h.Write(encoded[:])
}
