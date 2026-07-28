package run

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	contentmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"go.uber.org/zap"
)

type testRuntimeRunnerOpener func(
	context.Context,
	*testRuntimeSelections,
	serviceedges.Edges,
) (RuntimeRunner, error)

// testRuntimeSelections is a detached observation view used by legacy CLI
// behavior tests. Production receives only owner-bounded opening requests.
type testRuntimeSelections struct {
	Dir                                     string
	RunnerID                                string
	OperatorDefaults                        operatorconfig.ResolvedDefaults
	ExecutionBaseDir                        string
	DurableSessionPersistencePolicy         factorysessions.PersistencePolicy
	RuntimeMode                             interfaces.RuntimeMode
	Port                                    int
	AutoPort                                bool
	RuntimeHostObserver                     factorysessions.RuntimeHostObserver
	Logger                                  *zap.Logger
	Verbose                                 bool
	RuntimeInstanceID                       string
	BackendScopeID                          string
	SystemConfigHomeDir                     string
	SystemConfigPath                        string
	RuntimeLogDir                           string
	RuntimeFileLoggingPolicy                factoryruntime.RuntimeFileLoggingPolicy
	RuntimeLogConfig                        logging.RuntimeLogConfig
	RuntimeMetricsPolicy                    factoryruntime.RuntimeMetricsPolicy
	RuntimeMetricsDir                       string
	RuntimeMetricsConfig                    platformmetrics.RuntimeMetricsConfig
	WorkFile                                string
	RecordPath                              string
	ReplayPath                              string
	WorkflowID                              string
	MockWorkersConfig                       *workers.MockWorkersConfig
	InvocationSkipPermissionsOverride       *bool
	RecordFlushInterval                     time.Duration
	SkipBuiltInRunnerPrerequisiteValidation bool
	ModelCacheDir                           string
}

func testRuntimeOpeningRequestFactory(
	cfg RunConfig,
	mockWorkers *workers.MockWorkersConfig,
	observer factorysessions.RuntimeHostObserver,
) factorysessions.ApplicationOpeningRequest {
	mode := interfaces.RuntimeModeBatch
	if cfg.Continuously {
		mode = interfaces.RuntimeModeService
	}
	logDirectory := cfg.RuntimeLogDir
	metricsDirectory := cfg.RuntimeMetricsDir
	if cfg.HomeDir != "" {
		if logDirectory == "" {
			logDirectory = logging.RuntimeLogsRoot(cfg.HomeDir)
		}
		if metricsDirectory == "" {
			metricsDirectory = metrics.RuntimeMetricsRoot(cfg.HomeDir)
		}
	}
	return factorysessions.ApplicationOpeningRequest{Runtime: &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: interfaces.RuntimeOpeningRequest{
			Directory: cfg.Dir, ExecutionBaseDir: cfg.ExecutionBaseDir,
		},
		FactoryRuntime: factoryruntime.RuntimeOpeningRequest{
			Mode: mode, Verbose: cfg.Verbose,
			LogDirectory: logDirectory, LogConfig: factoryruntime.RuntimeLogStorageConfig{
				MaxSize: cfg.RuntimeLogConfig.MaxSize, MaxBackups: cfg.RuntimeLogConfig.MaxBackups,
				MaxAge: cfg.RuntimeLogConfig.MaxAge, Compress: cfg.RuntimeLogConfig.Compress,
			},
			MetricsDirectory: metricsDirectory, MetricsConfig: factoryruntime.RuntimeMetricsStorageConfig{
				MaxSize: cfg.RuntimeMetricsConfig.MaxSize, MaxBackups: cfg.RuntimeMetricsConfig.MaxBackups,
				MaxAge: cfg.RuntimeMetricsConfig.MaxAge, Compress: cfg.RuntimeMetricsConfig.Compress,
			},
		},
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			SystemConfigHome: cfg.HomeDir, WorkFile: cfg.WorkFile,
			Host: factorysessions.RuntimeHostRequest{
				Directory: cfg.Dir, RuntimeMode: mode, WorkFile: cfg.WorkFile,
				MockWorkers: mockWorkers != nil, Port: cfg.Port, AutoPort: cfg.AutoPort,
			},
		},
		Workers: workers.RuntimeOpeningRequest{
			RunnerID: cfg.RunnerID, MockWorkers: mockWorkers,
			InvocationSkipPermissionsOverride: cfg.InvocationSkipPermissionsOverride,
		},
		Recordings: recordings.RuntimeOpeningRequest{
			RecordPath: cfg.RecordPath, ReplayPath: cfg.ReplayPath, WorkflowID: cfg.Workflow,
		},
		Models:           models.RuntimeOpeningRequest{CacheDirectory: cfg.ModelCacheDir},
		OperatorDefaults: cfg.OperatorDefaults,
	}, Ports: factorysessions.ApplicationOpeningPorts{
		InvocationMetricsRecorder: cfg.InvocationMetricsRecorder,
		RuntimeHostObserver:       observer,
	}}
}

