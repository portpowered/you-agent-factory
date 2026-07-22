package logicaltarget

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// DeriveLogicalSessionKeyID returns a stable opaque identifier derived from
// ref. Equivalent canonical references always produce the same id, and the id
// changes when the normalized target's meaningful backend, provider, folder,
// or named-target boundary changes.
func DeriveLogicalSessionKeyID(ref CanonicalReference) string {
	return factorysessions.DeriveLogicalSessionKeyID(ref)
}

// IsLogicalSessionKeyID reports whether value matches the opaque logical
// session key format exposed to API clients.
func IsLogicalSessionKeyID(value string) bool {
	return factorysessions.IsLogicalSessionKeyID(value)
}
