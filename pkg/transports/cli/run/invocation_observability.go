package run

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionscli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/runconfig"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
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

func (runner hostedInvocationRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	return runtimeLogDiagnosticsForRunner(runner.runner)
}

func (runner hostedInvocationRunner) GetEngineStateSnapshot(
	ctx context.Context,
) (*interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net], error) {
	provider, ok := runner.runner.(state.LegacySnapshotProvider)
	if !ok {
		return nil, errors.New("runtime engine snapshot is unavailable")
	}
	return provider.GetEngineStateSnapshot(ctx)
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
	var visualizationSinkID factoryvisualization.RuntimeSinkID
	if visualizations != nil {
		sink := runVisualizationSink(cfg, presentation)
		if sink != nil {
			visualizationSinkID, err = visualizations.RegisterRuntimeSink(sink)
			if err != nil {
				return nil, fmt.Errorf("register Factory Visualization sink: %w", err)
			}
			openingRequest.VisualizationSinkID = factorysessions.VisualizationSinkID(visualizationSinkID)
		}
	}
	factorySvc, err = buildRunner(ctx, openingRequest)
	if err != nil {
		if visualizations != nil {
			visualizations.CloseRuntimeSink(visualizationSinkID)
		}
		return nil, err
	}
	if factorySvc == nil {
		if visualizations != nil {
			visualizations.CloseRuntimeSink(visualizationSinkID)
		}
		return nil, fmt.Errorf("construct local runtime: builder returned nil runner")
	}
	var historicalReplay *factorysessions.HistoricalReplayInspection
	if provider, ok := factorySvc.(interface {
		HistoricalReplay() *factorysessions.HistoricalReplayInspection
	}); ok {
		historicalReplay = provider.HistoricalReplay()
	}
	var hostedInvocation HostedInvocationOperation
	if provider, ok := factorySvc.(interface {
		HostedInvocation() HostedInvocationOperation
	}); ok {
		hostedInvocation = provider.HostedInvocation()
	}
	factorySvc = startupObservingRunner{runner: factorySvc, onReady: onBound}
	if operation != nil {
		operation.runner = factorySvc
		operation.hostedInvocation = hostedInvocation
		operation.historicalReplay = historicalReplay
		operation.openingPresentations = presentations
		operation.visualizations = visualizations
		operation.visualizationSinkID = visualizationSinkID
		return operation, nil
	}

	return &Operation{
		cfg: cfg, logger: logger, runner: factorySvc, recordPath: recordPath,
		prepareWorkTarget: prepareWorkTarget, hostedInvocation: hostedInvocation,
		historicalReplay:     historicalReplay,
		openingPresentations: presentations, visualizations: visualizations,
		visualizationSinkID: visualizationSinkID,
	}, nil
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
		emitVerboseStartupDiagnostics(resolved, recordPath, requestedPort)
		emitStartupMessages(resolved, diagnostics())
		if shouldOpenDashboard(resolved) {
			openDashboardAtBoundEndpoint(ctx, resolved, cfg.BrowserOpener)
		}
	}
}

type startupObservingRunner struct {
	runner  initializer.LocalRuntimeRunner
	onReady func(initializer.RuntimeHostBinding)
}

