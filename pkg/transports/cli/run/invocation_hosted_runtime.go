package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

// HostedInvocationOperation is the narrow capability retained by the CLI
// after application opening. It is transport-local; the Factory Sessions root
// remains the sole named service interface.
type HostedInvocationOperation interface {
	InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorysessions.InvocationResult, error)
	SubscribeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
	ReadDurableFactorySessionEventStream(context.Context, string, factorysessions.EventReconnectRequest) (*interfaces.FactoryEventStream, error)
}

// factoryEventReader aliases the anonymous presentation-bridge reader shape
// required by OpeningPresentationOwner without publishing a root interface.
type factoryEventReader = interface {
	SubscribeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
	ReadDurableFactorySessionEventStream(context.Context, string, factorysessions.EventReconnectRequest) (*interfaces.FactoryEventStream, error)
}

type historicalReplayRunner struct {
	runner initializer.LocalRuntimeRunner
	replay *factorysessions.HistoricalReplayInspection
}

func (runner historicalReplayRunner) Run(ctx context.Context) error {
	return runner.runner.Run(ctx)
}

func (runner historicalReplayRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	managed, ok := runner.runner.(initializer.CompletionRuntimeRunner)
	if !ok {
		return runner.runner.Run(ctx)
	}
	return managed.RunWithCompletion(ctx, completion)
}

func (runner historicalReplayRunner) RuntimeHostBinding(ctx context.Context) (initializer.RuntimeHostBinding, error) {
	reader, ok := runner.runner.(interface {
		RuntimeHostBinding(context.Context) (initializer.RuntimeHostBinding, error)
	})
	if !ok {
		return initializer.RuntimeHostBinding{}, initializer.ErrRuntimeHostReadinessUnavailable
	}
	return reader.RuntimeHostBinding(ctx)
}

func (runner historicalReplayRunner) RuntimeHostReadinessConfigured() bool {
	provider, ok := runner.runner.(interface{ RuntimeHostReadinessConfigured() bool })
	return ok && provider.RuntimeHostReadinessConfigured()
}

func (runner historicalReplayRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	return runtimeLogDiagnosticsForRunner(runner.runner)
}

func (runner historicalReplayRunner) HistoricalReplay() *factorysessions.HistoricalReplayInspection {
	return runner.replay
}

func (runner historicalReplayRunner) HostedInvocation() HostedInvocationOperation {
	provider, ok := runner.runner.(interface {
		HostedInvocation() HostedInvocationOperation
	})
	if !ok {
		return nil
	}
	return provider.HostedInvocation()
}

// WithHistoricalReplay keeps the detached replay read model beside the
// initializer runner so the CLI can render it without adding replay services
// to the neutral lifecycle contract.
func WithHistoricalReplay(
	runner initializer.LocalRuntimeRunner,
	replay *factorysessions.HistoricalReplayInspection,
) initializer.LocalRuntimeRunner {
	if runner == nil || replay == nil {
		return runner
	}
	return historicalReplayRunner{runner: runner, replay: replay}
}

type replayMetadataWarningRunner struct {
	runner   initializer.LocalRuntimeRunner
	warnings []recordings.MetadataMismatchWarning
}

func (runner replayMetadataWarningRunner) Run(ctx context.Context) error {
	return runner.runner.Run(ctx)
}

func (runner replayMetadataWarningRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	managed, ok := runner.runner.(initializer.CompletionRuntimeRunner)
	if !ok {
		return runner.runner.Run(ctx)
	}
	return managed.RunWithCompletion(ctx, completion)
}

func (runner replayMetadataWarningRunner) RuntimeHostBinding(ctx context.Context) (initializer.RuntimeHostBinding, error) {
	reader, ok := runner.runner.(interface {
		RuntimeHostBinding(context.Context) (initializer.RuntimeHostBinding, error)
	})
	if !ok {
		return initializer.RuntimeHostBinding{}, initializer.ErrRuntimeHostReadinessUnavailable
	}
	return reader.RuntimeHostBinding(ctx)
}