func flattenTestRuntimeRequest(request *factorysessions.RuntimeOpeningRequest) *testRuntimeSelections {
	if request == nil {
		return nil
	}
	return &testRuntimeSelections{
		Dir: request.FactoryDefinition.Directory, ExecutionBaseDir: request.FactoryDefinition.ExecutionBaseDir,
		RunnerID: request.Workers.RunnerID, OperatorDefaults: request.OperatorDefaults,
		DurableSessionPersistencePolicy: request.FactorySession.PersistencePolicy,
		RuntimeMode:                     request.FactoryRuntime.Mode, Port: request.FactorySession.Host.Port,
		AutoPort: request.FactorySession.Host.AutoPort,
		Verbose:  request.FactoryRuntime.Verbose, RuntimeInstanceID: request.FactoryRuntime.RuntimeInstanceID,
		BackendScopeID:      request.FactorySession.BackendScopeID,
		SystemConfigHomeDir: request.FactorySession.SystemConfigHome, SystemConfigPath: request.FactorySession.SystemConfigPath,
		RuntimeLogDir: request.FactoryRuntime.LogDirectory, RuntimeFileLoggingPolicy: request.FactoryRuntime.FileLoggingPolicy,
		RuntimeLogConfig: logging.RuntimeLogConfig{
			MaxSize: request.FactoryRuntime.LogConfig.MaxSize, MaxBackups: request.FactoryRuntime.LogConfig.MaxBackups,
			MaxAge: request.FactoryRuntime.LogConfig.MaxAge, Compress: request.FactoryRuntime.LogConfig.Compress,
		}, RuntimeMetricsPolicy: request.FactoryRuntime.MetricsPolicy,
		RuntimeMetricsDir: request.FactoryRuntime.MetricsDirectory, RuntimeMetricsConfig: platformmetrics.RuntimeMetricsConfig{
			MaxSize: request.FactoryRuntime.MetricsConfig.MaxSize, MaxBackups: request.FactoryRuntime.MetricsConfig.MaxBackups,
			MaxAge: request.FactoryRuntime.MetricsConfig.MaxAge, Compress: request.FactoryRuntime.MetricsConfig.Compress,
		},
		WorkFile: request.FactorySession.WorkFile, RecordPath: request.Recordings.RecordPath,
		ReplayPath: request.Recordings.ReplayPath, WorkflowID: request.Recordings.WorkflowID,
		MockWorkersConfig:                       request.Workers.MockWorkers,
		InvocationSkipPermissionsOverride:       request.Workers.InvocationSkipPermissionsOverride,
		RecordFlushInterval:                     request.Recordings.FlushInterval,
		SkipBuiltInRunnerPrerequisiteValidation: request.Workers.SkipBuiltInPrerequisiteValidation,
		ModelCacheDir:                           request.Models.CacheDirectory,
	}
}

func adaptTestRuntimeRunnerOpener[T RuntimeRunner](
	build func(context.Context, *testRuntimeSelections, serviceedges.Edges) (T, error),
) testRuntimeRunnerOpener {
	return func(ctx context.Context, cfg *testRuntimeSelections, edges serviceedges.Edges) (RuntimeRunner, error) {
		return build(ctx, cfg, edges)
	}
}

type testInvocationRunnerOpener func(
	context.Context,
	*testRuntimeSelections,
	serviceedges.Edges,
) (InvocationRunner, error)

type InvocationRunner interface {
	Run(context.Context) error
	apisurface.InvocationAPI
	GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error)
	CloseFactorySession(context.Context, string) error
}

type sessionInvocationRunner = InvocationRunner

// Package tests retain a replaceable builder so startup behavior can be
// isolated without exposing a fallback constructor in the production package.
var openTestRuntimeRunner testRuntimeRunnerOpener = missingTestRuntimeRunner
var openTestInvocationRunner testInvocationRunnerOpener

type testRunnerOpeners struct {
	runtime    testRuntimeRunnerOpener
	invocation testInvocationRunnerOpener
}

