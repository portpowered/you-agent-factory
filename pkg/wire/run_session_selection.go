package wire

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

// runSessionStartRequest maps CLI selections to the canonical process-owned
// Factory Session request. It performs no runtime construction or activation.
func runSessionStartRequest(cfg runcli.RunConfig, mocks *workers.MockWorkersConfig) factorysessions.SessionStartRequest {
	logDirectory := cfg.RuntimeLogDir
	if strings.TrimSpace(logDirectory) == "" && strings.TrimSpace(cfg.HomeDir) != "" {
		logDirectory = logging.RuntimeLogsRoot(cfg.HomeDir)
	}
	metricsDirectory := cfg.RuntimeMetricsDir
	if strings.TrimSpace(metricsDirectory) == "" && strings.TrimSpace(cfg.HomeDir) != "" {
		metricsDirectory = platformmetrics.RuntimeMetricsRoot(cfg.HomeDir)
	}
	mode := factorysessions.SessionRuntimeModeBatch
	if cfg.Continuously {
		mode = factorysessions.SessionRuntimeModeService
	}
	return factorysessions.SessionStartRequest{
		SessionID:   cfg.FactorySessionID,
		Mode:        factorysessions.SessionOperationModeLive,
		FolderPath:  cfg.Dir,
		Persistence: factorysessions.PersistencePolicyEnabled,
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{
			SystemConfigHome:              cfg.HomeDir,
			DefinitionSourcePath:          cfg.FactoryConfigPath,
			DefinitionInvocationArguments: work.CloneInvocationArguments(cfg.InvocationArguments),
			ExecutionBaseDir:              cfg.ExecutionBaseDir,
			CanonicalSessionID:            cfg.CanonicalSessionID,
			WorkFile:                      cfg.WorkFile,
			ModelCacheDirectory:           cfg.ModelCacheDir,
			Mode:                          mode,
			Verbose:                       cfg.Verbose,
			LogDirectory:                  logDirectory,
			LogPolicy:                     factorysessions.SessionArtifactPolicyEnabled,
			LogConfig: factorysessions.SessionArtifactStorageConfig{
				MaxSize: cfg.RuntimeLogConfig.MaxSize, MaxBackups: cfg.RuntimeLogConfig.MaxBackups,
				MaxAge: cfg.RuntimeLogConfig.MaxAge, Compress: cfg.RuntimeLogConfig.Compress,
			},
			MetricsDirectory: metricsDirectory,
			MetricsPolicy:    factorysessions.SessionArtifactPolicyEnabled,
			MetricsConfig: factorysessions.SessionArtifactStorageConfig{
				MaxSize: cfg.RuntimeMetricsConfig.MaxSize, MaxBackups: cfg.RuntimeMetricsConfig.MaxBackups,
				MaxAge: cfg.RuntimeMetricsConfig.MaxAge, Compress: cfg.RuntimeMetricsConfig.Compress,
			},
			Host: factorysessions.RuntimeHostRequest{
				Directory: cfg.Dir, RuntimeMode: factorydefinitions.RuntimeMode(mode),
				WorkFile: cfg.WorkFile, MockWorkers: mocks != nil,
				Host: cfg.BindHost, Port: cfg.Port, AutoPort: cfg.AutoPort, Pprof: cfg.Pprof,
			},
			Workers: factorysessions.SessionWorkerSelection{
				RunnerID: cfg.RunnerID, Worktree: cfg.Worktree,
				WorkerReasoningEffort:             cfg.WorkerReasoningEffort,
				MockWorkers:                       mocks,
				InvocationSkipPermissionsOverride: cfg.InvocationSkipPermissionsOverride,
			},
			Recording: factorysessions.SessionRecordingSelection{
				RecordPath: cfg.RecordPath, ReplayPath: cfg.ReplayPath,
				ResumePath: cfg.ResumePath, WorkflowID: cfg.Workflow,
			},
			OperatorDefaults: cfg.OperatorDefaults,
		},
	}
}
