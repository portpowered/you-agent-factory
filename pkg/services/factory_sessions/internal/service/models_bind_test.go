package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type recordingLeaseCloser func() error

func (close recordingLeaseCloser) Close() error { return close() }

func TestRuntimeOpeningCleanupReleasesLeaseAfterTerminalPublicationFailure(t *testing.T) {
	t.Parallel()
	cleanup := &runtimeOpeningCleanup{}
	cause := errors.New("terminal publication failed")
	var closes, releases int
	cleanup.OwnRecordingTarget(recordingLeaseCloser(func() error { releases++; return nil }))
	cleanup.Add(func() error {
		closes++
		return &recordings.RecordingCleanupError{Cause: cause, Complete: true}
	})
	err := cleanup.Close()
	var terminal *recordings.RecordingCleanupError
	if !errors.Is(err, cause) || !errors.As(err, &terminal) || !terminal.Complete || releases != 1 {
		t.Fatalf("terminal cleanup = %v; releases=%d", err, releases)
	}
	if err := cleanup.Close(); err != nil || closes != 1 || releases != 1 {
		t.Fatalf("completed cleanup retried: err=%v closes=%d releases=%d", err, closes, releases)
	}
}

func TestRuntimeOpeningCleanupRetainsRecordingLeaseUntilConsumerJoins(t *testing.T) {
	t.Parallel()
	cleanup := &runtimeOpeningCleanup{}
	consumerErr := errors.New("writer has not joined")
	joined := false
	releases := 0
	cleanup.OwnRecordingTarget(recordingLeaseCloser(func() error {
		releases++
		if !joined {
			t.Error("recording ownership released before writer joined")
		}
		return nil
	}))
	cleanup.Add(func() error {
		if !joined {
			return consumerErr
		}
		return nil
	})
	if err := cleanup.Close(); !errors.Is(err, consumerErr) || releases != 0 {
		t.Fatalf("failed join: err=%v releases=%d", err, releases)
	}
	joined = true
	if err := cleanup.Close(); err != nil || releases != 1 {
		t.Fatalf("successful retry: err=%v releases=%d", err, releases)
	}
	if err := cleanup.Close(); err != nil || releases != 1 {
		t.Fatalf("repeat close: err=%v releases=%d", err, releases)
	}
}

func TestProjectOperatorModelOverlaysDetachesSourceSettings(t *testing.T) {
	if got := projectOperatorModelOverlays(nil); got != nil {
		t.Fatalf("empty model overlays = %+v", got)
	}
	source, backend := "model.gguf", "llama"
	policy := operatorconfig.LoadPolicy("on-demand")
	configured := map[string]operatorconfig.ModelConfig{
		"review": {Source: &source, Backend: &backend, Operations: []string{"chat"}, LoadPolicy: &policy},
	}
	projected := projectOperatorModelOverlays(configured)
	model := projected["review"]
	if model.Source == nil || *model.Source != source || model.Backend == nil || *model.Backend != backend || len(model.Operations) != 1 || model.LoadPolicy == nil {
		t.Fatalf("projected model = %+v", model)
	}
	source = "changed"
	backend = "changed"
	configured["review"].Operations[0] = "embedding"
	policy = "changed"
	if *model.Source != "model.gguf" || *model.Backend != "llama" || model.Operations[0] != "chat" || string(*model.LoadPolicy) != "on-demand" {
		t.Fatalf("model overlay retained source aliases: %+v", model)
	}
}

type recordingModelsService struct {
	openRequests  []models.OpenRuntimeScopeRequest
	closeRequests []models.CloseRuntimeScopeRequest
	events        *[]string
}

type failedOpeningModelsService struct {
	*recordingModelsService
	openingErr error
	closingErr error
	noScope    bool
}

func (fake *failedOpeningModelsService) OpenRuntimeScope(ctx context.Context, request models.OpenRuntimeScopeRequest) (models.OpenRuntimeScopeResult, error) {
	opened, err := fake.recordingModelsService.OpenRuntimeScope(ctx, request)
	if err != nil {
		return opened, err
	}
	if fake.noScope {
		opened.Scope = models.RuntimeScopeRef{}
	}
	return opened, fake.openingErr
}

