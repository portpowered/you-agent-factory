package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
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
	t.Run("cancellation during generation retains capacity until exit", runRunningCancellation)
}

type cancellationBlockingRuntime struct {
	started   chan struct{}
	allowExit chan struct{}
}

func (runtime cancellationBlockingRuntime) Invoke(ctx context.Context, _ inference.InvocationRuntimeRequest) (inference.InvocationRuntimeResult, error) {
	close(runtime.started)
	<-ctx.Done()
	<-runtime.allowExit
	return inference.InvocationRuntimeResult{}, ctx.Err()
}

func runRunningCancellation(t *testing.T) {
	t.Parallel()
	scopes, scope, lease, host := releaseFixture(t, "release-running-cancel", models.OperationOMNI)
	runtime := cancellationBlockingRuntime{started: make(chan struct{}), allowExit: make(chan struct{})}
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, runtime, fixedClock(), nil)
	type outcome struct {
		result models.InvokeModelResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := service.InvokeModelWithLease(context.Background(), releaseRequest(scope, lease, models.OperationOMNI))
		finished <- outcome{result, err}
	}()
	<-runtime.started
	invocation, err := (models.ModelInvocationRef{}).Parse("models-inference:1")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{Scope: scope, Invocation: invocation})
	if err != nil || cancelled.LeaseDisposition != models.InvocationLeaseRetained {
		t.Fatalf("cancel while running = (%#v, %v), want retained until backend exits", cancelled, err)
	}
	if host.releaseCalls != 0 {
		t.Fatalf("lease released before backend exit: %d calls", host.releaseCalls)
	}
	close(runtime.allowExit)
	ended := <-finished
	if !errors.Is(ended.err, models.ErrInferenceCancelled) || ended.result.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("generation exit = (%#v, %v)", ended.result, ended.err)
	}
	assertOneLeaseRelease(t, host)
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
	ctx := modelseffects.WithRuntimeLeaseReleaseTracker(context.Background())
	result, err := service.InvokeModelWithLease(ctx, releaseRequest(scope, lease, models.OperationOMNI))
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("missing runtime error = %v, want ErrUnavailable", err)
	}
	if !result.Invocation.IsZero() || result.Status != models.ModelInvocationStatusFailed ||
		result.Lease != lease || result.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("missing runtime result = %#v, want failed/no invocation/released lease", result)
	}
	if !modelseffects.RuntimeLeaseReleaseAttempted(ctx) {
		t.Fatal("missing runtime did not record its release attempt")
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

// Host inspection precedes ClaimInvocationLease. Its failure leaves the
// caller-owned lease active; it must not be mistaken for runtime cleanup.
func TestHostInspectionFailurePreservesUnclaimedLeaseForRetry(t *testing.T) {
	t.Parallel()
	scopes, scope, lease, host := releaseFixture(t, "inspection-retry", "generate")
	wantErr := models.ErrHostRuntimeNotReady
	failingHost := &inspectionFailureHost{recordingInferenceHost: host, failure: wantErr}
	runtime := &recordingInvocationRuntime{}
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), failingHost, runtime, fixedClock(), nil)
	request := releaseRequest(scope, lease, "generate")
	result, err := service.InvokeModelWithLease(t.Context(), request)
	if !errors.Is(err, wantErr) || !result.Invocation.IsZero() || len(result.Content) != 0 || len(result.Artifacts) != 0 {
		t.Fatalf("inspection failure = (%#v, %v), want original error without invocation/output", result, err)
	}
	if runtime.invokeCalls != 0 || failingHost.claims != 0 || host.releaseCalls != 0 || host.leases[lease.String()].Status != models.ModelLeaseStatusActive {
		t.Fatalf("pre-claim failure invoked/claimed/released capacity: runtime=%d claims=%d releases=%d lease=%#v", runtime.invokeCalls, failingHost.claims, host.releaseCalls, host.leases[lease.String()])
	}
	failingHost.failure = nil
	result, err = service.InvokeModelWithLease(t.Context(), request)
	assertReleaseOutcome(t, result, err, models.ModelInvocationStatusCompleted, nil)
	if runtime.invokeCalls != 1 || failingHost.claims != 1 {
		t.Fatalf("retry runtime=%d claims=%d, want one accepted invocation", runtime.invokeCalls, failingHost.claims)
	}
	assertOneLeaseRelease(t, host)
}

