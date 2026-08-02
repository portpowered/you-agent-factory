package service_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferenceartifacts "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/internal/artifacts"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/internal/service"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestInvokeModelWithLeaseValidatesPeerControlledInput(t *testing.T) {
	t.Parallel()

	service := newInferenceService(t, "invoke-validate", nil)

	tests := []struct {
		name    string
		request models.InvokeModelRequest
		want    error
	}{
		{
			name:    "scope",
			request: models.InvokeModelRequest{Lease: mustLeaseRef(t, "lease-1"), Holder: "worker", ModelName: "model", Operation: "generate"},
			want:    models.ErrRuntimeScopeInvalid,
		},
		{
			name:    "lease",
			request: models.InvokeModelRequest{Scope: mustScopeRef(t, "scope-1"), Holder: "worker", ModelName: "model", Operation: "generate"},
			want:    models.ErrHostLeaseNotFound,
		},
		{
			name:    "holder",
			request: models.InvokeModelRequest{Scope: mustScopeRef(t, "scope-1"), Lease: mustLeaseRef(t, "lease-1"), ModelName: "model", Operation: "generate"},
			want:    models.ErrHostInvalidHolder,
		},
		{
			name:    "model",
			request: models.InvokeModelRequest{Scope: mustScopeRef(t, "scope-1"), Lease: mustLeaseRef(t, "lease-1"), Holder: "worker", Operation: "generate"},
			want:    models.ErrNotFound,
		},
		{
			name:    "operation",
			request: models.InvokeModelRequest{Scope: mustScopeRef(t, "scope-1"), Lease: mustLeaseRef(t, "lease-1"), Holder: "worker", ModelName: "model"},
			want:    models.ErrUnsupportedModelOperation,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.InvokeModelWithLease(context.Background(), test.request)
			if !errors.Is(err, test.want) {
				t.Fatalf("InvokeModelWithLease = %v, want %v", err, test.want)
			}
		})
	}
}

func TestInvokeModelWithLeaseRejectsUnknownAndExpiredLeases(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-lease", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	activeLease := mustLeaseRef(t, "lease-active")
	expiredLease := mustLeaseRef(t, "lease-expired")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			activeLease.String(): {
				Lease: activeLease, Scope: scope, ModelName: "scoped-model",
				Holder: "worker-1", Status: models.ModelLeaseStatusActive,
				ExpiresAt: now.Add(time.Minute),
			},
			expiredLease.String(): {
				Lease: expiredLease, Scope: scope, ModelName: "scoped-model",
				Holder: "worker-1", Status: models.ModelLeaseStatusExpired,
				ExpiresAt: now.Add(-time.Minute),
			},
		},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, &recordingInvocationRuntime{}, fixedClock(), nil)

	_, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, activeLease, "worker-2", "scoped-model", "generate"))
	if !errors.Is(err, models.ErrHostInvalidHolder) {
		t.Fatalf("holder mismatch = %v, want ErrHostInvalidHolder", err)
	}

	_, err = service.InvokeModelWithLease(context.Background(), invokeRequest(scope, mustLeaseRef(t, "missing"), "worker-1", "scoped-model", "generate"))
	if !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("unknown lease = %v, want ErrHostLeaseNotFound", err)
	}

	_, err = service.InvokeModelWithLease(context.Background(), invokeRequest(scope, expiredLease, "worker-1", "scoped-model", "generate"))
	if !errors.Is(err, models.ErrHostLeaseExpired) {
		t.Fatalf("expired lease = %v, want ErrHostLeaseExpired", err)
	}
}

func TestInvokeModelWithLeaseRejectsUnknownModelAndUnsupportedOperation(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-catalog", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "missing-model", "worker-1"),
		},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, &recordingInvocationRuntime{}, fixedClock(), nil)

	_, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "missing-model", "generate"))
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("unknown model = %v, want ErrNotFound", err)
	}

	scopedLease := mustLeaseRef(t, "lease-2")
	host.leases[scopedLease.String()] = activeLease(scope, scopedLease, "scoped-model", "worker-1")
	_, err = service.InvokeModelWithLease(context.Background(), invokeRequest(scope, scopedLease, "worker-1", "scoped-model", "unsupported"))
	if !errors.Is(err, models.ErrUnsupportedModelOperation) {
		t.Fatalf("unsupported operation = %v, want ErrUnsupportedModelOperation", err)
	}
}