func (runner replayMetadataWarningRunner) RuntimeHostReadinessConfigured() bool {
	provider, ok := runner.runner.(interface{ RuntimeHostReadinessConfigured() bool })
	return ok && provider.RuntimeHostReadinessConfigured()
}

func (runner replayMetadataWarningRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	return runtimeLogDiagnosticsForRunner(runner.runner)
}

func (runner replayMetadataWarningRunner) HostedInvocation() HostedInvocationOperation {
	provider, ok := runner.runner.(interface {
		HostedInvocation() HostedInvocationOperation
	})
	if !ok {
		return nil
	}
	return provider.HostedInvocation()
}

func (runner replayMetadataWarningRunner) ReplayMetadataWarnings() []recordings.MetadataMismatchWarning {
	return append([]recordings.MetadataMismatchWarning(nil), runner.warnings...)
}

// WithReplayMetadataWarnings retains replay drift details at the CLI edge
// without adding replay-specific state to the neutral initializer contract.
func WithReplayMetadataWarnings(
	runner initializer.LocalRuntimeRunner,
	warnings []recordings.MetadataMismatchWarning,
) initializer.LocalRuntimeRunner {
	if runner == nil || len(warnings) == 0 {
		return runner
	}
	return replayMetadataWarningRunner{
		runner:   runner,
		warnings: append([]recordings.MetadataMismatchWarning(nil), warnings...),
	}
}

func replayMetadataWarningsForRunner(
	runner initializer.LocalRuntimeRunner,
) []recordings.MetadataMismatchWarning {
	provider, ok := runner.(interface {
		ReplayMetadataWarnings() []recordings.MetadataMismatchWarning
	})
	if !ok {
		return nil
	}
	return append([]recordings.MetadataMismatchWarning(nil), provider.ReplayMetadataWarnings()...)
}

type hostedInvocationRunner struct {
	runner     initializer.LocalRuntimeRunner
	invocation HostedInvocationOperation
}

func (runner hostedInvocationRunner) Run(ctx context.Context) error {
	return runner.runner.Run(ctx)
}

func (runner hostedInvocationRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	managed, ok := runner.runner.(initializer.CompletionRuntimeRunner)
	if !ok {
		return runner.runner.Run(ctx)
	}
	return managed.RunWithCompletion(ctx, completion)
}

func (runner hostedInvocationRunner) HostedInvocation() HostedInvocationOperation {
	return runner.invocation
}

func (runner hostedInvocationRunner) RuntimeHostBinding(ctx context.Context) (initializer.RuntimeHostBinding, error) {
	reader, ok := runner.runner.(interface {
		RuntimeHostBinding(context.Context) (initializer.RuntimeHostBinding, error)
	})
	if !ok {
		return initializer.RuntimeHostBinding{}, initializer.ErrRuntimeHostReadinessUnavailable
	}
	return reader.RuntimeHostBinding(ctx)
}

func (runner hostedInvocationRunner) RuntimeHostReadinessConfigured() bool {
	provider, ok := runner.runner.(interface{ RuntimeHostReadinessConfigured() bool })
	return ok && provider.RuntimeHostReadinessConfigured()
}

func (runner hostedInvocationRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	return runtimeLogDiagnosticsForRunner(runner.runner)
}

// WithHostedInvocation retains the opened runtime's narrow invocation
// capability beside the lifecycle runner. The capability is an operation
// result, not part of the immutable application-opening request.
func WithHostedInvocation(
	runner initializer.LocalRuntimeRunner,
	invocation HostedInvocationOperation,
) initializer.LocalRuntimeRunner {
	if runner == nil || invocation == nil {
		return runner
	}
	return hostedInvocationRunner{runner: runner, invocation: invocation}
}

// WithCleanInvocationSnapshot remains a source-compatible Wire seam while
// terminal classification is owned by Factory Sessions results. The Runtime
// projection is intentionally not attached to the CLI runner anymore.
func WithCleanInvocationSnapshot(
	runner initializer.LocalRuntimeRunner,
	_ interface{},
) initializer.LocalRuntimeRunner {
	return runner
}

