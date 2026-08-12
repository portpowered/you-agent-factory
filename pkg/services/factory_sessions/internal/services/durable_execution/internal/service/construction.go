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
	childExecutorMode string,
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
	generateResponseEventID factorysessions.ResponseEventIDGenerator,
	responseStreams responsestreamservice.Service,
	liveChangeCoordinator factorysessions.LiveChangeCoordinator,
) (*Service, error) {
	persistence, err := factorysessionexecution.PersistenceChoiceForPolicy(
		persistencePolicy,
		projectRoot,
		adaptRuntimePersistenceStoreFactory(stores),
	)
	if err != nil {
		return nil, err
	}
	// A runtime-backed live session invokes its children as Workers through its
	// own Factory Runtime, so it takes no direct provider edge of its own. The
	// mode still arrives from composition: a session with no provider behind it
	// runs fake children, exactly as before.
	execution, err := factorysessionexecution.NewJavaScriptExecutionService(
		projectRoot,
		childExecutorMode,
		nil,
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
		generateResponseEventID,
		responseStreams,
		liveChangeCoordinator,
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
	liveChangeCoordinator factorysessions.LiveChangeCoordinator,
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
		// Standalone CLI/MCP opening follows the interim application policy:
		// in-memory only. Callers that need restart/resume snapshots compose
		// through NewDurable with PersistencePolicyEnabled instead.
		persistence, err := factorysessionexecution.PersistenceChoiceForPolicy(
			factorysessions.PersistencePolicyDisabled,
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
			liveChangeCoordinator,
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