func (f testRunnerOpeners) BuildRunner(
	ctx context.Context,
	request factorysessions.ApplicationOpeningRequest,
	_ *zap.Logger,
	visualizationSink factoryvisualization.Sink,
) (initializer.LocalRuntimeRunner, error) {
	if f.runtime == nil {
		return nil, errors.New("construct local runtime: dependency-injected builder is required")
	}
	selections := flattenTestRuntimeRequest(request.Runtime)
	edges := serviceedges.Edges{
		InvocationMetricsRecorder: request.Ports.InvocationMetricsRecorder,
		RuntimeHostObserver:       request.Ports.RuntimeHostObserver,
	}
	if selections != nil {
		selections.RuntimeHostObserver = edges.RuntimeHostObserver
	}
	runner, err := f.runtime(ctx, selections, edges)
	if err != nil || visualizationSink == nil {
		return runner, err
	}
	snapshots, ok := runner.(engineStateSnapshotProvider)
	if !ok {
		return runner, nil
	}
	snapshot, err := snapshots.GetEngineStateSnapshot(ctx)
	if err != nil {
		return runner, nil
	}
	return testDashboardRenderingRunner{
		LocalRuntimeRunner: runner,
		sink:               visualizationSink,
		input: factoryvisualization.View{
			Runtime: factoryvisualization.RuntimeObservation{
				TickCount:     snapshot.TickCount,
				FactoryState:  snapshot.FactoryState,
				RuntimeStatus: snapshot.RuntimeStatus,
				Uptime:        snapshot.Uptime,
			},
		},
	}, nil
}

type testDashboardRenderingRunner struct {
	initializer.LocalRuntimeRunner
	sink  factoryvisualization.Sink
	input factoryvisualization.View
}

type runFuncRunner func(context.Context) error

func (run runFuncRunner) Run(ctx context.Context) error { return run(ctx) }

func TestOpenRunScopedServerAttachesInvocationCompletionAndKeepsOneShotResult(t *testing.T) {
	prompt := "ship it"
	var output strings.Builder
	var opening factorysessions.ApplicationOpeningRequest
	runnerCalls := 0
	invocationCalls := 0
	operation, err := Open(
		t.Context(),
		ensureTestRecordingsCLI(RunConfig{
			Dir:                      "factory",
			FactoryConfigPath:        "factory/factory.json",
			InvocationPositionalText: &prompt,
			WithServer:               true,
			Port:                     7437,
			DisableDefaultRecording:  true,
			Output:                   &output,
		}),
		func(
			_ context.Context,
			request factorysessions.ApplicationOpeningRequest,
			_ *zap.Logger,
			_ factoryvisualization.Sink,
		) (initializer.LocalRuntimeRunner, error) {
			opening = request
			return runFuncRunner(func(context.Context) error {
				runnerCalls++
				return nil
			}), nil
		},
		testInvocationOperation{invokeFactory: func(
			context.Context,
			factorysessions.InvocationTarget,
			factorysessions.InvocationRequest,
			factorysessions.FactoryEventConsumer,
		) (factorysessions.FactoryInvocationOutcome, error) {
			invocationCalls++
			return factorysessions.FactoryInvocationOutcome{
				Result: interfaces.FactoryInvocationResult{
					Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "done",
					}},
				},
			}, nil
		}},
		nil,
		prepareSingleWorkTargetForTest,
		testMockWorkersConfigLoader,
		testRuntimeOpeningRequestFactory,
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if opening.Runtime.FactoryRuntime.Mode != interfaces.RuntimeModeService {
		t.Fatalf("hosted runtime mode = %q, want service until terminal completion", opening.Runtime.FactoryRuntime.Mode)
	}
	if opening.Completion == nil || opening.Ports.RuntimeHostObserver == nil {
		t.Fatal("run-scoped server omitted readiness-gated terminal completion")
	}

	if invocationCalls != 0 {
		t.Fatal("invocation started while opening the hosted lifecycle")
	}
	opening.Ports.RuntimeHostObserver(factorysessions.RuntimeHostBinding{Port: 7437})
	if err := opening.Completion(t.Context()); err != nil {
		t.Fatalf("completion: %v", err)
	}
	if invocationCalls != 1 || output.String() != "done" {
		t.Fatalf("invocation calls = %d, output = %q", invocationCalls, output.String())
	}
	if err := operation.Run(t.Context()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runnerCalls != 1 {
		t.Fatalf("owned lifecycle runner calls = %d, want 1", runnerCalls)
	}
}

