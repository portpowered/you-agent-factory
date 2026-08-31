package root_composition_test

import "testing"

func acquireRootCompositionFixtureSlot(t testing.TB) {
	t.Helper()
	// Kept as a source-compatible helper for the existing parallel tests. The
	// package owns one process, and every scenario selects its route by path;
	// there is deliberately no package-wide capacity semaphore here.
}
