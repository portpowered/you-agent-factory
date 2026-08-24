package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionshttp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	interruptPhaseTransport = "TRANSPORT"
	interruptPhaseResponse  = "RESPONSE"
	interruptPhaseWait      = "WAIT"
)

// LocalInterruptBoundary is the exact local Worker Sessions seam used by the
// interrupt command. Local placement never turns this operation into HTTP;
// production wiring supplies the already-composed Worker Sessions service.
type LocalInterruptBoundary interface {
	Interrupt(context.Context, workersessions.InterruptRequest) (workersessions.InterruptResult, error)
	LocalObservationBoundary
}

// InterruptConfig holds the source identity and replacement input accepted by
// the manifest-authored interrupt command. Local placement is the default;
// remote placement uses only the explicitly selected server.
type InterruptConfig struct {
	Context context.Context
	Server  string
	Remote  bool

	RequestID                string
	SourceWorkerSessionID    string
	SuccessorWorkerSessionID string
	ReplacementMessage       string
	Prompt                   []string
	Stdin                    io.Reader
	StdinIsTTY               bool
	Async                    bool

	OutputFormat string
	JSON         bool
	Verbose      bool
	Debug        bool
	Output       io.Writer
	Diagnostics  io.Writer
	HTTP         clihttp.Protocol
	Local        LocalInterruptBoundary
	GenerateID   IDGenerator
}

// InterruptOperation is the composition-facing Worker Session interrupt
// operation.
type InterruptOperation func(InterruptConfig) error

// BindInterrupt binds the exact remote protocol and local Worker Sessions
// boundary. A remote failure is returned to the caller and never falls back
// to the local boundary.
func BindInterrupt(transport clihttp.Protocol, local LocalInterruptBoundary, effects ...Effects) InterruptOperation {
	selected := selectEffects(effects)
	return func(config InterruptConfig) error {
		config.HTTP = transport
		if local != nil {
			config.Local = local
		}
		if config.GenerateID == nil {
			config.GenerateID = selected.GenerateID
		}
		return interruptWorkerSession(config)
	}
}

// NewInterrupt returns an interrupt operation with injected effects for
// focused CLI and functional tests.
func NewInterrupt(transport clihttp.Protocol, local LocalInterruptBoundary, effects ...Effects) InterruptOperation {
	return BindInterrupt(transport, local, effects...)
}

type normalizedInterruptRequest struct {
	API     factoryapi.WorkerSessionInterruptRequest
	Service workersessions.InterruptRequest
}

type interruptSnapshot struct {
	WorkerSessionID string `json:"workerSessionId"`
	State           string `json:"state"`
	EventTopic      string `json:"eventTopic"`
}

type interruptResult struct {
	RequestID                string            `json:"requestId"`
	SourceWorkerSessionID    string            `json:"sourceWorkerSessionId"`
	SuccessorWorkerSessionID string            `json:"successorWorkerSessionId"`
	Phase                    string            `json:"phase"`
	Accepted                 bool              `json:"accepted"`
	Source                   interruptSnapshot `json:"source"`
	Successor                interruptSnapshot `json:"successor"`
	Observation              string            `json:"observation"`
	Output                   string            `json:"output,omitempty"`
	StructuredResult         any               `json:"structuredResult,omitempty"`
}

func interruptWorkerSession(config InterruptConfig) error {
	jsonOutput := config.JSON || strings.EqualFold(strings.TrimSpace(config.OutputFormat), "json")
	if err := validateInterruptConfig(config); err != nil {
		return emitInterruptCLIError(config, jsonOutput, err)
	}
	format, err := normalizeOutputFormat(config.OutputFormat)
	if err != nil {
		return emitInterruptCLIError(config, jsonOutput, err)
	}
	jsonOutput = config.JSON || format == "json"
	request, err := normalizeInterruptRequest(config)
	if err != nil {
		return emitInterruptCLIError(config, jsonOutput, err)
	}
	if config.Remote {
		return interruptRemote(config, request, jsonOutput)
	}
	return interruptLocal(config, request, jsonOutput)
}