func TestInvokeModelWithLeaseReturnsDetachedCompletedResult(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-success", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	runtime := &recordingInvocationRuntime{
		content: []models.InferenceContent{{
			ContentType: "text/plain",
			Content:     "models-owned-output",
		}},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, runtime, fixedClock(), nil)

	result, err := service.InvokeModelWithLease(context.Background(), models.InvokeModelRequest{
		Scope:     scope,
		Lease:     lease,
		Holder:    "worker-1",
		ModelName: "scoped-model",
		Operation: "generate",
		Input: models.InferenceInput{
			ContentType: "text/plain",
			Content:     "hello",
		},
	})
	if err != nil {
		t.Fatalf("InvokeModelWithLease: %v", err)
	}
	if result.Invocation.IsZero() ||
		result.Status != models.ModelInvocationStatusCompleted ||
		result.LeaseDisposition != models.InvocationLeaseReleased ||
		result.Lease != lease {
		t.Fatalf("result = %#v, want completed released lease-backed invocation", result)
	}
	if len(result.Content) != 1 || result.Content[0].Content != "models-owned-output" {
		t.Fatalf("content = %#v, want detached runtime output", result.Content)
	}
	if runtime.invokeCalls != 1 {
		t.Fatalf("runtime invoke calls = %d, want 1", runtime.invokeCalls)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want 1", host.releaseCalls)
	}
	released := host.leases[lease.String()]
	if released.Status != models.ModelLeaseStatusReleased {
		t.Fatalf("lease status = %q, want RELEASED", released.Status)
	}
}

func TestInvokeModelWithLeaseRejectsUnavailableAssets(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-unavailable-assets", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	service := newInferenceServiceWithHost(
		t,
		scopes,
		catalog,
		host,
		&recordingInvocationRuntime{},
		fixedClock(),
		&recordingInferenceAssets{
			inspection: scopedassets.RuntimeCacheInspection{Supported: true},
		},
	)

	_, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"))
	if !errors.Is(err, models.ErrAssetUnavailable) {
		t.Fatalf("uninstalled assets = %v, want ErrAssetUnavailable", err)
	}
	if host.releaseCalls != 0 {
		t.Fatalf("lease release calls = %d, want 0 for pre-acceptance failure", host.releaseCalls)
	}

	service = newInferenceServiceWithHost(
		t,
		scopes,
		catalog,
		host,
		&recordingInvocationRuntime{},
		fixedClock(),
		&recordingInferenceAssets{err: fmt.Errorf("%w: scoped-model", models.ErrAssetUnavailable)},
	)
	_, err = service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"))
	if !errors.Is(err, models.ErrAssetUnavailable) {
		t.Fatalf("asset inspection error = %v, want ErrAssetUnavailable", err)
	}
}

func TestInvokeModelWithLeaseRejectsUnsupportedResponseMode(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-response-mode", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, &recordingInvocationRuntime{}, fixedClock(), nil)
	request := invokeRequest(scope, lease, "worker-1", "scoped-model", "generate")
	request.ResponseMode = models.ResponseMode("JSON")

	_, err := service.InvokeModelWithLease(context.Background(), request)
	if !errors.Is(err, models.ErrUnsupportedResponseMode) {
		t.Fatalf("unsupported response mode = %v, want ErrUnsupportedResponseMode", err)
	}
}

func TestInvokeModelWithLeaseNormalizesRuntimeFailure(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-failure", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	runtime := &recordingInvocationRuntime{invokeErr: models.ErrInferenceFailed}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, runtime, fixedClock(), nil)

	result, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"))
	if !errors.Is(err, models.ErrInferenceFailed) {
		t.Fatalf("runtime failure = %v, want ErrInferenceFailed", err)
	}
	if result.Invocation.IsZero() ||
		result.Status != models.ModelInvocationStatusFailed ||
		result.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("result = %#v, want failed released post-acceptance invocation", result)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want 1", host.releaseCalls)
	}
}

