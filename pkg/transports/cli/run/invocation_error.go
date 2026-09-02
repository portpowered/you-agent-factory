package run

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factoryconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

const (
	InvocationErrorCodeFailed          = factoryruntimecli.InvocationErrorCodeFailed
	InvocationErrorCodeCancelled       = factoryruntimecli.InvocationErrorCodeCancelled
	InvocationErrorCodeTimeout         = factoryruntimecli.InvocationErrorCodeTimeout
	InvocationArgumentMissingValueCode = factoryruntimecli.InvocationArgumentMissingValueCode
	InvocationArgumentInvalidValueCode = factoryruntimecli.InvocationArgumentInvalidValueCode
	CurrentFactoryNotFoundCode         = factoryruntimecli.CurrentFactoryNotFoundCode
	CurrentFactoryInvalidCode          = factoryruntimecli.CurrentFactoryInvalidCode
	InvocationOutputConflictCode       = factoryruntimecli.InvocationOutputConflictCode
	InvocationOutputUnsupportedCode    = factoryruntimecli.InvocationOutputUnsupportedCode
	RemoteLocalHostingConflictCode     = factoryruntimecli.RemoteLocalHostingConflictCode
	ServerBindFailedCode               = factoryruntimecli.ServerBindFailedCode
	ServerStartFailedCode              = factoryruntimecli.ServerStartFailedCode
	InvocationOutputPrimaryResult      = factoryruntimecli.InvocationOutputPrimaryResult
	InvocationOutputResponseStream     = factoryruntimecli.InvocationOutputResponseStream
)

type InvocationError = factoryruntimecli.InvocationError

// WriteInvocationError renders the stable clean-invocation failure contract to
// stderr. It returns true when err matched an invocation contract error.
func WriteInvocationError(w io.Writer, err error, quiet bool) bool {
	handled := factoryruntimecli.WriteInvocationError(w, err, quiet)
	if handled {
		clidiag.MarkDiagnosticRendered(w)
	}
	return handled
}

// WriteIncompleteDrainError renders the finite-run failure contract for a
// drained runtime that still owns non-terminal customer Work. This is kept at
// the human CLI boundary so the runtime error remains useful to other callers.
func WriteIncompleteDrainError(w io.Writer, err error) bool {
	var incompleteDrainErr *factoryruntime.IncompleteDrainError
	if !errors.As(err, &incompleteDrainErr) {
		return false
	}
	if w != nil {
		_, _ = fmt.Fprintf(w, "Error: %s\n", incompleteDrainErr.Error())
	}
	return true
}

// MapCurrentFactoryFailure classifies failures from the exact Current Factory
// selection before they cross the public run-command error boundary.
func MapCurrentFactoryFailure(err error) error {
	return factoryruntimecli.MapCurrentFactoryFailure(err)
}

// MapInvocationFailure preserves authored invocation errors and classifies
// pre-terminal failures that occurred before an InvocationResponse existed.
func MapInvocationFailure(err error) error {
	return factoryruntimecli.MapInvocationFailure(err)
}

func NormalizeInvocationOutputMode(raw string) (string, error) {
	return factoryruntimecli.NormalizeInvocationOutputMode(raw)
}

// ValidateInvocationOutputSelection rejects competing public stdout selectors.
// JSON plus response-stream is one accepted JSON-stream selection; quiet cannot
// be combined with either global JSON or an explicit --output selection.
func ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput bool) error {
	return factoryruntimecli.ValidateInvocationOutputSelection(quiet, jsonOutput, explicitOutput)
}

func validateInvocationOutputMode(cfg RunConfig, invocationMode bool) error {
	return factoryruntimecli.ValidateInvocationOutputMode(factoryruntimecli.ValidateInvocationOutputModeRequest{
		InvocationOutputMode: cfg.InvocationOutputMode,
		Continuously:         cfg.Continuously,
		InvocationMode:       invocationMode,
	})
}

func isResponseStreamOutputMode(mode string) bool {
	return strings.TrimSpace(mode) == InvocationOutputResponseStream
}

