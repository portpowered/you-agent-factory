//go:build functionallong

package support

import (
	"testing"
	"time"
)

// Done exposes legacy server completion only to the remaining long-tag replay
// scenarios. Customer-boundary scenarios should join RootRunFunctionalHost.
func (fs *FunctionalAPIServer) Done() <-chan struct{} {
	return fs.done
}

// Stop flushes legacy recording servers used by the remaining long-tag replay
// scenarios. Customer-boundary scenarios should shut down RootRunFunctionalHost.
func (fs *FunctionalAPIServer) Stop(t *testing.T) {
	t.Helper()

	fs.cancel()
	select {
	case <-fs.done:
	case <-time.After(functionalServerReadyTimeout):
		t.Fatal("FunctionalServer: timed out waiting for shutdown")
	}
}
