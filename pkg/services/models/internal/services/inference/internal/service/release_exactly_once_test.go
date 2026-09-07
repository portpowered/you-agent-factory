package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// TestInvocationLeaseReleaseIsExactlyOnce is the bounded release witness for
// the private invocation lifecycle. Each case owns a fresh scope, lease, host,
// and runtime so race/repeat runs cannot share lifecycle state.
func TestInvocationLeaseReleaseIsExactlyOnce(t *testing.T) {
	t.Parallel()

	t.Run("success", runReleaseSuccess)
	t.Run("backend error", runReleaseBackendError)
	t.Run("timeout", runReleaseTimeout)
	t.Run("context cancellation", runReleaseContextCancellation)
	t.Run("explicit cancellation has no late completion", runReleaseExplicitCancellation)
	t.Run("cancellation release failure stays retained", runCancellationReleaseFailure)
	t.Run("missing runtime releases acquired lease", runReleaseMissingRuntime)
	t.Run("release failure cannot publish success", runReleaseFailureCannotPublishSuccess)
}

func runReleaseSuccess(t *testing.T) {
	t.Parallel()
	t.Helper()
	scopes, scope, lease, host := releaseFixture(t, "release-success", models.OperationOMNI)
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, &recordingInvocationRuntime{}, fixedClock(), nil)
	result, err := service.InvokeModelWithLease(context.Background(), releaseRequest(scope, lease, models.OperationOMNI))
	assertReleaseOutcome(t, result, err, models.ModelInvocationStatusCompleted, nil)
	assertOneLeaseRelease(t, host)
}

func runReleaseBackendError(t *testing.T) {
	t.Parallel()
	t.Helper()
	scopes, scope, lease, host := releaseFixture(t, "release-error", models.OperationOMNI)
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, &recordingInvocationRuntime{invokeErr: models.ErrInferenceFailed}, fixedClock(), nil)
	result, err := service.InvokeModelWithLease(context.Background(), releaseRequest(scope, lease, models.OperationOMNI))
	assertReleaseOutcome(t, result, err, models.ModelInvocationStatusFailed, models.ErrInferenceFailed)
	assertOneLeaseRelease(t, host)
}

func runReleaseTimeout(t *testing.T) {
	t.Parallel()
	t.Helper()
	scopes, scope, lease, host := releaseFixture(t, "release-timeout", models.OperationOMNI)
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, deadlineInvocationRuntime{}, fixedClock(), nil, func() time.Duration { return time.Millisecond })
	result, err := service.InvokeModelWithLease(context.Background(), releaseRequest(scope, lease, models.OperationOMNI))
	assertReleaseOutcome(t, result, err, models.ModelInvocationStatusFailed, models.ErrInferenceTimeout)
	assertOneLeaseRelease(t, host)
}

func runReleaseContextCancellation(t *testing.T) {
	t.Parallel()
	t.Helper()
	scopes, scope, lease, host := releaseFixture(t, "release-context-cancel", models.OperationOMNI)
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, deadlineInvocationRuntime{}, fixedClock(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := service.InvokeModelWithLease(ctx, releaseRequest(scope, lease, models.OperationOMNI))
	assertReleaseOutcome(t, result, err, models.ModelInvocationStatusCancelled, models.ErrInferenceCancelled)
	assertOneLeaseRelease(t, host)
}

func runReleaseExplicitCancellation(t *testing.T) {
	t.Parallel()
	t.Helper()
	scopes, scope, lease, host := releaseFixture(t, "release-explicit-cancel", "hold")
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, holdInvocationRuntime{}, fixedClock(), nil)
	accepted, err := service.InvokeModelWithLease(context.Background(), releaseRequest(scope, lease, "hold"))
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
}

func runReleaseFailureCannotPublishSuccess(t *testing.T) {
	t.Parallel()
	t.Helper()

	const sentinel = "PRIVATE_LEASE_RELEASE_FAILURE_SENTINEL"
	scopes, scope, lease, host := releaseFixture(t, "release-failure", models.OperationOMNI)
	host.releaseErr = errors.New(sentinel)
	runtime := &recordingInvocationRuntime{}
	service := newInferenceServiceWithHost(
		t, scopes, mustCatalog(t, scopes), host, runtime, fixedClock(), nil,
	)
	result, err := service.InvokeModelWithLease(
		context.Background(), releaseRequest(scope, lease, models.OperationOMNI),
	)
	if err == nil || !errors.Is(err, models.ErrInferenceFailed) {
		t.Fatalf("release failure error = %v, want ErrInferenceFailed", err)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("release failure leaked private sentinel: %v", err)
	}
	stage, class, ok := modelseffects.ClassifyRuntimeFailure(err)
	if !ok || stage != modelseffects.RuntimeStageInvoke || class != modelseffects.RuntimeFailureInvocationFailed {
		t.Fatalf("release failure classification = %q/%q/%t, want INVOKE/INVOCATION_FAILED", stage, class, ok)
	}
	diagnostic := modelseffects.ProjectRuntimeFailure(err, 0)
	if diagnostic.Subcause != modelseffects.RuntimeSubcauseCleanup {
		t.Fatalf("release failure diagnostic = %#v, want CLEANUP subcause", diagnostic)
	}
	if result.Status != models.ModelInvocationStatusFailed ||
		result.LeaseDisposition != models.InvocationLeaseRetained ||
		len(result.Content) != 0 || len(result.Artifacts) != 0 || len(result.Outputs) != 0 {
		t.Fatalf("release failure result = %#v, want failed/retained with no output", result)
	}
	if runtime.invokeCalls != 1 {
		t.Fatalf("runtime invoke calls = %d, want 1", runtime.invokeCalls)
	}
	assertOneLeaseRelease(t, host)
	if got := host.leases[lease.String()].Status; got != models.ModelLeaseStatusActive {
		t.Fatalf("lease status after failed release = %q, want ACTIVE", got)
	}
}