func invocationResultFailure(result apisurface.FactoryInvocationResult) error {
	return invocationCLIError{
		Code:      strings.TrimSpace(result.ErrorCode),
		Message:   strings.TrimSpace(result.Message),
		SessionID: strings.TrimSpace(result.SessionID),
		WorkID:    strings.TrimSpace(result.WorkID),
		WorkName:  strings.TrimSpace(result.WorkName),
		WorkState: strings.TrimSpace(result.WorkState),
	}
}

func writeInvocationFailure(
	cfg RunConfig,
	result apisurface.FactoryInvocationResult,
	streamRenderer visualizationcli.FactoryEventRenderer,
) error {
	if streamRenderer != nil {
		if err := streamRenderer.WriteFinalInvocationResult(result); err != nil {
			return err
		}
	} else if cfg.JSONOutput {
		if err := writeInvocationJSON(cfg, result); err != nil {
			return err
		}
	}
	return invocationResultFailure(result)
}

func writeInvocationSuccess(
	cfg RunConfig,
	result apisurface.FactoryInvocationResult,
	streamRenderer visualizationcli.FactoryEventRenderer,
) error {
	if streamRenderer != nil {
		return streamRenderer.WriteFinalInvocationResult(result)
	}
	if cfg.JSONOutput {
		return writeInvocationJSON(cfg, result)
	}

	text, err := invocationPrimaryResultText(result.PrimaryResult)
	if err != nil {
		return err
	}
	output := cfg.Output
	if output == nil {
		return fmt.Errorf("write invocation result: process output is required")
	}
	_, err = fmt.Fprint(output, text)
	return err
}

func writeFactoryInvocationOutcome(
	cfg RunConfig,
	result apisurface.FactoryInvocationResult,
	streamRenderer visualizationcli.FactoryEventRenderer,
) error {
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return writeInvocationFailure(cfg, result, streamRenderer)
	}
	return writeInvocationSuccess(cfg, result, streamRenderer)
}

func finishFactoryInvocation(
	runErr error,
	writeErr error,
	outputWriter *responseStreamCancelOnWriteError,
	result apisurface.FactoryInvocationResult,
) error {
	if outputWriter != nil {
		if recordedErr := outputWriter.Err(); recordedErr != nil {
			// The writer failure caused cancellation; preserve that established
			// root cause instead of allowing the derived cancellation or a later
			// cleanup error to win classification.
			return recordedErr
		}
	}
	if result.Status != "" && isContextDerivedInvocationFailure(runErr) {
		// The invocation result is the authoritative terminal outcome. A
		// cancellation or timeout returned alongside it can be the caller's
		// context reaching the post-result cleanup path; mapping that context
		// first would replace INVOCATION_CANCELED (or another canonical result
		// code) with the generic RUN_* wrapper.
		return writeErr
	}
	if runErr != nil {
		if writeErr != nil {
			return errors.Join(MapInvocationFailure(runErr), writeErr)
		}
		return MapInvocationFailure(runErr)
	}
	return writeErr
}

func isContextDerivedInvocationFailure(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return true
	}
	var invocationErr *InvocationError
	if errors.As(err, &invocationErr) {
		return invocationErr.Code == InvocationErrorCodeCancelled ||
			invocationErr.Code == InvocationErrorCodeTimeout
	}
	var cliErr factoryruntimecli.InvocationCLIError
	if errors.As(err, &cliErr) {
		return cliErr.InvocationErrorCode() == InvocationErrorCodeCancelled ||
			cliErr.InvocationErrorCode() == InvocationErrorCodeTimeout
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !isContextDerivedInvocationFailure(cause) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return isContextDerivedInvocationFailure(wrapped.Unwrap())
	}
	return false
}

func writeInvocationJSON(cfg RunConfig, result apisurface.FactoryInvocationResult) error {
	output := cfg.Output
	if output == nil {
		return fmt.Errorf("write invocation JSON: process output is required")
	}
	encoded, err := json.Marshal(apisurface.InvocationResponseFromResult(result))
	if err != nil {
		return fmt.Errorf("marshal invocation response: %w", err)
	}
	_, err = fmt.Fprintln(output, string(encoded))
	return err
}

