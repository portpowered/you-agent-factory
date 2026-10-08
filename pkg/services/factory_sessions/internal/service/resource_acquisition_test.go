package service

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func resourceRequest(id string) RuntimeResourceRequest {
	return RuntimeResourceRequest{FactoryRootDir: "/" + id, Configured: preparedRuntime{
		Session:             factorysessions.SessionStartRequest{SessionID: id, RuntimeSelection: &factorysessions.SessionRuntimeSelection{SystemConfigPath: "/" + id + "/operator.json"}},
		Definition:          factorydefinitions.RuntimeSelection{Directory: "/" + id},
		Runtime:             factoryruntime.RuntimeSelection{RuntimeInstanceID: id + "-runtime"},
		ModelCacheDirectory: "/" + id + "/cache",
	}, ModelsRuntime: &models.RuntimeConfig{FactoryDirectory: "/" + id,
		Workers: []models.RuntimeWorker{{Name: "worker", Command: id, Args: []string{id + "-endpoint"}}}}}
}

func resourceOpeningStub(owner durableexecution.Service, release func(context.Context) error, failure error, fail bool) DurableResourceOpening {
	return func(context.Context, string, factorydefinitions.RuntimeSelection, factorysessions.PersistencePolicy, string, string,
		operatorsettings.ResolvedDefaults, RuntimeRoot, factoryruntime.Clock, providers.Service, *workers.MockWorkersConfig) (DurableExecution, error) {
		if fail {
			return DurableExecution{Release: release}, failure
		}
		return DurableExecution{Service: owner, Release: release}, nil
	}
}

// The Models collaborator retains only the supplied facts. A gate forces two
// acquisitions to overlap before either can return its distinct scope.
type acquisitionModels struct {
	models.Service
	mu       sync.Mutex
	requests map[string]models.RuntimeScopeConfig
	closed   []models.RuntimeScopeRef
	entered  chan string
	resume   chan struct{}
}

func (fake *acquisitionModels) OpenRuntimeScope(ctx context.Context, request models.OpenRuntimeScopeRequest) (models.OpenRuntimeScopeResult, error) {
	id := request.Config.Runtime.FactoryDirectory
	fake.mu.Lock()
	fake.requests[id] = request.Config
	fake.mu.Unlock()
	fake.entered <- id
	select {
	case <-fake.resume:
	case <-ctx.Done():
		return models.OpenRuntimeScopeResult{}, ctx.Err()
	}
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:test:" + id)
	return models.OpenRuntimeScopeResult{Scope: scope}, err
}

func (fake *acquisitionModels) CloseRuntimeScope(ctx context.Context, request models.CloseRuntimeScopeRequest) (models.CloseRuntimeScopeResult, error) {
	if err := ctx.Err(); err != nil {
		return models.CloseRuntimeScopeResult{}, err
	}
	fake.mu.Lock()
	fake.closed = append(fake.closed, request.Scope)
	fake.mu.Unlock()
	return models.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
}

