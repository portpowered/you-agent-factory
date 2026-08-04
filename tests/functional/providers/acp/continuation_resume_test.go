package acp_test

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

// daemonWorkingDirectory returns a directory for a daemon that this test
// intentionally never shuts down (providers.Service exposes no Close, and a
// resumed or stale-but-still-connected ACP daemon is deliberately pooled for
// reuse, not killed - see invalidateDisconnected). Unlike t.TempDir(), a
// removal failure here (the still-running subprocess holding its cwd open)
// is not itself a test failure.
func daemonWorkingDirectory(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "acp-continuation-")
	if err != nil {
		t.Fatalf("create ACP daemon working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// newDirectACPProvidersService constructs the real Providers root against a
// live ACP daemon subprocess (the functionalRPCPeer helper process), without
// the full factory/HTTP stack this package's other tests use. Continuation is
// a Providers root operation any direct caller can invoke; exercising it
// through a real OS process boundary here proves the exact behavior this
// packet's stories add - session/load resume, stale classification, and
// truthful negotiated-capability resolution - crosses the real ACP daemon
// unchanged.
func newDirectACPProvidersService(t *testing.T, starts *atomic.Int32) providers.Service {
	t.Helper()
	service, err := providerswire.NewService(
		providerswire.WithACPIntegrations(providers.ACPIntegration{
			ID:        "entry-1",
			Name:      "cursor-acp",
			Transport: "stdio",
			Command:   "cursor-agent acp",
		}),
		providerswire.WithCommandFactory(acpHelperCommandFactory(starts)),
		providerswire.WithExecutableLocator(availableExecutableLocator{}),
	)
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	return service
}

// TestProvidersACPContinueResumesExactSessionThroughRealDaemon proves a
// Continue call against a real ACP daemon subprocess resumes the referenced
// Provider Session through the genuine session/load RPC - not a fresh
// session/new - and that the truthful live-negotiated-capability check
// (resolveContinuationProvider -> acp.Service.NegotiatedCapabilities) does
// not block a daemon that has not yet completed its first handshake. The
// peer's "resume" mode does not implement session/new at all, so any
// regression back to unconditionally starting a fresh session fails this
// test instead of silently substituting a different Provider Session.
func TestProvidersACPContinueResumesExactSessionThroughRealDaemon(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "resume")
	var starts atomic.Int32
	service := newDirectACPProvidersService(t, &starts)

	result, err := service.Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{
			Provider: "cursor-acp",
			Kind:     providers.SessionIDKind,
			ID:       "resume-target-session",
		},
		Attempt: providers.ExecuteRequest{
			Provider:         "cursor-acp",
			AttemptID:        "attempt-resume",
			UserMessage:      "continue the prior turn",
			WorkingDirectory: daemonWorkingDirectory(t),
		},
	})
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil - the peer only implements session/load in resume mode", err)
	}
	if result.Outcome != providers.ContinuationOutcomeResumed {
		t.Fatalf("Continue() outcome = %q, want %q", result.Outcome, providers.ContinuationOutcomeResumed)
	}
	if result.Result.SessionRef == nil || result.Result.SessionRef.ID != "resume-target-session" {
		t.Fatalf("Continue() result SessionRef = %#v, want the exact resumed session id unchanged", result.Result.SessionRef)
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("ACP daemon starts = %d, want exactly 1", got)
	}
}

// TestProvidersACPContinueClassifiesResourceNotFoundAsStale proves a
// session/load ResourceNotFound failure from a real ACP daemon (a stale or
// unknown session) surfaces through Continue as the typed stale continuation
// failure, and that no fresh-session fallback attempt is made. The peer's
// "resume-not-found" mode also does not implement session/new, so a
// regression to a fresh-session fallback would fail this test rather than
// masking the failure with an unrelated new session.
func TestProvidersACPContinueClassifiesResourceNotFoundAsStale(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "resume-not-found")
	var starts atomic.Int32
	service := newDirectACPProvidersService(t, &starts)

	_, err := service.Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{
			Provider: "cursor-acp",
			Kind:     providers.SessionIDKind,
			ID:       "stale-session",
		},
		Attempt: providers.ExecuteRequest{
			Provider:         "cursor-acp",
			AttemptID:        "attempt-resume-not-found",
			UserMessage:      "continue the prior turn",
			WorkingDirectory: daemonWorkingDirectory(t),
		},
	})
	var failure providers.ContinuationFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Continue() error = %v (%T), want ContinuationFailure - a session/load failure must be reported, not silently retried as a fresh session", err, err)
	}
	if failure.Kind != providers.ContinuationFailureKindStale {
		t.Fatalf("ContinuationFailure.Kind = %q, want %q", failure.Kind, providers.ContinuationFailureKindStale)
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("ACP daemon starts = %d, want exactly 1 - a stale reference must not start a second daemon or fresh-session attempt", got)
	}
}
