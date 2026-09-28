package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// applicationSessionState resolves application lifecycle values from the
// canonical live-session record. No application runtime registry is allocated.
func (r *Root) applicationSessionState(sessionID string) (*runtimebinding.SessionState, error) {
	if r == nil || r.Assembly == nil {
		return nil, fmt.Errorf("Factory Sessions process root is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("application Factory Session ID is required")
	}
	session := r.Resolve(sessionID)
	if session == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Process == nil {
		return nil, fmt.Errorf("%w: application process for session %q", factorysessions.ErrRuntimeNotAvailable, sessionID)
	}
	return bound, nil
}

// RunApplicationTransport hosts the selected Factory Session through its
// process-owned transport. The runtime remains attached to one session record.
func (r *Root) RunApplicationTransport(ctx context.Context, sessionID string, handler http.Handler) error {
	if handler == nil {
		return fmt.Errorf("run Factory Session transport: HTTP handler is required")
	}
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return err
	}
	return bound.Process.RunTransport(ctx, handler)
}

// ApplicationReady returns the selected session's runtime-host readiness.
func (r *Root) ApplicationReady(sessionID string) (<-chan factorysessions.RuntimeHostBinding, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return nil, err
	}
	ready, ok := bound.Process.(interface {
		RuntimeHostReady() <-chan factorysessions.RuntimeHostBinding
	})
	if !ok {
		return nil, nil
	}
	return ready.RuntimeHostReady(), nil
}

// ApplicationDiagnostics reports artifact locations selected for this session.
func (r *Root) ApplicationDiagnostics(sessionID string) (factoryruntime.RuntimeLogDiagnostics, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return factoryruntime.RuntimeLogDiagnostics{}, err
	}
	return bound.Diagnostics, nil
}