func TestRuntimeResourceAcquisitionIsolatesOverlappingFactsEffectsAndObservations(t *testing.T) {
	t.Parallel()
	fake := &acquisitionModels{requests: map[string]models.RuntimeScopeConfig{}, entered: make(chan string, 2), resume: make(chan struct{})}
	logger, clock := zap.NewNop(), openingCoordinatorClock{}
	provider := &resourceProvider{}
	mock := &workers.MockWorkersConfig{}
	var mutations, progress [2][]string
	owners := [2]*durableOpeningObservationStub{
		{mutations: &mutations[0], progress: &progress[0]}, {mutations: &mutations[1], progress: &progress[1]},
	}
	open := resourceOpeningWithEffects(owners, logger, clock, provider, mock)
	acquisition := NewRuntimeResourceAcquisition(open, fake, provider)
	var results [2]RuntimeResourceResult
	var cleanups [2]runtimeOpeningCleanup
	var requests [2]RuntimeResourceRequest
	var errors [2]error
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var group sync.WaitGroup
	for i, id := range []string{"first", "second"} {
		requests[i] = resourceRequest(id)
		requests[i].Configured.Workers.MockWorkers = mock
		group.Add(1)
		go func() {
			defer group.Done()
			results[i], errors[i] = acquisition.Acquire(ctx, requests[i], clock, logger, &cleanups[i])
		}()
	}
	for range 2 {
		select {
		case <-fake.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(fake.resume)
	group.Wait()
	cancel()
	for i, id := range []string{"first", "second"} {
		if errors[i] != nil {
			t.Fatal(errors[i])
		}
		assertAcquiredResource(t, id, requests[i], results[i], results[1-i].ModelsScope, owners[i], fake, &cleanups[i])
	}
}

type resourceProvider struct{ providers.Service }

func TestRuntimeResourceAcquisitionRejectsIncompleteResources(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		owner     durableexecution.Service
		zeroScope bool
	}{
		{name: "missing both observations", owner: &resourceOwner{}},
		{name: "missing mutations", owner: &resourceProgressOwner{}},
		{name: "missing progress", owner: &mutationOnlyOpeningOwner{}},
		{name: "zero Models scope", owner: &earlyOpeningDurableExecution{}, zeroScope: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &failedOpeningModelsService{recordingModelsService: &recordingModelsService{}, noScope: true}
			releases := 0
			open := resourceOpeningStub(test.owner, func(ctx context.Context) error {
				if ctx.Err() != nil {
					t.Fatal("release context remained canceled")
				}
				releases++
				return nil
			}, nil, false)
			cleanup := &runtimeOpeningCleanup{}
			ctx, cancel := context.WithCancel(t.Context())
			_, err := NewRuntimeResourceAcquisition(open, fake, nil).Acquire(ctx, resourceRequest(test.name), openingCoordinatorClock{}, zap.NewNop(), cleanup)
			cancel()
			if err == nil {
				t.Fatal("incomplete resource succeeded")
			}
			if (len(fake.openRequests) == 1) != test.zeroScope {
				t.Fatal("Models opened before required observations were validated")
			}
			if err := cleanup.Close(); err != nil {
				t.Fatal(err)
			}
			if releases != 1 || len(fake.closeRequests) != 0 {
				t.Fatal("cleanup did not preserve issued-resource ownership")
			}
		})
	}
}

type resourceOwner struct{ durableexecution.Service }
type resourceProgressOwner struct{ durableexecution.Service }

func (*resourceProgressOwner) PublishWorkerProgress(workers.ProgressFragment) {}

func resourceOpeningWithEffects(owners [2]*durableOpeningObservationStub, logger *zap.Logger, clock factoryruntime.Clock, provider providers.Service, mock *workers.MockWorkersConfig) DurableResourceOpening {
	return func(_ context.Context, id string, definition factorydefinitions.RuntimeSelection, _ factorysessions.PersistencePolicy,
		_, _ string, _ operatorsettings.ResolvedDefaults, root RuntimeRoot, selectedClock factoryruntime.Clock,
		selectedProvider providers.Service, selectedMock *workers.MockWorkersConfig) (DurableExecution, error) {
		if root.FactoryRootDir != definition.Directory || root.RuntimeInstanceID != id+"-runtime" || root.BaseLogger != logger ||
			selectedClock != clock || selectedProvider != provider || selectedMock != mock {
			return DurableExecution{}, fmt.Errorf("selected effects or facts changed for %s", id)
		}
		index := 0
		if id == "second" {
			index = 1
		}
		source := id + "-overlay"
		return DurableExecution{Service: owners[index], OperatorModels: map[string]models.ModelOverlay{"model": {Source: &source}}}, nil
	}
}

func assertAcquiredResource(t *testing.T, id string, request RuntimeResourceRequest, result RuntimeResourceResult, peerScope models.RuntimeScopeRef, owner *durableOpeningObservationStub, fake *acquisitionModels, cleanup *runtimeOpeningCleanup) {
	t.Helper()
	if result.Observations != owner || result.ModelsScope.IsZero() || result.ModelsScope == peerScope {
		t.Fatalf("scope or observation owner crossed: %+v", result)
	}
	request.ModelsRuntime.Workers[0].Args[0] = "changed"
	retained := fake.requests["/"+id]
	if retained.Runtime.Workers[0].Args[0] != id+"-endpoint" || *retained.OperatorModels["model"].Source != id+"-overlay" {
		t.Fatalf("Models facts crossed or retained mutable input: %+v", retained)
	}
	if err := result.Observations.RecordPetriTokenMutations(id, nil); err != nil {
		t.Fatal(err)
	}
	result.Observations.PublishWorkerProgress(workers.ProgressFragment{Payload: id})
	if !reflect.DeepEqual(*owner.mutations, []string{id}) || !reflect.DeepEqual(*owner.progress, []string{id}) {
		t.Fatal("observation routing crossed")
	}
	if err := cleanup.Close(); err != nil {
		t.Fatal(err)
	}
	if fake.closed[len(fake.closed)-1] != result.ModelsScope {
		t.Fatal("cleanup released a peer scope")
	}
}
