package support

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// MockWorkerGate owns deterministic arrival and release signals for one
// functional mock-worker dispatch. It lets tests observe an active dispatch
// without sleeping or replacing the provider boundary.
type MockWorkerGate struct {
	arrivedFile string
	releaseFile string
}

func NewMockWorkerGate(t testing.TB) *MockWorkerGate {
	t.Helper()
	dir := t.TempDir()
	gate := &MockWorkerGate{
		arrivedFile: filepath.Join(dir, "arrived"),
		releaseFile: filepath.Join(dir, "release"),
	}
	t.Cleanup(gate.Release)
	return gate
}

// Config returns the customer-facing mock-worker gate declaration.
func (gate *MockWorkerGate) Config(timeout time.Duration) *workers.MockWorkerGateConfig {
	return &workers.MockWorkerGateConfig{
		ArrivedFile: gate.arrivedFile,
		ReleaseFile: gate.releaseFile,
		Timeout:     timeout.String(),
	}
}

// WaitForArrival blocks until the matched dispatch creates its arrival signal.
func (gate *MockWorkerGate) WaitForArrival(t testing.TB, timeout time.Duration) {
	t.Helper()
	_, err := WaitForObservation(
		timeout,
		func() (bool, error) {
			_, statErr := os.Stat(gate.arrivedFile)
			switch {
			case statErr == nil:
				return true, nil
			case errors.Is(statErr, os.ErrNotExist):
				return false, nil
			default:
				return false, statErr
			}
		},
		func(arrived bool) bool { return arrived },
	)
	if err != nil {
		t.Fatalf("wait for mock-worker gate arrival: %v", err)
	}
}

// Release lets the blocked dispatch continue into its configured run type.
func (gate *MockWorkerGate) Release() {
	if gate == nil {
		return
	}
	_ = os.WriteFile(gate.releaseFile, []byte("release\n"), 0o600)
}