func validateInterruptConfig(config InterruptConfig) error {
	if config.Context == nil {
		return fmt.Errorf("context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if strings.TrimSpace(config.SourceWorkerSessionID) == "" {
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_INVALID", "source Worker Session identity is required", string(workersessions.InterruptPhaseValidation), nil)
	}
	if config.Remote && config.HTTP == nil {
		return newInterruptCLIError("WORKER_SESSION_HTTP_UNAVAILABLE", "remote Worker Session interrupt requires a CLI HTTP protocol", interruptPhaseTransport, nil)
	}
	if !config.Remote && config.Local == nil {
		return newInterruptCLIError("WORKER_SESSION_LOCAL_UNAVAILABLE", "local Worker Sessions boundary is unavailable", string(workersessions.InterruptPhaseValidation), nil)
	}
	return nil
}

func normalizeInterruptRequest(config InterruptConfig) (normalizedInterruptRequest, error) {
	replacement, err := resolveInterruptInput(config)
	if err != nil {
		return normalizedInterruptRequest{}, err
	}
	if strings.TrimSpace(replacement) == "" {
		return normalizedInterruptRequest{}, newInterruptCLIError("WORKER_SESSION_INPUT_MISSING", "a non-empty replacement input is required", string(workersessions.InterruptPhaseValidation), nil)
	}
	requestID := strings.TrimSpace(config.RequestID)
	if requestID == "" {
		if config.GenerateID == nil {
			return normalizedInterruptRequest{}, newInterruptCLIError("WORKER_SESSION_IDENTITY_UNAVAILABLE", "Worker Session interrupt identity generator is unavailable", string(workersessions.InterruptPhaseValidation), nil)
		}
		requestID = config.GenerateID()
	}
	successorID := strings.TrimSpace(config.SuccessorWorkerSessionID)
	if successorID == "" {
		if config.GenerateID == nil {
			return normalizedInterruptRequest{}, newInterruptCLIError("WORKER_SESSION_IDENTITY_UNAVAILABLE", "Worker Session interrupt identity generator is unavailable", string(workersessions.InterruptPhaseValidation), nil)
		}
		successorID = config.GenerateID()
	}
	apiRequest := factoryapi.WorkerSessionInterruptRequest{
		RequestId:                requestID,
		SuccessorWorkerSessionId: successorID,
		ReplacementMessage:       replacement,
	}
	serviceRequest, err := workersessionshttp.WorkerSessionInterruptRequestFromAPI(config.SourceWorkerSessionID, apiRequest)
	if err != nil {
		return normalizedInterruptRequest{}, newInterruptCLIError("WORKER_SESSION_INTERRUPT_INVALID", "invalid Worker Session interrupt request", string(workersessions.InterruptPhaseValidation), err)
	}
	return normalizedInterruptRequest{API: apiRequest, Service: serviceRequest}, nil
}

func resolveInterruptInput(config InterruptConfig) (string, error) {
	if config.ReplacementMessage != "" {
		return config.ReplacementMessage, nil
	}
	if len(config.Prompt) > 0 {
		parts := make([]string, len(config.Prompt))
		for index, part := range config.Prompt {
			parts[index] = strings.TrimSpace(part)
		}
		return strings.TrimSpace(strings.Join(parts, " ")), nil
	}
	if config.StdinIsTTY || config.Stdin == nil {
		return "", nil
	}
	data, err := readBoundedWorkerSessionStdin(
		config.Stdin,
		maxWorkerSessionMessageStdinBytes,
		"Worker Session replacement input stdin",
		"use --replacement-message or positional input for larger input",
	)
	if err != nil {
		return "", newInterruptCLIError("WORKER_SESSION_INPUT_FAILED", fmt.Sprintf("failed to read Worker Session replacement input from stdin: %v", err), string(workersessions.InterruptPhaseValidation), err)
	}
	return strings.TrimSpace(string(data)), nil
}

func interruptLocal(config InterruptConfig, request normalizedInterruptRequest, jsonOutput bool) error {
	admitted, err := config.Local.Interrupt(config.Context, request.Service)
	if err != nil {
		return emitInterruptCLIError(config, jsonOutput, mapInterruptServiceError(err))
	}
	result := interruptAdmissionResult(request.Service, admitted)
	if config.Async {
		return writeInterruptResult(config, jsonOutput, result, false)
	}
	capture, streamErr := waitLocalTerminal(config.Context, config.Local, admitted.Successor.ID)
	if streamErr != nil {
		return emitInterruptCLIError(config, jsonOutput, mapInterruptWaitError(streamErr))
	}
	if capture.State == "" {
		capture.State = string(admitted.Successor.State)
	}
	if err := terminalInvokeError(capture); err != nil {
		return emitInterruptCLIError(config, jsonOutput, mapInterruptWaitError(err))
	}
	return writeInterruptResult(config, jsonOutput, interruptResultFromCapture(result, capture), true)
}

func interruptRemote(config InterruptConfig, request normalizedInterruptRequest, jsonOutput bool) error {
	admitted, err := admitRemoteInterrupt(config, request)
	if err != nil {
		return emitInterruptCLIError(config, jsonOutput, err)
	}
	result := interruptResultFromAPI(admitted, request.Service.RequestID)
	if config.Async {
		return writeInterruptResult(config, jsonOutput, result, false)
	}
	capture, err := waitRemoteTerminal(InvokeConfig{
		Context: config.Context, Server: config.Server, HTTP: config.HTTP,
		Diagnostics: config.Diagnostics, Verbose: config.Verbose, Debug: config.Debug,
	}, admitted.SuccessorWorkerSessionId)
	if err != nil {
		return emitInterruptCLIError(config, jsonOutput, mapInterruptWaitError(err))
	}
	if err := terminalInvokeError(capture); err != nil {
		return emitInterruptCLIError(config, jsonOutput, mapInterruptWaitError(err))
	}
	return writeInterruptResult(config, jsonOutput, interruptResultFromCapture(result, capture), true)
}

func admitRemoteInterrupt(config InterruptConfig, request normalizedInterruptRequest) (factoryapi.WorkerSessionInterruptResponse, error) {
	endpoint, err := interruptEndpoint(config.Server, request.Service.SourceWorkerSessionID)
	if err != nil {
		return factoryapi.WorkerSessionInterruptResponse{}, newInterruptCLIError("WORKER_SESSION_ENDPOINT_INVALID", "remote Worker Session interrupt endpoint is invalid", interruptPhaseTransport, err)
	}
	body, err := json.Marshal(request.API)
	if err != nil {
		return factoryapi.WorkerSessionInterruptResponse{}, newInterruptCLIError("WORKER_SESSION_INTERRUPT_INVALID", "failed to encode Worker Session interrupt request", string(workersessions.InterruptPhaseValidation), err)
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions interrupt request placement=remote endpointPath=%s requestID=%s sourceWorkerSessionID=%s successorWorkerSessionID=%s async=%t",
		endpoint.Path, request.Service.RequestID, request.Service.SourceWorkerSessionID, request.Service.SuccessorWorkerSessionID, config.Async)
	httpRequest, err := http.NewRequestWithContext(config.Context, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return factoryapi.WorkerSessionInterruptResponse{}, newInterruptCLIError("WORKER_SESSION_ENDPOINT_INVALID", "failed to build remote Worker Session interrupt request", interruptPhaseTransport, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	response, requestErr := config.HTTP.Execute(httpRequest)
	if requestErr != nil {
		return factoryapi.WorkerSessionInterruptResponse{}, remoteInterruptTransportError(config, requestErr)
	}
	if response.HTTP == nil {
		return factoryapi.WorkerSessionInterruptResponse{}, newInterruptCLIError("FACTORY_UNREACHABLE", "remote Worker Session interrupt returned no HTTP response", interruptPhaseTransport, nil)
	}
	defer response.HTTP.Body.Close()
	if response.HTTP.StatusCode != http.StatusAccepted {
		return factoryapi.WorkerSessionInterruptResponse{}, remoteInterruptHTTPError(response.HTTP, response.HTTP.StatusCode)
	}
	var admitted factoryapi.WorkerSessionInterruptResponse
	if err := json.NewDecoder(response.HTTP.Body).Decode(&admitted); err != nil {
		return factoryapi.WorkerSessionInterruptResponse{}, newInterruptCLIError("WORKER_SESSION_INTERRUPT_RESPONSE_INVALID", "remote Worker Session interrupt response is invalid", interruptPhaseResponse, err)
	}
	if err := validateRemoteInterruptAdmission(admitted, request.Service); err != nil {
		return factoryapi.WorkerSessionInterruptResponse{}, err
	}
	return admitted, nil
}

func validateRemoteInterruptAdmission(
	admitted factoryapi.WorkerSessionInterruptResponse,
	request workersessions.InterruptRequest,
) error {
	if !admitted.Accepted ||
		strings.TrimSpace(admitted.RequestId) != request.RequestID ||
		strings.TrimSpace(admitted.SourceWorkerSessionId) != request.SourceWorkerSessionID ||
		strings.TrimSpace(admitted.SuccessorWorkerSessionId) != request.SuccessorWorkerSessionID ||
		admitted.Phase != factoryapi.WorkerSessionInterruptResponsePhaseSuccessorAdmission ||
		strings.TrimSpace(admitted.Source.WorkerSessionId) != request.SourceWorkerSessionID ||
		admitted.Source.State != factoryapi.WorkerSessionInterruptSnapshotStateCanceled ||
		strings.TrimSpace(admitted.Source.EventTopic) == "" ||
		strings.TrimSpace(admitted.Successor.WorkerSessionId) != request.SuccessorWorkerSessionID ||
		admitted.Successor.State != factoryapi.WorkerSessionInterruptSnapshotStateRunning ||
		strings.TrimSpace(admitted.Successor.EventTopic) == "" {
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_RESPONSE_INVALID", "remote Worker Session interrupt was not admitted with the requested source and successor", interruptPhaseResponse, nil)
	}
	return nil
}

func interruptEndpoint(server, sourceWorkerSessionID string) (*url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, sessionpath.TopLevelWorkerSessionInterruptPath(sourceWorkerSessionID))
	if err != nil {
		return nil, err
	}
	return parseInvokeURL(endpointURL)
}

func interruptAdmissionResult(request workersessions.InterruptRequest, admitted workersessions.InterruptResult) interruptResult {
	source := interruptSnapshotFromSession(admitted.Source, request.SourceWorkerSessionID)
	successor := interruptSnapshotFromSession(admitted.Successor, request.SuccessorWorkerSessionID)
	return interruptResult{
		RequestID:                request.RequestID,
		SourceWorkerSessionID:    request.SourceWorkerSessionID,
		SuccessorWorkerSessionID: request.SuccessorWorkerSessionID,
		Phase:                    string(admitted.Phase),
		Accepted:                 admitted.Accepted,
		Source:                   source,
		Successor:                successor,
		Observation:              observationGuidance(successor.WorkerSessionID),
	}
}

func interruptResultFromAPI(admitted factoryapi.WorkerSessionInterruptResponse, fallbackRequestID string) interruptResult {
	requestID := strings.TrimSpace(admitted.RequestId)
	if requestID == "" {
		requestID = fallbackRequestID
	}
	return interruptResult{
		RequestID:                requestID,
		SourceWorkerSessionID:    admitted.SourceWorkerSessionId,
		SuccessorWorkerSessionID: admitted.SuccessorWorkerSessionId,
		Phase:                    string(admitted.Phase),
		Accepted:                 admitted.Accepted,
		Source: interruptSnapshot{
			WorkerSessionID: admitted.Source.WorkerSessionId,
			State:           string(admitted.Source.State),
			EventTopic:      admitted.Source.EventTopic,
		},
		Successor: interruptSnapshot{
			WorkerSessionID: admitted.Successor.WorkerSessionId,
			State:           string(admitted.Successor.State),
			EventTopic:      admitted.Successor.EventTopic,
		},
		Observation: observationGuidance(admitted.SuccessorWorkerSessionId),
	}
}

func interruptSnapshotFromSession(session workersessions.Session, fallbackID string) interruptSnapshot {
	id := strings.TrimSpace(session.ID)
	if id == "" {
		id = strings.TrimSpace(fallbackID)
	}
	return interruptSnapshot{
		WorkerSessionID: id,
		State:           string(session.State),
		EventTopic:      string(workersessions.Topic(id)),
	}
}

func interruptResultFromCapture(result interruptResult, capture invokeCapture) interruptResult {
	result.Successor.State = capture.State
	if capture.HasOutput {
		result.Output = capture.Output
	}
	if capture.HasStructured {
		result.StructuredResult = capture.StructuredResult
	}
	return result
}

func remoteInterruptTransportError(config InterruptConfig, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(config.Context.Err(), context.Canceled) {
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_INTERRUPTED", "remote Worker Session interrupt was interrupted", interruptPhaseWait, context.Canceled)
	}
	return newInterruptCLIError("FACTORY_UNREACHABLE", fmt.Sprintf("factory not reachable at %s", safeRemoteServer(config.Server)), interruptPhaseTransport, err)
}

func remoteInterruptHTTPError(response *http.Response, status int) error {
	payload := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Phase   string `json:"phase"`
	}{}
	if response != nil && response.Body != nil {
		if err := json.NewDecoder(response.Body).Decode(&payload); err == nil && strings.TrimSpace(payload.Message) != "" {
			code := strings.TrimSpace(payload.Code)
			if code == "" {
				code = interruptHTTPErrorCode(status)
			}
			phase := strings.TrimSpace(payload.Phase)
			if phase == "" {
				phase = interruptHTTPErrorPhase(status)
			}
			return newInterruptCLIError(code, payload.Message, phase, nil)
		}
	}
	return newInterruptCLIError(interruptHTTPErrorCode(status), fmt.Sprintf("remote Worker Session interrupt failed (%d)", status), interruptHTTPErrorPhase(status), nil)
}

func interruptHTTPErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "WORKER_SESSION_INTERRUPT_CONFLICT"
	case http.StatusServiceUnavailable:
		return "WORKER_SESSION_INTERRUPT_ADMISSION_FAILED"
	default:
		return "INTERNAL_ERROR"
	}
}