func (fake *failedOpeningModelsService) CloseRuntimeScope(ctx context.Context, request models.CloseRuntimeScopeRequest) (models.CloseRuntimeScopeResult, error) {
	if err := ctx.Err(); err != nil {
		return models.CloseRuntimeScopeResult{}, err
	}
	closed, err := fake.recordingModelsService.CloseRuntimeScope(ctx, request)
	if err != nil {
		return closed, err
	}
	if len(fake.closeRequests) == 1 && fake.closingErr != nil {
		return models.CloseRuntimeScopeResult{}, fake.closingErr
	}
	return closed, nil
}

func TestBindModelsRuntimeScopeRetainsFailedOpeningForCleanupRetry(t *testing.T) {
	t.Parallel()
	for _, noScope := range []bool{false, true} {
		t.Run(fmt.Sprintf("noScope=%t", noScope), func(t *testing.T) {
			t.Parallel()
			openingErr, closingErr := errors.New("Models opening failed"), errors.New("Models release failed")
			fake := &failedOpeningModelsService{
				recordingModelsService: &recordingModelsService{},
				openingErr:             openingErr, closingErr: closingErr, noScope: noScope,
			}
			ctx, cancel := context.WithCancel(t.Context())
			bind, err := (&RuntimeResourceAcquisition{modelService: fake}).bindModelsRuntimeScope(ctx, "", nil, nil)
			if !errors.Is(err, openingErr) || bind.Root != fake || bind.Scope.IsZero() != noScope {
				t.Fatalf("failed opening = (%+v, %v), want issued scope and original failure", bind, err)
			}
			cancel()
			cleanup := &runtimeOpeningCleanup{}
			cleanup.OwnModelsScope(context.WithoutCancel(ctx), bind)
			activation, validationErr := newRuntimeActivation(runtimeProducts{closeArtifacts: cleanup.Close})
			if validationErr == nil || activation.Service != nil {
				t.Fatal("failed opening published a runnable activation")
			}
			closeErr := activation.Close(ctx)
			if noScope {
				if closeErr != nil || len(fake.closeRequests) != 0 {
					t.Fatalf("unissued scope cleanup = %v, calls = %d", closeErr, len(fake.closeRequests))
				}
				return
			}
			if !errors.Is(closeErr, closingErr) {
				t.Fatalf("first cleanup = %v, want release failure", closeErr)
			}
			for range 2 {
				if err := activation.Close(ctx); err != nil {
					t.Fatalf("cleanup retry = %v", err)
				}
			}
			if len(fake.closeRequests) != 2 {
				t.Fatalf("release calls = %d, want failed release plus one successful retry", len(fake.closeRequests))
			}
			assertOnlyOwnedModelsScopeClosed(t, fake.closeRequests, bind.Scope)
		})
	}
}

func assertOnlyOwnedModelsScopeClosed(t *testing.T, requests []models.CloseRuntimeScopeRequest, scope models.RuntimeScopeRef) {
	t.Helper()
	for _, request := range requests {
		if request.Scope != scope {
			t.Fatalf("closed scope = %v, want owned scope %v", request.Scope, scope)
		}
	}
}

type earlyOpeningDurableExecution struct {
	durableexecution.Service
	closeErr error
	closes   int
	events   *[]string
}

func (*earlyOpeningDurableExecution) RecordPetriTokenMutations(string, []factorydefinitions.TokenMutationRecord) error {
	return nil
}

func (*earlyOpeningDurableExecution) PublishWorkerProgress(workers.ProgressFragment) {}

func (execution *earlyOpeningDurableExecution) Close() error {
	execution.closes++
	*execution.events = append(*execution.events, "durable-close")
	if execution.closes == 1 {
		return execution.closeErr
	}
	return nil
}