func TestInvokeModelFailureOutcomesRemainDistinct(t *testing.T) {
	t.Parallel()

	failures := []error{
		models.ErrAssetUnavailable,
		models.ErrHostRuntimeNotReady,
		models.ErrHostCapacityExhausted,
		models.ErrHostLeaseExpired,
		models.ErrUnsupportedModelOperation,
		models.ErrInferenceTimeout,
		models.ErrInferenceFailed,
	}
	for _, want := range failures {
		for _, other := range failures {
			if want != other && errors.Is(want, other) {
				t.Fatalf("failure classifications must stay distinct: %v vs %v", want, other)
			}
		}
	}

	scopes, scope := openInferenceScope(t, "invoke-distinct", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	service := newInferenceServiceWithHost(
		t,
		scopes,
		catalog,
		host,
		&recordingInvocationRuntime{invokeErr: models.ErrInferenceFailed},
		fixedClock(),
		nil,
	)
	result, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"))
	if !errors.Is(err, models.ErrInferenceFailed) {
		t.Fatalf("normalized failure = %v, want ErrInferenceFailed", err)
	}
	for _, other := range failures {
		if other != models.ErrInferenceFailed && errors.Is(err, other) {
			t.Fatalf("normalized failure must stay distinct from %v: %v", other, err)
		}
	}
	if result.Status != models.ModelInvocationStatusFailed {
		t.Fatalf("result status = %q, want FAILED", result.Status)
	}
}

func TestInvokeModelWithLeaseTimesOutWithReleasedLeaseDisposition(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-timeout", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	now := time.Now()
	service := newInferenceServiceWithHost(
		t,
		scopes,
		catalog,
		host,
		&deadlineInvocationRuntime{},
		func() time.Time { return now },
		nil,
		func() time.Duration { return time.Millisecond },
	)

	result, err := service.InvokeModelWithLease(context.Background(), invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"))
	if !errors.Is(err, models.ErrInferenceTimeout) {
		t.Fatalf("timeout = %v, want ErrInferenceTimeout", err)
	}
	if result.Status != models.ModelInvocationStatusFailed ||
		result.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("result = %#v, want failed released timeout outcome", result)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want 1", host.releaseCalls)
	}
}

func TestCancelInvocationReleasesAcceptedInvocationCapacity(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-cancel", "scoped-model", "hold")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, holdInvocationRuntime{}, fixedClock(), nil)

	accepted, err := service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, lease, "worker-1", "scoped-model", "hold"),
	)
	if err != nil {
		t.Fatalf("InvokeModelWithLease hold: %v", err)
	}
	if accepted.Status != models.ModelInvocationStatusAccepted ||
		accepted.LeaseDisposition != models.InvocationLeaseRetained {
		t.Fatalf("accepted = %#v, want retained accepted invocation", accepted)
	}

	cancelled, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope:      scope,
		Invocation: accepted.Invocation,
	})
	if err != nil {
		t.Fatalf("CancelInvocation: %v", err)
	}
	if cancelled.Outcome != models.InvocationCancellationRequested ||
		cancelled.Status != models.ModelInvocationStatusCancelled ||
		cancelled.LeaseDisposition != models.InvocationLeaseReleased {
		t.Fatalf("cancelled = %#v, want cancelled released outcome", cancelled)
	}

	repeated, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope:      scope,
		Invocation: accepted.Invocation,
	})
	if err != nil {
		t.Fatalf("CancelInvocation repeated: %v", err)
	}
	if repeated.Outcome != models.InvocationCancellationAlreadyCancelled {
		t.Fatalf("repeated cancellation = %#v, want ALREADY_CANCELLED", repeated)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want 1", host.releaseCalls)
	}
}

func TestCancelInvocationReportsAlreadyCompletedForFinishedInvocation(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-cancel-complete", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, &recordingInvocationRuntime{}, fixedClock(), nil)

	result, err := service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"),
	)
	if err != nil {
		t.Fatalf("InvokeModelWithLease: %v", err)
	}

	cancelled, err := service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope:      scope,
		Invocation: result.Invocation,
	})
	if err != nil {
		t.Fatalf("CancelInvocation completed: %v", err)
	}
	if cancelled.Outcome != models.InvocationCancellationAlreadyCompleted {
		t.Fatalf("late cancellation = %#v, want ALREADY_COMPLETED", cancelled)
	}
}