type inspectionFailureHost struct {
	*recordingInferenceHost
	failure error
	claims  int
}

func (host *inspectionFailureHost) InspectModelHost(ctx context.Context, request models.InspectModelHostRequest) (models.InspectModelHostResult, error) {
	if host.failure != nil {
		return models.InspectModelHostResult{}, host.failure
	}
	return host.recordingInferenceHost.InspectModelHost(ctx, request)
}

func (host *inspectionFailureHost) ClaimInvocationLease(ctx context.Context, request models.InvokeModelRequest) (models.ModelLease, error) {
	host.claims++
	return host.recordingInferenceHost.ClaimInvocationLease(ctx, request)
}

func TestRuntimeAndReleaseFailuresPreserveBothClassificationsWithoutOutput(t *testing.T) {
	t.Parallel()
	scopes, scope, lease, host := releaseFixture(t, "runtime-cleanup-failure", "generate")
	host.releaseErr = errors.New("PRIVATE_CLEANUP_DETAIL")
	runtime := &recordingInvocationRuntime{invokeErr: models.ErrInferenceTimeout}
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, runtime, fixedClock(), nil)
	result, err := service.InvokeModelWithLease(t.Context(), releaseRequest(scope, lease, "generate"))
	if !errors.Is(err, models.ErrInferenceTimeout) || strings.Contains(err.Error(), "PRIVATE_CLEANUP_DETAIL") {
		t.Fatalf("joined error = %v, want preserved timeout and redacted cleanup", err)
	}
	diagnostic := modelseffects.ProjectRuntimeFailure(err, 0)
	if diagnostic.Subcause != modelseffects.RuntimeSubcauseCleanup {
		t.Fatalf("joined diagnostic = %#v, want cleanup subcause", diagnostic)
	}
	if result.Status != models.ModelInvocationStatusFailed || result.LeaseDisposition != models.InvocationLeaseRetained ||
		len(result.Content) != 0 || len(result.Artifacts) != 0 || len(result.Outputs) != 0 {
		t.Fatalf("joined failure = %#v, want failed/retained without output", result)
	}
	if runtime.invokeCalls != 1 || host.leases[lease.String()].Status != models.ModelLeaseStatusActive {
		t.Fatalf("runtime calls=%d lease=%#v, want one attempt and retained capacity", runtime.invokeCalls, host.leases[lease.String()])
	}
	assertOneLeaseRelease(t, host)
}

func TestCancelAcceptedInvocationPreservesPeerScopeCapacity(t *testing.T) {
	t.Parallel()
	scopes, selectedScope, selectedLease, host := releaseFixture(t, "cancel-peer", "hold")
	peerRef, err := scopes.Open(models.RuntimeBinding{RuntimeConfig: func() *models.RuntimeConfig {
		return &models.RuntimeConfig{Workers: []models.RuntimeWorker{inferenceWorker("peer-model", "hold")}}
	}})
	if err != nil {
		t.Fatal(err)
	}
	peerScope := mustScopeRef(t, string(peerRef))
	peerLease := mustLeaseRef(t, "peer-lease")
	host.leases[peerLease.String()] = activeLease(peerScope, peerLease, "peer-model", "peer-worker")
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, holdInvocationRuntime{}, fixedClock(), nil)
	selected, err := service.InvokeModelWithLease(t.Context(), releaseRequest(selectedScope, selectedLease, "hold"))
	if err != nil || selected.Status != models.ModelInvocationStatusAccepted {
		t.Fatalf("selected invocation = (%#v, %v), want accepted", selected, err)
	}
	peer, err := service.InvokeModelWithLease(t.Context(), invokeRequest(peerScope, peerLease, "peer-worker", "peer-model", "hold"))
	if err != nil || peer.Status != models.ModelInvocationStatusAccepted || peer.Invocation == selected.Invocation {
		t.Fatalf("peer invocation = (%#v, %v), want distinct accepted invocation", peer, err)
	}
	cancelled, err := service.CancelInvocation(t.Context(), models.CancelInvocationRequest{Scope: selectedScope, Invocation: selected.Invocation})
	if err != nil || cancelled.Status != models.ModelInvocationStatusCancelled || cancelled.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("selected cancellation = (%#v, %v)", cancelled, err)
	}
	assertOneLeaseRelease(t, host)
	if host.leases[peerLease.String()].Status != models.ModelLeaseStatusActive {
		t.Fatalf("selected cancellation changed peer capacity: %#v", host.leases[peerLease.String()])
	}
	assertPeerCancellationScope(t, service, host, selectedScope, peer)
}