func (runner startupObservingRunner) Run(ctx context.Context) error {
	reader, ok := runner.runner.(interface {
		RuntimeHostBinding(context.Context) (initializer.RuntimeHostBinding, error)
	})
	if !ok {
		return runner.runner.Run(ctx)
	}
	runResult := make(chan error, 1)
	go func() { runResult <- runner.runner.Run(ctx) }()
	readyResult := make(chan struct {
		binding initializer.RuntimeHostBinding
		err     error
	}, 1)
	go func() {
		binding, err := reader.RuntimeHostBinding(ctx)
		readyResult <- struct {
			binding initializer.RuntimeHostBinding
			err     error
		}{binding: binding, err: err}
	}()
	select {
	case result := <-readyResult:
		if result.err == nil && runner.onReady != nil {
			runner.onReady(result.binding)
		}
		return <-runResult
	case err := <-runResult:
		if err == nil {
			// Small synchronous runners used by transport adapters may publish
			// readiness immediately before returning. Give that detached value a
			// chance to reach the reader without delaying ordinary shutdown.
			select {
			case result := <-readyResult:
				if result.err == nil && runner.onReady != nil {
					runner.onReady(result.binding)
				}
			case <-time.After(10 * time.Millisecond):
			}
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner startupObservingRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	managed, ok := runner.runner.(initializer.CompletionRuntimeRunner)
	if !ok {
		return runner.runner.Run(ctx)
	}
	return managed.RunWithCompletion(ctx, func(completionCtx context.Context) error {
		if reader, ok := runner.runner.(interface {
			RuntimeHostBinding(context.Context) (initializer.RuntimeHostBinding, error)
		}); ok {
			binding, err := reader.RuntimeHostBinding(completionCtx)
			if err != nil && !errors.Is(err, initializer.ErrRuntimeHostReadinessUnavailable) {
				return err
			}
			if err == nil && runner.onReady != nil {
				runner.onReady(binding)
			}
		}
		return completion(completionCtx)
	})
}

func (runner startupObservingRunner) RuntimeHostBinding(ctx context.Context) (initializer.RuntimeHostBinding, error) {
	reader, ok := runner.runner.(interface {
		RuntimeHostBinding(context.Context) (initializer.RuntimeHostBinding, error)
	})
	if !ok {
		return initializer.RuntimeHostBinding{}, initializer.ErrRuntimeHostReadinessUnavailable
	}
	return reader.RuntimeHostBinding(ctx)
}

func (runner startupObservingRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	return runtimeLogDiagnosticsForRunner(runner.runner)
}

// GetEngineStateSnapshot preserves the migration-only clean-invocation
// observation capability while this transport wrapper owns host-readiness
// presentation. The capability is forwarded; it is not part of the opening
// contract.
func (runner startupObservingRunner) GetEngineStateSnapshot(
	ctx context.Context,
) (*interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net], error) {
	provider, ok := runner.runner.(state.LegacySnapshotProvider)
	if !ok {
		return nil, errors.New("runtime engine snapshot is unavailable")
	}
	return provider.GetEngineStateSnapshot(ctx)
}

const (
	cleanInvocationLogMessageCompleted = "run.invocation.completed"
	cleanInvocationLogMessageRejected  = "run.invocation.rejected"
	cleanInvocationModeLabel           = "clean"
	cleanInvocationRejectReason        = "ambiguous_input"
	cleanInvocationOutcomeSuccess      = "success"
	cleanInvocationOutcomeFailure      = "failure"
	cleanInvocationOutcomeCancelled    = "cancelled"
	cleanInvocationOutcomeTimeout      = "timeout"
	cleanInvocationErrorSummaryLimit   = 160
)

type runtimeLogDiagnosticsProvider interface {
	RuntimeLogDiagnostics() runtimeartifact.Diagnostics
}

const (
	remoteFactoryEventMaxReconnectAttempts = 5
	remoteFactoryEventReconnectInterval    = remoteDurableResultPollInterval
)

// RemoteInvocationEventCursor identifies the last canonical Factory Event
// rendered by a remote invocation response stream. Both fields are sent on a
// reconnect so the server can resume after the last accepted event.
type RemoteInvocationEventCursor struct {
	EventID  string
	Sequence *int
}

// RemoteInvocationEventRequest identifies one cursor-aware canonical event
// stream read for an accepted durable Factory Session.
type RemoteInvocationEventRequest struct {
	Server        string
	SessionID     string
	AfterEventID  string
	AfterSequence *int
	Diagnostics   io.Writer
	Verbose       bool
}

// RemoteInvocationEventStream is the finite retained-event stream returned by
// the canonical Factory Session events endpoint.
type RemoteInvocationEventStream interface {
	Next(context.Context) (factoryapi.FactoryEvent, error)
	Close() error
}

// RemoteInvocationEventOperation opens the server-owned canonical event lane.
type RemoteInvocationEventOperation interface {
	OpenFactorySessionEvents(context.Context, RemoteInvocationEventRequest) (RemoteInvocationEventStream, error)
}

type remoteFactoryEventStream struct {
	reader *bufio.Reader
	body   io.ReadCloser
}

func (client remoteInvocationClient) OpenFactorySessionEvents(
	ctx context.Context,
	cfg RemoteInvocationEventRequest,
) (RemoteInvocationEventStream, error) {
	if ctx == nil {
		return nil, &InvocationError{Code: RemoteDurableResultCode, Message: "remote Factory Event stream: context is required"}
	}
	if client.transport == nil {
		return nil, &InvocationError{Code: RemoteDurableResultCode, Message: "remote Factory Event stream: CLI HTTP protocol is required"}
	}
	request, endpointURL, err := newRemoteFactoryEventRequest(ctx, cfg)
	if err != nil {
		return nil, err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"remote Factory Event stream open endpointPath=%s endpoint=%s sessionId=%s afterEventId=%s afterSequence=%s",
		request.URL.Path,
		request.URL.String(),
		strings.TrimSpace(cfg.SessionID),
		strings.TrimSpace(cfg.AfterEventID),
		formatRemoteEventSequence(cfg.AfterSequence),
	)
	response, err := client.transport.Execute(request)
	if err != nil {
		return nil, &remoteInvocationEventTransportError{
			status:  0,
			message: fmt.Sprintf("remote Factory Event stream failed at %s: %v", safeRemoteEndpoint(endpointURL), err),
			cause:   err,
		}
	}
	return openRemoteFactoryEventStreamResponse(response, endpointURL)
}

func newRemoteFactoryEventRequest(
	ctx context.Context,
	cfg RemoteInvocationEventRequest,
) (*http.Request, string, error) {
	endpointURL, err := cliserver.RequestURL(cfg.Server, sessionpath.FactoryEventsPath(cfg.SessionID))
	if err != nil {
		return nil, "", &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: fmt.Sprintf("remote Factory Event stream endpoint %q is invalid", safeRemoteEndpoint(cfg.Server)),
			Cause:   err,
		}
	}
	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return nil, "", &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: fmt.Sprintf("remote Factory Event stream endpoint %q is invalid", safeRemoteEndpoint(cfg.Server)),
			Cause:   err,
		}
	}
	query := parsed.Query()
	if eventID := strings.TrimSpace(cfg.AfterEventID); eventID != "" {
		query.Set("after_event_id", eventID)
	}
	if cfg.AfterSequence != nil {
		query.Set("after_sequence", strconv.Itoa(*cfg.AfterSequence))
	}
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", &InvocationError{Code: RemoteDurableResultCode, Message: fmt.Sprintf("build remote Factory Event stream request: %v", err), Cause: err}
	}
	request.Header.Set("Accept", "text/event-stream")
	return request, endpointURL, nil
}

