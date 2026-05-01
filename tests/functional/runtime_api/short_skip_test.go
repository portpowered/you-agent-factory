package runtime_api

import "testing"

func skipSlowFunctionalSmokeInShort(t *testing.T, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skip(reason)
	}
}