func assertPeerCancellationScope(t *testing.T, service inference.Service, host *recordingInferenceHost, selectedScope models.RuntimeScopeRef, peer models.InvokeModelResult) {
	t.Helper()
	wrongScope, err := service.CancelInvocation(t.Context(), models.CancelInvocationRequest{Scope: selectedScope, Invocation: peer.Invocation})
	if !errors.Is(err, models.ErrInvocationNotFound) || wrongScope.Outcome != "" || host.releaseCalls != 1 {
		t.Fatalf("foreign cancellation = (%#v, %v), releases=%d", wrongScope, err, host.releaseCalls)
	}
	peerCancelled, err := service.CancelInvocation(t.Context(), models.CancelInvocationRequest{Scope: peer.Scope, Invocation: peer.Invocation})
	if err != nil || peerCancelled.Outcome != models.InvocationCancellationRequested || peerCancelled.LeaseDisposition != models.InvocationLeaseReleased || host.releaseCalls != 2 {
		t.Fatalf("peer-owned cancellation = (%#v, %v), releases=%d", peerCancelled, err, host.releaseCalls)
	}
}

func TestScopedInvocationAndRepeatKeepSelectedModelAndOutput(t *testing.T) {
	t.Parallel()
	scopes, firstScope, firstLease, host := releaseFixture(t, "scoped-repeat", "generate")
	peerRef, err := scopes.Open(models.RuntimeBinding{RuntimeConfig: func() *models.RuntimeConfig {
		return &models.RuntimeConfig{Workers: []models.RuntimeWorker{inferenceWorker("peer-model", "generate")}}
	}})
	if err != nil {
		t.Fatal(err)
	}
	peerScope := mustScopeRef(t, string(peerRef))
	runtime := &recordingInvocationRuntime{}
	service := newInferenceServiceWithHost(t, scopes, mustCatalog(t, scopes), host, runtime, fixedClock(), nil)
	var previous models.ModelInvocationRef
	for index, scenario := range []struct {
		scope models.RuntimeScopeRef
		model string
		lease models.ModelLeaseRef
		text  string
	}{
		{firstScope, "scoped-model", firstLease, "first output"},
		{peerScope, "peer-model", mustLeaseRef(t, "peer-lease"), "peer output"},
		{firstScope, "scoped-model", mustLeaseRef(t, "repeat-lease"), "repeated output"},
	} {
		host.leases[scenario.lease.String()] = activeLease(scenario.scope, scenario.lease, scenario.model, "worker-1")
		request := invokeRequest(scenario.scope, scenario.lease, "worker-1", scenario.model, "generate")
		request.Input = models.InferenceInput{ContentType: "TEXT", Content: scenario.text}
		result, err := service.InvokeModelWithLease(t.Context(), request)
		assertReleaseOutcome(t, result, err, models.ModelInvocationStatusCompleted, nil)
		if result.Scope != scenario.scope || result.ModelName != scenario.model || result.Lease != scenario.lease ||
			result.Invocation.IsZero() || result.Invocation == previous || len(result.Content) != 1 || result.Content[0].Content != scenario.text {
			t.Fatalf("scenario %d result = %#v, want selected identity and %q", index, result, scenario.text)
		}
		if host.leases[scenario.lease.String()].Status != models.ModelLeaseStatusReleased || host.releaseCalls != index+1 {
			t.Fatalf("scenario %d release calls=%d lease=%#v", index, host.releaseCalls, host.leases[scenario.lease.String()])
		}
		previous = result.Invocation
	}
	if runtime.invokeCalls != 3 || runtime.reusedHostSlots != 3 {
		t.Fatalf("selected runtime calls=%d reused slots=%d, want three scoped executions", runtime.invokeCalls, runtime.reusedHostSlots)
	}
}