func openRemoteFactoryEventStreamResponse(
	response clihttp.Response,
	endpointURL string,
) (RemoteInvocationEventStream, error) {
	if response.HTTP == nil {
		return nil, &remoteInvocationEventTransportError{
			status:  0,
			message: fmt.Sprintf("remote Factory Event stream failed at %s: HTTP response is unavailable", safeRemoteEndpoint(endpointURL)),
		}
	}
	if response.HTTP.StatusCode != http.StatusOK {
		if response.HTTP.Body != nil {
			defer response.HTTP.Body.Close()
		}
		message := fmt.Sprintf("remote Factory Event stream failed at %s (%d)", safeRemoteEndpoint(endpointURL), response.HTTP.StatusCode)
		if apiError, ok := clihttp.DecodeAPIError(response.HTTP); ok {
			message = fmt.Sprintf("remote Factory Event stream failed at %s (%d): %s", safeRemoteEndpoint(endpointURL), response.HTTP.StatusCode, apiError.Message)
		}
		return nil, &remoteInvocationEventTransportError{status: response.HTTP.StatusCode, message: message}
	}
	if response.HTTP.Body == nil {
		return nil, &remoteInvocationEventTransportError{
			status:  0,
			message: fmt.Sprintf("remote Factory Event stream failed at %s: HTTP response has no body", safeRemoteEndpoint(endpointURL)),
		}
	}
	if !strings.Contains(strings.ToLower(response.HTTP.Header.Get("Content-Type")), "text/event-stream") {
		defer response.HTTP.Body.Close()
		return nil, &remoteInvocationEventTransportError{
			status:  0,
			message: fmt.Sprintf("remote Factory Event stream at %s returned content type %q", safeRemoteEndpoint(endpointURL), response.HTTP.Header.Get("Content-Type")),
		}
	}
	return &remoteFactoryEventStream{reader: bufio.NewReader(response.HTTP.Body), body: response.HTTP.Body}, nil
}