func TestRuntimeOpeningRetainsEarlyScopeCleanupBeforeReturningFailure(t *testing.T) {
	t.Parallel()
	for _, durableFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("durableFailure=%t", durableFailure), func(t *testing.T) {
			t.Parallel()
			openingErr, closeErr := errors.New("scope opening failed"), errors.New("durable release failed")
			var events []string
			execution := &earlyOpeningDurableExecution{closeErr: closeErr, events: &events}
			modelService := &failedOpeningModelsService{
				recordingModelsService: &recordingModelsService{events: &events}, openingErr: openingErr,
			}
			cleanup := &runtimeOpeningCleanup{}
			acquisition := NewRuntimeResourceAcquisition(resourceOpeningStub(execution, func(ctx context.Context) error {
				if ctx.Err() != nil {
					t.Fatal("cleanup retained cancellation")
				}
				return execution.Close()
			}, openingErr, durableFailure), modelService, nil)
			ctx, cancel := context.WithCancel(t.Context())
			_, err := acquisition.Acquire(ctx, resourceRequest("early-opening"), openingCoordinatorClock{}, zap.NewNop(), cleanup)
			cancel()
			err = errors.Join(err, cleanup.Close())
			if !errors.Is(err, openingErr) || !errors.Is(err, closeErr) {
				t.Fatalf("opening failure = %v, want opening and cleanup causes", err)
			}
			if execution.closes != 1 {
				t.Fatalf("durable closes = %d", execution.closes)
			}
			if len(modelService.closeRequests) != 0 {
				t.Fatal("failed durable consumer lost its Models dependency before cleanup retry")
			}
			for range 2 {
				if err := cleanup.Close(); err != nil {
					t.Fatalf("explicit cleanup retry: %v", err)
				}
			}
			if execution.closes != 2 {
				t.Fatalf("durable closes = %d, want one failed release and one successful retry", execution.closes)
			}
			wantModelCloses := 1
			wantEvents := []string{"models-open", "durable-close", "durable-close", "models-close"}
			if durableFailure {
				wantModelCloses = 0
				wantEvents = []string{"durable-close", "durable-close"}
			}
			if len(modelService.closeRequests) != wantModelCloses {
				t.Fatalf("Models closes = %d, want %d owned scopes only", len(modelService.closeRequests), wantModelCloses)
			}
			if !slices.Equal(events, wantEvents) {
				t.Fatalf("scope lifetime = %v, want %v", events, wantEvents)
			}
		})
	}
}

// An invalid observation owner must be rejected while its cleanup is already
// registered, before opening any resources that depend on those observations.
func TestRuntimeOpeningRejectsMissingObservationsBeforeModelsAndRetainsCleanup(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("durable release failed")
	var events []string
	closer := &earlyOpeningDurableExecution{closeErr: closeErr, events: &events}
	execution := &mutationOnlyClosableOpeningOwner{closer: closer}
	modelsService := &recordingModelsService{events: &events}
	cleanup := &runtimeOpeningCleanup{}
	acquisition := NewRuntimeResourceAcquisition(resourceOpeningStub(execution, func(ctx context.Context) error {
		if ctx.Err() != nil {
			t.Fatal("cleanup retained cancellation")
		}
		return execution.Close()
	}, nil, false), modelsService, nil)
	_, err := acquisition.Acquire(t.Context(), resourceRequest("invalid-observations"), openingCoordinatorClock{}, zap.NewNop(), cleanup)
	err = errors.Join(err, cleanup.Close())
	if err == nil || !strings.Contains(err.Error(), "must record mutations and publish worker progress") || !errors.Is(err, closeErr) {
		t.Fatalf("invalid observation opening = %v, want capability and release failures", err)
	}
	if len(modelsService.openRequests) != 0 {
		t.Fatalf("invalid owner opened Models=%d", len(modelsService.openRequests))
	}
	for range 2 {
		if err := cleanup.Close(); err != nil {
			t.Fatalf("retry owned durable cleanup: %v", err)
		}
	}
	if !slices.Equal(events, []string{"durable-close", "durable-close"}) {
		t.Fatalf("cleanup effects = %v, want failed release plus one successful retry", events)
	}
}

type mutationOnlyClosableOpeningOwner struct {
	mutationOnlyOpeningOwner
	closer *earlyOpeningDurableExecution
}

func (owner *mutationOnlyClosableOpeningOwner) Close() error { return owner.closer.Close() }

