package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/uuid"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionshttp "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// LocalInvokeBoundary is the deliberately small local seam used by invoke.
// The CLI never turns a local request into an HTTP request: production wiring
// or a functional test supplies the already-open Worker Sessions boundary.
type LocalInvokeBoundary interface {
	Start(context.Context, workersessions.StartRequest) (workersessions.StartResult, error)
	Continue(context.Context, workersessions.ContinueRequest) (workersessions.ContinueResult, error)
	StreamObservationsByWorkerSessionID(context.Context, workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error)
}

// InvokeConfig holds the direct Worker execution inputs accepted by the
// manifest-authored invoke command. The execution document supplies the base
// request; explicit flags override document fields, followed by positional
// input and finally non-TTY stdin for the user message.
type InvokeConfig struct {
	Context context.Context
	Server  string
	Remote  bool

	RequestID        string
	WorkerSessionID  string
	DispatchID       string
	WorkstationName  string
	WorkerType       string
	RunnerID         string
	Provider         string
	Model            string
	ReasoningEffort  string
	SystemPrompt     string
	UserMessage      string
	ExecutionJSON    string
	Prompt           []string
	Stdin            io.Reader
	StdinIsTTY       bool
	Async            bool
	RetryMaxAttempts int

	OutputFormat string
	JSON         bool
	Verbose      bool
	Debug        bool
	Output       io.Writer
	Diagnostics  io.Writer
	HTTP         clihttp.Protocol
	Local        LocalInvokeBoundary
}

// InvokeOperation is the composition-facing direct Worker operation.
type InvokeOperation func(InvokeConfig) error

// BindInvoke binds remote HTTP and an explicit local Worker Sessions boundary.
// A nil local boundary remains a hard failure for local placement; it is never
// replaced with a request to the configured server.
func BindInvoke(transport clihttp.Protocol, local LocalInvokeBoundary) InvokeOperation {
	return func(config InvokeConfig) error {
		config.HTTP = transport
		if local != nil {
			config.Local = local
		}
		return invoke(config)
	}
}

// NewInvoke returns an invoke operation with its effects supplied by the
// caller. It is useful to focused CLI and functional tests that need to prove
// local and remote placement separately.
func NewInvoke(transport clihttp.Protocol, local LocalInvokeBoundary) InvokeOperation {
	return BindInvoke(transport, local)
}

type normalizedInvokeRequest struct {
	API     factoryapi.WorkerSessionStartRequest
	Service workersessions.StartRequest
}

type invokeResult struct {
	RequestID        string `json:"requestId"`
	WorkerSessionID  string `json:"workerSessionId"`
	Accepted         bool   `json:"accepted"`
	State            string `json:"state"`
	EventTopic       string `json:"eventTopic,omitempty"`
	Observation      string `json:"observation"`
	Output           string `json:"output,omitempty"`
	StructuredResult any    `json:"structuredResult,omitempty"`
}

type invokeCapture struct {
	State            string
	Output           string
	HasOutput        bool
	StructuredResult any
	HasStructured    bool
	Error            string
}

func invoke(config InvokeConfig) error {
	jsonOutput := config.JSON || strings.EqualFold(strings.TrimSpace(config.OutputFormat), "json")
	if err := validateInvokeConfig(config); err != nil {
		return emitInvokeCLIError(config, jsonOutput, err)
	}
	format, err := normalizeOutputFormat(config.OutputFormat)
	if err != nil {
		return emitInvokeCLIError(config, jsonOutput, err)
	}
	jsonOutput = config.JSON || format == "json"

	request, err := normalizeInvokeRequest(config)
	if err != nil {
		return emitInvokeCLIError(config, jsonOutput, err)
	}
	if config.Remote {
		return invokeRemote(config, request, jsonOutput)
	}
	return invokeLocal(config, request, jsonOutput)
}