func invocationPrimaryResultText(parts []work.WorkContentPart) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("invocation primary result is empty")
	}

	textParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type.Normalized() != work.WorkContentPartTypeText {
			return "", fmt.Errorf("invocation primary result is not plain text; use --json")
		}
		textParts = append(textParts, part.Text)
	}
	return strings.Join(textParts, "\n"), nil
}

const (
	// RemoteInvocationInputRequiredCode is returned before any remote request
	// when run was selected without an invocation payload.
	RemoteInvocationInputRequiredCode = "REMOTE_INVOCATION_INPUT_REQUIRED"
	// RemoteDurableRequestInvalidCode is returned before any remote request when
	// the local run input cannot be represented by the durable start contract.
	RemoteDurableRequestInvalidCode = "REMOTE_DURABLE_REQUEST_INVALID"
	// RemoteDurableStartCode classifies transport and admission failures from
	// the selected server without allowing the root to fall back locally.
	RemoteDurableStartCode = "REMOTE_DURABLE_START_FAILED"
	// RemoteDurableResponseInvalidCode classifies a successful HTTP response
	// that does not contain a canonical durable-session identity.
	RemoteDurableResponseInvalidCode = "REMOTE_DURABLE_RESPONSE_INVALID"
	// RemoteDurableResultCode classifies transport and retrieval failures after
	// a durable Factory Session has been accepted by the selected server.
	RemoteDurableResultCode         = "REMOTE_DURABLE_RESULT_FAILED"
	remoteDurableStartPath          = "/factory-sessions/async"
	remoteDurableResultPath         = "/factory-sessions/%s/results"
	remoteDurableResultPollInterval = 50 * time.Millisecond
	invalidRemoteEndpointLabel      = "<invalid endpoint>"
)

// RemoteInvocationRequest is the normalized CLI transport request sent to a
// selected You server. Placement is resolved by the caller; this type carries
// only the endpoint and the already-normalized operation request.
type RemoteInvocationRequest struct {
	Server      string
	Request     factoryapi.FactorySessionExecutionRequest
	Diagnostics io.Writer
	Verbose     bool
}

// RemoteInvocationOperation is the injected HTTP adapter for one remote
// durable Factory Session start. It deliberately does not expose a local
// service dependency.
type RemoteInvocationOperation interface {
	StartFactorySession(context.Context, RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error)
}

// RemoteExistingSessionInvocationRequest targets one already-open Factory
// Session through the public compatibility invocation route.
type RemoteExistingSessionInvocationRequest struct {
	Server      string
	SessionID   string
	Request     factoryapi.InvocationRequest
	Diagnostics io.Writer
	Verbose     bool
}

// RemoteExistingSessionInvocationOperation is implemented by production HTTP
// adapters that can invoke an explicitly selected live Factory Session.
type RemoteExistingSessionInvocationOperation interface {
	InvokeFactorySession(context.Context, RemoteExistingSessionInvocationRequest) (factoryapi.InvocationResponse, error)
}

// RemoteInvocationResultRequest identifies the server-owned durable result
// read that follows one accepted remote invocation.
type RemoteInvocationResultRequest struct {
	Server      string
	SessionID   string
	Diagnostics io.Writer
	Verbose     bool
}

// RemoteInvocationResultOperation is the optional result-read half of the
// remote durable invocation adapter. Keeping it separate preserves the small
// start seam used by placement tests while real HTTP adapters expose the full
// start-and-result lifecycle.
type RemoteInvocationResultOperation interface {
	GetFactorySessionResult(context.Context, RemoteInvocationResultRequest) (factoryapi.FactorySessionResult, error)
}

type remoteInvocationClient struct {
	transport clihttp.Protocol
}

// NewRemoteInvocation binds the CLI HTTP protocol to the remote durable start
// operation. The protocol is the only external effect owned by this adapter.
func NewRemoteInvocation(transport clihttp.Protocol) RemoteInvocationOperation {
	return remoteInvocationClient{transport: transport}
}