func (fake *recordingModelsService) OpenRuntimeScope(
	_ context.Context,
	request models.OpenRuntimeScopeRequest,
) (models.OpenRuntimeScopeResult, error) {
	fake.openRequests = append(fake.openRequests, request)
	if fake.events != nil {
		*fake.events = append(*fake.events, "models-open")
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:1")
	if err != nil {
		return models.OpenRuntimeScopeResult{}, err
	}
	return models.OpenRuntimeScopeResult{Scope: scope}, nil
}

func (fake *recordingModelsService) CloseRuntimeScope(
	_ context.Context,
	request models.CloseRuntimeScopeRequest,
) (models.CloseRuntimeScopeResult, error) {
	fake.closeRequests = append(fake.closeRequests, request)
	if fake.events != nil {
		*fake.events = append(*fake.events, "models-close")
	}
	return models.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
}

func (fake *recordingModelsService) ListCatalog(
	context.Context,
	models.ListModelsRequest,
) (models.ListModelsResult, error) {
	return models.ListModelsResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) GetCatalogModel(
	context.Context,
	models.GetModelRequest,
) (models.GetModelResult, error) {
	return models.GetModelResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) GetModelReadiness(
	context.Context,
	models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	return models.GetModelReadinessResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) ResolveModelReference(
	context.Context,
	models.ResolveModelReferenceRequest,
) (models.ResolveModelReferenceResult, error) {
	return models.ResolveModelReferenceResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) PreflightModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PreflightModelAssetsResult, error) {
	return models.PreflightModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) PrepareModelAssets(
	context.Context,
	models.PrepareModelAssetsRequest,
) (models.PrepareModelAssetsResult, error) {
	return models.PrepareModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InspectModelAssets(
	context.Context,
	models.InspectModelAssetsRequest,
) (models.InspectModelAssetsResult, error) {
	return models.InspectModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) RemoveModelAssets(
	context.Context,
	models.RemoveModelAssetsRequest,
) (models.RemoveModelAssetsResult, error) {
	return models.RemoveModelAssetsResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InvokeModelWithLease(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InvokeModel(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	return models.InvokeModelResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) CancelInvocation(
	context.Context,
	models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) ListModels(context.Context) (models.List, error) {
	return models.List{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) PullModelForScope(
	context.Context,
	models.PullModelRequest,
) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) InspectRuntime(context.Context, string) (models.Runtime, error) {
	return models.Runtime{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) AcquireLease(
	context.Context,
	models.AcquireLeaseRequest,
) (models.HostLease, error) {
	return models.HostLease{}, models.ErrUnsupportedOperation
}

func (fake *recordingModelsService) ReleaseLease(
	context.Context,
	models.ReleaseLeaseRequest,
) error {
	return models.ErrUnsupportedOperation
}

func TestBindModelsRuntimeScopeOpensDetachedScope(t *testing.T) {
	t.Parallel()

	fake := &recordingModelsService{}
	runtimeConfig := &models.RuntimeConfig{
		FactoryDirectory: "/factory",
		BaseDirectory:    "/runtime",
	}
	bind, err := (&RuntimeResourceAcquisition{modelService: fake}).bindModelsRuntimeScope(
		context.Background(),
		"/cache/models",
		runtimeConfig,
		nil,
	)
	if err != nil {
		t.Fatalf("bindModelsRuntimeScope() error = %v, want nil", err)
	}
	if root, ok := bind.Root.(*recordingModelsService); !ok || root != fake {
		t.Fatal("bindModelsRuntimeScope() did not keep the process-scoped Models root")
	}
	if bind.Scope.IsZero() {
		t.Fatal("bindModelsRuntimeScope() returned zero runtime scope")
	}
	if len(fake.openRequests) != 1 {
		t.Fatalf("OpenRuntimeScope requests = %d, want 1", len(fake.openRequests))
	}
	got := fake.openRequests[0].Config
	if got.CacheDirectory != "/cache/models" {
		t.Fatalf("scope cache directory = %q, want /cache/models", got.CacheDirectory)
	}
	if got.Runtime.FactoryDirectory != runtimeConfig.FactoryDirectory {
		t.Fatalf("scope runtime factory directory = %q, want %q", got.Runtime.FactoryDirectory, runtimeConfig.FactoryDirectory)
	}
}

func TestAssembleRuntimeProductsCarriesModelsScopeIntoOpenedRuntime(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:assembled")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}

	opened := assembleRuntimeProducts(
		context.Background(),
		nil,
		nil,
		scope,
		inertHostedInstance{},
		nil,
		nil,
		"/factory",
		"runtime-1",
		"backend-1",
		func() error { return nil },
	)

	if opened.modelsScope != scope {
		t.Fatalf("opened Models scope = %q, want %q", opened.modelsScope, scope)
	}
}

func TestAssembleRuntimeProductsRetainsEffectiveSessionFacts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, requested, resolved, want string
		err                             error
	}{
		{name: "resolved", requested: " selected ", resolved: " canonical ", want: "canonical"},
		{name: "empty projection", requested: " selected ", want: "selected"},
		{name: "missing", requested: " selected ", want: "selected", err: factorysessions.ErrDurableSessionNotFound},
		{name: "default", resolved: "canonical"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			gateway := &runtimeProductsSessionsRole{readLiveSession: func(id string) (factorysessions.LiveControlSnapshot, error) {
				calls++
				if id != "selected" {
					t.Fatalf("lookup ID = %q", id)
				}
				return factorysessions.LiveControlSnapshot{Context: factorysessions.ProjectionContext{FactorySessionID: test.resolved}}, test.err
			}}
			scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:assembled")
			if err != nil {
				t.Fatal(err)
			}
			opened := assembleRuntimeProducts(t.Context(), gateway, nil, scope, inertHostedInstance{}, nil, nil,
				"/factory", "runtime-1", "backend-1", nil, test.requested)
			facts := opened.modelInvocation
			if facts.FactorySessionID != test.want || facts.Scope != scope || facts.RuntimeID != "runtime-1" ||
				facts.FactoryDirectory != "/factory" || facts.WorkingDirectory != "/factory" || opened.backendScopeID != "backend-1" {
				t.Fatalf("opened facts = %+v, backend = %q", facts, opened.backendScopeID)
			}
			if (test.requested == "" && calls != 0) || (test.requested != "" && calls != 1) {
				t.Fatalf("ID lookup calls = %d", calls)
			}
			if opened.replayExecution != nil {
				t.Fatal("live opening acquired historical execution")
			}
		})
	}
}

type t17hRuntimeFacts struct {
	inertHostedInstance
	logger *zap.Logger
}

func (instance t17hRuntimeFacts) StreamGeneration() string   { return "generation-selected" }
func (instance t17hRuntimeFacts) RuntimeLogger() *zap.Logger { return instance.logger }
func (instance t17hRuntimeFacts) RuntimeDiagnostics() factoryruntime.RuntimeLogDiagnostics {
	return factoryruntime.RuntimeLogDiagnostics{Path: "/selected/log", StartTimeUTC: time.Unix(17, 0)}
}

func TestT17HAssemblyKeepsGenerationAndDiagnostics(t *testing.T) {
	t.Parallel()
	logger := zap.NewNop()
	opened := assembleRuntimeProducts(t.Context(), nil, nil, models.RuntimeScopeRef{}, t17hRuntimeFacts{logger: logger},
		nil, nil, "/selected", "runtime-selected", "backend-selected", nil, "session-selected")
	if opened.modelInvocation.GenerationID != "generation-selected" || opened.logger != logger ||
		opened.diagnostics.Path != "/selected/log" || !opened.diagnostics.StartTimeUTC.Equal(time.Unix(17, 0)) {
		t.Fatalf("runtime generation/diagnostics drifted: %+v %+v", opened.modelInvocation, opened.diagnostics)
	}
}

func TestAssembledRuntimeResourcesCloseAcquiredResourcesInReverseOrder(t *testing.T) {
	t.Parallel()

	var events []string
	cleanup := &runtimeOpeningCleanup{}
	cleanup.Add(func() error {
		events = append(events, "models-close")
		return nil
	})
	cleanup.Add(func() error {
		events = append(events, "workers-close")
		return nil
	})

	opened := assembleRuntimeProducts(
		context.Background(),
		nil,
		nil,
		models.RuntimeScopeRef{},
		inertHostedInstance{},
		nil,
		nil,
		"/factory",
		"runtime-1",
		"backend-1",
		cleanup.Close,
	)
	if err := opened.closeArtifacts(); err != nil {
		t.Fatalf("opened runtime resource Close() error = %v, want nil", err)
	}
	if !slices.Equal(events, []string{"workers-close", "models-close"}) {
		t.Fatalf("runtime close events = %v, want reverse acquisition order", events)
	}
	if err := opened.closeArtifacts(); err != nil {
		t.Fatalf("second opened runtime resource Close() error = %v, want nil", err)
	}
	if !slices.Equal(events, []string{"workers-close", "models-close"}) {
		t.Fatalf("runtime close events after second close = %v, want each resource closed once", events)
	}
}

func TestRuntimeOpeningCleanupClosesModelsScopeAfterLaterResourceOnFailure(t *testing.T) {
	t.Parallel()

	var events []string
	root := &recordingModelsService{events: &events}
	bind, err := (&RuntimeResourceAcquisition{modelService: root}).bindModelsRuntimeScope(
		context.Background(),
		"/cache/models",
		&models.RuntimeConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("bindModelsRuntimeScope() error = %v, want nil", err)
	}

	cleanup := &runtimeOpeningCleanup{}
	cleanup.OwnModelsScope(context.Background(), bind)
	cleanup.Add(func() error {
		events = append(events, "later-close")
		return nil
	})
	openingErr := errors.New("later opening step failed")
	if err := errors.Join(openingErr, cleanup.Close()); !errors.Is(err, openingErr) {
		t.Fatalf("opening failure cleanup error = %v, want opening failure", err)
	}
	if !slices.Equal(events, []string{"models-open", "later-close", "models-close"}) {
		t.Fatalf("cleanup events = %v, want reverse acquisition order", events)
	}
	if len(root.closeRequests) != 1 || root.closeRequests[0].Scope != bind.Scope {
		t.Fatalf("CloseRuntimeScope requests = %#v, want issued scope exactly once", root.closeRequests)
	}

	if err := cleanup.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
	if len(root.closeRequests) != 1 {
		t.Fatalf("CloseRuntimeScope requests after second close = %d, want 1", len(root.closeRequests))
	}
}

func TestRuntimeOpeningCleanupPreservesPrimaryErrorAndAggregatesCleanupFailures(t *testing.T) {
	t.Parallel()

	var events []string
	firstCleanupErr := errors.New("first cleanup failed")
	secondCleanupErr := errors.New("second cleanup failed")
	openingErr := errors.New("runtime assembly failed")
	cleanup := &runtimeOpeningCleanup{}
	cleanup.Add(func() error {
		events = append(events, "first-close")
		return firstCleanupErr
	})
	cleanup.Add(func() error {
		events = append(events, "second-close")
		return secondCleanupErr
	})

	err := errors.Join(openingErr, cleanup.Close())
	for _, expected := range []error{openingErr, firstCleanupErr, secondCleanupErr} {
		if !errors.Is(err, expected) {
			t.Fatalf("opening failure cleanup error = %v, want to retain %v", err, expected)
		}
	}
	if !slices.Equal(events, []string{"second-close", "first-close"}) {
		t.Fatalf("cleanup events = %v, want reverse acquisition order", events)
	}

	if err := cleanup.Close(); !errors.Is(err, firstCleanupErr) || !errors.Is(err, secondCleanupErr) {
		t.Fatalf("second Close() error = %v, want pending cleanup errors", err)
	}
	if !slices.Equal(events, []string{"second-close", "first-close", "second-close", "first-close"}) {
		t.Fatalf("cleanup events after second Close() = %v, want failed releases retried in reverse order", events)
	}
}

func TestRuntimeOpeningCleanupRetriesOnlyPendingOwnership(t *testing.T) {
	t.Parallel()
	cleanup := &runtimeOpeningCleanup{}
	closeErr := errors.New("resource still owned")
	var events []string
	fail := true
	cleanup.Add(func() error {
		events = append(events, "first")
		return nil
	})
	cleanup.Add(func() error {
		events = append(events, "second")
		if fail {
			return closeErr
		}
		return nil
	})
	cleanup.Add(func() error {
		events = append(events, "third")
		return nil
	})
	if err := cleanup.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close() = %v, want pending ownership", err)
	}
	fail = false
	var closers sync.WaitGroup
	for range 2 {
		closers.Add(1)
		go func() {
			defer closers.Done()
			if err := cleanup.Close(); err != nil {
				t.Errorf("retry Close() = %v", err)
			}
		}()
	}
	closers.Wait()
	if !slices.Equal(events, []string{"third", "second", "first", "second"}) {
		t.Fatalf("release events = %v, want successful resources released once", events)
	}
}