func validateInvokeConfig(config InvokeConfig) error {
	if config.Context == nil {
		return fmt.Errorf("context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if config.RetryMaxAttempts < 0 {
		return newCLIError("WORKER_SESSION_RETRY_INVALID", "--retry-max-attempts must not be negative", nil)
	}
	if config.Remote && config.HTTP == nil {
		return newCLIError("WORKER_SESSION_HTTP_UNAVAILABLE", "remote invoke requires a CLI HTTP protocol", nil)
	}
	if !config.Remote && config.Local == nil {
		return newCLIError("WORKER_SESSION_LOCAL_UNAVAILABLE", "local Worker Sessions boundary is unavailable", nil)
	}
	return nil
}

func normalizeInvokeRequest(config InvokeConfig) (normalizedInvokeRequest, error) {
	request, err := readInvokeRequest(config)
	if err != nil {
		return normalizedInvokeRequest{}, err
	}
	if err := applyInvokeOverrides(&request, config); err != nil {
		return normalizedInvokeRequest{}, err
	}
	ensureInvokeIdentities(&request)

	serviceRequest, err := workersessionshttp.WorkerSessionStartRequestFromAPI(request)
	if err != nil {
		return normalizedInvokeRequest{}, newCLIError("WORKER_SESSION_INVOKE_INVALID", "invalid direct Worker execution request", err)
	}
	normalizedAPI, err := workersessionshttp.WorkerSessionStartRequestToAPI(serviceRequest)
	if err != nil {
		return normalizedInvokeRequest{}, newCLIError("WORKER_SESSION_INVOKE_INVALID", "failed to normalize direct Worker execution request", err)
	}
	return normalizedInvokeRequest{API: normalizedAPI, Service: serviceRequest}, nil
}

func applyInvokeOverrides(request *factoryapi.WorkerSessionStartRequest, config InvokeConfig) error {
	// CLI identity and execution flags are the explicit highest-precedence
	// source. The document itself remains detached and is never mutated by the
	// caller after this function returns.
	applyInvokeIdentityOverrides(request, config)
	applyInvokeExecutionOverrides(request, config)
	if err := applyInvokeMessageOverride(request, config); err != nil {
		return err
	}
	applyInvokeRetryOverride(request, config)
	return nil
}

func applyInvokeIdentityOverrides(request *factoryapi.WorkerSessionStartRequest, config InvokeConfig) {
	if value := strings.TrimSpace(config.RequestID); value != "" {
		request.RequestId = value
	}
	if value := strings.TrimSpace(config.WorkerSessionID); value != "" {
		request.WorkerSessionId = value
	}
}

func applyInvokeExecutionOverrides(request *factoryapi.WorkerSessionStartRequest, config InvokeConfig) {
	if value := strings.TrimSpace(config.WorkstationName); value != "" {
		request.Execution.WorkstationName = value
		request.Execution.Dispatch.WorkstationName = value
	}
	if value := strings.TrimSpace(config.DispatchID); value != "" {
		request.Execution.Dispatch.DispatchId = value
	}
	if value := strings.TrimSpace(config.WorkerType); value != "" {
		request.Execution.WorkerType = invokeStringPointer(value)
		request.Execution.Dispatch.WorkerType = invokeStringPointer(value)
	}
	if value := strings.TrimSpace(config.RunnerID); value != "" {
		request.Execution.RunnerId = invokeStringPointer(value)
	}
	if value := strings.TrimSpace(config.Provider); value != "" {
		request.Execution.ModelProvider = invokeStringPointer(value)
	}
	if value := strings.TrimSpace(config.Model); value != "" {
		request.Execution.Model = invokeStringPointer(value)
	}
	if value := strings.TrimSpace(config.ReasoningEffort); value != "" {
		request.Execution.ReasoningEffort = invokeStringPointer(value)
	}
	if config.SystemPrompt != "" {
		request.Execution.SystemPrompt = invokeStringPointer(config.SystemPrompt)
	}
}

func applyInvokeMessageOverride(request *factoryapi.WorkerSessionStartRequest, config InvokeConfig) error {
	if config.UserMessage != "" {
		request.Execution.UserMessage = invokeStringPointer(config.UserMessage)
		return nil
	}
	if len(config.Prompt) > 0 {
		message, err := resolveInvokeMessage(config)
		if err != nil {
			return err
		}
		request.Execution.UserMessage = invokeStringPointer(message)
		return nil
	}
	if request.Execution.UserMessage != nil && strings.TrimSpace(derefString(request.Execution.UserMessage)) != "" {
		return nil
	}
	if strings.TrimSpace(config.ExecutionJSON) == "-" && len(config.Prompt) == 0 {
		return newCLIError("WORKER_SESSION_INPUT_MISSING", "--execution - supplies stdin to the execution document; provide the user message in that document or use --user-message", nil)
	}
	message, err := resolveInvokeMessage(config)
	if err != nil {
		return err
	}
	if message != "" {
		request.Execution.UserMessage = invokeStringPointer(message)
	}
	return nil
}

func applyInvokeRetryOverride(request *factoryapi.WorkerSessionStartRequest, config InvokeConfig) {
	if config.RetryMaxAttempts > 0 {
		attempts := config.RetryMaxAttempts
		request.Retry = &factoryapi.WorkerSessionStartRetryPolicy{MaxAttempts: &attempts}
	}
}

func ensureInvokeIdentities(request *factoryapi.WorkerSessionStartRequest) {
	if strings.TrimSpace(request.RequestId) == "" {
		request.RequestId = uuid.NewString()
	}
	if strings.TrimSpace(request.WorkerSessionId) == "" {
		request.WorkerSessionId = uuid.NewString()
	}
	if strings.TrimSpace(request.Execution.WorkstationName) == "" {
		request.Execution.WorkstationName = strings.TrimSpace(request.Execution.Dispatch.WorkstationName)
	}
	if strings.TrimSpace(request.Execution.Dispatch.WorkstationName) == "" {
		request.Execution.Dispatch.WorkstationName = strings.TrimSpace(request.Execution.WorkstationName)
	}
	if strings.TrimSpace(request.Execution.Dispatch.DispatchId) == "" {
		request.Execution.Dispatch.DispatchId = uuid.NewString()
	}
	if request.Execution.Dispatch.Execution == nil {
		request.Execution.Dispatch.Execution = &factoryapi.WorkerSessionExecutionMetadata{}
	}
	if request.Execution.Dispatch.Execution.RequestId == nil {
		request.Execution.Dispatch.Execution.RequestId = invokeStringPointer(request.RequestId)
	}
}

func readInvokeRequest(config InvokeConfig) (factoryapi.WorkerSessionStartRequest, error) {
	input := strings.TrimSpace(config.ExecutionJSON)
	if input == "" {
		return factoryapi.WorkerSessionStartRequest{}, nil
	}
	var data []byte
	if input == "-" {
		if config.Stdin == nil {
			return factoryapi.WorkerSessionStartRequest{}, newCLIError("WORKER_SESSION_INPUT_MISSING", "--execution - requires JSON on stdin", nil)
		}
		var err error
		data, err = io.ReadAll(config.Stdin)
		if err != nil {
			return factoryapi.WorkerSessionStartRequest{}, newCLIError("WORKER_SESSION_INPUT_FAILED", "failed to read direct Worker execution from stdin", err)
		}
	} else if strings.HasPrefix(input, "{") {
		data = []byte(input)
	} else {
		var err error
		data, err = os.ReadFile(input)
		if err != nil {
			return factoryapi.WorkerSessionStartRequest{}, newCLIError("WORKER_SESSION_INPUT_FAILED", "failed to read direct Worker execution file", err)
		}
	}
	var request factoryapi.WorkerSessionStartRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return factoryapi.WorkerSessionStartRequest{}, newCLIError("WORKER_SESSION_INPUT_INVALID", "direct Worker execution input is not valid JSON", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return factoryapi.WorkerSessionStartRequest{}, newCLIError("WORKER_SESSION_INPUT_INVALID", "direct Worker execution input must contain exactly one JSON object", err)
	}
	return request, nil
}

func resolveInvokeMessage(config InvokeConfig) (string, error) {
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
	data, err := io.ReadAll(config.Stdin)
	if err != nil {
		return "", newCLIError("WORKER_SESSION_INPUT_FAILED", "failed to read direct Worker input from stdin", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func invokeLocal(config InvokeConfig, request normalizedInvokeRequest, jsonOutput bool) error {
	if config.Async {
		admitted, err := config.Local.Start(config.Context, request.Service)
		if err != nil {
			return emitInvokeCLIError(config, jsonOutput, mapInvokeServiceError(err, true))
		}
		result := invokeResult{
			RequestID:       request.Service.RequestID,
			WorkerSessionID: admitted.Session.ID,
			Accepted:        true,
			State:           string(admitted.Session.State),
			EventTopic:      string(workersessions.Topic(admitted.Session.ID)),
			Observation:     observationGuidance(admitted.Session.ID),
		}
		return writeInvokeResult(config, jsonOutput, result, false)
	}

	started, err := config.Local.Start(config.Context, request.Service)
	if err != nil {
		return emitInvokeCLIError(config, jsonOutput, mapInvokeServiceError(err, false))
	}
	capture, streamErr := waitLocalTerminal(config.Context, config.Local, started.Session.ID)
	if streamErr != nil {
		return emitInvokeCLIError(config, jsonOutput, streamErr)
	}
	if capture.State == "" {
		capture.State = string(started.Session.State)
	}
	if err := terminalInvokeError(capture); err != nil {
		return emitInvokeCLIError(config, jsonOutput, err)
	}
	return writeInvokeResult(config, jsonOutput, invokeResultFromCapture(request.Service.RequestID, started.Session.ID, capture), true)
}

func waitLocalTerminal(ctx context.Context, boundary LocalInvokeBoundary, workerSessionID string) (invokeCapture, error) {
	subscription, err := boundary.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: workerSessionID})
	if err != nil {
		return invokeCapture{}, newCLIError("WORKER_SESSION_STREAM_FAILED", "failed to open Worker Session observation stream", err)
	}
	defer subscription.Close()
	var capture invokeCapture
	for {
		delivery := subscription.Next(ctx)
		switch delivery.Kind {
		case workersessions.ObservationDeliveryRecord, workersessions.ObservationDeliveryTerminal, workersessions.ObservationDeliveryTerminalReplay:
			captureEvent(&capture, delivery.Event.SchemaID, delivery.Event.Payload)
			if delivery.Kind == workersessions.ObservationDeliveryTerminal || delivery.Kind == workersessions.ObservationDeliveryTerminalReplay {
				return capture, nil
			}
		case workersessions.ObservationDeliveryCanceled:
			return invokeCapture{}, newCLIError("WORKER_SESSION_INVOKE_INTERRUPTED", "Worker Session observation stream was canceled", context.Canceled)
		case workersessions.ObservationDeliverySourceFailure:
			return invokeCapture{}, newCLIError("WORKER_SESSION_STREAM_SOURCE_FAILURE", "Worker Session observation source failed", delivery.Err)
		case workersessions.ObservationDeliveryClosed:
			return invokeCapture{}, newCLIError("WORKER_SESSION_STREAM_CLOSED", "Worker Session observation stream closed before terminal", delivery.Err)
		case workersessions.ObservationDeliveryReplaySummary:
			// A complete replay summary only marks the retained prefix. The
			// live stream still owns terminal completion for a waiting invoke.
		}
	}
}

func invokeRemote(config InvokeConfig, request normalizedInvokeRequest, jsonOutput bool) error {
	admitted, err := admitRemoteWorkerSession(config, request)
	if err != nil {
		return emitInvokeCLIError(config, jsonOutput, err)
	}
	result := invokeResult{
		RequestID:       remoteRequestID(admitted, request.Service.RequestID),
		WorkerSessionID: admitted.WorkerSessionId,
		Accepted:        true,
		State:           string(admitted.State),
		EventTopic:      admitted.EventTopic,
		Observation:     observationGuidance(admitted.WorkerSessionId),
	}
	if config.Async {
		return writeInvokeResult(config, jsonOutput, result, false)
	}
	capture, err := waitRemoteTerminal(config, admitted.WorkerSessionId)
	if err != nil {
		return emitInvokeCLIError(config, jsonOutput, err)
	}
	if err := terminalInvokeError(capture); err != nil {
		return emitInvokeCLIError(config, jsonOutput, err)
	}
	result.State = capture.State
	if capture.HasOutput {
		result.Output = capture.Output
	}
	if capture.HasStructured {
		result.StructuredResult = capture.StructuredResult
	}
	return writeInvokeResult(config, jsonOutput, result, true)
}

func admitRemoteWorkerSession(config InvokeConfig, request normalizedInvokeRequest) (factoryapi.WorkerSessionStartResponse, error) {
	endpoint, err := invokeStartEndpoint(config.Server)
	if err != nil {
		return factoryapi.WorkerSessionStartResponse{}, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "remote Worker Session endpoint is invalid", err)
	}
	body, err := json.Marshal(request.API)
	if err != nil {
		return factoryapi.WorkerSessionStartResponse{}, newCLIError("WORKER_SESSION_INVOKE_INVALID", "failed to encode direct Worker execution request", err)
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions invoke request placement=remote endpointPath=%s requestID=%s workerSessionID=%s async=%t",
		endpoint.Path, request.Service.RequestID, request.Service.ID, config.Async)
	httpRequest, err := http.NewRequestWithContext(config.Context, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return factoryapi.WorkerSessionStartResponse{}, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "failed to build remote Worker Session request", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	response, requestErr := config.HTTP.Execute(httpRequest)
	if requestErr != nil {
		return factoryapi.WorkerSessionStartResponse{}, remoteInvokeTransportError(config, requestErr)
	}
	if response.HTTP == nil {
		return factoryapi.WorkerSessionStartResponse{}, newCLIError("FACTORY_UNREACHABLE", "remote Worker Session start returned no HTTP response", nil)
	}
	defer response.HTTP.Body.Close()
	if response.HTTP.StatusCode != http.StatusAccepted {
		return factoryapi.WorkerSessionStartResponse{}, remoteInvokeHTTPError(response.HTTP, response.HTTP.StatusCode)
	}
	var admitted factoryapi.WorkerSessionStartResponse
	if err := json.NewDecoder(response.HTTP.Body).Decode(&admitted); err != nil {
		return factoryapi.WorkerSessionStartResponse{}, newCLIError("WORKER_SESSION_INVOKE_FAILED", "remote Worker Session admission response is invalid", err)
	}
	if !admitted.Accepted || strings.TrimSpace(admitted.WorkerSessionId) == "" {
		return factoryapi.WorkerSessionStartResponse{}, newCLIError("WORKER_SESSION_ADMISSION_FAILED", "remote Worker Session was not admitted", nil)
	}
	return admitted, nil
}

func remoteRequestID(response factoryapi.WorkerSessionStartResponse, fallback string) string {
	if requestID := strings.TrimSpace(response.RequestId); requestID != "" {
		return requestID
	}
	return fallback
}

func waitRemoteTerminal(config InvokeConfig, workerSessionID string) (invokeCapture, error) {
	response, err := openRemoteObservationResponse(config, workerSessionID)
	if err != nil {
		return invokeCapture{}, err
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	var capture invokeCapture
	for {
		payload, atEOF, readErr := readSSEData(reader)
		if readErr != nil {
			return invokeCapture{}, remoteStreamReadError(config, readErr)
		}
		if len(payload) == 0 {
			if atEOF {
				return invokeCapture{}, remoteStreamClosedError()
			}
			continue
		}
		terminal, frameErr := applyRemoteObservationFrame(&capture, payload)
		if frameErr != nil {
			return invokeCapture{}, frameErr
		}
		if terminal {
			return capture, nil
		}
		if atEOF {
			return invokeCapture{}, remoteStreamClosedError()
		}
	}
}

func openRemoteObservationResponse(config InvokeConfig, workerSessionID string) (*http.Response, error) {
	endpoint, err := invokeEventsEndpoint(config.Server, workerSessionID)
	if err != nil {
		return nil, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "remote Worker Session event endpoint is invalid", err)
	}
	request, err := http.NewRequestWithContext(config.Context, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "failed to build remote Worker Session event request", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, requestErr := config.HTTP.Execute(request)
	if requestErr != nil {
		return nil, remoteInvokeStreamTransportError(config, requestErr)
	}
	if response.HTTP == nil {
		return nil, newCLIError("WORKER_SESSION_STREAM_FAILED", "remote Worker Session stream returned no HTTP response", nil)
	}
	if response.HTTP.StatusCode != http.StatusOK {
		response.HTTP.Body.Close()
		return nil, remoteInvokeStreamHTTPError(response.HTTP, response.HTTP.StatusCode)
	}
	return response.HTTP, nil
}

func remoteStreamReadError(config InvokeConfig, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(config.Context.Err(), context.Canceled) {
		return newCLIError("WORKER_SESSION_INVOKE_INTERRUPTED", "remote Worker Session stream was interrupted", context.Canceled)
	}
	return newCLIError("WORKER_SESSION_STREAM_FAILED", "failed to read remote Worker Session event stream", err)
}

func remoteStreamClosedError() error {
	return newCLIError("WORKER_SESSION_STREAM_CLOSED", "remote Worker Session stream closed before terminal", nil)
}

func applyRemoteObservationFrame(capture *invokeCapture, payload []byte) (bool, error) {
	var frame streamJSONFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return false, newCLIError("WORKER_SESSION_STREAM_FAILED", "remote Worker Session stream returned invalid JSON", err)
	}
	switch frame.Delivery {
	case "RECORD", "TERMINAL", "TERMINAL_REPLAY":
		if frame.Event != nil {
			captureEvent(capture, frame.Event.SchemaID, frame.Event.Payload)
		}
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			if capture.State == "" {
				capture.State = "COMPLETED"
			}
			return true, nil
		}
	case "SOURCE_FAILURE":
		code := stringValue(frame.ErrorCode, "WORKER_SESSION_STREAM_SOURCE_FAILURE")
		message := stringValue(frame.ErrorMessage, "Worker Session observation source failed")
		return false, newCLIError(code, message, nil)
	case "REPLAY_SUMMARY":
		// The replay prefix is complete, but waiting continues until an
		// authoritative terminal frame arrives.
	}
	return false, nil
}

func captureEvent(capture *invokeCapture, schemaID string, payload json.RawMessage) {
	if capture == nil || len(payload) == 0 {
		return
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(payload, &values); err != nil {
		return
	}
	captureEventState(capture, values)
	captureEventOutput(capture, values)
	captureEventStructuredResult(capture, values)
	captureEventError(capture, values)
	if capture.State == "" {
		capture.State = inferInvokeState(schemaID)
	}
}

func captureEventState(capture *invokeCapture, values map[string]json.RawMessage) {
	for _, key := range []string{"state", "status"} {
		var value string
		if json.Unmarshal(values[key], &value) == nil && strings.TrimSpace(value) != "" {
			capture.State = strings.ToUpper(strings.TrimSpace(value))
			break
		}
	}
}

func captureEventOutput(capture *invokeCapture, values map[string]json.RawMessage) {
	raw := values["output"]
	if len(raw) == 0 {
		return
	}
	var output string
	if json.Unmarshal(raw, &output) == nil {
		capture.Output, capture.HasOutput = output, true
	}
}

func captureEventStructuredResult(capture *invokeCapture, values map[string]json.RawMessage) {
	raw := values["structuredResult"]
	if len(raw) == 0 {
		return
	}
	var structured any
	if json.Unmarshal(raw, &structured) == nil {
		capture.StructuredResult, capture.HasStructured = structured, true
	}
}

func captureEventError(capture *invokeCapture, values map[string]json.RawMessage) {
	for _, key := range []string{"error", "failureDetail"} {
		var value string
		if json.Unmarshal(values[key], &value) == nil && strings.TrimSpace(value) != "" {
			capture.Error = strings.TrimSpace(value)
			break
		}
	}
}

func inferInvokeState(schemaID string) string {
	lower := strings.ToLower(schemaID)
	switch {
	case strings.Contains(lower, "canceled"):
		return "CANCELED"
	case strings.Contains(lower, "terminated"):
		return "TERMINATED"
	case strings.Contains(lower, "failed"):
		return "FAILED"
	case strings.Contains(lower, "completed"):
		return "COMPLETED"
	default:
		return ""
	}
}

func terminalInvokeError(capture invokeCapture) error {
	switch strings.ToUpper(strings.TrimSpace(capture.State)) {
	case "COMPLETED":
		return nil
	case "FAILED":
		message := capture.Error
		if message == "" {
			message = "direct Worker Session failed"
		}
		return newCLIError("WORKER_SESSION_FAILED", message, nil)
	case "CANCELED":
		return newCLIError("WORKER_SESSION_CANCELED", "direct Worker Session was canceled", context.Canceled)
	case "TERMINATED":
		return newCLIError("WORKER_SESSION_TERMINATED", "direct Worker Session was terminated", nil)
	case "":
		return newCLIError("WORKER_SESSION_INVOKE_FAILED", "terminal Worker Session outcome did not include a lifecycle state", nil)
	default:
		return newCLIError("WORKER_SESSION_INVOKE_FAILED", "terminal Worker Session outcome has an unsupported lifecycle state", nil)
	}
}

func invokeResultFromCapture(requestID, workerSessionID string, capture invokeCapture) invokeResult {
	result := invokeResult{
		RequestID:       requestID,
		WorkerSessionID: workerSessionID,
		Accepted:        true,
		State:           capture.State,
		Observation:     observationGuidance(workerSessionID),
	}
	if capture.HasOutput {
		result.Output = capture.Output
	}
	if capture.HasStructured {
		result.StructuredResult = capture.StructuredResult
	}
	return result
}

func writeInvokeResult(config InvokeConfig, jsonOutput bool, result invokeResult, synchronous bool) error {
	if jsonOutput {
		return json.NewEncoder(config.Output).Encode(result)
	}
	if synchronous {
		if result.Output != "" {
			if _, err := io.WriteString(config.Output, result.Output); err != nil {
				return newCLIError("WORKER_SESSION_OUTPUT_FAILED", "failed to write direct Worker output", err)
			}
			if !strings.HasSuffix(result.Output, "\n") {
				_, err := io.WriteString(config.Output, "\n")
				return err
			}
			return nil
		}
		_, err := fmt.Fprintf(config.Output, "Worker Session %s completed (%s)\n", result.WorkerSessionID, result.State)
		return err
	}
	_, err := fmt.Fprintf(config.Output,
		"Worker Session admitted\nRequest ID:\t%s\nWorker Session ID:\t%s\nState:\t%s\nObserve:\t%s\n",
		result.RequestID, result.WorkerSessionID, result.State, result.Observation)
	return err
}

func observationGuidance(workerSessionID string) string {
	return fmt.Sprintf("you worker-sessions show --worker-session-id %s", workerSessionID)
}

func invokeStartEndpoint(server string) (*url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, sessionpath.TopLevelWorkerSessionsCollectionPath())
	if err != nil {
		return nil, err
	}
	return parseInvokeURL(endpointURL)
}

func invokeEventsEndpoint(server, workerSessionID string) (*url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, sessionpath.TopLevelWorkerSessionEventsPath(workerSessionID))
	if err != nil {
		return nil, err
	}
	return parseInvokeURL(endpointURL)
}

func parseInvokeURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func remoteInvokeTransportError(config InvokeConfig, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(config.Context.Err(), context.Canceled) {
		return newCLIError("WORKER_SESSION_INVOKE_INTERRUPTED", "remote Worker Session request was interrupted", context.Canceled)
	}
	return newCLIError("FACTORY_UNREACHABLE", fmt.Sprintf("factory not reachable at %s", safeRemoteServer(config.Server)), err)
}

func remoteInvokeStreamTransportError(config InvokeConfig, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(config.Context.Err(), context.Canceled) {
		return newCLIError("WORKER_SESSION_INVOKE_INTERRUPTED", "remote Worker Session stream was interrupted", context.Canceled)
	}
	return newCLIError("WORKER_SESSION_STREAM_FAILED", "remote Worker Session stream could not be opened", err)
}

func remoteInvokeHTTPError(response *http.Response, status int) error {
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		code := strings.TrimSpace(string(apiError.Code))
		switch code {
		case "BAD_REQUEST":
			code = "WORKER_SESSION_INVOKE_INVALID"
		case "CONFLICT":
			code = "WORKER_SESSION_INVOKE_CONFLICT"
		case "INTERNAL_ERROR":
			code = "WORKER_SESSION_ADMISSION_FAILED"
		}
		if code == "" {
			code = "WORKER_SESSION_ADMISSION_FAILED"
		}
		return newCLIError(code, apiError.Message, nil)
	}
	return newCLIError("WORKER_SESSION_ADMISSION_FAILED", fmt.Sprintf("remote Worker Session admission failed (%d)", status), nil)
}