func (client remoteInvocationClient) StartFactorySession(
	ctx context.Context,
	cfg RemoteInvocationRequest,
) (factoryapi.FactorySessionExecutionResponse, error) {
	if ctx == nil {
		return factoryapi.FactorySessionExecutionResponse{}, fmt.Errorf("remote durable start: context is required")
	}
	if client.transport == nil {
		return factoryapi.FactorySessionExecutionResponse{}, fmt.Errorf("remote durable start: CLI HTTP protocol is required")
	}
	endpointURL, err := remoteDurableStartURL(cfg.Server)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	body, err := json.Marshal(cfg.Request)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, &InvocationError{
			Code:    RemoteDurableRequestInvalidCode,
			Message: fmt.Sprintf("marshal remote durable start request: %v", err),
			Cause:   err,
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(body))
	if err != nil {

		return factoryapi.FactorySessionExecutionResponse{}, &InvocationError{
			Code: RemoteDurableRequestInvalidCode,

			Message: fmt.Sprintf("build remote durable start request: %v", err),
			Cause:   err,
		}
	}
	request.Header.Set("Content-Type", "application/json")
	endpointLabel := safeRemoteEndpoint(endpointURL)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"remote durable start request endpointPath=%s endpoint=%s requestId=%s requestBytes=%d",
		remoteDurableStartPath,
		endpointLabel,
		cfg.Request.RequestId,
		len(body),
	)

	response, err := client.transport.Execute(request)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, remoteDurableTransportError(
			cfg, endpointLabel, response.Duration.Milliseconds(), err,
		)
	}
	return decodeRemoteDurableStartResponse(cfg, endpointLabel, response)
}

func (client remoteInvocationClient) InvokeFactorySession(
	ctx context.Context,
	cfg RemoteExistingSessionInvocationRequest,
) (factoryapi.InvocationResponse, error) {
	if ctx == nil {
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableRequestInvalidCode, Message: "remote Factory Session invocation: context is required"}
	}
	if client.transport == nil {
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableStartCode, Message: "remote Factory Session invocation: CLI HTTP protocol is required"}
	}
	endpointURL, err := cliserver.RequestURL(cfg.Server, sessionpath.FactoryInvocationsPath(cfg.SessionID))
	if err != nil {
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableRequestInvalidCode, Message: "remote Factory Session invocation endpoint is invalid", Cause: err}
	}
	body, err := json.Marshal(cfg.Request)
	if err != nil {
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableRequestInvalidCode, Message: fmt.Sprintf("marshal remote Factory Session invocation: %v", err), Cause: err}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(body))
	if err != nil {
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableRequestInvalidCode, Message: fmt.Sprintf("build remote Factory Session invocation: %v", err), Cause: err}
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.transport.Execute(request)
	if err != nil {
		if result, ok := remoteExistingSessionContextResult(err, cfg.SessionID, cfg.Request); ok {
			return apisurface.InvocationResponseFromResult(result), nil
		}
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableStartCode, Message: fmt.Sprintf("remote Factory Session invocation failed at %s: %v", safeRemoteEndpoint(endpointURL), err), Cause: err}
	}
	if response.HTTP == nil {
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableResponseInvalidCode, Message: "remote Factory Session invocation returned no HTTP response"}
	}
	if response.HTTP.Body != nil {
		defer response.HTTP.Body.Close()
	}
	if response.HTTP.StatusCode != http.StatusOK {
		message := fmt.Sprintf("remote Factory Session invocation failed at %s (%d)", safeRemoteEndpoint(endpointURL), response.HTTP.StatusCode)
		if apiError, ok := clihttp.DecodeAPIError(response.HTTP); ok {
			message += ": " + apiError.Message
		}
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableStartCode, Message: message}
	}
	if response.HTTP.Body == nil {
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableResponseInvalidCode, Message: "remote Factory Session invocation response has no body"}
	}
	var result factoryapi.InvocationResponse
	if err := json.NewDecoder(response.HTTP.Body).Decode(&result); err != nil {
		return factoryapi.InvocationResponse{}, &InvocationError{Code: RemoteDurableResponseInvalidCode, Message: fmt.Sprintf("decode remote Factory Session invocation response: %v", err), Cause: err}
	}
	return result, nil
}

