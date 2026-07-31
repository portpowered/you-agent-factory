package support

import "path/filepath"

// ProviderSessionFixtureRoot is the repo-relative root for provider-session
// golden fixtures owned by the functional-test support package.
const ProviderSessionFixtureRoot = "tests/functional/internal/support/testdata/provider-sessions"

// ProviderSessionFixturePath joins path segments under the tracked fixture root.
func ProviderSessionFixturePath(parts ...string) string {
	segments := append([]string{ProviderSessionFixtureRoot}, parts...)
	return filepath.ToSlash(filepath.Join(segments...))
}