func (r testDashboardRenderingRunner) Run(ctx context.Context) error {
	if err := r.LocalRuntimeRunner.Run(ctx); err != nil {
		return err
	}
	r.input.ObservedAt = time.Now()
	r.sink.PresentFactoryView(r.input)
	return nil
}

func (f testRunnerOpeners) Invocation() InvocationOperation {
	return testInvocationOperation{open: f.invocation}
}

type testInvocationOperation struct {
	open          testInvocationRunnerOpener
	invokeFactory func(context.Context, factorysessions.InvocationTarget, factorysessions.InvocationRequest, factorysessions.FactoryEventConsumer) (factorysessions.FactoryInvocationOutcome, error)
}

func (testInvocationOperation) ResolveModelInvocationFactoryDir(dir string) (string, error) {
	return dir, nil
}

func (testInvocationOperation) ExportModelInvocationArtifact(string, string) error {
	return errors.New("model artifact export is not supported by the Factory test operation")
}

func (o testInvocationOperation) InvokeModel(
	context.Context,
	factorysessions.InvocationTarget,
	string,
	models.Request,
) (models.Result, error) {
	return models.Result{}, errors.New("model invocation is not supported by the Factory test operation")
}

func (o testInvocationOperation) InvokeFactory(
	ctx context.Context,
	target factorysessions.InvocationTarget,
	request factorysessions.InvocationRequest,
	consume factorysessions.FactoryEventConsumer,
) (factorysessions.FactoryInvocationOutcome, error) {
	if o.invokeFactory != nil {
		return o.invokeFactory(ctx, target, request, consume)
	}
	if o.open == nil {
		return factorysessions.FactoryInvocationOutcome{}, errors.New("construct factory invocation: test operation is required")
	}
	cfg := testInvocationRuntimeConfig(target)
	runner, err := o.open(ctx, cfg, serviceedges.Edges{InvocationMetricsRecorder: target.MetricsRecorder})
	if err != nil {
		return factorysessions.FactoryInvocationOutcome{}, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	runErr := make(chan error, 1)
	go func() { runErr <- runner.Run(runCtx) }()
	if err := waitForTestInvocationReady(runCtx, runner, runErr); err != nil {
		cancel()
		return factorysessions.FactoryInvocationOutcome{}, err
	}
	result, err := runner.InvokeFactorySession(runCtx, factorysessions.DefaultSessionID, generatedTestInvocationRequest(request))
	if source, ok := runner.(interface {
		GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error)
	}); ok && consume != nil {
		events, _ := source.GetFactoryEvents(runCtx)
		consume(events)
	}
	closeErr := runner.CloseFactorySession(runCtx, factorysessions.DefaultSessionID)
	cancel()
	lifecycleErr := <-runErr
	if err == nil {
		err = closeErr
	}
	if err == nil && lifecycleErr != nil && !errors.Is(lifecycleErr, context.Canceled) {
		err = lifecycleErr
	}
	return factorysessions.FactoryInvocationOutcome{Result: result}, err
}

func testInvocationRuntimeConfig(target factorysessions.InvocationTarget) *testRuntimeSelections {
	return &testRuntimeSelections{
		Dir: target.FactoryDir, RunnerID: target.RunnerID,
		OperatorDefaults: target.OperatorDefaults, ExecutionBaseDir: target.ExecutionBaseDir,
		SystemConfigHomeDir: target.HomeDir, Logger: target.Logger, Verbose: target.Verbose,
		RecordPath: target.RecordPath, ReplayPath: target.ReplayPath,
		RuntimeLogDir: target.RuntimeLogDir, RuntimeLogConfig: logging.RuntimeLogConfig{
			MaxSize: target.RuntimeLogConfig.MaxSize, MaxBackups: target.RuntimeLogConfig.MaxBackups,
			MaxAge: target.RuntimeLogConfig.MaxAge, Compress: target.RuntimeLogConfig.Compress,
		},
		RuntimeMetricsDir: target.RuntimeMetricsDir, RuntimeMetricsConfig: platformmetrics.RuntimeMetricsConfig{
			MaxSize: target.RuntimeMetricsConfig.MaxSize, MaxBackups: target.RuntimeMetricsConfig.MaxBackups,
			MaxAge: target.RuntimeMetricsConfig.MaxAge, Compress: target.RuntimeMetricsConfig.Compress,
		},
		ModelCacheDir: target.ModelCacheDir, WorkflowID: target.WorkflowID,
		MockWorkersConfig:                 target.MockWorkersConfig,
		InvocationSkipPermissionsOverride: target.SkipPermissionsOverride,
		Port:                              0, RuntimeMode: interfaces.RuntimeModeService,
	}
}