func remoteExistingSessionContextResult(
	err error,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, bool) {
	requestID := ""
	if request.RequestId != nil {
		requestID = *request.RequestId
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return remoteDurableInvocationFailure(
			requestID,
			sessionID,
			interfaces.InvocationTerminalStatusTimedOut,
			string(factoryapi.INVOCATIONTIMEDOUT),
			"invocation timed out while waiting for primary result",
			nil,
		), true
	case errors.Is(err, context.Canceled):
		return remoteDurableInvocationFailure(
			requestID,
			sessionID,
			interfaces.InvocationTerminalStatusCanceled,
			string(factoryapi.INVOCATIONCANCELED),
			"invocation was canceled while waiting for primary result",
			nil,
		), true
	default:
		return apisurface.FactoryInvocationResult{}, false
	}
}

func (client remoteInvocationClient) GetFactorySessionResult(
	ctx context.Context,
	cfg RemoteInvocationResultRequest,
) (factoryapi.FactorySessionResult, error) {
	if ctx == nil {
		return factoryapi.FactorySessionResult{}, &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: "remote durable result: context is required",
		}
	}
	if client.transport == nil {
		return factoryapi.FactorySessionResult{}, &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: "remote durable result: CLI HTTP protocol is required",
		}
	}
	endpointURL, err := remoteDurableResultURL(cfg.Server, cfg.SessionID)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	endpointLabel := safeRemoteEndpoint(endpointURL)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"remote durable result request endpointPath=/factory-sessions/{session_id}/results endpoint=%s sessionId=%s",
		endpointLabel,
		strings.TrimSpace(cfg.SessionID),
	)
	var result factoryapi.FactorySessionResult
	response, err := client.transport.GetJSON(ctx, endpointURL, &result)
	if err != nil {
		return factoryapi.FactorySessionResult{}, remoteDurableResultTransportError(
			cfg, endpointLabel, response.Duration.Milliseconds(), err,
		)
	}
	if response.HTTP == nil {
		return factoryapi.FactorySessionResult{}, &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: fmt.Sprintf("remote durable result failed at %s: HTTP response is unavailable", endpointLabel),
		}
	}
	if response.HTTP.Body != nil {
		defer response.HTTP.Body.Close()
	}
	if response.HTTP.StatusCode != http.StatusOK {
		return remoteDurableResultHTTPError(endpointLabel, response, response.HTTP.StatusCode)
	}
	if strings.TrimSpace(result.SessionId) == "" || strings.TrimSpace(string(result.ResultStatus)) == "" {
		return factoryapi.FactorySessionResult{}, &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: fmt.Sprintf("remote durable result response at %s has no canonical session result identity", endpointLabel),
		}
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"remote durable result response endpointPath=/factory-sessions/{session_id}/results status=%d durationMillis=%d sessionId=%s resultStatus=%s sessionStatus=%s",
		response.HTTP.StatusCode,
		response.Duration.Milliseconds(),
		result.SessionId,
		result.ResultStatus,
		remoteDurableLifecycleStatus(result.SessionStatus),
	)
	return result, nil
}

func remoteDurableResultURL(server, sessionID string) (string, error) {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return "", &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: "remote durable result requires a canonical session identity",
		}
	}
	path := fmt.Sprintf(remoteDurableResultPath, url.PathEscape(trimmedSessionID))
	endpointURL, err := cliserver.RequestURL(server, path)
	if err != nil {
		return "", &InvocationError{
			Code: RemoteDurableResultCode,
			Message: fmt.Sprintf(
				"remote durable result endpoint %q is invalid",
				safeRemoteEndpoint(server),
			),
			Cause: err,
		}
	}
	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return "", &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: fmt.Sprintf("remote durable result endpoint %q is invalid", safeRemoteEndpoint(server)),
			Cause:   err,
		}
	}
	query := parsed.Query()
	query.Set("mode", "final")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func remoteDurableResultTransportError(
	cfg RemoteInvocationResultRequest,
	endpointLabel string,
	durationMillis int64,
	err error,
) error {
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"remote durable result response endpointPath=/factory-sessions/{session_id}/results error=unreachable durationMillis=%d",
		durationMillis,
	)
	return &InvocationError{
		Code: RemoteDurableResultCode,
		Message: fmt.Sprintf(
			"remote durable result failed at %s: %v",
			endpointLabel,
			err,
		),
		Cause: err,
	}
}

