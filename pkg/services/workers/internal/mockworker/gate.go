// Package mockworker owns behavior shared by the Workers mock execution paths.
package mockworker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const gatePollInterval = 10 * time.Millisecond

// GateFileSystem is the exact filesystem effect required by a blocking mock
// gate. Composition selects the implementation so mock behavior remains
// deterministic without hiding an ambient production adapter.
type GateFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
	Stat(string) (fs.FileInfo, error)
}

// WaitForGate signals that a matched dispatch arrived, then waits for an
// explicit release while honoring both invocation cancellation and the
// authored bounded timeout.
func WaitForGate(ctx context.Context, config workers.MockWorkerGateConfig, files GateFileSystem) error {
	if files == nil {
		return errors.New("wait for mock worker gate: filesystem is required")
	}
	timeout, err := time.ParseDuration(config.Timeout)
	if err != nil {
		return fmt.Errorf("wait for mock worker gate: parse timeout: %w", err)
	}
	if err := files.MkdirAll(filepath.Dir(config.ArrivedFile), 0o755); err != nil {
		return fmt.Errorf("wait for mock worker gate: create arrival directory: %w", err)
	}
	if err := files.WriteFile(config.ArrivedFile, []byte("arrived\n"), 0o600); err != nil {
		return fmt.Errorf("wait for mock worker gate: signal arrival: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(gatePollInterval)
	defer ticker.Stop()
	for {
		if _, err := files.Stat(config.ReleaseFile); err == nil {
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("wait for mock worker gate: observe release: %w", err)
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("wait for mock worker gate release %q: %w", config.ReleaseFile, waitCtx.Err())
		case <-ticker.C:
		}
	}
}
