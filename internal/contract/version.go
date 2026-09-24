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

type EvidenceReader struct {
	Name   string
	Size   uint64
	Reader io.Reader
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
	return VerifiedVersionReaders([]EvidenceReader{{Name: name, Size: size, Reader: reader}}, maxBytes)
}

// VerifiedVersionReaders computes the v0 provider-content identifier from
// ordered, named readers without retaining their content in memory. Chunk names
// must already be in ascending order so callers cannot accidentally hash a
// different order from VerifiedVersion.
func VerifiedVersionReaders(chunks []EvidenceReader, maxBytes uint64) (VerifiedVersionID, error) {
	if len(chunks) == 0 {
		return VerifiedVersionID{}, errors.New("verified evidence is empty")
	}
	h := sha256.New()
	_, _ = h.Write([]byte("agent-sessions:verified-version:v0\x00"))
	var total uint64
	for i, chunk := range chunks {
		if !validEvidenceName(chunk.Name) {
			return VerifiedVersionID{}, errors.New("invalid evidence chunk name")
		}
		if i > 0 && chunks[i-1].Name >= chunk.Name {
			if chunks[i-1].Name == chunk.Name {
				return VerifiedVersionID{}, errors.New("duplicate evidence chunk name")
			}
			return VerifiedVersionID{}, errors.New("evidence chunks are not ordered")
		}
		if chunk.Size > maxBytes-total || chunk.Size > math.MaxInt64 {
			return VerifiedVersionID{}, errors.New("verified evidence exceeds size limit")
		}
		total += chunk.Size
		writeUint64(h, uint64(len(chunk.Name)))
		_, _ = h.Write([]byte(chunk.Name))
		writeUint64(h, chunk.Size)
		written, err := io.CopyN(h, chunk.Reader, int64(chunk.Size))
		if err != nil || uint64(written) != chunk.Size {
			return VerifiedVersionID{}, errors.New("verified evidence is shorter than declared")
		}
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
