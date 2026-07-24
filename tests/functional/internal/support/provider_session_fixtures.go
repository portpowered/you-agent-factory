package support

import "path/filepath"

// ProviderSessionFixtureRoot is the tracked repo-relative root for provider
// session golden fixtures. It is narrowly excepted from docs/temp/** ignore.
const ProviderSessionFixtureRoot = "docs/temp/functional/provider-sessions"

// ProviderSessionFixturePath joins path segments under the tracked fixture root.
func ProviderSessionFixturePath(parts ...string) string {
	segments := append([]string{ProviderSessionFixtureRoot}, parts...)
	return filepath.ToSlash(filepath.Join(segments...))
}
