package logicaltarget

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// DeriveLogicalSessionKeyID returns a stable opaque identifier derived from
// ref. Equivalent canonical references always produce the same id, and the id
// changes when the normalized target's meaningful backend, provider, folder,
// or named-target boundary changes.
func DeriveLogicalSessionKeyID(ref CanonicalReference) string {
	sum := sha256.Sum256([]byte(signature(ref)))
	return "lsk-" + hex.EncodeToString(sum[:16])
}

// IsLogicalSessionKeyID reports whether value matches the opaque logical
// session key format exposed to API clients.
func IsLogicalSessionKeyID(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "lsk-") {
		return false
	}
	payload := strings.TrimPrefix(trimmed, "lsk-")
	if len(payload) != 32 {
		return false
	}
	_, err := hex.DecodeString(payload)
	return err == nil
}

// LegacyLiveSessionKeyID preserves the pre-canonical logical key used by the
// in-memory live-session registry during compatibility lookup.
func LegacyLiveSessionKeyID(session *factorysessions.LiveSession) string {
	if session == nil {
		return ""
	}
	folderPath := filepath.Clean(strings.TrimSpace(session.FolderPath))
	if folderPath == "." {
		folderPath = ""
	}
	folderPath = filepath.ToSlash(folderPath)
	targetKind := strings.TrimSpace(string(session.Target.Kind))
	targetName := strings.TrimSpace(session.Target.Name)
	if targetKind == "" {
		targetKind = string(factorysessions.TargetKindDefault)
	}
	return strings.Join([]string{folderPath, targetKind, targetName}, "::")
}