func TestRuntimeOpeningCleanupRetainsOwnershipAddedDuringClose(t *testing.T) {
	t.Parallel()
	cleanup := &runtimeOpeningCleanup{}
	var events []string
	modelService := &recordingModelsService{events: &events}
	bind, err := (&RuntimeResourceAcquisition{modelService: modelService}).bindModelsRuntimeScope(t.Context(), "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cleanup.OwnModelsScope(t.Context(), bind)
	cleanup.Add(func() error {
		events = append(events, "initial")
		cleanup.Add(func() error {
			events = append(events, "later")
			return nil
		})
		return nil
	})
	activation, err := newRuntimeActivation(runtimeProducts{closeArtifacts: cleanup.Close})
	if err == nil || activation == nil || activation.Service != nil {
		t.Fatalf("partial activation = %v, %v, want cleanup without a live service", activation, err)
	}
	if err := activation.Close(t.Context()); !errors.Is(err, errRuntimeOpeningCleanupPending) {
		t.Fatalf("partial activation cleanup = %v, want pending ownership", err)
	}
	if len(modelService.closeRequests) != 0 {
		t.Fatal("Models dependency closed while newly registered consumer still owns cleanup")
	}
	if err := activation.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := activation.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(events, []string{"models-open", "initial", "later", "models-close"}) {
		t.Fatalf("release events = %v, want later ownership preserved", events)
	}
}

func TestRuntimeOpeningCleanupReportsModelsOwnershipAddedDuringRelease(t *testing.T) {
	t.Parallel()
	cleanup := &runtimeOpeningCleanup{}
	var events []string
	modelService := &recordingModelsService{events: &events}
	bind, err := (&RuntimeResourceAcquisition{modelService: modelService}).bindModelsRuntimeScope(t.Context(), "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cleanup.Add(func() error {
		events = append(events, "consumer-close")
		cleanup.OwnModelsScope(t.Context(), bind)
		return nil
	})
	if err := cleanup.Close(); !errors.Is(err, errRuntimeOpeningCleanupPending) {
		t.Fatalf("cleanup = %v, want retained Models ownership", err)
	}
	if len(modelService.closeRequests) != 0 {
		t.Fatal("new Models ownership was released in the original batch")
	}
	for range 2 {
		if err := cleanup.Close(); err != nil {
			t.Fatalf("cleanup retry = %v", err)
		}
	}
	if !slices.Equal(events, []string{"models-open", "consumer-close", "models-close"}) {
		t.Fatalf("release events = %v, want each owned resource released once", events)
	}
	assertOnlyOwnedModelsScopeClosed(t, modelService.closeRequests, bind.Scope)
}