func TestInvokeContextCancellationConvergesWithExplicitCancel(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-context-cancel", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, &deadlineInvocationRuntime{}, fixedClock(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	contextResult, err := service.InvokeModelWithLease(
		ctx,
		invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"),
	)
	if !errors.Is(err, models.ErrInferenceCancelled) {
		t.Fatalf("context cancellation = %v, want ErrInferenceCancelled", err)
	}
	if contextResult.Status != models.ModelInvocationStatusCancelled ||
		contextResult.LeaseDisposition != models.InvocationLeaseReleased ||
		contextResult.CancellationOutcome != models.InvocationCancellationRequested {
		t.Fatalf("context cancellation = %#v, want cancelled released outcome", contextResult)
	}
	if host.releaseCalls != 1 {
		t.Fatalf("lease release calls = %d, want 1", host.releaseCalls)
	}
}

func TestCancelInvocationRejectsUnknownInvocation(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-cancel-unknown", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	service := newInferenceServiceWithHost(
		t,
		scopes,
		catalog,
		&recordingInferenceHost{},
		&recordingInvocationRuntime{},
		fixedClock(),
		nil,
	)
	unknown, err := (models.ModelInvocationRef{}).Parse("models-inference:unknown")
	if err != nil {
		t.Fatalf("parse unknown invocation: %v", err)
	}
	_, err = service.CancelInvocation(context.Background(), models.CancelInvocationRequest{
		Scope:      scope,
		Invocation: unknown,
	})
	if !errors.Is(err, models.ErrInvocationNotFound) {
		t.Fatalf("CancelInvocation unknown = %v, want ErrInvocationNotFound", err)
	}
}

func newInferenceService(
	t *testing.T,
	issuer string,
	host *recordingInferenceHost,
) inference.Service {
	t.Helper()
	scopes, scope := openInferenceScope(t, issuer, "scoped-model", "generate")
	if host == nil {
		lease := mustLeaseRef(t, "lease-1")
		host = &recordingInferenceHost{
			leases: map[string]models.ModelLease{
				lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
			},
		}
	}
	return newInferenceServiceWithHost(
		t,
		scopes,
		mustCatalog(t, scopes),
		host,
		&recordingInvocationRuntime{},
		fixedClock(),
		nil,
	)
}

func newInferenceServiceWithHost(
	t *testing.T,
	scopes runtimescopes.Service,
	catalog modelcatalog.Service,
	host runtimehost.Service,
	runtime internalservice.InvocationRuntime,
	clock func() time.Time,
	assets scopedassets.Service,
	deadline ...func() time.Duration,
) inference.Service {
	t.Helper()
	if assets == nil {
		assets = availableInferenceAssets{}
	}
	var executionDeadline func() time.Duration
	if len(deadline) > 0 {
		executionDeadline = deadline[0]
	}
	registrar := mustTestArtifactRegistrar(t)
	return internalservice.New(
		scopes,
		assets,
		catalog,
		host,
		runtime,
		registrar,
		clock,
		executionDeadline,
	)
}

func mustTestArtifactRegistrar(t *testing.T) *inferenceartifacts.Registrar {
	t.Helper()
	registrar, err := inferenceartifacts.NewRegistrar(&memoryArtifactFilesystem{files: map[string]string{}})
	if err != nil {
		t.Fatalf("construct artifact registrar: %v", err)
	}
	return registrar
}

func openInferenceScope(
	t *testing.T,
	issuer string,
	model string,
	operation string,
) (runtimescopes.Service, models.RuntimeScopeRef) {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return issuer })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{
				Workers: []models.RuntimeWorker{inferenceWorker(model, operation)},
			}
		},
	})
	if err != nil {
		t.Fatalf("open scope: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(privateRef))
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	return scopes, scope
}

func inferenceWorker(model, operation string) models.RuntimeWorker {
	return models.RuntimeWorker{
		Name: "worker", Type: models.RuntimeWorkerTypeInference,
		Model: model, ModelLocality: models.RuntimeModelLocalityLocal,
		Operations: []models.RuntimeOperation{{
			Name: operation,
			Inputs: []models.RuntimeOperationSlot{{
				Name: "input", ContentTypes: []string{models.RuntimeContentTypeText},
			}},
		}},
		Resources: []models.RuntimeResource{{Name: "model-cache"}},
	}
}

func mustCatalog(t *testing.T, scopes runtimescopes.Service) modelcatalog.Service {
	t.Helper()
	service, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}
	return service
}