func remoteDurableResultHTTPError(
	endpointLabel string,
	response clihttp.Response,
	status int,
) (factoryapi.FactorySessionResult, error) {
	if response.HTTP.Body != nil {
		if apiError, ok := clihttp.DecodeAPIError(response.HTTP); ok {
			return factoryapi.FactorySessionResult{}, &InvocationError{
				Code: RemoteDurableResultCode,
				Message: fmt.Sprintf(
					"remote durable result failed at %s (%d): %s",
					endpointLabel,
					status,
					apiError.Message,
				),
			}
		}
	}
	return factoryapi.FactorySessionResult{}, &InvocationError{
		Code:    RemoteDurableResultCode,
		Message: fmt.Sprintf("remote durable result failed at %s (%d)", endpointLabel, status),
	}
}

func remoteDurableStartURL(server string) (string, error) {
	endpointURL, err := cliserver.RequestURL(server, remoteDurableStartPath)
	if err == nil {
		return endpointURL, nil
	}
	return "", &InvocationError{
		Code: RemoteDurableStartCode,
		Message: fmt.Sprintf(
			"remote durable start endpoint %q is invalid",
			safeRemoteEndpoint(server),
		),
		Cause: err,
	}
}

func remoteDurableTransportError(
	cfg RemoteInvocationRequest,
	endpointLabel string,
	durationMillis int64,
	err error,
) error {
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"remote durable start response endpointPath=%s error=unreachable durationMillis=%d",
		remoteDurableStartPath,

		durationMillis,
	)
	return &InvocationError{
		Code: RemoteDurableStartCode,
		Message: fmt.Sprintf(
			"remote durable start failed at %s: %v",
			endpointLabel,
			err,
		),
		Cause: err,
	}
}

func decodeRemoteDurableStartResponse(
	cfg RemoteInvocationRequest,
	endpointLabel string,
	response clihttp.Response,
) (factoryapi.FactorySessionExecutionResponse, error) {
	if response.HTTP == nil {
		return factoryapi.FactorySessionExecutionResponse{}, &InvocationError{
			Code:    RemoteDurableStartCode,
			Message: fmt.Sprintf("remote durable start failed at %s: HTTP response is unavailable", endpointLabel),
		}
	}
	if response.HTTP.Body != nil {
		defer response.HTTP.Body.Close()
	}
	status := response.HTTP.StatusCode
	if status != http.StatusAccepted && status != http.StatusOK {
		return remoteDurableHTTPError(endpointLabel, response, status)
	}
	if response.HTTP.Body == nil {
		return factoryapi.FactorySessionExecutionResponse{}, &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: fmt.Sprintf("remote durable start response at %s has no body", endpointLabel),
		}
	}
	var result factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(response.HTTP.Body).Decode(&result); err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: fmt.Sprintf("decode remote durable start response at %s: %v", endpointLabel, err),
			Cause:   err,
		}
	}
	if strings.TrimSpace(result.SessionId) == "" || strings.TrimSpace(string(result.Status)) == "" {
		return factoryapi.FactorySessionExecutionResponse{}, &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: fmt.Sprintf("remote durable start response at %s has no canonical session identity", endpointLabel),
		}
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"remote durable start response endpointPath=%s status=%d durationMillis=%d sessionId=%s requestId=%s",
		remoteDurableStartPath,
		status,
		response.Duration.Milliseconds(),
		result.SessionId,
		cfg.Request.RequestId,
	)
	return result, nil
}

