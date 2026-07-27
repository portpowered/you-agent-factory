// Package wire constructs the owner-private durable execution capability.
package wire

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	durableexecutionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewService binds an existing durable implementation behind the private
// capability contract without starting execution or performing IO.
func NewService(execution factorysessions.ExecutionService) (durableexecution.Service, error) {
	service, err := durableexecutionservice.New(execution)
	if err != nil {
		return nil, err
	}
	return service, nil
}

// NewDurable constructs the configured runtime-backed capability.
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
) (durableexecution.Service, error) {
	return durableexecutionservice.NewDurable(
		projectRoot, persistencePolicy, stores, executor, clock, syncWaits,
		checkpointSummaries, workflows, orchestration, childValues,
		workerPresetIDs, workerSettings,
		recordingWriter, generateSessionID, liveChildInvocation, generateResponseEventID, responseStreams,
	)
}

// NewStandalone constructs an explicitly selected standalone capability.
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
) (durableexecution.Service, error) {
	return durableexecutionservice.NewStandalone(
		provider, projectRoot, stores, fixtureCatalogPath, childExecutorMode,
		executor, clock, syncWaits, checkpointSummaries, workflows, orchestration, childValues,
		recordingWriter, generateSessionID, fixtureFiles,
	)
}
