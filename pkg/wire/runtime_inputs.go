package wire

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"go.uber.org/zap"
)

func provideWorkersMockWorkersConfigFileSystem(
	edges serviceedges.Edges,
) workers.MockWorkersConfigFileSystem {
	if edges.WorkersMockWorkersConfigFileSystem != nil {
		return edges.WorkersMockWorkersConfigFileSystem
	}
	return platformfilesystem.Local{}
}

// provideRuntimeOpeningRequestFactory is the sole mapping from transport
// selections into the bounded owner requests consumed by Factory Sessions.
func provideRuntimeOpeningRequestFactory() runcli.RuntimeOpeningRequestFactory {
	return func(
		cfg runcli.RunConfig,
		mockWorkers *workers.MockWorkersConfig,
		observer factorysessions.RuntimeHostObserver,
	) factorysessionwire.ApplicationOpeningRequest {
		logDirectory := cfg.RuntimeLogDir
		if strings.TrimSpace(logDirectory) == "" && strings.TrimSpace(cfg.HomeDir) != "" {
			logDirectory = logging.RuntimeLogsRoot(cfg.HomeDir)
		}
		metricsDirectory := cfg.RuntimeMetricsDir
		if strings.TrimSpace(metricsDirectory) == "" && strings.TrimSpace(cfg.HomeDir) != "" {
			metricsDirectory = platformmetrics.RuntimeMetricsRoot(cfg.HomeDir)
		}
		mode := factorydefinitions.RuntimeModeBatch
		if cfg.Continuously {
			mode = factorydefinitions.RuntimeModeService
		}
		request := &factorysessions.RuntimeOpeningRequest{
			FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{
				Directory:        cfg.Dir,
				SourcePath:       cfg.FactoryConfigPath,
				ExecutionBaseDir: cfg.ExecutionBaseDir,
			},
			FactoryRuntime: factoryruntime.RuntimeOpeningRequest{
				Mode:         mode,
				Verbose:      cfg.Verbose,
				LogDirectory: logDirectory,
				LogConfig: factoryruntime.RuntimeLogStorageConfig{
					MaxSize: cfg.RuntimeLogConfig.MaxSize, MaxBackups: cfg.RuntimeLogConfig.MaxBackups,
					MaxAge: cfg.RuntimeLogConfig.MaxAge, Compress: cfg.RuntimeLogConfig.Compress,
				},
				MetricsDirectory: metricsDirectory,
				MetricsConfig: factoryruntime.RuntimeMetricsStorageConfig{
					MaxSize: cfg.RuntimeMetricsConfig.MaxSize, MaxBackups: cfg.RuntimeMetricsConfig.MaxBackups,
					MaxAge: cfg.RuntimeMetricsConfig.MaxAge, Compress: cfg.RuntimeMetricsConfig.Compress,
				},
			},
			FactorySession: factorysessions.SessionRuntimeOpeningRequest{
				SystemConfigHome: cfg.HomeDir,
				WorkFile:         cfg.WorkFile,
				Host: factorysessions.RuntimeHostRequest{
					Directory:   cfg.Dir,
					RuntimeMode: mode,
					WorkFile:    cfg.WorkFile,
					MockWorkers: mockWorkers != nil,
					Host:        cfg.BindHost,
					Port:        cfg.Port,
					AutoPort:    cfg.AutoPort,
				},
			},
			Workers: workers.RuntimeOpeningRequest{
				RunnerID:                          cfg.RunnerID,
				Worktree:                          cfg.Worktree,
				WorkerReasoningEffort:             cfg.WorkerReasoningEffort,
				MockWorkers:                       mockWorkers,
				InvocationSkipPermissionsOverride: cfg.InvocationSkipPermissionsOverride,
			},
			Recordings: recordings.RuntimeOpeningRequest{
				RecordPath: cfg.RecordPath,
				ReplayPath: cfg.ReplayPath,
				WorkflowID: cfg.Workflow,
			},
			ModelCacheDirectory: cfg.ModelCacheDir,
			OperatorDefaults:    cfg.OperatorDefaults,
		}
		return factorysessionwire.ApplicationOpeningRequest{
			Runtime: request,
			Ports: factorysessionwire.ApplicationOpeningPorts{
				InvocationMetricsRecorder: cfg.InvocationMetricsRecorder,
				RuntimeHostObserver:       observer,
			},
		}
	}
}

