package service

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// runtimeOwnerFixture keeps the owner-specific values used by the runtime
// construction tests. Production admission uses SessionStartRequest directly.
type runtimeOwnerFixture struct {
	FactoryDefinition   factorydefinitions.RuntimeOpeningRequest
	FactoryRuntime      factoryruntime.RuntimeOpeningRequest
	FactorySession      factorysessions.SessionRuntimeOpeningRequest
	Workers             workers.RuntimeOpeningRequest
	Recordings          recordings.RuntimeOpeningRequest
	ModelCacheDirectory string
	OperatorDefaults    operatorsettings.ResolvedDefaults
}

func (fixture runtimeOwnerFixture) startRequest() factorysessions.SessionStartRequest {
	return factorysessions.SessionStartRequest{
		SessionID:   fixture.FactorySession.FactorySessionID,
		Mode:        factorysessions.SessionOperationModeLive,
		FolderPath:  fixture.FactoryDefinition.Directory,
		Persistence: fixture.FactorySession.PersistencePolicy,
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{
			DefinitionSourcePath:          fixture.FactoryDefinition.SourcePath,
			DefinitionInvocationArguments: work.CloneInvocationArguments(fixture.FactoryDefinition.InvocationArguments),
			ExecutionBaseDir:              fixture.FactoryDefinition.ExecutionBaseDir,
			CanonicalSessionID:            fixture.FactorySession.CanonicalSessionID,
			BackendScopeID:                fixture.FactorySession.BackendScopeID,
			SystemConfigHome:              fixture.FactorySession.SystemConfigHome,
			SystemConfigPath:              fixture.FactorySession.SystemConfigPath,
			WorkFile:                      fixture.FactorySession.WorkFile,
			Host:                          fixture.FactorySession.Host,
			Mode:                          factorysessions.SessionRuntimeMode(fixture.FactoryRuntime.Mode),
			Verbose:                       fixture.FactoryRuntime.Verbose,
			RuntimeInstanceID:             fixture.FactoryRuntime.RuntimeInstanceID,
			LogDirectory:                  fixture.FactoryRuntime.LogDirectory,
			LogPolicy:                     factorysessions.SessionArtifactPolicy(fixture.FactoryRuntime.FileLoggingPolicy),
			LogConfig:                     factorysessions.SessionArtifactStorageConfig(fixture.FactoryRuntime.LogConfig),
			MetricsDirectory:              fixture.FactoryRuntime.MetricsDirectory,
			MetricsPolicy:                 factorysessions.SessionArtifactPolicy(fixture.FactoryRuntime.MetricsPolicy),
			MetricsConfig:                 factorysessions.SessionArtifactStorageConfig(fixture.FactoryRuntime.MetricsConfig),
			ModelCacheDirectory:           fixture.ModelCacheDirectory,
			OperatorDefaults:              fixture.OperatorDefaults,
			Workers: factorysessions.SessionWorkerSelection{
				RunnerID: fixture.Workers.RunnerID, Worktree: fixture.Workers.Worktree,
				WorkerReasoningEffort:             fixture.Workers.WorkerReasoningEffort,
				MockWorkers:                       fixture.Workers.MockWorkers,
				InvocationSkipPermissionsOverride: fixture.Workers.InvocationSkipPermissionsOverride,
				SkipBuiltInPrerequisiteValidation: fixture.Workers.SkipBuiltInPrerequisiteValidation,
			},
			Recording: factorysessions.SessionRecordingSelection{
				RecordPath: fixture.Recordings.RecordPath, ReplayPath: fixture.Recordings.ReplayPath,
				ResumePath: fixture.Recordings.ResumePath, WorkflowID: fixture.Recordings.WorkflowID,
				FlushInterval: fixture.Recordings.FlushInterval,
			},
		},
	}
}
