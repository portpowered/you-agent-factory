package operatorsettings

import (
	"fmt"
	"strings"
)

const providerBackendScopePrefix = "provider"

// DeriveProviderBackendScopeID returns a stable backend scope identifier for one
// provider-backed runtime boundary without embedding secret material.
func DeriveProviderBackendScopeID(provider, kind, boundary string) string {
	return fmt.Sprintf(
		"%s-%s-%s-%s",
		providerBackendScopePrefix,
		sanitizeBackendScopeSegment(provider),
		sanitizeBackendScopeSegment(kind),
		sanitizeBackendScopeSegment(boundary),
	)
}

func sanitizeBackendScopeSegment(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(
		" ", "-",
		"/", "-",
		"\\", "-",
		":", "-",
		"|", "-",
	)
	return replacer.Replace(trimmed)
}