func runReleaseMissingRuntime(t *testing.T) {
	t.Parallel()
	t.Helper()

	scopes, scope, lease, host := releaseFixture(t, "release-missing-runtime", models.OperationOMNI)
	service := newInferenceServiceWithHost(
		t, scopes, mustCatalog(t, scopes), host, nil, fixedClock(), nil,
	)
	result, err := service.InvokeModelWithLease(
		context.Background(), releaseRequest(scope, lease, models.OperationOMNI),
	)
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("missing runtime error = %v, want ErrUnavailable", err)
	}
	if !result.Invocation.IsZero() {
		t.Fatalf("missing runtime result = %#v, want no invocation", result)
	}
	assertOneLeaseRelease(t, host)
	if got := host.leases[lease.String()].Status; got != models.ModelLeaseStatusReleased {
		t.Fatalf("lease status after missing runtime = %q, want RELEASED", got)
	}
}

func runCancellationReleaseFailure(t *testing.T) {
	t.Parallel()
	t.Helper()

	const sentinel = "PRIVATE_CANCEL_RELEASE_FAILURE_SENTINEL"
	scopes, scope, lease, host := releaseFixture(t, "cancel-release-failure", "hold")
	host.releaseErr = errors.New(sentinel)
	service := newInferenceServiceWithHost(
		t, scopes, mustCatalog(t, scopes), host, holdInvocationRuntime{}, fixedClock(), nil,
	)
	accepted, err := service.InvokeModelWithLease(
		context.Background(), releaseRequest(scope, lease, "hold"),
	)
	if err != nil || accepted.Status != models.ModelInvocationStatusAccepted {
		t.Fatalf("accepted hold result = %#v, error = %v", accepted, err)
	}
	cancelled, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope: scope, Invocation: accepted.Invocation,
	})
	if err == nil || !errors.Is(err, models.ErrInferenceCancelled) {
		t.Fatalf("cancel release failure error = %v, want ErrInferenceCancelled", err)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("cancel release failure leaked private sentinel: %v", err)
	}
	if cancelled.Status != models.ModelInvocationStatusCancelled ||
		cancelled.LeaseDisposition != models.InvocationLeaseRetained {
		t.Fatalf("cancelled result = %#v, want cancelled/retained", cancelled)
	}
	assertOneLeaseRelease(t, host)
	if got := host.leases[lease.String()].Status; got != models.ModelLeaseStatusActive {
		t.Fatalf("lease status after cancelled release failure = %q, want ACTIVE", got)
	}
}

func releaseFixture(
	t *testing.T,
	name, operation string,
) (runtimescopes.Service, models.RuntimeScopeRef, models.ModelLeaseRef, *recordingInferenceHost) {
	t.Helper()
	scopes, scope := openInferenceScope(t, name, "scoped-model", operation)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{leases: map[string]models.ModelLease{
		lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
	}}
	return scopes, scope, lease, host
}

func releaseRequest(
	scope models.RuntimeScopeRef,
	lease models.ModelLeaseRef,
	operation string,
) models.InvokeModelRequest {
	if operation == models.OperationOMNI {
		return omniInvocationRequest(scope, lease)
	}
	return invokeRequest(scope, lease, "worker-1", "scoped-model", operation)
}

func assertReleaseOutcome(
	t *testing.T,
	result models.InvokeModelResult,
	err error,
	wantStatus models.ModelInvocationStatus,
	wantErr error,
) {
	t.Helper()
	if wantErr == nil && err != nil {
		t.Fatalf("release result = %#v, error = %v, want no error", result, err)
	}
	if wantErr != nil && !errors.Is(err, wantErr) {
		t.Fatalf("release result = %#v, error = %v, want %v", result, err, wantErr)
	}
	if result.Status != wantStatus || result.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("release result = %#v, want status %q/released", result, wantStatus)
	}
}

func assertOneLeaseRelease(t *testing.T, host *recordingInferenceHost) {
	t.Helper()
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want exactly one", host.releaseCalls)
	}
}