func TestRuntimeOpeningCleanupRetainsDependenciesWhenModelsReleaseAddsConsumer(t *testing.T) {
	t.Parallel()
	for _, releaseFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("releaseFails=%t", releaseFails), func(t *testing.T) {
			t.Parallel()
			cleanup := &runtimeOpeningCleanup{}
			var events []string
			dependency := &recordingModelsService{events: &events}
			scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:1")
			if err != nil {
				t.Fatal(err)
			}
			cleanup.OwnModelsScope(t.Context(), modelsRuntimeBind{Root: dependency, Scope: scope})
			releaseErr := errors.New("Models release failed")
			registering := &consumerRegisteringModelsService{
				recordingModelsService: &recordingModelsService{}, cleanup: cleanup, events: &events,
			}
			if releaseFails {
				registering.failure = releaseErr
			}
			cleanup.OwnModelsScope(t.Context(), modelsRuntimeBind{Root: registering, Scope: scope})
			activation, _ := newRuntimeActivation(runtimeProducts{closeArtifacts: cleanup.Close})
			closeErr := activation.Close(t.Context())
			if releaseFails && !errors.Is(closeErr, releaseErr) {
				t.Fatalf("cleanup = %v, want original release failure", closeErr)
			}
			if !releaseFails && !errors.Is(closeErr, errRuntimeOpeningCleanupPending) {
				t.Fatalf("cleanup = %v, want pending ownership", closeErr)
			}
			if len(dependency.closeRequests) != 0 {
				t.Fatal("Models dependency released before its newly registered consumer")
			}
			for range 2 {
				if err := activation.Close(t.Context()); err != nil {
					t.Fatalf("explicit cleanup retry: %v", err)
				}
			}
			want := []string{"registering-models-close", "new-consumer-close"}
			if releaseFails {
				want = append(want, "registering-models-close")
			}
			want = append(want, "models-close")
			if !slices.Equal(events, want) {
				t.Fatalf("release order = %v, want %v", events, want)
			}
			assertOnlyOwnedModelsScopeClosed(t, dependency.closeRequests, scope)
		})
	}
}

