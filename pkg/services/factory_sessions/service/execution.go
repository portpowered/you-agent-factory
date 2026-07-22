package service

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewDurableExecution constructs the durable Factory Session executor while
// keeping persistence selection inside the owning service.
func NewDurableExecution(
	projectRoot string,
	persistencePolicy factorysessions.PersistencePolicy,
	stores factorysessions.RuntimePersistenceStoreFactory,
	executor workers.InvocationExecutor,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	workerPresetIDs map[string]struct{},
	workerSettings factoryruntime.JavaScriptWorkerSettings,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorysessions.ExecutionService, error) {
	persistence, err := factorysessionexecution.PersistenceChoiceForPolicy(
		persistencePolicy,
		projectRoot,
		adaptRuntimePersistenceStoreFactory(stores),
	)
	if err != nil {
		return nil, err
	}
	childExecutorMode := factorysessions.ChildExecutorModeFake
	if executor != nil {
		childExecutorMode = factorysessions.ChildExecutorModeLive
	}
	return factorysessionexecution.NewJavaScriptExecutionService(
		projectRoot,
		childExecutorMode,
		executor,
		persistence,
		clock,
		syncWaits,
		checkpointSummaries,
		workflows,
		workflows,
		workflows,
		workerPresetIDs,
		workerSettings,
		recordingWriter,
		generateSessionID,
	)
}

// NewStandaloneExecution constructs an explicitly selected durable execution
// backend for CLI entrypoints.
func NewStandaloneExecution(
	provider factorysessions.ExecutionProvider,
	projectRoot string,
	stores factorysessions.RuntimePersistenceStoreFactory,
	fixtureCatalogPath string,
	childExecutorMode string,
	executor workers.InvocationExecutor,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
	fixtureFiles fileeffects.ContractFixtureReader,
) (factorysessions.ExecutionService, error) {
	switch provider {
	case factorysessions.ExecutionProviderFake:
		return factorysessionexecution.NewFakeServiceFromContractFixtures(fixtureCatalogPath, clock, fixtureFiles)
	case factorysessions.ExecutionProviderJavaScriptRuntime:
		persistence, err := factorysessionexecution.ProjectPersistence(
			projectRoot,
			adaptRuntimePersistenceStoreFactory(stores),
		)
		if err != nil {
			return nil, err
		}
		return factorysessionexecution.NewJavaScriptExecutionService(
			projectRoot,
			childExecutorMode,
			executor,
			persistence,
			clock,
			syncWaits,
			checkpointSummaries,
			workflows,
			workflows,
			workflows,
			nil,
			factoryruntime.JavaScriptWorkerSettings{},
			recordingWriter,
			generateSessionID,
		)
	default:
		return nil, factorysessions.NewExecutionValidationError(
			"provider",
			"unsupported execution provider",
		)
	}
}

func adaptRuntimePersistenceStoreFactory(
	stores factorysessions.RuntimePersistenceStoreFactory,
) func(string) (runtimepersist.Store, error) {
	if stores == nil {
		return nil
	}
	return func(projectRoot string) (runtimepersist.Store, error) {
		return stores(projectRoot)
	}
}
