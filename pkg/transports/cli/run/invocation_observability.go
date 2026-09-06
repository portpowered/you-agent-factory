package run

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionscli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
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
	ReplayOnly    bool
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
	reader            *bufio.Reader
	body              io.ReadCloser
	retainedRemaining *int
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
	return openRemoteFactoryEventStreamResponse(response, endpointURL, cfg.ReplayOnly)
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
	replayOnly bool,
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
	stream := &remoteFactoryEventStream{reader: bufio.NewReader(response.HTTP.Body), body: response.HTTP.Body}
	if replayOnly {
		retained, err := strconv.Atoi(strings.TrimSpace(response.HTTP.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader)))
		if err != nil || retained < 0 {
			response.HTTP.Body.Close()
			return nil, &remoteInvocationEventTransportError{
				status:  response.HTTP.StatusCode,
				message: fmt.Sprintf("remote Factory Event replay at %s returned invalid retained-event count", safeRemoteEndpoint(endpointURL)),
			}
		}
		stream.retainedRemaining = &retained
	}
	return stream, nil
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
	if stream.retainedRemaining != nil && *stream.retainedRemaining == 0 {
		return factoryapi.FactoryEvent{}, io.EOF
	}
	event, err := readRemoteFactoryEventSSE(stream.reader)
	if err != nil && stream.retainedRemaining != nil && *stream.retainedRemaining > 0 && errors.Is(err, io.EOF) {
		return factoryapi.FactoryEvent{}, &remoteFactoryEventReplayTruncatedError{
			remaining: *stream.retainedRemaining,
			cause:     err,
		}
	}
	if err == nil && stream.retainedRemaining != nil {
		(*stream.retainedRemaining)--
	}
	return event, err
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

type remoteFactoryEventReplayTruncatedError struct {
	remaining int
	cause     error
}

func (err *remoteFactoryEventReplayTruncatedError) Error() string {
	if err == nil {
		return "remote Factory Event replay ended before the retained event count"
	}
	return fmt.Sprintf(
		"remote Factory Event replay ended before the retained event count (%d remaining)",
		err.remaining,
	)
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

func emitVerboseStartupDiagnostics(cfg RunConfig, recordPath resolvedRunRecordPath, requestedPort int) {
	resolvedFactoryDir := resolveFactoryDirForDiagnostics(cfg.Dir, cfg.ResolveCurrentFactoryDir)
	diagnosticsEnabled := terminalpolicy.DiagnosticsEnabled(cfg.TerminalPolicy, cfg.Verbose)
	clidiag.Printf(
		cfg.Diagnostics,
		diagnosticsEnabled,
		"run startup factoryDir=%q configuredDir=%q runtimeMode=%s workflow=%q mockWorkers=%t mockWorkersConfigPath=%q recording=%s runtimeLogDir=%q runtimeLogRoll=%s runtimeMetricsDir=%q runtimeMetricsRoll=%s dashboardPort=%d requestedDashboardPort=%d autoPort=%s",
		resolvedFactoryDir,
		cfg.Dir,
		runtimeModeForRun(cfg),
		workflowLabel(cfg.Workflow),
		cfg.MockWorkersEnabled,
		cfg.MockWorkersConfigPath,
		recordingDiagnostics(cfg.RecordingsCLI, recordPath, cfg.ReplayPath),
		runtimeLogDirLabel(cfg.RuntimeLogDir),
		rollingPolicyDiagnostics(cfg.RuntimeLogConfig.MaxSize, cfg.RuntimeLogConfig.MaxBackups, cfg.RuntimeLogConfig.MaxAge, cfg.RuntimeLogConfig.Compress),
		runtimeMetricsDirLabel(cfg.RuntimeMetricsDir),
		rollingPolicyDiagnostics(cfg.RuntimeMetricsConfig.MaxSize, cfg.RuntimeMetricsConfig.MaxBackups, cfg.RuntimeMetricsConfig.MaxAge, cfg.RuntimeMetricsConfig.Compress),
		cfg.Port,
		requestedPort,
		autoPortDiagnostics(cfg.AutoPort, requestedPort, cfg.Port),
	)
	clidiag.Printf(cfg.Diagnostics, diagnosticsEnabled, "%s", cfg.OperatorDefaults.DiagnosticsLine())
}

func emitNamedFactoryResolutionDiagnostics(cfg RunConfig, logger *zap.Logger) {
	resolution := cfg.NamedFactoryResolution
	if resolution == nil {
		return
	}

	clidiag.Printf(
		cfg.Diagnostics,
		terminalpolicy.DiagnosticsEnabled(cfg.TerminalPolicy, cfg.Verbose),
		"run named-factory resolution name=%q source=%s resolvedFactoryDir=%q projectRoot=%q globalRoot=%q precedence=%s",
		resolution.Name,
		resolution.Source,
		resolution.FactoryDir,
		resolution.ProjectRoot,
		resolution.GlobalRoot,
		resolution.PrecedenceDecision,
	)
	logger.Info(
		"named factory resolved",
		zap.String("named_factory_name", resolution.Name),
		zap.String("named_factory_resolution_source", string(resolution.Source)),
		zap.String("named_factory_dir", resolution.FactoryDir),
		zap.String("named_factory_project_root", resolution.ProjectRoot),
		zap.String("named_factory_global_root", resolution.GlobalRoot),
		zap.String("named_factory_precedence_decision", string(resolution.PrecedenceDecision)),
	)
	if resolution.PrecedenceDecision == interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal {
		logger.Info(
			"named factory precedence selected",
			zap.String("named_factory_name", resolution.Name),
			zap.String("named_factory_precedence_decision", string(resolution.PrecedenceDecision)),
			zap.String("named_factory_resolution_source", string(resolution.Source)),
		)
	}
}

func resolveFactoryDirForDiagnostics(
	dir string,
	resolve interfaces.CurrentFactoryDirectoryResolver,
) string {
	if resolve == nil {
		return "unresolved"
	}
	resolved, err := resolve(dir)
	if err != nil {
		return "unresolved"
	}
	return resolved
}

func workflowLabel(workflow string) string {
	if strings.TrimSpace(workflow) == "" {
		return "all"
	}
	return workflow
}

func runtimeLogDirLabel(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return "default"
	}
	return dir
}

