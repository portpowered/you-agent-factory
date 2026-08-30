package root_composition_test

import "testing"

// Independent root fixtures may overlap, but the local packaged-factory
// installation and listener boundaries have finite host capacity. Bounding
// fixture ownership keeps the scheduling optimization from turning startup
// contention into a false "server starter was never invoked" failure.
var rootCompositionFixtureSlots = make(chan struct{}, 8)

func acquireRootCompositionFixtureSlot(t testing.TB) {
	t.Helper()
	rootCompositionFixtureSlots <- struct{}{}
	t.Cleanup(func() { <-rootCompositionFixtureSlots })
}