// provideRuntimeInputResolver copies transport-owned request values into the
// stable application-opening input. External effects are selected by the
// process graph and are not projected from this operation callback.
func provideRuntimeInputResolver() factorysessionwire.ApplicationRuntimeInputResolver {
	return func(
		ctx context.Context,
		request *factorysessions.RuntimeOpeningRequest,
		logger *zap.Logger,
	) (factorysessionwire.ApplicationRuntimeInputs, error) {
		if err := validateRuntimeOpeningInputs(ctx, request, logger); err != nil {
			return factorysessionwire.ApplicationRuntimeInputs{}, err
		}
		configured := *request
		return factorysessionwire.ApplicationRuntimeInputs{
			Request: &configured,
			Logger:  logger,
		}, nil
	}
}

func validateRuntimeOpeningInputs(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	logger *zap.Logger,
) error {
	switch {
	case ctx == nil:
		return errors.New("context is required")
	case ctx.Err() != nil:
		return ctx.Err()
	case request == nil:
		return errors.New("runtime opening request is required")
	case logger == nil:
		return errors.New("runtime logger is required")
	default:
		return nil
	}
}

// The following providers select the process-owned external effects once for
// the long-lived runtime-opening Factory. Operation calls receive only
// invocation data and observation fallbacks; they do not re-read Edges or
// manufacture replacement runners.
func provideFactoryRuntimeClock(edges serviceedges.Edges) factoryruntime.Clock {
	if edges.Clock != nil {
		return edges.Clock
	}
	return platformclock.Real{}
}

func provideFactoryRuntimeProviderOverride(edges serviceedges.Edges) workers.Provider {
	return edges.ProviderOverride
}

func provideFactoryRuntimeSubmissionRecorder(edges serviceedges.Edges) recordings.SubmissionRecorder {
	return edges.SubmissionRecorder
}

func provideFactoryRuntimeDispatchRecorder(edges serviceedges.Edges) recordings.DispatchRecorder {
	return edges.DispatchRecorder
}

func provideFactorySessionInvocationMetricsRecorder(edges serviceedges.Edges) factorysessionwire.InvocationMetricsRecorder {
	return edges.InvocationMetricsRecorder
}

func provideFactorySessionRuntimeHostObserver(edges serviceedges.Edges) factorysessions.RuntimeHostObserver {
	return edges.RuntimeHostObserver
}

func provideFactoryRuntimeProviderCommandRunner(
	edges serviceedges.Edges,
) (factorysessionwire.ProviderCommandRunner, error) {
	if edges.ProviderCommandRunner != nil {
		return workers.AdaptCommandRunner(edges.ProviderCommandRunner), nil
	}
	runner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	return workers.AdaptCommandRunner(runner), nil
}

func provideFactoryRuntimeScriptCommandRunner(
	edges serviceedges.Edges,
) (factorysessionwire.ScriptCommandRunner, error) {
	if edges.ScriptCommandRunner != nil {
		return workers.AdaptCommandRunner(edges.ScriptCommandRunner), nil
	}
	runner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	return workers.AdaptCommandRunner(runner), nil
}