func runtimeMetricsDirLabel(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return "default"
	}
	return dir
}

func rollingPolicyDiagnostics(maxSize, maxBackups, maxAge int, compress bool) string {
	return fmt.Sprintf("size_mb=%d backups=%d age_days=%d compress=%t", maxSize, maxBackups, maxAge, compress)
}

func recordingDiagnostics(
	adapter recordingscli.Adapter,
	recordPath resolvedRunRecordPath,
	replayPath string,
) string {
	if adapter == nil {
		return "disabled"
	}
	return adapter.RecordingDiagnosticsLabel(recordingscli.ResolvedRecordPath{
		ServicePath:   recordPath.servicePath,
		ReportedPath:  recordPath.reportedPath,
		AutoGenerated: recordPath.autoGenerated,
	}, replayPath)
}

func autoPortDiagnostics(autoPort bool, requestedPort, resolvedPort int) string {
	switch {
	case requestedPort <= 0:
		return "dashboard-disabled"
	case !autoPort:
		return "disabled"
	case requestedPort == resolvedPort:
		return "preferred-available"
	default:
		return "fallback"
	}
}

func bindDashboardHost(cfg RunConfig) string {
	if strings.TrimSpace(cfg.BindHost) != "" {
		return cfg.BindHost
	}
	return "localhost"
}

// DashboardURL returns the embedded browser dashboard URL for the configured
// local factory server host and port.
func DashboardURL(host string, port int) string {
	if port <= 0 {
		return ""
	}
	if strings.TrimSpace(host) == "" {
		host = "localhost"
	}
	authority := net.JoinHostPort(host, strconv.Itoa(port))
	return "http://" + authority + "/dashboard/ui"
}

func shouldOpenDashboard(cfg RunConfig) bool {
	return cfg.OpenDashboard
}

func reportRecordingPathOnShutdown(
	output io.Writer,
	recordPath resolvedRunRecordPath,
	adapter recordingscli.Adapter,
) {
	if adapter == nil {
		return
	}
	adapter.ReportRecordingPathOnShutdown(output, recordingscli.ResolvedRecordPath{
		ServicePath:   recordPath.servicePath,
		ReportedPath:  recordPath.reportedPath,
		AutoGenerated: recordPath.autoGenerated,
	})
}

func openDashboardAtBoundEndpoint(
	ctx context.Context,
	cfg RunConfig,
	openDashboard func(context.Context, string) error,
) {
	url := DashboardURL(bindDashboardHost(cfg), cfg.Port)
	if openDashboard == nil {
		if cfg.StartupOutput != nil {
			fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open unavailable: browser opener is required\nOpen the dashboard at %s\n", url)
		}
		return
	}
	if err := openDashboard(ctx, url); err != nil {
		if cfg.StartupOutput != nil {
			fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open unavailable: %v\nOpen the dashboard at %s\n", err, url)
		}
		return
	}
	if cfg.StartupOutput != nil {
		fmt.Fprintf(cfg.StartupOutput, "Opening dashboard: %s\n", url)
	}
}