func interruptHTTPErrorPhase(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
		return string(workersessions.InterruptPhaseValidation)
	default:
		return string(workersessions.InterruptPhaseSuccessorAdmission)
	}
}

func mapInterruptServiceError(err error) error {
	if errors.Is(err, context.Canceled) {
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_INTERRUPTED", "Worker Session interrupt was interrupted", interruptPhaseWait, context.Canceled)
	}
	phase := string(workersessions.InterruptPhaseValidation)
	var typed *workersessions.InterruptError
	if errors.As(err, &typed) && typed != nil {
		phase = string(typed.Phase)
	}
	switch {
	case errors.Is(err, workersessions.ErrInvalidInterruptRequestID),
		errors.Is(err, workersessions.ErrInvalidInterruptLineage),
		errors.Is(err, workersessions.ErrInvalidInterruptMessage),
		errors.Is(err, workersessions.ErrInterruptValidation):
		return newInterruptCLIError("BAD_REQUEST", "invalid Worker Session interrupt request", phase, err)
	case errors.Is(err, workersessions.ErrInterruptSourceNotFound):
		return newInterruptCLIError("NOT_FOUND", "Worker Session interrupt source not found", phase, err)
	case errors.Is(err, workersessions.ErrInterruptRequestIDConflict):
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT", "Worker Session interrupt requestId was reused with different inputs", phase, err)
	case errors.Is(err, workersessions.ErrInterruptSourceNotActive),
		errors.Is(err, workersessions.ErrInterruptSourceConflict),
		errors.Is(err, workersessions.ErrInterruptProviderSessionMissing),
		errors.Is(err, workersessions.ErrInterruptProviderSessionInvalid):
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_CONFLICT", "Worker Session interrupt conflicts with existing state", phase, err)
	case errors.Is(err, workersessions.ErrInterruptSourceCancellation),
		errors.Is(err, workersessions.ErrInterruptSourceCancellationFailed):
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_SOURCE_CANCELLATION_FAILED", "Workers could not cancel the Worker Session interrupt source", phase, err)
	case errors.Is(err, workersessions.ErrInterruptSuccessorAdmission),
		errors.Is(err, workersessions.ErrInterruptSuccessorAdmissionFailed):
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED", "Workers could not admit the Worker Session interrupt successor", phase, err)
	case errors.Is(err, workersessions.ErrInterruptExecutionUnavailable),
		errors.Is(err, workersessions.ErrInterruptServerStopping):
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_ADMISSION_FAILED", "Workers could not admit the Worker Session interrupt", phase, err)
	default:
		return newInterruptCLIError("INTERNAL_ERROR", "failed to interrupt Worker Session", phase, err)
	}
}

