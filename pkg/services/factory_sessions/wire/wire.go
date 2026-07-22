// Package wire is the Factory Sessions service composition boundary.
// Application Wire uses these providers without importing the service's
// concrete implementation package.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewRuntimeAssembly constructs the inert Factory Sessions runtime assembly
// selected by canonical application Wire. Runtime values are supplied later
// through the assembly's typed Complete operation.
func NewRuntimeAssembly(
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	interpolation factorydefinitions.InvocationInterpolationService,
	invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
	ttsObservability factorydefinitions.TTSObservabilityService,
	clock factoryruntime.Clock,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection factorysessions.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
	initialWorkFiles fileeffects.InitialWorkReader,
) (factorysessions.RuntimeAssembly, error) {
	if sessionResultProjection == nil {
		return nil, fmt.Errorf("construct Factory Sessions: session result projection is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("construct Factory Sessions: clock is required")
	}
	if eventIDs == nil {
		return nil, fmt.Errorf("construct Factory Sessions: response event ID generator is required")
	}
	if sessionIDs == nil {
		return nil, fmt.Errorf("construct Factory Sessions: session ID generator is required")
	}
	if resolveHome == nil {
		return nil, fmt.Errorf("construct Factory Sessions: home directory resolver is required")
	}
	if directoryInspection == nil {
		return nil, fmt.Errorf("construct Factory Sessions: directory inspection is required")
	}
	if namedPaths == nil {
		return nil, fmt.Errorf("construct Factory Sessions: named path resolver is required")
	}
	if invocationInputFiles == nil {
		return nil, fmt.Errorf("construct Factory Sessions: invocation input reader is required")
	}
	if initialWorkFiles == nil {
		return nil, fmt.Errorf("construct Factory Sessions: initial Work reader is required")
	}

	assembly := factorysessionservice.NewAssembly(
		newJavaScriptCheckpointStore,
		sessionResultProjection,
		interpolation,
		invocationWorkTypes,
		ttsObservability,
		clock,
		eventIDs,
		sessionIDs,
		resolveHome,
		directoryInspection,
		namedPaths,
		invocationInputFiles,
		initialWorkFiles,
	)
	if assembly == nil {
		return nil, fmt.Errorf("construct Factory Sessions: implementation rejected its dependencies")
	}
	return assembly, nil
}

// NewDurableExecution constructs the configured durable execution capability
// without exposing its implementation package to application Wire.
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
	return factorysessionservice.NewDurableExecution(
		projectRoot, persistencePolicy, stores, executor, clock, syncWaits,
		checkpointSummaries, workflows, workerPresetIDs, workerSettings,
		recordingWriter, generateSessionID,
	)
}

// NewStandaloneExecution constructs the configured standalone execution
// capability without exposing its implementation package to application Wire.
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
	return factorysessionservice.NewStandaloneExecution(
		provider, projectRoot, stores, fixtureCatalogPath, childExecutorMode,
		executor, clock, syncWaits, checkpointSummaries, workflows,
		recordingWriter, generateSessionID, fixtureFiles,
	)
}
