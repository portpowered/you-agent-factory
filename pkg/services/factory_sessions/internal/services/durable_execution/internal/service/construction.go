package service

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewDurable constructs the runtime-backed durable execution capability while
// keeping persistence selection and concrete implementation choice private.
func NewDurable(
	projectRoot string,
	persistencePolicy factorysessions.PersistencePolicy,
	stores roles.RuntimePersistenceStoreFactory,
	executor workers.InvocationExecutor,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	childValues factoryruntime.JavaScriptChildValues,
	workerPresetIDs map[string]struct{},
	workerSettings factoryruntime.JavaScriptWorkerSettings,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
	liveChildInvocation factorysessionexecution.LiveChildInvocationFactory,
	generateResponseEventID factorysessions.ResponseEventIDGenerator,
	responseStreams responsestreamservice.Service,
) (*Service, error) {
	persistence, err := factorysessionexecution.PersistenceChoiceForPolicy(
		persistencePolicy,
		projectRoot,
		adaptRuntimePersistenceStoreFactory(stores),
	)
	if err != nil {
		return nil, err
	}
	childExecutorMode := factorysessions.ChildExecutorModeFake
	if executor != nil || liveChildInvocation != nil {
		childExecutorMode = factorysessions.ChildExecutorModeLive
	}
	execution, err := factorysessionexecution.NewJavaScriptExecutionService(
		projectRoot,
		childExecutorMode,
		executor,
		persistence,
		clock,
		syncWaits,
		checkpointSummaries,
		workflows,
		orchestration,
		childValues,
		workerPresetIDs,
		workerSettings,
		recordingWriter,
		generateSessionID,
		liveChildInvocation,
		generateResponseEventID,
		responseStreams,
	)
	if err != nil {
		return nil, err
	}
	return New(execution)
}

// NewStandalone constructs the explicitly selected durable backend used by
// standalone CLI and MCP execution opening.
func NewStandalone(
	provider factorysessions.ExecutionProvider,
	projectRoot string,
	stores roles.RuntimePersistenceStoreFactory,
	fixtureCatalogPath string,
	childExecutorMode string,
	executor workers.InvocationExecutor,
	clock factoryruntime.Clock,
	syncWaits factorysessionexecution.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	childValues factoryruntime.JavaScriptChildValues,
	recordingWriter recordings.PortableRecordingWriter,
	generateSessionID factorysessions.SessionIDGenerator,
	fixtureFiles fileeffects.ContractFixtureReader,
) (*Service, error) {
	switch provider {
	case factorysessions.ExecutionProviderFake:
		execution, err := factorysessionexecution.NewFakeServiceFromContractFixtures(
			fixtureCatalogPath,
			clock,
			fixtureFiles,
		)
		if err != nil {
			return nil, err
		}
		return New(execution)
	case factorysessions.ExecutionProviderJavaScriptRuntime:
		persistence, err := factorysessionexecution.ProjectPersistence(
			projectRoot,
			adaptRuntimePersistenceStoreFactory(stores),
		)
		if err != nil {
			return nil, err
		}
		execution, err := factorysessionexecution.NewJavaScriptExecutionService(
			projectRoot,
			childExecutorMode,
			executor,
			persistence,
			clock,
			syncWaits,
			checkpointSummaries,
			workflows,
			orchestration,
			childValues,
			nil,
			factoryruntime.JavaScriptWorkerSettings{},
			recordingWriter,
			generateSessionID,
			nil,
			nil,
			nil,
		)
		if err != nil {
			return nil, err
		}
		return New(execution)
	default:
		return nil, factorysessionexecution.NewValidationError(
			"provider",
			"unsupported execution provider",
		)
	}
}

func adaptRuntimePersistenceStoreFactory(
	stores roles.RuntimePersistenceStoreFactory,
) func(string) (runtimepersist.Store, error) {
	if stores == nil {
		return nil
	}
	return func(projectRoot string) (runtimepersist.Store, error) {
		return stores(projectRoot)
	}
}
