package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// TestInvocationLeaseReleaseIsExactlyOnce is the bounded release witness for
// the private invocation lifecycle. Each case owns a fresh scope, lease, host,
// and runtime so race/repeat runs cannot share lifecycle state.
func TestInvocationLeaseReleaseIsExactlyOnce(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		scopes, scope := openInferenceScope(t, "release-success", "scoped-model", "generate")
		lease := mustLeaseRef(t, "lease-1")
		host := &recordingInferenceHost{leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		}}
		service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, &recordingInvocationRuntime{}, fixedClock(), nil)

		result, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"))
		if err != nil || result.Status != models.ModelInvocationStatusCompleted || result.LeaseDisposition != models.InvocationLeaseReleased {
			t.Fatalf("success result = %#v, error = %v, want completed/released", result, err)
		}
		assertOneLeaseRelease(t, host)
	})

	t.Run("backend error", func(t *testing.T) {
		scopes, scope := openInferenceScope(t, "release-error", "scoped-model", "generate")
		lease := mustLeaseRef(t, "lease-1")
		host := &recordingInferenceHost{leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		}}
		service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, &recordingInvocationRuntime{invokeErr: models.ErrInferenceFailed}, fixedClock(), nil)

		result, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"))
		if !errors.Is(err, models.ErrInferenceFailed) || result.Status != models.ModelInvocationStatusFailed || result.LeaseDisposition != models.InvocationLeaseReleased {
			t.Fatalf("backend-error result = %#v, error = %v, want failed/released ErrInferenceFailed", result, err)
		}
		assertOneLeaseRelease(t, host)
	})

	t.Run("timeout", func(t *testing.T) {
		scopes, scope := openInferenceScope(t, "release-timeout", "scoped-model", "generate")
		lease := mustLeaseRef(t, "lease-1")
		host := &recordingInferenceHost{leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		}}
		service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, deadlineInvocationRuntime{}, fixedClock(), nil, func() time.Duration {
			return time.Millisecond
		})

		result, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"))
		if !errors.Is(err, models.ErrInferenceTimeout) || result.Status != models.ModelInvocationStatusFailed || result.LeaseDisposition != models.InvocationLeaseReleased {
			t.Fatalf("timeout result = %#v, error = %v, want failed/released ErrInferenceTimeout", result, err)
		}
		assertOneLeaseRelease(t, host)
	})

	t.Run("context cancellation", func(t *testing.T) {
		scopes, scope := openInferenceScope(t, "release-context-cancel", "scoped-model", "generate")
		lease := mustLeaseRef(t, "lease-1")
		host := &recordingInferenceHost{leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		}}
		service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, deadlineInvocationRuntime{}, fixedClock(), nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result, err := service.InvokeModelWithLease(ctx, invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"))
		if !errors.Is(err, models.ErrInferenceCancelled) || result.Status != models.ModelInvocationStatusCancelled || result.LeaseDisposition != models.InvocationLeaseReleased {
			t.Fatalf("context-cancel result = %#v, error = %v, want cancelled/released", result, err)
		}
		assertOneLeaseRelease(t, host)
	})

	t.Run("explicit cancellation has no late completion", func(t *testing.T) {
		scopes, scope := openInferenceScope(t, "release-explicit-cancel", "scoped-model", "hold")
		lease := mustLeaseRef(t, "lease-1")
		host := &recordingInferenceHost{leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		}}
		service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, holdInvocationRuntime{}, fixedClock(), nil)

		accepted, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "scoped-model", "hold"))
		if err != nil || accepted.Status != models.ModelInvocationStatusAccepted || accepted.LeaseDisposition != models.InvocationLeaseRetained {
			t.Fatalf("accepted hold result = %#v, error = %v, want accepted/retained", accepted, err)
		}
		cancelled, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{Scope: scope, Invocation: accepted.Invocation})
		if err != nil || cancelled.Outcome != models.InvocationCancellationRequested || cancelled.Status != models.ModelInvocationStatusCancelled || cancelled.LeaseDisposition != models.InvocationLeaseReleased {
			t.Fatalf("cancelled hold result = %#v, error = %v, want requested/cancelled/released", cancelled, err)
		}
		repeated, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{Scope: scope, Invocation: accepted.Invocation})
		if err != nil || repeated.Outcome != models.InvocationCancellationAlreadyCancelled {
			t.Fatalf("repeated cancellation = %#v, error = %v, want ALREADY_CANCELLED", repeated, err)
		}
		assertOneLeaseRelease(t, host)
	})
}

func assertOneLeaseRelease(t *testing.T, host *recordingInferenceHost) {
	t.Helper()
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want exactly one", host.releaseCalls)
	}
}
