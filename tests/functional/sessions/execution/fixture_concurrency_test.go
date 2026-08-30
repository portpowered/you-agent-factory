package execution_test

import "testing"

// Independent execution fixtures can overlap, but each root composition still
// performs filesystem and listener startup. Keep parallel scheduling below the
// host's contention cliff without sharing mutable Factory Session state.
var executionFixtureSlots = make(chan struct{}, 8)

func acquireExecutionFixtureSlot(t testing.TB) {
	t.Helper()
	executionFixtureSlots <- struct{}{}
	t.Cleanup(func() { <-executionFixtureSlots })
}