func remoteInvokeStreamHTTPError(response *http.Response, status int) error {
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		code := strings.TrimSpace(string(apiError.Code))
		if code == "" {
			code = "WORKER_SESSION_STREAM_FAILED"
		}
		return newCLIError(code, apiError.Message, nil)
	}
	return newCLIError("WORKER_SESSION_STREAM_FAILED", fmt.Sprintf("remote Worker Session stream failed (%d)", status), nil)
}

func mapInvokeServiceError(err error, async bool) error {
	if errors.Is(err, context.Canceled) {
		return newCLIError("WORKER_SESSION_INVOKE_INTERRUPTED", "Worker Session invocation was interrupted", context.Canceled)
	}
	if async {
		return newCLIError("WORKER_SESSION_ADMISSION_FAILED", "Worker Session admission failed", err)
	}
	return newCLIError("WORKER_SESSION_INVOKE_FAILED", "Worker Session invocation failed", err)
}

func emitInvokeCLIError(config InvokeConfig, jsonOutput bool, err error) error {
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
	code := "WORKER_SESSION_INVOKE_FAILED"
	message := err.Error()
	var typed *CLIError
	if errors.As(err, &typed) {
		if typed.Code != "" {
			code = typed.Code
		}
		if typed.Message != "" {
			message = typed.Message
		}
	}
	if encodeErr := json.NewEncoder(output).Encode(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message}); encodeErr != nil {
		return errors.Join(err, encodeErr)
	}
	if clidiag.CentralDiagnosticsEnabled(config.Context) {
		clidiag.MarkDiagnosticRendered(output)
	}
	return err
}

func invokeStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func safeRemoteServer(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "<remote>"
	}
	parsed.User = nil
	parsed.Path, parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", "", ""
	return parsed.String()
}
