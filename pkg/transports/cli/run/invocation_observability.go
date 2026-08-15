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
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionscli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/runconfig"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

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
	runServiceOperation        = "run.service"
	runServiceOutcomeSuccess   = "success"
	runServiceOutcomeCancelled = "cancelled"
	runServiceOutcomeFailure   = "failure"
	runServiceFailureNone      = "none"
	runServiceFailureBind      = "listener_bind"
	runServiceFailureStartup   = "runtime_startup_failed"
	runServiceFailureCoded     = "coded_failure"
	runServiceFailureRuntime   = "runtime_failure"
)

func logRunServiceOutcome(ctx context.Context, cfg RunConfig, err error) {
	if cfg.Logger == nil {
		return
	}
	outcome := runServiceOutcomeSuccess
	if errors.Is(err, context.Canceled) || (err == nil && ctx != nil && ctx.Err() != nil) {
		outcome = runServiceOutcomeCancelled
	} else if err != nil {
		outcome = runServiceOutcomeFailure
	}
	failureClass, errorCode := runServiceFailureFields(err)
	if outcome == runServiceOutcomeCancelled {
		failureClass, errorCode = runServiceFailureNone, ""
	}
	fields := []zap.Field{
		zap.String("operation", runServiceOperation),
		zap.String("outcome", outcome),
		zap.Bool("hosting_intent", cfg.WithServer || cfg.WithSite || cfg.Port > 0),
		zap.String("failure_class", failureClass),
	}
	if errorCode != "" {
		fields = append(fields, zap.String("error_code", errorCode))
	}
	if outcome == runServiceOutcomeFailure {
		cfg.Logger.Error("run service failed", fields...)
		return
	}
	cfg.Logger.Info("run service completed", fields...)
}

func runServiceFailureFields(err error) (string, string) {
	if err == nil {
		return runServiceFailureNone, ""
	}
	mapped := MapServerFailure(err)
	var invocationErr *InvocationError
	if errors.As(mapped, &invocationErr) {
		switch invocationErr.Code {
		case ServerBindFailedCode:
			return runServiceFailureBind, invocationErr.Code
		}
		var startupErr *initializer.RuntimeHostStartupError
		if errors.As(err, &startupErr) {
			return runServiceFailureStartup, invocationErr.Code
		}
		return runServiceFailureCoded, invocationErr.Code
	}
	var startupErr *initializer.RuntimeHostStartupError
	if errors.As(err, &startupErr) {
		return runServiceFailureStartup, ""
	}
	return runServiceFailureRuntime, ""
}

// MapServerFailure classifies local hosting failures at the CLI boundary while
// preserving all other errors for their owning mapper. Pre-readiness runtime
// exits receive the same operator-facing context as listener bind failures.
func MapServerFailure(err error) error {
	if err == nil {
		return nil
	}

	cause := err
	var startupErr *initializer.RuntimeHostStartupError
	if errors.As(err, &startupErr) {
		cause = errors.Unwrap(startupErr)
		if cause == nil {
			cause = initializer.ErrRuntimeHostExitedBeforeReadiness
		}
	}

	mapped := factoryruntimecli.MapServerFailure(cause)
	var mappedInvocationErr *InvocationError
	if errors.As(mapped, &mappedInvocationErr) && mappedInvocationErr.Code == ServerBindFailedCode {
		return withRequestedServerMessage(mapped, err)
	}
	if startupErr == nil {
		return mapped
	}

	if coded, ok := safeCLIError(cause); ok {
		return &InvocationError{
			Code:    coded.code,
			Message: fmt.Sprintf("requested server did not start: %s: %s", coded.code, coded.message),
			Cause:   err,
		}
	}
	return &InvocationError{
		Code:    ServerStartFailedCode,
		Message: "requested server did not start: runtime startup failed (failure_class=runtime_startup_failed)",
		Cause:   err,
	}
}

type safeCLIErrorFields struct {
	code    string
	message string
}

func safeCLIError(err error) (safeCLIErrorFields, bool) {
	if err == nil {
		return safeCLIErrorFields{}, false
	}
	var coded clidiag.CodedError
	if errors.As(err, &coded) {
		fields := safeCLIErrorFields{
			code:    strings.TrimSpace(coded.CLIErrorCode()),
			message: strings.TrimSpace(coded.CLIErrorMessage()),
		}
		if fields.code != "" && fields.message != "" {
			return fields, true
		}
	}
	var invocation clidiag.InvocationCodedError
	if errors.As(err, &invocation) {
		fields := safeCLIErrorFields{
			code:    strings.TrimSpace(invocation.InvocationErrorCode()),
			message: strings.TrimSpace(invocation.InvocationErrorMessage()),
		}
		if fields.code != "" && fields.message != "" {
			return fields, true
		}
	}
	return safeCLIErrorFields{}, false
}

func withRequestedServerMessage(err error, cause error) error {
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		return err
	}
	return &InvocationError{
		Code:    invocationErr.Code,
		Message: fmt.Sprintf("requested server did not start: %s", strings.TrimSpace(invocationErr.Message)),
		Cause:   cause,
	}
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
