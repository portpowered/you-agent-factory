package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/timedisplay"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func prepareCanonicalSessionIDForRun(cfg RunConfig) (RunConfig, error) {
	if !usesAutomaticRecording(cfg) || strings.TrimSpace(cfg.CanonicalSessionID) != "" {
		return cfg, nil
	}
	generator := cfg.CanonicalSessionIDGenerator
	if generator == nil {
		generator = uuid.NewString
	}
	canonicalID := strings.TrimSpace(generator())
	if canonicalID == "" {
		return RunConfig{}, fmt.Errorf("canonical Factory Session ID generator returned an empty identity")
	}
	cfg.CanonicalSessionID = canonicalID
	return cfg, nil
}

func usesAutomaticRecording(cfg RunConfig) bool {
	return strings.TrimSpace(cfg.RecordPath) == "" &&
		!cfg.DisableDefaultRecording &&
		strings.TrimSpace(cfg.ReplayPath) == ""
}

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

func (runner historicalReplayRunner) CleanInvocationSnapshot(
	ctx context.Context,
) (factoryruntime.CleanInvocationSnapshot, error) {
	provider, ok := runner.runner.(batchReportProvider)
	if !ok {
		return factoryruntime.CleanInvocationSnapshot{}, factoryruntime.ErrNotRunning
	}
	return provider.CleanInvocationSnapshot(ctx)
}