func invokeRequest(
	scope models.RuntimeScopeRef,
	lease models.ModelLeaseRef,
	holder string,
	model string,
	operation string,
) models.InvokeModelRequest {
	return models.InvokeModelRequest{
		Scope: scope, Lease: lease, Holder: holder,
		ModelName: model, Operation: operation,
	}
}

func activeLease(
	scope models.RuntimeScopeRef,
	lease models.ModelLeaseRef,
	model string,
	holder string,
) models.ModelLease {
	return models.ModelLease{
		Lease: lease, Scope: scope, ModelName: model, Holder: holder,
		Status:    models.ModelLeaseStatusActive,
		ExpiresAt: time.Date(2026, time.July, 28, 13, 0, 0, 0, time.UTC),
	}
}

func fixedClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	}
}

func mustScopeRef(t *testing.T, value string) models.RuntimeScopeRef {
	t.Helper()
	ref, err := (models.RuntimeScopeRef{}).Parse(value)
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	return ref
}

func mustLeaseRef(t *testing.T, value string) models.ModelLeaseRef {
	t.Helper()
	ref, err := (models.ModelLeaseRef{}).Parse(value)
	if err != nil {
		t.Fatalf("parse lease: %v", err)
	}
	return ref
}

type recordingInvocationRuntime struct {
	invokeCalls     int
	reusedHostSlots int
	content         []models.InferenceContent
	artifactSources []inference.InvocationArtifactSource
	invokeErr       error
}

func (runtime *recordingInvocationRuntime) Invoke(
	_ context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	runtime.invokeCalls++
	if request.HostSlot.Reused {
		runtime.reusedHostSlots++
	}
	if runtime.invokeErr != nil {
		return inference.InvocationRuntimeResult{}, runtime.invokeErr
	}
	result := inference.InvocationRuntimeResult{}
	if runtime.content != nil {
		result.Content = append([]models.InferenceContent(nil), runtime.content...)
	}
	if runtime.artifactSources != nil {
		result.Artifacts = append([]inference.InvocationArtifactSource(nil), runtime.artifactSources...)
	} else if len(result.Content) == 0 {
		result.Content = []models.InferenceContent{{
			ContentType: request.Request.Input.ContentType,
			Content:     request.Request.Input.Content,
		}}
	}
	return result, nil
}

type holdInvocationRuntime struct{}

func (holdInvocationRuntime) Invoke(
	context.Context,
	inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	return inference.InvocationRuntimeResult{}, inference.ErrInvocationInFlight
}

type deadlineInvocationRuntime struct{}

func (deadlineInvocationRuntime) Invoke(
	ctx context.Context,
	_ inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	<-ctx.Done()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return inference.InvocationRuntimeResult{}, models.ErrInferenceTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return inference.InvocationRuntimeResult{}, fmt.Errorf("%w: %v", models.ErrInferenceCancelled, ctx.Err())
	}
	return inference.InvocationRuntimeResult{}, ctx.Err()
}

type recordingInferenceHost struct {
	leases       map[string]models.ModelLease
	warmHosts    map[string]bool
	ensureCalls  int
	releaseCalls int
}

var _ runtimehost.Service = (*recordingInferenceHost)(nil)

func (host *recordingInferenceHost) hostKey(scope models.RuntimeScopeRef, modelName string) string {
	return scope.String() + ":" + modelName
}

func (host *recordingInferenceHost) InspectModelHost(
	_ context.Context,
	request models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	if host.warmHosts == nil {
		host.warmHosts = make(map[string]bool)
	}
	if host.warmHosts[host.hostKey(request.Scope, request.Name)] {
		return models.InspectModelHostResult{
			Host: models.ModelHostSnapshot{
				Scope:          request.Scope,
				ModelName:      request.Name,
				ReadinessState: models.ReadinessStateReady,
			},
		}, nil
	}
	return models.InspectModelHostResult{}, models.ErrHostRuntimeNotReady
}

