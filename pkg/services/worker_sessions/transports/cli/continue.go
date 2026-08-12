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

// ContinueConfig holds the source identity and follow-up input accepted by
// the manifest-authored continue command. Local placement is the default;
// remote placement uses only the explicitly selected server.
type ContinueConfig struct {
	Context context.Context
	Server  string
	Remote  bool

	RequestID                string
	SourceWorkerSessionID    string
	SuccessorWorkerSessionID string
	FollowUpInput            string
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
	Local        LocalInvokeBoundary
	GenerateID   IDGenerator
}

// ContinueOperation is the composition-facing Worker Session continuation
// operation.
type ContinueOperation func(ContinueConfig) error

// BindContinue binds the exact remote protocol and local Worker Sessions
// boundary. A remote failure is returned to the caller and never falls back
// to the local boundary.
func BindContinue(transport clihttp.Protocol, local LocalInvokeBoundary, effects ...Effects) ContinueOperation {
	selected := selectEffects(effects)
	return func(config ContinueConfig) error {
		config.HTTP = transport
		if local != nil {
			config.Local = local
		}
		if config.GenerateID == nil {
			config.GenerateID = selected.GenerateID
		}
		return continueWorkerSession(config)
	}
}

// NewContinue returns a continuation operation with injected effects for
// focused CLI and functional tests.
func NewContinue(transport clihttp.Protocol, local LocalInvokeBoundary, effects ...Effects) ContinueOperation {
	return BindContinue(transport, local, effects...)
}

type normalizedContinueRequest struct {
	API     factoryapi.WorkerSessionContinueRequest
	Service workersessions.ContinueRequest
}

type continueResult struct {
	RequestID                  string `json:"requestId"`
	SourceWorkerSessionID      string `json:"sourceWorkerSessionId"`
	SuccessorWorkerSessionID   string `json:"successorWorkerSessionId"`
	PredecessorWorkerSessionID string `json:"predecessorWorkerSessionId"`
	Accepted                   bool   `json:"accepted"`
	State                      string `json:"state"`
	EventTopic                 string `json:"eventTopic,omitempty"`
	Observation                string `json:"observation"`
	Output                     string `json:"output,omitempty"`
	StructuredResult           any    `json:"structuredResult,omitempty"`
}

func continueWorkerSession(config ContinueConfig) error {
	jsonOutput := config.JSON || strings.EqualFold(strings.TrimSpace(config.OutputFormat), "json")
	if err := validateContinueConfig(config); err != nil {
		return emitContinueCLIError(config, jsonOutput, err)
	}
	format, err := normalizeOutputFormat(config.OutputFormat)
	if err != nil {
		return emitContinueCLIError(config, jsonOutput, err)
	}
	jsonOutput = config.JSON || format == "json"
	request, err := normalizeContinueRequest(config)
	if err != nil {
		return emitContinueCLIError(config, jsonOutput, err)
	}
	if config.Remote {
		return continueRemote(config, request, jsonOutput)
	}
	return continueLocal(config, request, jsonOutput)
}