func formatRemoteEventSequence(sequence *int) string {
	if sequence == nil {
		return ""
	}
	return strconv.Itoa(*sequence)
}

func (stream *remoteFactoryEventStream) Next(ctx context.Context) (factoryapi.FactoryEvent, error) {
	if stream == nil || stream.reader == nil {
		return factoryapi.FactoryEvent{}, errors.New("remote Factory Event stream is unavailable")
	}
	if ctx == nil {
		return factoryapi.FactoryEvent{}, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.FactoryEvent{}, err
	}
	return readRemoteFactoryEventSSE(stream.reader)
}

func (stream *remoteFactoryEventStream) Close() error {
	if stream == nil || stream.body == nil {
		return nil
	}
	return stream.body.Close()
}

func readRemoteFactoryEventSSE(reader *bufio.Reader) (factoryapi.FactoryEvent, error) {
	var data []string
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if err != nil && len(line) == 0 && len(data) == 0 {
			return factoryapi.FactoryEvent{}, err
		}
		if line == "" {
			if len(data) == 0 {
				if err != nil {
					return factoryapi.FactoryEvent{}, err
				}
				continue
			}
			return decodeRemoteFactoryEventSSE(data)
		}
		if strings.HasPrefix(line, ":") {
			if err != nil {
				return factoryapi.FactoryEvent{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			data = append(data, value)
		}
		if err != nil {
			if len(data) == 0 {
				return factoryapi.FactoryEvent{}, err
			}
			return decodeRemoteFactoryEventSSE(data)
		}
	}
}

func decodeRemoteFactoryEventSSE(data []string) (factoryapi.FactoryEvent, error) {
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &event); err != nil {
		return factoryapi.FactoryEvent{}, &remoteMalformedFactoryEventError{cause: err}
	}
	if strings.TrimSpace(event.Id) == "" || strings.TrimSpace(string(event.Type)) == "" {
		return factoryapi.FactoryEvent{}, &remoteMalformedFactoryEventError{cause: errors.New("canonical Factory Event has no id or type")}
	}
	return event, nil
}

type remoteInvocationEventTransportError struct {
	status  int
	message string
	cause   error
}

func (err *remoteInvocationEventTransportError) Error() string {
	if err == nil {
		return ""
	}
	return err.message
}

func (err *remoteInvocationEventTransportError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *remoteInvocationEventTransportError) retryable() bool {
	return err != nil && (err.status == 0 || err.status == http.StatusRequestTimeout || err.status == http.StatusTooManyRequests || err.status >= http.StatusInternalServerError)
}

type remoteMalformedFactoryEventError struct {
	cause error
}

func (err *remoteMalformedFactoryEventError) Error() string {
	if err == nil || err.cause == nil {
		return "decode canonical Factory Event SSE data"
	}
	return fmt.Sprintf("decode canonical Factory Event SSE data: %v", err.cause)
}

func (err *remoteMalformedFactoryEventError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func remoteFactoryEventCursor(event factoryapi.FactoryEvent) RemoteInvocationEventCursor {
	sequence := event.Context.Sequence
	if event.Context.SessionSequence != nil {
		sequence = *event.Context.SessionSequence
	}
	return RemoteInvocationEventCursor{EventID: strings.TrimSpace(event.Id), Sequence: &sequence}
}

func remoteFactoryEventRetryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
		return false
	}
	var malformed *remoteMalformedFactoryEventError
	if errors.As(err, &malformed) {
		return false
	}
	var transport *remoteInvocationEventTransportError
	if errors.As(err, &transport) {
		return transport.retryable()
	}
	return true
}

func waitForRemoteEventReconnect(ctx context.Context) error {
	return factorysessionscli.Wait(ctx, remoteFactoryEventReconnectInterval)
}

func runtimeLogDiagnosticsForRunner(runner factoryServiceRunner) runtimeartifact.Diagnostics {
	if provider, ok := runner.(runtimeLogDiagnosticsProvider); ok {
		return provider.RuntimeLogDiagnostics()
	}
	return runtimeartifact.Diagnostics{}
}

type cleanInvocationCounterSet struct {
	attempts          atomic.Int64
	successes         atomic.Int64
	failures          atomic.Int64
	ambiguityRejected atomic.Int64
	cancellations     atomic.Int64
}

