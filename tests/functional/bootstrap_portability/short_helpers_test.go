//go:build !functionallong

package bootstrap_portability

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func waitForCurrentFactoryRuntimeIdle(t *testing.T, serverURL string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastStatus interfaces.RuntimeStatus
	for time.Now().Before(deadline) {
		status := getGeneratedJSON[factoryapi.StatusResponse](t, serverURL+"/status")
		if status.RuntimeStatus == string(interfaces.RuntimeStatusIdle) {
			return
		}
		lastStatus = interfaces.RuntimeStatus(status.RuntimeStatus)
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for idle runtime; last status=%q", lastStatus)
}