func openHostedRuntime(
	ctx context.Context,
	cfg RunConfig,
	logger *zap.Logger,
	invocationRequest *factoryapi.InvocationRequest,
	recordPath resolvedRunRecordPath,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	mockWorkersConfig *workers.MockWorkersConfig,
	invocationMode bool,
	requestedPort int,
	buildRunner RuntimeRunnerBuilder,
	buildRuntimeRequest RuntimeOpeningRequestFactory,
	presentations factorysessions.OpeningPresentationOwner,
	visualizations factoryvisualization.RuntimeSinkOwner,
) (*Operation, error) {
	if buildRunner == nil {
		return nil, errors.New("construct local runtime: injected runtime runner builder is required")
	}
	if buildRuntimeRequest == nil {
		return nil, errors.New("construct local runtime: runtime opening request factory is required")
	}
	operation, runtimeCfg, err := prepareHostedInvocation(
		ctx, cfg, logger, invocationRequest, recordPath, invocation,
		presentation, mockWorkersConfig, invocationMode,
	)
	if err != nil {
		return nil, err
	}
	openingRequest := buildRuntimeRequest(runtimeCfg, mockWorkersConfig)
	var factorySvc initializer.LocalRuntimeRunner
	onBound := newRuntimeHostObserver(
		ctx, cfg, recordPath, requestedPort,
		func() runtimeartifact.Diagnostics { return runtimeLogDiagnosticsForRunner(factorySvc) },
	)
	if cfg.Port <= 0 {
		emitVerboseStartupDiagnostics(cfg, recordPath, requestedPort)
	}
	visualizationSinkID, err := registerRuntimeVisualizationSink(visualizations, cfg, presentation)
	if err != nil {
		return nil, err
	}
	factorySvc, err = buildRunner(ctx, openingRequest, cfg.Cancellation, factorysessions.VisualizationSinkID(visualizationSinkID))
	if err != nil {
		closeRuntimeVisualizationSink(visualizations, visualizationSinkID)
		return nil, err
	}
	if factorySvc == nil {
		closeRuntimeVisualizationSink(visualizations, visualizationSinkID)
		return nil, fmt.Errorf("construct local runtime: builder returned nil runner")
	}
	replayMetadataWarnings := replayMetadataWarningsForRunner(factorySvc)
	historicalReplay, hostedInvocation := hostedRuntimeCapabilities(factorySvc)
	if !cfg.CleanInvocation && (cfg.WithServer || cfg.WithSite || cfg.Port > 0) {
		factorySvc = runtimeapplication.WithRuntimeHostObserver(factorySvc, onBound)
	}
	if operation != nil {
		operation.runner = factorySvc
		operation.hostedInvocation = hostedInvocation
		operation.historicalReplay = historicalReplay
		operation.replayMetadataWarnings = replayMetadataWarnings
		operation.openingPresentations = presentations
		operation.visualizations = visualizations
		operation.visualizationSinkID = visualizationSinkID
		return operation, nil
	}

	return &Operation{
		cfg: cfg, logger: logger, runner: factorySvc, recordPath: recordPath,
		hostedInvocation: hostedInvocation, historicalReplay: historicalReplay,
		replayMetadataWarnings: replayMetadataWarnings,
		openingPresentations:   presentations, visualizations: visualizations,
		visualizationSinkID: visualizationSinkID,
	}, nil
}

func registerRuntimeVisualizationSink(
	visualizations factoryvisualization.RuntimeSinkOwner,
	cfg RunConfig,
	presentation factoryvisualization.ResponsePresentation,
) (factoryvisualization.RuntimeSinkID, error) {
	if visualizations == nil {
		return "", nil
	}
	sink := runVisualizationSink(cfg, presentation)
	if sink == nil {
		return "", nil
	}
	id, err := visualizations.RegisterRuntimeSink(sink)
	if err != nil {
		return "", fmt.Errorf("register Factory Visualization sink: %w", err)
	}
	return id, nil
}

func closeRuntimeVisualizationSink(
	visualizations factoryvisualization.RuntimeSinkOwner,
	id factoryvisualization.RuntimeSinkID,
) {
	if visualizations != nil {
		visualizations.CloseRuntimeSink(id)
	}
}