type consumerRegisteringModelsService struct {
	*recordingModelsService
	cleanup *runtimeOpeningCleanup
	events  *[]string
	failure error
	added   bool
}

func (service *consumerRegisteringModelsService) CloseRuntimeScope(_ context.Context, request models.CloseRuntimeScopeRequest) (models.CloseRuntimeScopeResult, error) {
	*service.events = append(*service.events, "registering-models-close")
	if !service.added {
		service.added = true
		service.cleanup.Add(func() error {
			*service.events = append(*service.events, "new-consumer-close")
			return nil
		})
		if service.failure != nil {
			return models.CloseRuntimeScopeResult{}, service.failure
		}
	}
	return models.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
}

func TestRuntimeOpeningCleanupRetainsModelsAcrossConsumerAndDependencyFailures(t *testing.T) {
	t.Parallel()
	consumerErr, modelsErr := errors.New("consumer close failed"), errors.New("Models close failed")
	var events []string
	modelService := &failedOpeningModelsService{
		recordingModelsService: &recordingModelsService{events: &events}, closingErr: modelsErr,
	}
	bind, err := (&RuntimeResourceAcquisition{modelService: modelService}).bindModelsRuntimeScope(t.Context(), "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := &runtimeOpeningCleanup{}
	cleanup.OwnModelsScope(t.Context(), bind)
	consumerCalls := 0
	cleanup.Add(func() error {
		consumerCalls++
		events = append(events, "consumer-close")
		if consumerCalls == 1 {
			return consumerErr
		}
		return nil
	})
	cleanup.Add(func() error { events = append(events, "independent-close"); return nil })
	if err := cleanup.Close(); !errors.Is(err, consumerErr) || len(modelService.closeRequests) != 0 {
		t.Fatalf("consumer cleanup = %v, Models closes = %d, want retained dependency", err, len(modelService.closeRequests))
	}
	if err := cleanup.Close(); !errors.Is(err, modelsErr) {
		t.Fatalf("dependency cleanup = %v, want retryable Models failure", err)
	}
	for range 2 {
		if err := cleanup.Close(); err != nil {
			t.Fatalf("dependency retry = %v", err)
		}
	}
	want := []string{"models-open", "independent-close", "consumer-close", "consumer-close", "models-close", "models-close"}
	if !slices.Equal(events, want) {
		t.Fatalf("release events = %v, want %v", events, want)
	}
	assertOnlyOwnedModelsScopeClosed(t, modelService.closeRequests, bind.Scope)
}

type runtimeProductsSessionsRole struct {
	roles.SessionGateway
	readLiveSession func(string) (factorysessions.LiveControlSnapshot, error)
}

func (role *runtimeProductsSessionsRole) GetFactorySession(_ context.Context, sessionID string) (factorysessions.LiveControlSnapshot, error) {
	return role.readLiveSession(sessionID)
}

type openingCoordinatorProjection struct {
	recordings.ProjectionService
}

type openingCoordinatorClock struct{}

func (openingCoordinatorClock) Now() time.Time {
	return time.Unix(1, 0)
}

type inertHostedInstance struct{}

func (inertHostedInstance) RuntimeService() factoryruntime.Service { return nil }
func (inertHostedInstance) Directory() string                      { return "" }
func (inertHostedInstance) FolderDirectory() string                { return "" }
func (inertHostedInstance) BackendScope() string                   { return "" }
func (inertHostedInstance) StartTime() time.Time                   { return time.Time{} }
func (inertHostedInstance) LoadedRuntimeConfig() factoryruntime.LoadedConfig {
	return nil
}
func (inertHostedInstance) CanonicalEvents() []factorydefinitions.FactoryEvent { return nil }
func (inertHostedInstance) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {
}
func (inertHostedInstance) AddEventTypeRecorderWithReady(func(factorydefinitions.FactoryEventType), func()) {
}
func (inertHostedInstance) StreamGeneration() string { return "" }
func (inertHostedInstance) RuntimeLogger() *zap.Logger {
	return zap.NewNop()
}
func (inertHostedInstance) RuntimeMetrics() factoryruntime.MetricsEmitter { return nil }
func (inertHostedInstance) RuntimeDiagnostics() factoryruntime.RuntimeLogDiagnostics {
	return factoryruntime.RuntimeLogDiagnostics{}
}
func (inertHostedInstance) RecordingLedger() recordings.Ledger { return nil }
func (inertHostedInstance) CloseArtifacts() error              { return nil }