func remoteDurableHTTPError(
	endpointLabel string,
	response clihttp.Response,
	status int,
) (factoryapi.FactorySessionExecutionResponse, error) {
	if response.HTTP.Body != nil {
		if apiError, ok := clihttp.DecodeAPIError(response.HTTP); ok {
			return factoryapi.FactorySessionExecutionResponse{}, &InvocationError{
				Code: RemoteDurableStartCode,
				Message: fmt.Sprintf(
					"remote durable start failed at %s (%d): %s",
					endpointLabel,
					status,
					apiError.Message,
				),
			}
		}
	}
	return factoryapi.FactorySessionExecutionResponse{}, &InvocationError{
		Code:    RemoteDurableStartCode,
		Message: fmt.Sprintf("remote durable start failed at %s (%d)", endpointLabel, status),
	}
}

func remoteRequestID(request factoryapi.FactorySessionExecutionRequest) string {
	request.RequestId = ""
	encoded, err := json.Marshal(request)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("you-run-%x", digest[:])
}

func remoteDurableRequestFromRunConfig(
	cfg RunConfig,
	invocation factoryapi.InvocationRequest,
) (factoryapi.FactorySessionExecutionRequest, error) {
	if invocation.Content != nil && invocation.Args == nil {
		return factoryapi.FactorySessionExecutionRequest{}, &InvocationError{
			Code:    RemoteDurableRequestInvalidCode,
			Message: "remote durable run requires normalized invocation arguments; compatibility content input is not supported",
		}
	}

	source, policy, err := remoteDurableSourceFromRunConfig(cfg)
	if err != nil {
		return factoryapi.FactorySessionExecutionRequest{}, err
	}
	args := map[string]any{}
	if invocation.Args != nil {
		for name, value := range *invocation.Args {
			args[name] = value
		}
	}
	request := factoryapi.FactorySessionExecutionRequest{
		Source:          source,
		Args:            &args,
		RequestedPolicy: policy,
	}
	if invocation.RequestId != nil {
		request.RequestId = strings.TrimSpace(*invocation.RequestId)
	}
	if invocation.TimeoutMillis != nil {
		request.Wait = &factoryapi.FactorySessionExecutionWaitOptions{
			TimeoutMillis: invocation.TimeoutMillis,
		}
	}
	if request.RequestId == "" {
		request.RequestId = remoteRequestID(request)
	}
	if request.RequestId == "" {
		return factoryapi.FactorySessionExecutionRequest{}, &InvocationError{
			Code:    RemoteDurableRequestInvalidCode,
			Message: "remote durable run could not derive a stable request identity",
		}
	}
	return request, nil
}

func remoteDurableSourceFromRunConfig(
	cfg RunConfig,
) (factoryapi.FactorySessionExecutionSource, *factoryapi.FactorySessionRequestedPolicy, error) {
	factoryName := strings.TrimSpace(cfg.NamedFactoryName)

	if factoryName == "" && cfg.NamedFactoryResolution != nil {
		factoryName = strings.TrimSpace(cfg.NamedFactoryResolution.Name)
	}
	if factoryName != "" {
		return remoteNamedFactorySource(cfg, factoryName)
	}
	if workflowName := strings.TrimSpace(cfg.Workflow); workflowName != "" {
		return factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &workflowName,
		}, nil, nil
	}

	configPath, err := remoteFactoryConfigPath(cfg)
	if err != nil {
		return factoryapi.FactorySessionExecutionSource{}, nil, err
	}
	if remoteRunFactorySourceUsesJavaScript(configPath) {
		return factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &configPath,
		}, nil, nil
	}
	return remoteInlineFactorySource(cfg, configPath)
}

func remoteNamedFactorySource(
	cfg RunConfig,
	factoryName string,
) (factoryapi.FactorySessionExecutionSource, *factoryapi.FactorySessionRequestedPolicy, error) {
	var policy *factoryapi.FactorySessionRequestedPolicy
	if cfg.LoadFactoryConfigFile != nil && strings.TrimSpace(cfg.Dir) != "" {
		definition, err := cfg.LoadFactoryConfigFile(filepath.Join(cfg.Dir, interfaces.FactoryConfigFile))
		if err != nil {
			return factoryapi.FactorySessionExecutionSource{}, nil, &InvocationError{
				Code:    RemoteDurableRequestInvalidCode,
				Message: fmt.Sprintf("load remote named Factory policy %q: %v", factoryName, err),
				Cause:   err,
			}
		}
		policy = remoteRequestedPolicy(definition)
	}
	return factoryapi.FactorySessionExecutionSource{
		Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
		FactoryId: &factoryName,
	}, policy, nil
}