func (host *recordingInferenceHost) EnsureModelHost(
	_ context.Context,
	request models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	if host.warmHosts == nil {
		host.warmHosts = make(map[string]bool)
	}
	key := host.hostKey(request.Scope, request.Name)
	outcome := models.HostEnsureBecameReady
	if host.warmHosts[key] {
		outcome = models.HostEnsureAlreadyReady
	} else {
		host.ensureCalls++
		host.warmHosts[key] = true
	}
	return models.EnsureModelHostResult{
		Host: models.ModelHostSnapshot{
			Scope:          request.Scope,
			ModelName:      request.Name,
			ReadinessState: models.ReadinessStateReady,
		},
		Outcome: outcome,
	}, nil
}

func (host *recordingInferenceHost) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (host *recordingInferenceHost) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (host *recordingInferenceHost) GetModelLease(
	_ context.Context,
	request models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	lease, ok := host.leases[request.Lease.String()]
	if !ok || lease.Scope != request.Scope {
		return models.GetModelLeaseResult{}, models.ErrHostLeaseNotFound
	}
	if lease.Status == models.ModelLeaseStatusExpired {
		return models.GetModelLeaseResult{Lease: lease}, models.ErrHostLeaseExpired
	}
	return models.GetModelLeaseResult{Lease: lease}, nil
}

func (host *recordingInferenceHost) ReleaseModelLease(
	_ context.Context,
	request models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	host.releaseCalls++
	lease, ok := host.leases[request.Lease.String()]
	if !ok || lease.Scope != request.Scope {
		return models.ReleaseModelLeaseResult{}, models.ErrHostLeaseNotFound
	}
	lease.Status = models.ModelLeaseStatusReleased
	host.leases[request.Lease.String()] = lease
	return models.ReleaseModelLeaseResult{
		Lease:   lease,
		Outcome: models.ModelLeaseReleased,
	}, nil
}

type availableInferenceAssets struct{}

var _ scopedassets.Service = availableInferenceAssets{}

func (availableInferenceAssets) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (availableInferenceAssets) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (availableInferenceAssets) ResolveRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheLayout, error) {
	return scopedassets.RuntimeCacheLayout{}, models.ErrUnsupportedOperation
}

func (availableInferenceAssets) InspectRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheInspection, error) {
	return scopedassets.RuntimeCacheInspection{
		Supported: true,
		Installed: true,
	}, nil
}

type recordingInferenceAssets struct {
	inspection scopedassets.RuntimeCacheInspection
	err        error
}

var _ scopedassets.Service = (*recordingInferenceAssets)(nil)

func (assets *recordingInferenceAssets) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *recordingInferenceAssets) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (assets *recordingInferenceAssets) ResolveRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheLayout, error) {
	return scopedassets.RuntimeCacheLayout{}, models.ErrUnsupportedOperation
}

func (assets *recordingInferenceAssets) InspectRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheInspection, error) {
	if assets.err != nil {
		return scopedassets.RuntimeCacheInspection{}, assets.err
	}
	return assets.inspection, nil
}

func TestInputEchoInvocationRuntimeReturnsDetachedContent(t *testing.T) {
	t.Parallel()

	result, err := inference.InputEchoInvocationRuntime{}.Invoke(
		context.Background(),
		inference.InvocationRuntimeRequest{
			Request: models.InvokeModelRequest{
				Input: models.InferenceInput{ContentType: "text/plain", Content: "hello"},
			},
		},
	)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Content != "hello" {
		t.Fatalf("content = %#v, want echoed input", result.Content)
	}
}

func TestInvokeModelWithLeaseReusesWarmHostSlotForConsecutiveInvokes(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-handle-reuse", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	leaseTwo := mustLeaseRef(t, "lease-2")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String():    activeLease(scope, lease, "scoped-model", "worker-1"),
			leaseTwo.String(): activeLease(scope, leaseTwo, "scoped-model", "worker-1"),
		},
		warmHosts: make(map[string]bool),
	}
	runtime := &recordingInvocationRuntime{}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, runtime, fixedClock(), nil)

	_, err := service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"),
	)
	if err != nil {
		t.Fatalf("first invoke: %v", err)
	}
	_, err = service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, leaseTwo, "worker-1", "scoped-model", "generate"),
	)
	if err != nil {
		t.Fatalf("second invoke: %v", err)
	}
	if host.ensureCalls != 1 {
		t.Fatalf("ensure calls = %d, want 1 shared host slot", host.ensureCalls)
	}
	if runtime.reusedHostSlots != 1 {
		t.Fatalf("reused host slots = %d, want 1", runtime.reusedHostSlots)
	}
}

