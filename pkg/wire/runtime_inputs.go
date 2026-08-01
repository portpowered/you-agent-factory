package wire

import (
	"context"
	"errors"
	"os"
	"reflect"
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
				MockWorkers:                       mockWorkers,
				InvocationSkipPermissionsOverride: cfg.InvocationSkipPermissionsOverride,
			},
			Recordings: recordings.RuntimeOpeningRequest{
				RecordPath: cfg.RecordPath,
				ReplayPath: cfg.ReplayPath,
				WorkflowID: cfg.Workflow,
			},
			Models: models.RuntimeOpeningRequest{
				CacheDirectory: cfg.ModelCacheDir,
			},
			OperatorDefaults: cfg.OperatorDefaults,
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

// provideRuntimeInputResolver merges process edges into the exact opening
// effect ports. Per-operation selections are already owner-bounded by the
// canonical injector mapper.
func provideRuntimeInputResolver(
	defaultEdges serviceedges.Edges,
	resolveClock factoryruntime.ClockResolver,
) factorysessionwire.ApplicationRuntimeInputResolver {
	return func(
		ctx context.Context,
		request *factorysessions.RuntimeOpeningRequest,
		ports factorysessionwire.ApplicationOpeningPorts,
		logger *zap.Logger,
	) (factorysessionwire.ApplicationRuntimeInputs, error) {
		edges := defaultEdges
		if ports.InvocationMetricsRecorder != nil {
			edges.InvocationMetricsRecorder = ports.InvocationMetricsRecorder
		}
		if ports.RuntimeHostObserver != nil {
			edges.RuntimeHostObserver = ports.RuntimeHostObserver
		}
		if resolveClock != nil {
			edges.Clock = resolveClock(edges.Clock)
		}
		effects := projectRuntimeOpeningExternalEffects(edges)
		if err := validateResolvedRuntimeInputs(ctx, request, effects, logger); err != nil {
			return factorysessionwire.ApplicationRuntimeInputs{}, err
		}
		configured := *request
		return factorysessionwire.ApplicationRuntimeInputs{
			Request: &configured,
			Effects: effects,
			Logger:  logger,
		}, nil
	}
}

func validateResolvedRuntimeInputs(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	effects factorysessionwire.RuntimeOpeningExternalEffects,
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
	case isNilRuntimeInput(effects.Clock):
		return errors.New("runtime clock edge is required")
	default:
		return nil
	}
}