func remoteFactoryConfigPath(cfg RunConfig) (string, error) {
	if configPath := strings.TrimSpace(cfg.FactoryConfigPath); configPath != "" {
		return configPath, nil
	}

	if factoryRoot := strings.TrimSpace(cfg.Dir); factoryRoot != "" {
		return filepath.Join(factoryRoot, interfaces.FactoryConfigFile), nil
	}
	return "", &InvocationError{
		Code:    RemoteDurableRequestInvalidCode,
		Message: "remote durable run requires a Factory target such as --named, --factory, or --workflow",
	}
}

func remoteInlineFactorySource(
	cfg RunConfig,
	configPath string,
) (factoryapi.FactorySessionExecutionSource, *factoryapi.FactorySessionRequestedPolicy, error) {
	if cfg.LoadFactoryConfigFile == nil {
		return factoryapi.FactorySessionExecutionSource{}, nil, &InvocationError{
			Code:    RemoteDurableRequestInvalidCode,
			Message: "remote durable run requires the Factory config loader for an inline Factory target",
		}
	}
	definition, err := cfg.LoadFactoryConfigFile(configPath)
	if err != nil {
		return factoryapi.FactorySessionExecutionSource{}, nil, &InvocationError{
			Code:    RemoteDurableRequestInvalidCode,
			Message: fmt.Sprintf("load remote Factory target %q: %v", configPath, err),
			Cause:   err,
		}
	}
	if definition == nil {
		return factoryapi.FactorySessionExecutionSource{}, nil, &InvocationError{
			Code:    RemoteDurableRequestInvalidCode,
			Message: fmt.Sprintf("load remote Factory target %q: config is empty", configPath),
		}
	}
	publicFactory, err := factoryconfigmapping.FactoryConfigToOpenAPI(definition)
	if err != nil {
		return factoryapi.FactorySessionExecutionSource{}, nil, &InvocationError{
			Code:    RemoteDurableRequestInvalidCode,
			Message: fmt.Sprintf("normalize remote Factory target %q: %v", configPath, err),
			Cause:   err,
		}
	}
	return factoryapi.FactorySessionExecutionSource{
		Kind:          factoryapi.FactorySessionExecutionSourceKindFactoryInline,
		FactoryInline: &publicFactory,
	}, remoteRequestedPolicy(definition), nil
}

func remoteRunFactorySourceUsesJavaScript(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".js", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func remoteRequestedPolicy(definition *interfaces.FactoryConfig) *factoryapi.FactorySessionRequestedPolicy {
	if definition == nil || definition.Orchestrator == nil || definition.Orchestrator.JavaScript == nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(definition.Orchestrator.JavaScript.DefaultPolicy))
	if trimmed == "" {
		return nil
	}
	var policy map[string]any
	if err := json.Unmarshal([]byte(trimmed), &policy); err != nil || len(policy) == 0 {
		return nil
	}
	return &factoryapi.FactorySessionRequestedPolicy{AdditionalProperties: policy}
}

func writeRemoteDurableStartResult(cfg RunConfig, response factoryapi.FactorySessionExecutionResponse) error {
	output := cfg.Output
	if output == nil {
		output = cfg.StartupOutput
	}
	if output == nil {
		return fmt.Errorf("write remote durable start result: process output is required")
	}
	if cfg.JSONOutput {
		encoded, err := json.Marshal(response)
		if err != nil {
			return fmt.Errorf("marshal remote durable start response: %w", err)
		}
		_, err = fmt.Fprintln(output, string(encoded))
		return err
	}
	_, err := fmt.Fprintf(output, "Factory session %s accepted (%s).\n", response.SessionId, response.Status)
	return err
}