func mapInterruptWaitError(err error) error {
	if errors.Is(err, context.Canceled) {
		return newInterruptCLIError("WORKER_SESSION_INTERRUPT_INTERRUPTED", "Worker Session interrupt wait was interrupted", interruptPhaseWait, context.Canceled)
	}
	return newInterruptCLIError("WORKER_SESSION_INTERRUPT_WAIT_FAILED", "failed while waiting for the interrupt successor terminal output", interruptPhaseWait, err)
}

func writeInterruptResult(config InterruptConfig, jsonOutput bool, result interruptResult, synchronous bool) error {
	if jsonOutput {
		return json.NewEncoder(config.Output).Encode(result)
	}
	if synchronous {
		if _, err := fmt.Fprintf(config.Output,
			"Worker Session interrupt completed\nSource Worker Session ID:\t%s\nSuccessor Worker Session ID:\t%s\nSource State:\t%s\nSuccessor State:\t%s\nPhase:\t%s\nObserve:\t%s\n",
			result.SourceWorkerSessionID, result.SuccessorWorkerSessionID, result.Source.State, result.Successor.State, result.Phase, result.Observation); err != nil {
			return newCLIError("WORKER_SESSION_OUTPUT_FAILED", "failed to write Worker Session interrupt result", err)
		}
		if result.Output != "" {
			if _, err := io.WriteString(config.Output, result.Output); err != nil {
				return newCLIError("WORKER_SESSION_OUTPUT_FAILED", "failed to write Worker Session interrupt output", err)
			}
			if !strings.HasSuffix(result.Output, "\n") {
				_, err := io.WriteString(config.Output, "\n")
				return err
			}
		}
		return nil
	}
	_, err := fmt.Fprintf(config.Output,
		"Worker Session interrupt admitted\nRequest ID:\t%s\nSource Worker Session ID:\t%s\nSuccessor Worker Session ID:\t%s\nSource State:\t%s\nSuccessor State:\t%s\nPhase:\t%s\nObserve:\t%s\n",
		result.RequestID, result.SourceWorkerSessionID, result.SuccessorWorkerSessionID, result.Source.State, result.Successor.State, result.Phase, result.Observation)
	return err
}

