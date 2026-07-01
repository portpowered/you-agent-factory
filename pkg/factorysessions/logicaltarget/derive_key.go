package logicaltarget

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const logicalSessionKeyPrefix = "lsk-"

// DeriveLogicalSessionKeyID returns a stable opaque identifier derived from
// ref. Equivalent canonical references always produce the same id, and the id
// changes when the normalized target's meaningful backend, provider, folder,
// or named-target boundary changes.
func DeriveLogicalSessionKeyID(ref CanonicalReference) string {
	sum := sha256.Sum256([]byte(ref.Signature()))
	return logicalSessionKeyPrefix + hex.EncodeToString(sum[:16])
}

// IsLogicalSessionKeyID reports whether value matches the opaque logical
// session key format exposed to API clients.
func IsLogicalSessionKeyID(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, logicalSessionKeyPrefix) {
		return false
	}
	payload := strings.TrimPrefix(trimmed, logicalSessionKeyPrefix)
	if len(payload) != 32 {
		return false
	}
	_, err := hex.DecodeString(payload)
	return err == nil
}