func hostedRuntimeCapabilities(
	runner initializer.LocalRuntimeRunner,
) (*factorysessions.HistoricalReplayInspection, HostedInvocationOperation) {
	var historicalReplay *factorysessions.HistoricalReplayInspection
	if provider, ok := runner.(interface {
		HistoricalReplay() *factorysessions.HistoricalReplayInspection
	}); ok {
		historicalReplay = provider.HistoricalReplay()
	}
	var hostedInvocation HostedInvocationOperation
	if provider, ok := runner.(interface {
		HostedInvocation() HostedInvocationOperation
	}); ok {
		hostedInvocation = provider.HostedInvocation()
	}
	return historicalReplay, hostedInvocation
}

func prepareHostedInvocation(
	ctx context.Context,
	cfg RunConfig,
	logger *zap.Logger,
	request *factoryapi.InvocationRequest,
	recordPath resolvedRunRecordPath,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	mockWorkersConfig *workers.MockWorkersConfig,
	invocationMode bool,
) (*Operation, RunConfig, error) {
	if !invocationMode {
		return nil, cfg, nil
	}
	operation, err := openInvocation(
		ctx, cfg, logger, request, recordPath, invocation, presentation, mockWorkersConfig, nil,
	)
	if err != nil {
		return nil, RunConfig{}, err
	}
	// The hosted runtime remains alive until the invocation reaches its
	// terminal result; the customer-visible run is still one-shot.
	runtimeCfg := cfg
	runtimeCfg.Continuously = true
	if cfg.CleanInvocation {
		// A finite --work selection has already been projected into the
		// Sessions invocation request. Do not submit the same file again as
		// startup Work when a server-attached runtime is used.
		runtimeCfg.WorkFile = ""
	}
	return operation, runtimeCfg, nil
}

func newRuntimeHostObserver(
	ctx context.Context,
	cfg RunConfig,
	recordPath resolvedRunRecordPath,
	requestedPort int,
	diagnostics func() runtimeartifact.Diagnostics,
) factorysessions.RuntimeHostObserver {
	return func(binding factorysessions.RuntimeHostBinding) {
		resolved := cfg
		if strings.TrimSpace(binding.Host) != "" {
			resolved.BindHost = binding.Host
		}
		resolved.Port = binding.Port
		emitStartupMessages(resolved, diagnostics())
		emitVerboseStartupDiagnostics(resolved, recordPath, requestedPort)
		if shouldOpenDashboard(resolved) {
			openDashboardAtBoundEndpoint(ctx, resolved, cfg.BrowserOpener)
		}
	}
}

func visualizationCLIService(
	presentation factoryvisualization.ResponsePresentation,
) visualizationcli.Service {
	return visualizationcli.NewFromPresentation(presentation)
}

func runVisualizationSink(
	cfg RunConfig,
	presentation factoryvisualization.ResponsePresentation,
) factoryvisualization.Sink {
	service := visualizationCLIService(presentation)
	if service == nil {
		return nil
	}
	return service.BuildVisualizationSink(visualizationcli.SinkConfig{
		Output:                     cfg.Output,
		SuppressDashboardRendering: cfg.SuppressDashboardRendering,
	})
}

func invocationFactoryEventRenderer(
	cfg RunConfig,
	presentation factoryvisualization.ResponsePresentation,
) (visualizationcli.FactoryEventRenderer, error) {
	if !isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		return nil, nil
	}
	service := visualizationCLIService(presentation)
	if service == nil {
		return nil, fmt.Errorf("construct factory invocation: response presentation operation is required")
	}
	return service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               cfg.Output,
		ProgressOutput:       cfg.ProgressOutput,
		JSON:                 cfg.JSONOutput,
		Color:                cfg.OutputIsTTY && !cfg.JSONOutput,
		ProgressIsTTY:        cfg.ProgressIsTTY && !cfg.JSONOutput,
		InvocationOutputMode: cfg.InvocationOutputMode,
	})
}