func emitInterruptCLIError(config InterruptConfig, jsonOutput bool, err error) error {
	if !jsonOutput || err == nil {
		return err
	}
	output := config.Output
	if clidiag.CentralDiagnosticsEnabled(config.Context) {
		output = config.Diagnostics
	}
	if output == nil {
		return err
	}
	payload := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Phase   string `json:"phase"`
	}{
		Code:    "WORKER_SESSION_INTERRUPT_FAILED",
		Message: err.Error(),
		Phase:   interruptPhaseResponse,
	}
	var typed *CLIError
	if errors.As(err, &typed) && typed != nil {
		if typed.Code != "" {
			payload.Code = typed.Code
		}
		if typed.Message != "" {
			payload.Message = typed.Message
		}
		if typed.Phase != "" {
			payload.Phase = typed.Phase
		}
	}
	if encodeErr := json.NewEncoder(output).Encode(payload); encodeErr != nil {
		return errors.Join(err, encodeErr)
	}
	if clidiag.CentralDiagnosticsEnabled(config.Context) {
		clidiag.MarkDiagnosticRendered(output)
	}
	return err
}

func newInterruptCLIError(code, message, phase string, cause error) *CLIError {
	return &CLIError{Code: code, Message: message, Phase: phase, Cause: cause}
}
