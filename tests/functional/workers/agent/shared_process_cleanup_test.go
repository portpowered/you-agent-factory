package agent_test

import (
	"context"
	"testing"
)

func (fixture *agentSharedProcessFixture) close(t testing.TB) {
	t.Helper()
	if fixture.process == nil {
		return
	}
	if fixture.command != nil {
		// StartProcessCommand registers its cleanup after this fixture cleanup,
		// so its LIFO cleanup stops the invocation before the root closes.
		// Calling Stop here also makes this boundary safe if setup ordering is
		// changed by a future package-level fixture.
		fixture.command.Stop(t)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), agentSharedProcessTimeout)
	defer cancel()
	if err := fixture.process.Close(closeCtx); err != nil {
		t.Errorf("close shared agent application process: %v", err)
	}

	if fixture.command != nil {
		select {
		case <-fixture.apiClosed:
		case <-closeCtx.Done():
			t.Errorf("shared agent API server did not close: %v", closeCtx.Err())
		}
	}
	fixture.sessionsMu.Lock()
	if len(fixture.opened) != len(fixture.closed) {
		t.Errorf("closed shared Factory Sessions = %d, opened = %d; opened=%#v closed=%#v", len(fixture.closed), len(fixture.opened), fixture.opened, fixture.closed)
	}
	fixture.sessionsMu.Unlock()
	for _, scenario := range fixture.scenarios {
		if got := scenario.runner.activeCallCount(); got != 0 {
			t.Errorf("%s active agent command calls after process cleanup = %d, want zero", scenario.name, got)
		}
	}
	fixture.router.clearRoutes()
}