func (runner historicalReplayRunner) ControlWaitToComplete(
	req factoryruntime.WaitToCompleteRequest,
) factoryruntime.WaitToCompleteResult {
	provider, ok := runner.runner.(batchCompletionWaiter)
	if !ok || provider == nil {
		return factoryruntime.WaitToCompleteResult{}
	}
	return provider.ControlWaitToComplete(req)
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

type cleanInvocationSnapshotRunner struct {
	runner   initializer.LocalRuntimeRunner
	provider factoryruntime.Service
}

func (runner cleanInvocationSnapshotRunner) Run(ctx context.Context) error {
	return runner.runner.Run(ctx)
}

func (runner cleanInvocationSnapshotRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	managed, ok := runner.runner.(initializer.CompletionRuntimeRunner)
	if !ok {
		return runner.runner.Run(ctx)
	}
	return managed.RunWithCompletion(ctx, completion)
}

func (runner cleanInvocationSnapshotRunner) CleanInvocationSnapshot(
	ctx context.Context,
) (factoryruntime.CleanInvocationSnapshot, error) {
	if runner.provider == nil {
		return factoryruntime.CleanInvocationSnapshot{}, factoryruntime.ErrNotRunning
	}
	return runner.provider.CleanInvocationSnapshot(ctx)
}

func (runner cleanInvocationSnapshotRunner) ControlWaitToComplete(
	req factoryruntime.WaitToCompleteRequest,
) factoryruntime.WaitToCompleteResult {
	if runner.provider == nil {
		return factoryruntime.WaitToCompleteResult{}
	}
	return runner.provider.ControlWaitToComplete(req)
}

func (runner cleanInvocationSnapshotRunner) RuntimeHostBinding(ctx context.Context) (initializer.RuntimeHostBinding, error) {
	reader, ok := runner.runner.(interface {
		RuntimeHostBinding(context.Context) (initializer.RuntimeHostBinding, error)
	})
	if !ok {
		return initializer.RuntimeHostBinding{}, initializer.ErrRuntimeHostReadinessUnavailable
	}
	return reader.RuntimeHostBinding(ctx)
}

func (runner cleanInvocationSnapshotRunner) RuntimeHostReadinessConfigured() bool {
	provider, ok := runner.runner.(interface{ RuntimeHostReadinessConfigured() bool })
	return ok && provider.RuntimeHostReadinessConfigured()
}

func (runner cleanInvocationSnapshotRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	return runtimeLogDiagnosticsForRunner(runner.runner)
}

func (runner cleanInvocationSnapshotRunner) HostedInvocation() HostedInvocationOperation {
	provider, ok := runner.runner.(interface {
		HostedInvocation() HostedInvocationOperation
	})
	if !ok {
		return nil
	}
	return provider.HostedInvocation()
}

func (runner cleanInvocationSnapshotRunner) HistoricalReplay() *factorysessions.HistoricalReplayInspection {
	provider, ok := runner.runner.(interface {
		HistoricalReplay() *factorysessions.HistoricalReplayInspection
	})
	if !ok {
		return nil
	}
	return provider.HistoricalReplay()
}

// WithCleanInvocationSnapshot keeps the Runtime-owned terminal projection
// beside the neutral lifecycle runner for finite --work batch reporting.
func WithCleanInvocationSnapshot(
	runner initializer.LocalRuntimeRunner,
	provider factoryruntime.Service,
) initializer.LocalRuntimeRunner {
	if runner == nil || provider == nil {
		return runner
	}
	return cleanInvocationSnapshotRunner{runner: runner, provider: provider}
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
	startupDisclosure, err := prepareStartupBeforeRuntime(ctx, cfg)
	if err != nil {
		return nil, err
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
		startupDisclosure,
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
	if cfg.Port <= 0 {
		startupDisclosure.commit()
	}
	replayMetadataWarnings := replayMetadataWarningsForRunner(factorySvc)
	historicalReplay, hostedInvocation := hostedRuntimeCapabilities(factorySvc)
	batchProvider := batchReportProviderFromRunner(factorySvc)
	if !cfg.CleanInvocation && (cfg.WithServer || cfg.WithSite || cfg.Port > 0) {
		factorySvc = runtimeapplication.WithRuntimeHostObserver(factorySvc, onBound)
	}
	if operation != nil {
		operation.runner = factorySvc
		operation.batchReportProvider = batchProvider
		operation.hostedInvocation = hostedInvocation
		operation.historicalReplay = historicalReplay
		operation.replayMetadataWarnings = replayMetadataWarnings
		operation.openingPresentations = presentations
		operation.visualizations = visualizations
		operation.visualizationSinkID = visualizationSinkID
		operation.startupPrepared = true
		return operation, nil
	}

	return &Operation{
		cfg: cfg, logger: logger, runner: factorySvc, recordPath: recordPath,
		startupPrepared:     true,
		batchReportProvider: batchProvider,
		hostedInvocation:    hostedInvocation, historicalReplay: historicalReplay,
		replayMetadataWarnings: replayMetadataWarnings,
		openingPresentations:   presentations, visualizations: visualizations,
		visualizationSinkID: visualizationSinkID,
	}, nil
}

func batchReportProviderFromRunner(runner initializer.LocalRuntimeRunner) batchReportProvider {
	provider, _ := runner.(batchReportProvider)
	return provider
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
	startupDisclosure *startupDisclosure,
) factorysessions.RuntimeHostObserver {
	return func(binding factorysessions.RuntimeHostBinding) {
		resolved := cfg
		if strings.TrimSpace(binding.Host) != "" {
			resolved.BindHost = binding.Host
		}
		resolved.Port = binding.Port
		// Hosted startup prepares the disclosure before opening the runtime, then
		// commits it only after binding succeeds so a listener failure remains
		// free of human startup output.
		startupDisclosure.commit()
		emitStartupDetails(resolved, diagnostics())
		emitVerboseStartupDiagnostics(resolved, recordPath, requestedPort)
		if shouldOpenDashboard(resolved) {
			openDashboardAtBoundEndpoint(ctx, resolved, cfg.BrowserOpener)
		}
	}
}

// prepareStartupBeforeRuntime is the one-way process boundary for hosted
// runs. Runtime opening owns log/metrics creation and listener setup, so the
// process-owned preparation gate must complete before either effect occurs.
type startupDisclosure struct {
	output     io.Writer
	staged     *bytes.Buffer
	commitFunc func()
}

func (disclosure *startupDisclosure) commit() {
	if disclosure == nil {
		return
	}
	if disclosure.commitFunc != nil {
		disclosure.commitFunc()
		disclosure.commitFunc = nil
	}
	if disclosure.staged != nil && disclosure.output != nil {
		_, _ = io.Copy(disclosure.output, disclosure.staged)
		disclosure.staged = nil
	}
}

func prepareStartupBeforeRuntime(ctx context.Context, cfg RunConfig) (*startupDisclosure, error) {
	discloseHome := startupDisclosureEnabled(cfg)
	var disclosure *startupDisclosure
	var disclosureOutput io.Writer = cfg.StartupOutput
	if cfg.StartupDisclosureCommit != nil {
		disclosure = &startupDisclosure{commitFunc: cfg.StartupDisclosureCommit}
		disclosureOutput = io.Discard
	} else if discloseHome && cfg.StartupOutput != nil &&
		(cfg.DeferHomeDisclosureUntilHostReady || hasRecordingInput(cfg)) {
		staged := &bytes.Buffer{}
		disclosure = &startupDisclosure{output: cfg.StartupOutput, staged: staged}
		disclosureOutput = staged
	}
	if cfg.StartupPreparation != nil {
		if err := cfg.StartupPreparation(ctx, discloseHome, disclosureOutput); err != nil {
			return nil, err
		}
		return disclosure, nil
	}
	if discloseHome {
		emitHomeDirectoryDisclosureTo(cfg, disclosureOutput)
	}
	return disclosure, nil
}

func startupDisclosureEnabled(cfg RunConfig) bool {
	return cfg.StartupOutput != nil && !cfg.JSON && !cfg.JSONOutput &&
		!cfg.CleanInvocation && !cfg.SuppressDashboardRendering && !cfg.InvocationOutputExplicit
}

func hasRecordingInput(cfg RunConfig) bool {
	return strings.TrimSpace(cfg.ReplayPath) != "" || strings.TrimSpace(cfg.ResumePath) != ""
}

func emitStartupMessages(cfg RunConfig, runtimeLog runtimeartifact.Diagnostics) bool {
	if cfg.StartupOutput == nil {
		return false
	}

	emitHomeDirectoryDisclosure(cfg)
	return emitStartupDetails(cfg, runtimeLog)
}

func emitStartupDetails(cfg RunConfig, runtimeLog runtimeartifact.Diagnostics) bool {
	if cfg.StartupOutput == nil {
		return false
	}

	fmt.Fprintf(cfg.StartupOutput, "Factory initiated: %s\n", cfg.Dir)
	if cfg.Bootstrap {
		fmt.Fprintf(cfg.StartupOutput, "Factory directory ready: %s\n", cfg.Dir)
	}
	if cfg.Continuously {
		fmt.Fprintln(cfg.StartupOutput, "Runtime mode: continuous")
	}
	if strings.TrimSpace(runtimeLog.Path) != "" {
		fmt.Fprintf(cfg.StartupOutput, "Runtime log: %s\n", runtimeLog.Path)
		fmt.Fprintf(cfg.StartupOutput, "Runtime log start (UTC): %s\n", timedisplay.Timestamp(runtimeLog.StartTimeUTC))
	}
	if strings.TrimSpace(runtimeLog.MetricsPath) != "" {
		fmt.Fprintf(cfg.StartupOutput, "Runtime metrics: %s\n", runtimeLog.MetricsPath)
		fmt.Fprintf(cfg.StartupOutput, "Runtime metrics start (UTC): %s\n", timedisplay.Timestamp(runtimeLog.MetricsStartTimeUTC))
	}
	if cfg.Port <= 0 {
		fmt.Fprintln(cfg.StartupOutput, "Dashboard server disabled")
		return false
	}

	url := DashboardURL(bindDashboardHost(cfg), cfg.Port)
	fmt.Fprintf(cfg.StartupOutput, "Dashboard URL: %s\n", url)
	if !cfg.OpenDashboard {
		fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open disabled; open %s\n", url)
		return false
	}
	return true
}

func emitHomeDirectoryDisclosure(cfg RunConfig) {
	emitHomeDirectoryDisclosureTo(cfg, cfg.StartupOutput)
}

func emitHomeDirectoryDisclosureTo(cfg RunConfig, output io.Writer) {
	if output == nil ||
		strings.TrimSpace(cfg.HomeDir) == "" ||
		cfg.JSON || cfg.JSONOutput || cfg.CleanInvocation ||
		cfg.SuppressDashboardRendering || cfg.InvocationOutputExplicit {
		return
	}
	_, _ = fmt.Fprintf(output, "Home directory: %s\n", cfg.HomeDir)
}

// DiscloseHomeDirectory writes the human startup home line using the same
// output policy as the run transport at the process startup boundary.
func DiscloseHomeDirectory(cfg RunConfig) {
	emitHomeDirectoryDisclosure(cfg)
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

const batchFailureCode = "RUN_BATCH_FAILED"

type batchReportProvider interface {
	CleanInvocationSnapshot(context.Context) (factoryruntime.CleanInvocationSnapshot, error)
}

type batchCompletionWaiter interface {
	ControlWaitToComplete(factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult
}

type batchReport struct {
	Status   string         `json:"status"`
	Failures []batchFailure `json:"failures"`
}

type batchFailure struct {
	WorkID    string `json:"workId,omitempty"`
	WorkName  string `json:"workName"`
	WorkState string `json:"workState"`
	Reason    string `json:"reason"`
}

func reportBatchResult(
	cfg RunConfig,
	snapshot factoryruntime.CleanInvocationSnapshot,
) error {
	report := buildBatchReport(snapshot)
	output := cfg.Output
	if output == nil {
		output = cfg.StartupOutput
	}
	if output == nil {
		return fmt.Errorf("write batch result: process output is required")
	}

	if cfg.JSON || cfg.JSONOutput {
		if err := json.NewEncoder(output).Encode(report); err != nil {
			return fmt.Errorf("write batch JSON result: %w", err)
		}
	} else if err := writeHumanBatchReport(output, report); err != nil {
		return err
	}

	if len(report.Failures) == 0 {
		return nil
	}
	return &InvocationError{
		Code:    batchFailureCode,
		Message: batchFailureMessage(report.Failures),
	}
}

func buildBatchReport(snapshot factoryruntime.CleanInvocationSnapshot) batchReport {
	failuresByKey := make(map[string]batchFailure)
	for _, work := range snapshot.Work {
		if work.StateCategory != string(factoryruntime.StateCategoryFailed) {
			continue
		}
		failure := batchFailure{
			WorkID:    strings.TrimSpace(work.WorkID),
			WorkName:  batchWorkName(work),
			WorkState: batchWorkState(work),
			Reason:    batchFailureReason(work, snapshot.DispatchHistory),
		}
		key := failure.WorkID
		if key == "" {
			key = failure.WorkName + "\x00" + failure.WorkState
		}
		if current, exists := failuresByKey[key]; !exists || batchFailureLess(failure, current) {
			failuresByKey[key] = failure
		}
	}

	failures := make([]batchFailure, 0, len(failuresByKey))
	for _, failure := range failuresByKey {
		failures = append(failures, failure)
	}
	sort.Slice(failures, func(i, j int) bool {
		return batchFailureLess(failures[i], failures[j])
	})
	status := "COMPLETED"
	if len(failures) > 0 {
		status = "FAILED"
	}
	return batchReport{Status: status, Failures: failures}
}

func batchFailureLess(left, right batchFailure) bool {
	if left.WorkID != right.WorkID {
		return left.WorkID < right.WorkID
	}
	if left.WorkName != right.WorkName {
		return left.WorkName < right.WorkName
	}
	if left.WorkState != right.WorkState {
		return left.WorkState < right.WorkState
	}
	return left.Reason < right.Reason
}

func batchWorkName(work factoryruntime.CleanInvocationWork) string {
	if name := strings.TrimSpace(work.Name); name != "" {
		return name
	}
	if workID := strings.TrimSpace(work.WorkID); workID != "" {
		return workID
	}
	return "<unnamed Work>"
}

func batchWorkState(work factoryruntime.CleanInvocationWork) string {
	state := strings.TrimSpace(work.State)
	if state == "" {
		state = strings.ToLower(strings.TrimSpace(work.StateCategory))
	}
	if state == "" {
		state = "failed"
	}
	workTypeID := strings.TrimSpace(work.WorkTypeID)
	if workTypeID == "" {
		return state
	}
	return workTypeID + ":" + state
}

func batchFailureReason(
	work factoryruntime.CleanInvocationWork,
	dispatches []factoryruntime.CleanInvocationDispatch,
) string {
	if reason := strings.TrimSpace(work.FailureReason); reason != "" {
		return reason
	}
	for index := len(dispatches) - 1; index >= 0; index-- {
		dispatch := dispatches[index]
		if dispatch.Outcome != "FAILED" || !batchDispatchMatches(work, dispatch) {
			continue
		}
		if reason := strings.TrimSpace(dispatch.Reason); reason != "" {
			return reason
		}
		if failureType := strings.TrimSpace(dispatch.FailureType); failureType != "" {
			return "worker dispatch failed (" + failureType + ")"
		}
	}
	return "Work reached a failed terminal state; inspect the latest dispatch for recovery guidance."
}

func batchDispatchMatches(
	work factoryruntime.CleanInvocationWork,
	dispatch factoryruntime.CleanInvocationDispatch,
) bool {
	for _, candidate := range dispatch.Consumed {
		if batchWorkMatches(work, candidate) {
			return true
		}
	}
	for _, candidate := range dispatch.Outputs {
		if batchWorkMatches(work, candidate) {
			return true
		}
	}
	return false
}

func batchWorkMatches(left, right factoryruntime.CleanInvocationWork) bool {
	if left.WorkID != "" && right.WorkID != "" {
		return left.WorkID == right.WorkID
	}
	if left.TraceID != "" && right.TraceID != "" {
		return left.TraceID == right.TraceID
	}
	return left.Name != "" && left.Name == right.Name && left.WorkTypeID == right.WorkTypeID
}

func writeHumanBatchReport(output io.Writer, report batchReport) error {
	if len(report.Failures) == 0 {
		if _, err := fmt.Fprintln(output, "Batch completed successfully."); err != nil {
			return fmt.Errorf("write batch result: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprintln(output, "Batch failed:"); err != nil {
		return fmt.Errorf("write batch result: %w", err)
	}
	for _, failure := range report.Failures {
		if _, err := fmt.Fprintf(
			output,
			"Work %q reached failed terminal state %s: %s\n",
			failure.WorkName,
			failure.WorkState,
			failure.Reason,
		); err != nil {
			return fmt.Errorf("write batch result: %w", err)
		}
	}
	return nil
}

func batchFailureMessage(failures []batchFailure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		parts = append(parts, fmt.Sprintf(
			"Work %q reached %s: %s",
			failure.WorkName,
			failure.WorkState,
			failure.Reason,
		))
	}
	return strings.Join(parts, "; ")
}