func provideSessionExecutionOpeningFactory(
	runtimes factorysessionwire.ExecutionRuntimeOpening,
	providerCommandRunner factorysessionwire.ProviderCommandRunner,
	build factorysessionwire.StandaloneSessionExecutionFactory,
	invocation factorysessionwire.WorkerInvocationFactory,
	resolveClock factoryruntime.ClockResolver,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	paths factorysessionwire.ExecutionOpeningFileSystem,
	allocator workers.PTYAllocator,
	logger *zap.Logger,
) (*factorysessionwire.ExecutionOpeningFactory, error) {
	return factorysessionwire.NewExecutionOpeningFactory(
		runtimes, providerCommandRunner, allocator,
		build, invocation, resolveClock, artifactRoots, paths, logger,
	)
}

func provideInvocationOperation(
	openRuntime factorysessionwire.InvocationRuntimeOpening,
	modelsRoot models.Service,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter modelswire.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
) (factorysessionwire.InvocationOperation, error) {
	return factorysessionwire.NewInvocationOperation(
		openRuntime,
		modelsRoot,
		workingDirectory,
		resolveCurrentDir,
		artifactExporter,
		modelTimeout,
		artifactRoots,
		generateSessionID,
		logger,
	)
}

func provideAgyPTYAllocator(edges serviceedges.Edges) (workers.PTYAllocator, error) {
	clock := edges.AgyPTYClock
	if clock == nil {
		clock = platformclock.Real{}
	}
	host := edges.AgyPTYHost
	if host == nil {
		host = platformpty.NewHost()
	}
	return providerswire.NewAgyPTYAllocator(host, clock)
}

func provideWorkerCommandRunnerAdapter() factorysessionwire.WorkerCommandRunnerAdapter {
	return workers.AdaptCommandRunner
}

func provideWorkRequestIDGenerator(edges serviceedges.Edges) work.RequestIDGenerator {
	if edges.WorkRequestIDGenerator != nil {
		return edges.WorkRequestIDGenerator
	}
	return uuid.NewString
}

func provideFactorySessionRuntimeInstanceIDGenerator(edges serviceedges.Edges) factorysessions.RuntimeInstanceIDGenerator {
	if edges.FactorySessionRuntimeInstanceIDGenerator != nil {
		return edges.FactorySessionRuntimeInstanceIDGenerator
	}
	return uuid.NewString
}

func provideFactorySessionIDGenerator(edges serviceedges.Edges) factorysessions.SessionIDGenerator {
	if edges.FactorySessionIDGenerator != nil {
		return edges.FactorySessionIDGenerator
	}
	return uuid.NewString
}

func provideFactorySessionResponseEventIDGenerator(edges serviceedges.Edges) factorysessions.ResponseEventIDGenerator {
	if edges.FactorySessionResponseEventIDGenerator != nil {
		return edges.FactorySessionResponseEventIDGenerator
	}
	return uuid.NewString
}

func provideWorkSubmittedFileReader(edges serviceedges.Edges) work.SubmittedFileReader {
	if edges.WorkSubmittedFileReader != nil {
		return edges.WorkSubmittedFileReader
	}
	return os.ReadFile
}

func provideWorkSubmittedFilePathInspector(edges serviceedges.Edges) work.SubmittedFilePathInspector {
	if edges.WorkSubmittedFilePathInspector != nil {
		return edges.WorkSubmittedFilePathInspector
	}
	return os.Stat
}

func provideWorkFactory(
	readFile work.SubmittedFileReader,
	inspectPath work.SubmittedFilePathInspector,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
) factorysessionwire.WorkFactory {
	return func(runtimes work.RuntimeResolver) work.Service {
		return workwire.NewRuntimeService(runtimes, readFile, inspectPath, contentStaging, contentMaterializer)
	}
}

// provideWorkMaterializationService supplies Runtime's canonical Worker-output
// materialization seam without introducing a Runtime→Work implementation
// import. Session-scoped admission/state access receives its own resolver-bound
// Work root; this root is intentionally policy-only.
func provideWorkMaterializationService(
	contentMaterializer work.ContentMaterializer,
) work.Service {
	return workwire.NewRuntimeService(nil, nil, nil, nil, contentMaterializer)
}