func validateContinueConfig(config ContinueConfig) error {
	if config.Context == nil {
		return fmt.Errorf("context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if strings.TrimSpace(config.SourceWorkerSessionID) == "" {
		return newCLIError("WORKER_SESSION_CONTINUATION_INVALID", "source Worker Session identity is required", nil)
	}
	if config.Remote && config.HTTP == nil {
		return newCLIError("WORKER_SESSION_HTTP_UNAVAILABLE", "remote Worker Session continuation requires a CLI HTTP protocol", nil)
	}
	if !config.Remote && config.Local == nil {
		return newCLIError("WORKER_SESSION_LOCAL_UNAVAILABLE", "local Worker Sessions boundary is unavailable", nil)
	}
	return nil
}

func normalizeContinueRequest(config ContinueConfig) (normalizedContinueRequest, error) {
	followUp, err := resolveContinueInput(config)
	if err != nil {
		return normalizedContinueRequest{}, err
	}
	if strings.TrimSpace(followUp) == "" {
		return normalizedContinueRequest{}, newCLIError("WORKER_SESSION_INPUT_MISSING", "a non-empty follow-up input is required", nil)
	}
	requestID := strings.TrimSpace(config.RequestID)
	if requestID == "" {
		if config.GenerateID == nil {
			return normalizedContinueRequest{}, newCLIError("WORKER_SESSION_IDENTITY_UNAVAILABLE", "Worker Session continuation identity generator is unavailable", nil)
		}
		requestID = config.GenerateID()
	}
	successorID := strings.TrimSpace(config.SuccessorWorkerSessionID)
	if successorID == "" {
		if config.GenerateID == nil {
			return normalizedContinueRequest{}, newCLIError("WORKER_SESSION_IDENTITY_UNAVAILABLE", "Worker Session continuation identity generator is unavailable", nil)
		}
		successorID = config.GenerateID()
	}
	apiRequest := factoryapi.WorkerSessionContinueRequest{
		RequestId: requestID, SuccessorWorkerSessionId: successorID, FollowUpInput: followUp,
	}
	serviceRequest, err := workersessionshttp.WorkerSessionContinueRequestFromAPI(config.SourceWorkerSessionID, apiRequest)
	if err != nil {
		return normalizedContinueRequest{}, newCLIError("WORKER_SESSION_CONTINUATION_INVALID", "invalid Worker Session continuation request", err)
	}
	return normalizedContinueRequest{API: apiRequest, Service: serviceRequest}, nil
}

func resolveContinueInput(config ContinueConfig) (string, error) {
	if config.FollowUpInput != "" {
		return config.FollowUpInput, nil
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
	data, err := io.ReadAll(config.Stdin)
	if err != nil {
		return "", newCLIError("WORKER_SESSION_INPUT_FAILED", "failed to read Worker Session follow-up input from stdin", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func continueLocal(config ContinueConfig, request normalizedContinueRequest, jsonOutput bool) error {
	admitted, err := config.Local.Continue(config.Context, request.Service)
	if err != nil {
		return emitContinueCLIError(config, jsonOutput, mapContinueServiceError(err, config.Async))
	}
	result := continueAdmissionResult(request.Service, admitted)
	if config.Async {
		return writeContinueResult(config, jsonOutput, result, false)
	}
	capture, streamErr := waitLocalTerminal(config.Context, config.Local, admitted.Session.ID)
	if streamErr != nil {
		return emitContinueCLIError(config, jsonOutput, streamErr)
	}
	if capture.State == "" {
		capture.State = string(admitted.Session.State)
	}
	if err := terminalInvokeError(capture); err != nil {
		return emitContinueCLIError(config, jsonOutput, err)
	}
	return writeContinueResult(config, jsonOutput, continueResultFromCapture(result, capture), true)
}

func continueRemote(config ContinueConfig, request normalizedContinueRequest, jsonOutput bool) error {
	admitted, err := admitRemoteContinuation(config, request)
	if err != nil {
		return emitContinueCLIError(config, jsonOutput, err)
	}
	result := continueResult{
		RequestID:                  remoteContinueRequestID(admitted, request.Service.RequestID),
		SourceWorkerSessionID:      admitted.SourceWorkerSessionId,
		SuccessorWorkerSessionID:   admitted.SuccessorWorkerSessionId,
		PredecessorWorkerSessionID: admitted.PredecessorWorkerSessionId,
		Accepted:                   true,
		State:                      string(admitted.State),
		EventTopic:                 admitted.EventTopic,
		Observation:                observationGuidance(admitted.SuccessorWorkerSessionId),
	}
	if config.Async {
		return writeContinueResult(config, jsonOutput, result, false)
	}
	capture, err := waitRemoteContinuationTerminal(config, admitted.SuccessorWorkerSessionId)
	if err != nil {
		return emitContinueCLIError(config, jsonOutput, err)
	}
	if err := terminalInvokeError(capture); err != nil {
		return emitContinueCLIError(config, jsonOutput, err)
	}
	return writeContinueResult(config, jsonOutput, continueResultFromCapture(result, capture), true)
}

func waitRemoteContinuationTerminal(config ContinueConfig, workerSessionID string) (invokeCapture, error) {
	return waitRemoteTerminal(InvokeConfig{
		Context: config.Context, Server: config.Server, HTTP: config.HTTP,
		Diagnostics: config.Diagnostics, Verbose: config.Verbose, Debug: config.Debug,
	}, workerSessionID)
}

func continueAdmissionResult(request workersessions.ContinueRequest, admitted workersessions.ContinueResult) continueResult {
	return continueResult{
		RequestID: request.RequestID, SourceWorkerSessionID: request.SourceWorkerSessionID,
		SuccessorWorkerSessionID:   admitted.SuccessorWorkerSessionID,
		PredecessorWorkerSessionID: request.SourceWorkerSessionID, Accepted: true,
		State: string(admitted.Session.State), EventTopic: string(workersessions.Topic(admitted.Session.ID)),
		Observation: observationGuidance(admitted.Session.ID),
	}
}

func continueResultFromCapture(result continueResult, capture invokeCapture) continueResult {
	result.State = capture.State
	if capture.HasOutput {
		result.Output = capture.Output
	}
	if capture.HasStructured {
		result.StructuredResult = capture.StructuredResult
	}
	return result
}

func admitRemoteContinuation(config ContinueConfig, request normalizedContinueRequest) (factoryapi.WorkerSessionContinueResponse, error) {
	endpoint, err := continueEndpoint(config.Server, request.Service.SourceWorkerSessionID)
	if err != nil {
		return factoryapi.WorkerSessionContinueResponse{}, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "remote Worker Session continuation endpoint is invalid", err)
	}
	body, err := json.Marshal(request.API)
	if err != nil {
		return factoryapi.WorkerSessionContinueResponse{}, newCLIError("WORKER_SESSION_CONTINUATION_INVALID", "failed to encode Worker Session continuation request", err)
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions continue request placement=remote endpointPath=%s requestID=%s sourceWorkerSessionID=%s successorWorkerSessionID=%s async=%t",
		endpoint.Path, request.Service.RequestID, request.Service.SourceWorkerSessionID, request.Service.SuccessorWorkerSessionID, config.Async)
	httpRequest, err := http.NewRequestWithContext(config.Context, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return factoryapi.WorkerSessionContinueResponse{}, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "failed to build remote Worker Session continuation request", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	response, requestErr := config.HTTP.Execute(httpRequest)
	if requestErr != nil {
		return factoryapi.WorkerSessionContinueResponse{}, remoteContinueTransportError(config, requestErr)
	}
	if response.HTTP == nil {
		return factoryapi.WorkerSessionContinueResponse{}, newCLIError("FACTORY_UNREACHABLE", "remote Worker Session continuation returned no HTTP response", nil)
	}
	defer response.HTTP.Body.Close()
	if response.HTTP.StatusCode != http.StatusAccepted {
		return factoryapi.WorkerSessionContinueResponse{}, remoteContinueHTTPError(response.HTTP, response.HTTP.StatusCode)
	}
	var admitted factoryapi.WorkerSessionContinueResponse
	if err := json.NewDecoder(response.HTTP.Body).Decode(&admitted); err != nil {
		return factoryapi.WorkerSessionContinueResponse{}, newCLIError("WORKER_SESSION_CONTINUATION_FAILED", "remote Worker Session continuation response is invalid", err)
	}
	if !admitted.Accepted || strings.TrimSpace(admitted.SourceWorkerSessionId) == "" || strings.TrimSpace(admitted.SuccessorWorkerSessionId) == "" {
		return factoryapi.WorkerSessionContinueResponse{}, newCLIError("WORKER_SESSION_CONTINUATION_ADMISSION_FAILED", "remote Worker Session continuation was not admitted", nil)
	}
	return admitted, nil
}

func continueEndpoint(server, sourceWorkerSessionID string) (*url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, sessionpath.TopLevelWorkerSessionContinuePath(sourceWorkerSessionID))
	if err != nil {
		return nil, err
	}
	return parseInvokeURL(endpointURL)
}

func remoteContinueRequestID(response factoryapi.WorkerSessionContinueResponse, fallback string) string {
	if requestID := strings.TrimSpace(response.RequestId); requestID != "" {
		return requestID
	}
	return fallback
}

func remoteContinueTransportError(config ContinueConfig, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(config.Context.Err(), context.Canceled) {
		return newCLIError("WORKER_SESSION_CONTINUATION_INTERRUPTED", "remote Worker Session continuation was interrupted", context.Canceled)
	}
	return newCLIError("FACTORY_UNREACHABLE", fmt.Sprintf("factory not reachable at %s", safeRemoteServer(config.Server)), err)
}

func remoteContinueHTTPError(response *http.Response, status int) error {
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		code := strings.TrimSpace(string(apiError.Code))
		if code == "" {
			code = "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED"
		}
		return newCLIError(code, apiError.Message, nil)
	}
	return newCLIError("WORKER_SESSION_CONTINUATION_ADMISSION_FAILED", fmt.Sprintf("remote Worker Session continuation failed (%d)", status), nil)
}

func mapContinueServiceError(err error, _ bool) error {
	if errors.Is(err, context.Canceled) {
		return newCLIError("WORKER_SESSION_CONTINUATION_INTERRUPTED", "Worker Session continuation was interrupted", context.Canceled)
	}
	switch {
	case errors.Is(err, workersessions.ErrInvalidContinuationRequestID),
		errors.Is(err, workersessions.ErrInvalidContinuationLineage),
		errors.Is(err, workersessions.ErrInvalidContinuationInput):
		return newCLIError("BAD_REQUEST", "invalid Worker Session continuation request", err)
	case errors.Is(err, workersessions.ErrContinuationSourceNotFound):
		return newCLIError("NOT_FOUND", "Worker Session continuation source not found", err)
	case errors.Is(err, workersessions.ErrContinuationRequestIDConflict):
		return newCLIError("WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT", "Worker Session continuation requestId was reused with different inputs", err)
	case errors.Is(err, workersessions.ErrContinuationSourceActive),
		errors.Is(err, workersessions.ErrContinuationSourceConflict),
		errors.Is(err, workersessions.ErrContinuationSuccessorConflict):
		return newCLIError("WORKER_SESSION_CONTINUATION_CONFLICT", "Worker Session continuation conflicts with existing state", err)
	case errors.Is(err, workersessions.ErrContinuationProviderSessionMissing),
		errors.Is(err, workersessions.ErrContinuationProviderSessionInvalid):
		return newCLIError("WORKER_SESSION_PROVIDER_CONTINUATION_INVALID", "recorded Provider Session cannot be continued", err)
	case errors.Is(err, workersessions.ErrContinuationExecutionUnavailable),
		errors.Is(err, workersessions.ErrContinuationNotAccepted),
		errors.Is(err, workersessions.ErrContinuationServerStopping),
		errors.Is(err, workersessions.ErrEventTopicUnavailable),
		errors.Is(err, workersessions.ErrStartOpeningPublication):
		return newCLIError("WORKER_SESSION_CONTINUATION_ADMISSION_FAILED", "Workers could not admit the Worker Session continuation", err)
	default:
		return newCLIError("INTERNAL_ERROR", "failed to continue Worker Session", err)
	}
}

func writeContinueResult(config ContinueConfig, jsonOutput bool, result continueResult, synchronous bool) error {
	if jsonOutput {
		return json.NewEncoder(config.Output).Encode(result)
	}
	if synchronous {
		if _, err := fmt.Fprintf(config.Output,
			"Worker Session continuation completed\nSource Worker Session ID:\t%s\nSuccessor Worker Session ID:\t%s\nState:\t%s\nObserve:\t%s\n",
			result.SourceWorkerSessionID, result.SuccessorWorkerSessionID, result.State, result.Observation); err != nil {
			return newCLIError("WORKER_SESSION_OUTPUT_FAILED", "failed to write Worker Session continuation result", err)
		}
		if result.Output != "" {
			if _, err := io.WriteString(config.Output, result.Output); err != nil {
				return newCLIError("WORKER_SESSION_OUTPUT_FAILED", "failed to write Worker Session continuation output", err)
			}
			if !strings.HasSuffix(result.Output, "\n") {
				_, err := io.WriteString(config.Output, "\n")
				return err
			}
		}
		return nil
	}
	_, err := fmt.Fprintf(config.Output,
		"Worker Session continuation admitted\nRequest ID:\t%s\nSource Worker Session ID:\t%s\nSuccessor Worker Session ID:\t%s\nState:\t%s\nObserve:\t%s\n",
		result.RequestID, result.SourceWorkerSessionID, result.SuccessorWorkerSessionID, result.State, result.Observation)
	return err
}

func emitContinueCLIError(config ContinueConfig, jsonOutput bool, err error) error {
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
	code := "WORKER_SESSION_CONTINUATION_FAILED"
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