func TestInvokeModelWithLeaseReturnsDetachedArtifactMetadata(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-artifact", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
		warmHosts: make(map[string]bool),
	}
	runtime := &recordingInvocationRuntime{
		artifactSources: []inference.InvocationArtifactSource{{
			RefValue:   "models-inference:artifact:detached",
			SourcePath: "runtime/output.txt",
			Name:       "result.txt",
			MediaType:  "text/plain",
			SizeBytes:  19,
			Properties: map[string]string{"digest": "sha256:detached"},
		}},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, runtime, fixedClock(), nil)

	result, err := service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"),
	)
	if err != nil {
		t.Fatalf("InvokeModelWithLease: %v", err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Artifact.IsZero() {
		t.Fatalf("artifacts = %#v, want detached opaque artifact", result.Artifacts)
	}
	if result.Artifacts[0].Name != "result.txt" ||
		result.Artifacts[0].Properties["digest"] != "sha256:detached" {
		t.Fatalf("artifact metadata = %#v, want detached facts", result.Artifacts[0])
	}
	cloned := result.Clone()
	cloned.Artifacts[0].Properties["digest"] = "peer-mutated"
	if result.Artifacts[0].Properties["digest"] != "sha256:detached" {
		t.Fatalf("artifact metadata retained peer mutation: %#v", result.Artifacts)
	}
}

func TestInvokeModelWithLeaseRejectsInvalidArtifactReference(t *testing.T) {
	t.Parallel()

	scopes, scope := openInferenceScope(t, "invoke-artifact-invalid", "scoped-model", "generate")
	catalog := mustCatalog(t, scopes)
	lease := mustLeaseRef(t, "lease-1")
	host := &recordingInferenceHost{
		leases: map[string]models.ModelLease{
			lease.String(): activeLease(scope, lease, "scoped-model", "worker-1"),
		},
		warmHosts: make(map[string]bool),
	}
	runtime := &recordingInvocationRuntime{
		artifactSources: []inference.InvocationArtifactSource{{
			RefValue: "   ",
			Name:     "result.txt",
		}},
	}
	service := newInferenceServiceWithHost(t, scopes, catalog, host, runtime, fixedClock(), nil)

	result, err := service.InvokeModelWithLease(
		context.Background(),
		invokeRequest(scope, lease, "worker-1", "scoped-model", "generate"),
	)
	if !errors.Is(err, models.ErrInferenceArtifactInvalid) {
		t.Fatalf("invalid artifact = %v, want ErrInferenceArtifactInvalid", err)
	}
	for _, other := range []error{
		models.ErrInferenceTimeout,
		models.ErrInferenceCancelled,
		models.ErrAssetUnavailable,
	} {
		if errors.Is(err, other) {
			t.Fatalf("invalid artifact must stay distinct from %v: %v", other, err)
		}
	}
	if result.Status != models.ModelInvocationStatusFailed {
		t.Fatalf("result status = %q, want FAILED", result.Status)
	}
}

type memoryArtifactFilesystem struct {
	files map[string]string
}

func (fs *memoryArtifactFilesystem) Open(path string) (io.ReadCloser, error) {
	content, ok := fs.files[path]
	if !ok {
		return nil, fmt.Errorf("open %q: not found", path)
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func (fs *memoryArtifactFilesystem) Create(path string) (io.WriteCloser, error) {
	return &memoryArtifactWriter{fs: fs, path: path}, nil
}

type memoryArtifactWriter struct {
	fs   *memoryArtifactFilesystem
	path string
	buf  strings.Builder
}

func (w *memoryArtifactWriter) Write(p []byte) (int, error) {
	return w.buf.Write(p)
}

func (w *memoryArtifactWriter) Close() error {
	w.fs.files[w.path] = w.buf.String()
	return nil
}
