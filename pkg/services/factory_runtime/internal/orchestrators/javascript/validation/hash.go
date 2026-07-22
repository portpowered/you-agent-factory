package workflowvalidation

import (
	"crypto/sha256"
	"encoding/hex"
)

// SourceHash returns a stable sha256 digest for authored workflow source bytes.
func SourceHash(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