type CleanInvocationMetricsSnapshot struct {
	Attempts          int64
	Successes         int64
	Failures          int64
	AmbiguityRejected int64
	Cancellations     int64
}

type cleanInvocationCompletionLogInput struct {
	Duration time.Duration
	Snapshot *interfaces.EngineStateSnapshot[state.PetriMarkingSnapshot, *state.Net]
	Target   *cleanInvocationWorkTarget
	Success  *cleanInvocationSuccess
	Err      error
}

var cleanInvocationMetrics cleanInvocationCounterSet

func recordCleanInvocationAttempt() {
	cleanInvocationMetrics.attempts.Add(1)
}

func ObserveInvocationRejection(logger *zap.Logger, err error) {
	var ambiguousErr *AmbiguousInvocationInputError
	if !errors.As(err, &ambiguousErr) {
		return
	}
	recordCleanInvocationAttempt()
	cleanInvocationMetrics.ambiguityRejected.Add(1)
	cleanInvocationLogger(logger).Info(
		cleanInvocationLogMessageRejected,
		zap.String("mode", cleanInvocationModeLabel),
		zap.String("reason", cleanInvocationRejectReason),
		zap.Strings("conflictingSources", invocationInputSourceLogLabels(ambiguousErr.Sources)),
	)
}

func recordCleanInvocationCompletion(logger *zap.Logger, cfg RunConfig, input cleanInvocationCompletionLogInput) {
	logger = cleanInvocationLogger(logger)
	fields := []zap.Field{
		zap.String("mode", cleanInvocationModeLabel),
		zap.String("inputSource", invocationInputSourceLogLabel(cfg.CleanInvocationInputSource)),
		zap.Int64("durationMs", input.Duration.Milliseconds()),
	}

	if input.Success != nil {
		cleanInvocationMetrics.successes.Add(1)
		fields = append(fields,
			zap.String("outcome", cleanInvocationOutcomeSuccess),
			zap.String("workId", input.Success.WorkID),
			zap.String("workTypeName", input.Success.WorkTypeName),
		)
		if strings.TrimSpace(input.Success.TraceID) != "" {
			fields = append(fields, zap.String("traceId", input.Success.TraceID))
		}
		if strings.TrimSpace(input.Success.SessionID) != "" {
			fields = append(fields, zap.String("sessionId", input.Success.SessionID))
		}
		logger.Info(cleanInvocationLogMessageCompleted, fields...)
		return
	}

	if input.Target != nil {
		fields = append(fields,
			zap.String("workId", input.Target.WorkID),
			zap.String("workTypeName", input.Target.WorkTypeName),
		)
	}

	outcome, code, summary := cleanInvocationFailureLogFields(input.Err)
	switch outcome {
	case cleanInvocationOutcomeCancelled:
		cleanInvocationMetrics.cancellations.Add(1)
	case cleanInvocationOutcomeFailure, cleanInvocationOutcomeTimeout:
		cleanInvocationMetrics.failures.Add(1)
	}
	fields = append(fields,
		zap.String("outcome", outcome),
		zap.String("errorCode", code),
	)
	if summary != "" {
		fields = append(fields, zap.String("errorSummary", summary))
	}
	logger.Info(cleanInvocationLogMessageCompleted, fields...)
}

func cleanInvocationFailureLogFields(err error) (string, string, string) {
	var invocationErr *InvocationError
	if errors.As(err, &invocationErr) {
		switch invocationErr.Code {
		case InvocationErrorCodeCancelled:
			return cleanInvocationOutcomeCancelled, invocationErr.Code, boundedInvocationErrorSummary(invocationErr.Message)
		case InvocationErrorCodeTimeout:
			return cleanInvocationOutcomeTimeout, invocationErr.Code, boundedInvocationErrorSummary(invocationErr.Message)
		default:
			return cleanInvocationOutcomeFailure, invocationErr.Code, boundedInvocationErrorSummary(invocationErr.Message)
		}
	}
	summary := boundedInvocationErrorSummary(errString(err))
	if summary == "" {
		return cleanInvocationOutcomeFailure, InvocationErrorCodeFailed, ""
	}
	return cleanInvocationOutcomeFailure, InvocationErrorCodeFailed, summary
}

