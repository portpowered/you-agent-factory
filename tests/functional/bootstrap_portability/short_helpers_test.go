//go:build !functionallong

package bootstrap_portability

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
)

func waitForCurrentFactoryRuntimeIdle(t *testing.T, svc *service.FactoryService, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastStatus interfaces.RuntimeStatus
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err == nil && snap.RuntimeStatus == interfaces.RuntimeStatusIdle {
			return
		}
		if err == nil {
			lastStatus = snap.RuntimeStatus
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for idle runtime; last status=%q", lastStatus)
}
