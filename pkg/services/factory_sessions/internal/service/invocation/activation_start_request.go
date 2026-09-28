package invocation

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// cloneBool returns a detached copy of a bool pointer.
func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// ActivationOnlyStartRequest maps an invocation target plus resolved artifact
// roots to the canonical live ActivationOnly one-shot SessionStartRequest.
// It mirrors operation.runtimeConfig selections without opening any runtime.
// The input target arguments are never mutated; they are deep-copied.
func ActivationOnlyStartRequest(target roles.InvocationTarget, roots factoryruntime.RuntimeArtifactRoots) factorysessions.SessionStartRequest {
	logDir := target.RuntimeLogDir
	if logDir == "" {
		logDir = roots.Logs
	}
	metricsDir := target.RuntimeMetricsDir
	if metricsDir == "" {
		metricsDir = roots.Metrics
	}
	sessionID := strings.TrimSpace(target.FactorySessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	return factorysessions.SessionStartRequest{
		SessionID:      sessionID,
		Mode:           factorysessions.SessionOperationModeLive,
		FolderPath:     target.FactoryDir,
		ActivationOnly: true,
		Definition: factorysessions.SessionDefinitionSelection{
			SourceRef: target.FactorySourcePath,
		},
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{
			SystemConfigHome:              target.HomeDir,
			LogDirectory:                  logDir,
			MetricsDirectory:              metricsDir,
			OperatorDefaults:              target.OperatorDefaults,
			DefinitionSourcePath:          target.FactorySourcePath,
			DefinitionInvocationArguments: work.CloneInvocationArguments(target.InvocationArguments),
			ExecutionBaseDir:              target.ExecutionBaseDir,
			CanonicalSessionID:            target.CanonicalSessionID,
			WorkFile:                      "",
			ModelCacheDirectory:           target.ModelCacheDir,
			Mode:                          factorysessions.SessionRuntimeModeService,
			Verbose:                       target.Verbose,
			LogPolicy:                     factorysessions.SessionArtifactPolicyEnabled,
			LogConfig: factorysessions.SessionArtifactStorageConfig{
				MaxSize:    target.RuntimeLogConfig.MaxSize,
				MaxBackups: target.RuntimeLogConfig.MaxBackups,
				MaxAge:     target.RuntimeLogConfig.MaxAge,
				Compress:   target.RuntimeLogConfig.Compress,
			},
			MetricsPolicy: factorysessions.SessionArtifactPolicyEnabled,
			MetricsConfig: factorysessions.SessionArtifactStorageConfig{
				MaxSize:    target.RuntimeMetricsConfig.MaxSize,
				MaxBackups: target.RuntimeMetricsConfig.MaxBackups,
				MaxAge:     target.RuntimeMetricsConfig.MaxAge,
				Compress:   target.RuntimeMetricsConfig.Compress,
			},
			Host: factorysessions.RuntimeHostRequest{
				Port:        0,
				RuntimeMode: factorydefinitions.RuntimeModeService,
			},
			Workers: factorysessions.SessionWorkerSelection{
				RunnerID:                          target.RunnerID,
				Worktree:                          target.Worktree,
				WorkerReasoningEffort:             target.WorkerReasoningEffort,
				MockWorkers:                       target.MockWorkersConfig.Clone(),
				InvocationSkipPermissionsOverride: cloneBool(target.SkipPermissionsOverride),
				SkipBuiltInPrerequisiteValidation: target.SkipRunnerPrerequisiteValidation,
			},
			Recording: factorysessions.SessionRecordingSelection{
				RecordPath: target.RecordPath,
				ReplayPath: target.ReplayPath,
				ResumePath: target.ResumePath,
				WorkflowID: target.WorkflowID,
			},
		},
	}
}