func waitForTestInvocationReady(ctx context.Context, runner InvocationRunner, runErr <-chan error) error {
	for {
		if _, err := runner.GetCurrentFactoryForSession(ctx, factorysessions.DefaultSessionID); err == nil {
			return nil
		}
		select {
		case err := <-runErr:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
}

func generatedTestInvocationRequest(request factorysessions.InvocationRequest) factoryapi.InvocationRequest {
	if request.PreparedInvocationInput != nil {
		result, _, err := invocationRequestFromPreparedInput(*request.PreparedInvocationInput)
		if err == nil && result != nil {
			result.RequestId = request.RequestID
			result.TimeoutMillis = request.TimeoutMillis
			return *result
		}
	}
	result := factoryapi.InvocationRequest{
		Args: request.Args, RequestId: request.RequestID, TimeoutMillis: request.TimeoutMillis,
	}
	if request.ContentProvided {
		result.Content = contentmapping.GeneratedPtrFromParts(request.Content)
	}
	if request.SourceKind != nil {
		value := factoryapi.InvocationInputSourceKind(*request.SourceKind)
		result.SourceKind = &value
	}
	return result
}

func missingTestRuntimeRunner(
	context.Context,
	*testRuntimeSelections,
	serviceedges.Edges,
) (factoryServiceRunner, error) {
	return nil, errors.New("construct local runtime: dependency-injected builder is required")
}

func runWithTestRuntimeRunner(
	ctx context.Context,
	cfg RunConfig,
	builder testRuntimeRunnerOpener,
) error {
	return runWithTestRuntimeRunnerAndMockWorkersLoader(
		ctx, cfg, builder, testMockWorkersConfigLoader,
	)
}

func runWithTestRuntimeRunnerAndMockWorkersLoader(
	ctx context.Context,
	cfg RunConfig,
	builder testRuntimeRunnerOpener,
	loadMockWorkers workers.MockWorkersConfigLoader,
) error {
	cfg = ensureTestRecordingsCLI(cfg)
	if cfg.Clock == nil {
		cfg.Clock = platformclock.Real{}
	}
	if builder == nil {
		builder = openTestRuntimeRunner
	}
	factory := testRunnerOpeners{
		runtime: builder, invocation: openTestInvocationRunner,
	}
	operation, err := Open(
		ctx,
		cfg,
		factory.BuildRunner,
		factory.Invocation(),
		testResponsePresentation(),
		prepareSingleWorkTargetForTest,
		loadMockWorkers,
		testRuntimeOpeningRequestFactory,
	)
	if err != nil {
		return err
	}
	return operation.Run(ctx)
}

func testMockWorkersConfigLoader(string) (*workers.MockWorkersConfig, error) {
	return &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{}}, nil
}

func ensureTestRecordingsCLI(cfg RunConfig) RunConfig {
	if cfg.RecordingsCLI == nil {
		cfg.RecordingsCLI = recordingscli.New()
	}
	return cfg
}

func prepareSingleWorkTargetForTest(request work.WorkRequest) (work.SingleWorkTarget, error) {
	if len(request.Works) != 1 {
		return work.SingleWorkTarget{}, fmt.Errorf("test Work target count = %d, want 1", len(request.Works))
	}
	return work.SingleWorkTarget{
		WorkID:     request.Works[0].WorkID,
		WorkTypeID: request.Works[0].WorkTypeID,
	}, nil
}

func Run(ctx context.Context, cfg RunConfig) error {
	return runWithMockWorkersConfigLoader(ctx, cfg, testMockWorkersConfigLoader)
}

func runWithMockWorkersConfigLoader(
	ctx context.Context,
	cfg RunConfig,
	loadMockWorkers workers.MockWorkersConfigLoader,
) error {
	// Package-level run tests that do not exercise recording opt out explicitly
	// instead of discovering a user home outside the Process root.
	if cfg.HomeDir == "" && cfg.RecordPath == "" && cfg.ReplayPath == "" {
		cfg.DisableDefaultRecording = true
	}
	if cfg.StdinIsTTY == nil && cfg.Stdin == nil {
		cfg.StdinIsTTY = func() bool { return true }
	}
	return runWithTestRuntimeRunnerAndMockWorkersLoader(ctx, cfg, nil, loadMockWorkers)
}
