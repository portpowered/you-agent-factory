package host

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// SidecarStarter attaches hosted runtime sidecars after readiness.
type SidecarStarter func(ctx context.Context, handle *Handle) error

// StopSidecars cancels and waits for sidecar goroutines attached to handle.
func StopSidecars(handle *Handle) {
	if handle == nil {
		return
	}
	handle.SidecarMu.Lock()
	cancel := handle.SidecarCancel
	handle.SidecarCancel = nil
	handle.SidecarMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	handle.Sidecars.Wait()
}

// StartReplacement starts a replacement runtime, waits for readiness, and
// optionally attaches sidecars in service mode. Failed attempts stop the
// replacement runtime before returning.
func StartReplacement(
	readinessCtx context.Context,
	serviceCtx context.Context,
	bundle *Bundle,
	clock factory.Clock,
	attachSidecars SidecarStarter,
	attachSidecarsInServiceMode bool,
) (*Handle, error) {
	if bundle == nil {
		return nil, fmt.Errorf("replacement runtime bundle is required")
	}
	handle := Start(serviceCtx, bundle)
	if err := WaitForStart(readinessCtx, handle); err != nil {
		_ = Stop(handle, clock)
		return nil, fmt.Errorf("start replacement Runtime: %w", err)
	}
	if attachSidecarsInServiceMode && attachSidecars != nil {
		if err := attachSidecars(serviceCtx, handle); err != nil {
			_ = Stop(handle, clock)
			return nil, fmt.Errorf("start replacement runtime sidecars: %w", err)
		}
	}
	return handle, nil
}

// ReplacementAttempt pauses the current runtime's sidecars during a replacement
// attempt and restores them when the attempt fails.
type ReplacementAttempt struct {
	Current         *Handle
	ServiceCtx      context.Context
	ServiceMode     bool
	RestoreSidecars SidecarStarter
	restore         bool
}

// Begin pauses current runtime sidecars before a replacement attempt.
func (a *ReplacementAttempt) Begin() {
	if a == nil || !a.ServiceMode || a.Current == nil {
		return
	}
	StopSidecars(a.Current)
	a.restore = true
}

// Commit marks the replacement attempt successful so prior sidecars are not restored.
func (a *ReplacementAttempt) Commit() {
	if a == nil {
		return
	}
	a.restore = false
}

// End restores prior runtime sidecars when the replacement attempt failed.
func (a *ReplacementAttempt) End() {
	if a == nil || !a.restore || a.Current == nil || a.RestoreSidecars == nil {
		return
	}
	_ = a.RestoreSidecars(a.ServiceCtx, a.Current)
}
