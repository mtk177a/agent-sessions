package contract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
)

const MaxEvidenceBytes = 64 << 20

var evidenceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,254}$`)
var verifiedValuePattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

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

// VerifiedVersionReader computes the v0 provider-content identifier without
// retaining the evidence in memory. It encodes one named chunk exactly as
// VerifiedVersion does.
func VerifiedVersionReader(name string, size uint64, reader io.Reader, maxBytes uint64) (VerifiedVersionID, error) {
	if !validEvidenceName(name) {
		return VerifiedVersionID{}, errors.New("invalid evidence chunk name")
	}
	if size > maxBytes || size > math.MaxInt64 {
		return VerifiedVersionID{}, errors.New("verified evidence exceeds size limit")
	}
	h := sha256.New()
	_, _ = h.Write([]byte("agent-sessions:verified-version:v0\x00"))
	writeUint64(h, uint64(len(name)))
	_, _ = h.Write([]byte(name))
	writeUint64(h, size)
	written, err := io.CopyN(h, reader, int64(size))
	if err != nil || uint64(written) != size {
		return VerifiedVersionID{}, errors.New("verified evidence is shorter than declared")
	}
	return VerifiedVersionID{Algorithm: "sha256", Basis: "provider-content-v0", Value: "sha256:" + hex.EncodeToString(h.Sum(nil))}, nil
}

func ValidVerifiedVersion(version VerifiedVersionID) bool {
	return version.Algorithm == "sha256" && version.Basis == "provider-content-v0" && verifiedValuePattern.MatchString(version.Value)
}

func validEvidenceName(name string) bool {
	if !evidenceNamePattern.MatchString(name) {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func writeLength(h hash.Hash, length int) {
	writeUint64(h, uint64(length))
}

func writeUint64(h hash.Hash, length uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], length)
	_, _ = h.Write(encoded[:])
}