func cleanInvocationLogger(logger *zap.Logger) *zap.Logger {
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

func invocationInputSourceLogLabels(sources []InvocationInputSource) []string {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		labels = append(labels, invocationInputSourceLogLabel(source))
	}
	return labels
}

func invocationInputSourceLogLabel(source InvocationInputSource) string {
	switch source {
	case InvocationInputSourcePositional:
		return "positional_prompt"
	case InvocationInputSourceStdin:
		return "stdin"
	case InvocationInputSourceWorkFile:
		return "work_file"
	default:
		return "unknown"
	}
}

func boundedInvocationErrorSummary(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if len(message) <= cleanInvocationErrorSummaryLimit {
		return message
	}
	return message[:cleanInvocationErrorSummaryLimit] + "..."
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func snapshotCleanInvocationMetrics() CleanInvocationMetricsSnapshot {
	return CleanInvocationMetricsSnapshot{
		Attempts:          cleanInvocationMetrics.attempts.Load(),
		Successes:         cleanInvocationMetrics.successes.Load(),
		Failures:          cleanInvocationMetrics.failures.Load(),
		AmbiguityRejected: cleanInvocationMetrics.ambiguityRejected.Load(),
		Cancellations:     cleanInvocationMetrics.cancellations.Load(),
	}
}

func resetCleanInvocationMetricsForTest() {
	cleanInvocationMetrics.attempts.Store(0)
	cleanInvocationMetrics.successes.Store(0)
	cleanInvocationMetrics.failures.Store(0)
	cleanInvocationMetrics.ambiguityRejected.Store(0)
	cleanInvocationMetrics.cancellations.Store(0)
}

func recordCLIInvocationResolved(cfg RunConfig, source work.InputSourceLabel) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	logger.Info("factory invocation input resolved", zap.String("input_source", string(source)))
}

func recordCLIInvocationFailure(cfg RunConfig, err error) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	inputErr, ok := err.(*work.InputError)
	if !ok {
		return
	}
	if inputErr.Code == work.InputErrorCodeSourceConflict {
		recordInvocationMetric(cfg.InvocationMetricsRecorder, factorysessions.InvocationMetric{
			Name: "invocation.source_conflict",
			Labels: map[string]string{
				"input_source": "conflict",
			},
		})
		recordInvocationMetric(cfg.InvocationMetricsRecorder, factorysessions.InvocationMetric{
			Name: "invocation.failure",
			Labels: map[string]string{
				"input_source": "conflict",
			},
		})
		logger.Warn(
			"factory invocation input resolution failed",
			zap.String("failure_class", "source_conflict"),
			zap.Strings("conflicting_sources", invocationSourceLabels(inputErr.ConflictingSources)),
			zap.String("error_code", string(inputErr.Code)),
		)
		return
	}
	logger.Warn(
		"factory invocation input resolution failed",
		zap.String("failure_class", "input_invalid"),
		zap.String("error_code", string(inputErr.Code)),
	)
}

func recordInvocationMetric(
	recorder runconfig.InvocationMetricsRecorder,
	metric factorysessions.InvocationMetric,
) {
	if recorder == nil {
		return
	}
	recorder.RecordInvocationMetric(metric)
}

func invocationSourceLabels(labels []work.InputSourceLabel) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		out = append(out, string(label))
	}
	return out
}

func isPackagedTTSRun(cfg RunConfig) bool {
	return strings.TrimSpace(cfg.NamedFactoryName) == interfaces.PackagedTTSFactoryName
}

func logPackagedTTSInvocationStart(cfg RunConfig) {
	if !isPackagedTTSRun(cfg) {
		return
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	fields := []zap.Field{
		zap.String("packaged_factory_name", interfaces.PackagedTTSFactoryName),
		zap.String("tts_backend", interfaces.DefaultTTSModelName+"/"+interfaces.DefaultTTSBackendName),
		zap.String("readiness_outcome", interfaces.TTSFailureClassLoading),
	}
	if resolution := cfg.NamedFactoryResolution; resolution != nil {
		fields = append(fields,
			zap.String("named_factory_resolution_source", string(resolution.Source)),
			zap.String("named_factory_dir", resolution.FactoryDir),
		)
	}
	logger.Info("packaged tts invocation started", fields...)
}