func isNilRuntimeInput(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func provideSessionExecutionOpeningFactory(
	runtimes *factorysessionwire.RuntimeOpeningFactory,
	edges serviceedges.Edges,
	build factorysessionwire.StandaloneSessionExecutionFactory,
	invocation factorysessionwire.WorkerInvocationFactory,
	resolveClock factoryruntime.ClockResolver,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	adaptRunner factorysessionwire.WorkerCommandRunnerAdapter,
	paths factorysessionwire.ExecutionOpeningFileSystem,
	allocator workers.PTYAllocator,
	logger *zap.Logger,
) (*factorysessionwire.ExecutionOpeningFactory, error) {
	workerEdges, err := withStandaloneWorkerProductionEdges(edges)
	if err != nil {
		return nil, err
	}
	return factorysessionwire.NewExecutionOpeningFactory(
		runtimes, projectRuntimeOpeningExternalEffects(workerEdges), adaptRunner(workerEdges.ProviderCommandRunner), allocator,
		build, invocation, resolveClock, artifactRoots, adaptRunner, paths, logger,
	)
}

func provideInvocationOperation(
	openRuntime *factorysessionwire.RuntimeOpeningFactory,
	edges serviceedges.Edges,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter modelswire.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorysessionwire.InvocationOperation, error) {
	return factorysessionwire.NewInvocationOperation(
		openRuntime,
		projectRuntimeOpeningExternalEffects(edges),
		workingDirectory,
		resolveCurrentDir,
		artifactExporter,
		modelTimeout,
		artifactRoots,
		generateSessionID,
	)
}

// projectRuntimeOpeningExternalEffects is the sole selection from the process
// edge aggregate into the effects consumed by Factory Session runtime opening.
func projectRuntimeOpeningExternalEffects(edges serviceedges.Edges) factorysessionwire.RuntimeOpeningExternalEffects {
	providerRunner := edges.ProviderCommandRunner
	scriptRunner := edges.ScriptCommandRunner
	if providerRunner == nil || scriptRunner == nil {
		if defaultRunner, err := providePlatformProcessCommandRunner(edges); err == nil && defaultRunner != nil {
			if providerRunner == nil {
				providerRunner = defaultRunner
			}
			if scriptRunner == nil {
				scriptRunner = defaultRunner
			}
		}
	}
	return factorysessionwire.RuntimeOpeningExternalEffects{
		Clock:                            edges.Clock,
		ProviderOverride:                 edges.ProviderOverride,
		ModelPullMetricsRecorder:         adaptModelPullMetricsRecorder(edges.ModelPullMetricsRecorder),
		InvocationMetricsRecorder:        edges.InvocationMetricsRecorder,
		ProviderCommandRunner:            providerRunner,
		ScriptCommandRunner:              scriptRunner,
		SubmissionRecorder:               edges.SubmissionRecorder,
		DispatchRecorder:                 edges.DispatchRecorder,
		RuntimeHostObserver:              edges.RuntimeHostObserver,
		FactoryVisualizationSink:         edges.FactoryVisualizationSink,
		FactoryVisualizationRootObserver: edges.FactoryVisualizationRootObserver,
		HostedClock:                      edges.HostedClock,
		HostedHTTPClient:                 edges.HostedHTTPClient,
		HostedSecretResolver:             edges.HostedSecretResolver,
		HostedLinearEndpoint:             edges.HostedLinearEndpoint,
	}
}

type factorySessionModelPullMetricsAdapter struct {
	next serviceedges.PullMetricsRecorder
}

func (adapter factorySessionModelPullMetricsAdapter) RecordModelPullMetric(
	metric factorysessions.InvocationMetric,
) {
	if adapter.next == nil {
		return
	}
	labels := make(map[string]string, len(metric.Labels))
	for key, value := range metric.Labels {
		labels[key] = value
	}
	adapter.next.RecordModelPullMetric(serviceedges.PullMetric{
		Name:   metric.Name,
		Labels: labels,
	})
}

func adaptModelPullMetricsRecorder(
	recorder serviceedges.PullMetricsRecorder,
) factorysessionwire.ModelPullMetricsRecorder {
	if recorder == nil {
		return nil
	}
	return factorySessionModelPullMetricsAdapter{next: recorder}
}

func withStandaloneWorkerProductionEdges(overrides serviceedges.Edges) (serviceedges.Edges, error) {
	commandRunner, err := providePlatformProcessCommandRunner(overrides)
	if err != nil {
		return serviceedges.Edges{}, err
	}
	return serviceedges.Merge(serviceedges.Edges{
		ProviderCommandRunner: commandRunner,
		ScriptCommandRunner:   commandRunner,
	}, overrides), nil
}

func provideAgyPTYAllocator(edges serviceedges.Edges) (workers.PTYAllocator, error) {
	allocator, err := provideProvidersAgyPTYAllocator(edges)
	if err != nil {
		return nil, err
	}
	return providerPTYAllocatorAdapter{allocator: allocator}, nil
}

func provideProvidersAgyPTYAllocator(edges serviceedges.Edges) (providerswire.PTYAllocator, error) {
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

// providerPTYAllocatorAdapter keeps the Providers-owned PTY contract behind
// the Workers root PTY contract at the composition boundary.
type providerPTYAllocatorAdapter struct {
	allocator providerswire.PTYAllocator
}

func (adapter providerPTYAllocatorAdapter) Allocate(
	ctx context.Context,
	launch workers.PTYProcessLaunch,
	config workers.PTYSessionConfig,
) (workers.PTYSession, error) {
	if adapter.allocator == nil {
		return nil, workers.ErrPTYHostRequired
	}
	session, err := adapter.allocator.Allocate(ctx, providerswire.PTYProcessLaunch{
		Executable: launch.Executable,
		Argv:       launch.Argv,
		WorkDir:    launch.WorkDir,
		Env:        launch.Env,
	}, providerswire.PTYSessionConfig{
		MaxCaptureBytes: config.MaxCaptureBytes,
		IdleTimeout:     config.IdleTimeout,
		HardTimeout:     config.HardTimeout,
	})
	if err != nil {
		return nil, mapProviderPTYError(err)
	}
	return providerPTYSessionAdapter{session: session}, nil
}

type providerPTYSessionAdapter struct {
	session providerswire.PTYSession
}

func (adapter providerPTYSessionAdapter) Run(ctx context.Context) (workers.PTYSessionResult, error) {
	if adapter.session == nil {
		return workers.PTYSessionResult{}, workers.ErrPTYHostRequired
	}
	result, err := adapter.session.Run(ctx)
	return workers.PTYSessionResult{
		ExitCode:    result.ExitCode,
		RawBytes:    result.RawBytes,
		CleanedText: result.CleanedText,
		TimedOut:    result.TimedOut,
		CapacityHit: result.CapacityHit,
	}, mapProviderPTYError(err)
}

// mapProviderPTYError preserves the Workers-root error contract at the
// composition boundary while Providers keeps its own implementation sentinels
// private behind its wire package.
func mapProviderPTYError(err error) error {
	switch {
	case errors.Is(err, providerswire.ErrPTYUnsupportedPlatform):
		return mappedProviderPTYError{workerError: workers.ErrPTYUnsupportedPlatform, providerError: err}
	case errors.Is(err, providerswire.ErrPTYAllocationFailed):
		return mappedProviderPTYError{workerError: workers.ErrPTYAllocationFailed, providerError: err}
	case errors.Is(err, providerswire.ErrPTYSessionTimedOut):
		return mappedProviderPTYError{workerError: workers.ErrPTYSessionTimedOut, providerError: err}
	case errors.Is(err, providerswire.ErrPTYNonzeroExit):
		return mappedProviderPTYError{workerError: workers.ErrPTYNonzeroExit, providerError: err}
	case errors.Is(err, providerswire.ErrPTYClockRequired):
		return mappedProviderPTYError{workerError: workers.ErrPTYClockRequired, providerError: err}
	case errors.Is(err, providerswire.ErrPTYHostRequired):
		return mappedProviderPTYError{workerError: workers.ErrPTYHostRequired, providerError: err}
	default:
		return err
	}
}

type mappedProviderPTYError struct {
	workerError   error
	providerError error
}

func (err mappedProviderPTYError) Error() string {
	return err.workerError.Error()
}

func (err mappedProviderPTYError) Unwrap() error {
	return err.providerError
}

func (err mappedProviderPTYError) Is(target error) bool {
	return target == err.workerError
}

func (adapter providerPTYSessionAdapter) Close() error {
	if adapter.session == nil {
		return nil
	}
	return adapter.session.Close()
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

func provideWorkFactory(
	readFile work.SubmittedFileReader,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
) factorysessionwire.WorkFactory {
	return func(runtimes work.RuntimeResolver) work.Service {
		return workwire.NewRuntimeService(runtimes, readFile, contentStaging, contentMaterializer)
	}
}

// provideWorkMaterializationService supplies Runtime's canonical Worker-output
// materialization seam without introducing a Runtime→Work implementation
// import. Session-scoped admission/state access receives its own resolver-bound
// Work root; this root is intentionally policy-only.
func provideWorkMaterializationService(
	contentMaterializer work.ContentMaterializer,
) work.Service {
	return workwire.NewRuntimeService(nil, nil, nil, contentMaterializer)
}
