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
	data, err := readBoundedWorkerSessionStdin(
		config.Stdin,
		maxWorkerSessionMessageStdinBytes,
		"Worker Session follow-up input stdin",
		"use --user-message or positional input for larger input",
	)
	if err != nil {
		return "", newCLIError("WORKER_SESSION_INPUT_FAILED", fmt.Sprintf("failed to read Worker Session follow-up input from stdin: %v", err), err)
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

// LocalControlBoundary is the exact local Worker Sessions seam used by the
// pause, resume, cancel, and terminate commands. Local placement never turns
// a control into HTTP; production wiring supplies the already-composed root.
type LocalControlBoundary interface {
	Pause(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
	Resume(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
	Cancel(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
	Terminate(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
}

// ControlConfig holds one exact Worker Session lifecycle control. The action
// is selected by the manifest command and is never accepted from a request
// body or remote fallback path.
type ControlConfig struct {
	Context         context.Context
	Server          string
	Remote          bool
	WorkerSessionID string
	Action          workersessions.ControlAction

	OutputFormat string
	JSON         bool
	Verbose      bool
	Debug        bool
	Output       io.Writer
	Diagnostics  io.Writer
	HTTP         clihttp.Protocol
	Local        LocalControlBoundary
}

// ControlOperation is the composition-facing Worker Session lifecycle
// operation shared by the four manifest-authored control commands.
type ControlOperation func(ControlConfig) error

// BindControl binds the exact remote protocol and local Worker Sessions
// control boundary. A remote failure is returned to the caller and never
// falls back to the local boundary.
func BindControl(transport clihttp.Protocol, local LocalControlBoundary) ControlOperation {
	return func(config ControlConfig) error {
		config.HTTP = transport
		if local != nil {
			config.Local = local
		}
		return controlWorkerSession(config)
	}
}

// NewControl returns a control operation with injected effects for focused
// CLI and functional tests.
func NewControl(transport clihttp.Protocol, local LocalControlBoundary) ControlOperation {
	return BindControl(transport, local)
}

func controlWorkerSession(config ControlConfig) error {
	jsonOutput := config.JSON || strings.EqualFold(strings.TrimSpace(config.OutputFormat), "json")
	if err := validateControlConfig(config); err != nil {
		return emitControlCLIError(config, jsonOutput, err)
	}
	format, err := normalizeOutputFormat(config.OutputFormat)
	if err != nil {
		return emitControlCLIError(config, jsonOutput, err)
	}
	jsonOutput = config.JSON || format == "json"
	if config.Remote {
		return controlRemote(config, jsonOutput)
	}
	return controlLocal(config, jsonOutput)
}

// errControlOwnerUnreachable marks a control request that never reached a
// factory server. Reaching no server is not an answer about the addressed
// Worker Session, so a re-addressed local miss keeps the local root's answer
// instead of reporting the transport failure.
var errControlOwnerUnreachable = errors.New("factory server is unreachable")

// controlOwnerAddressable reports whether a factory server address exists to
// re-address a Worker Session the in-process root does not own. Every CLI
// invocation carries one, because --server defaults to the shared local URI,
// so this only rules out a caller that supplied no protocol or address at all.
// Whether a server is actually there is decided by the request outcome, not by
// the presence of an address.
func controlOwnerAddressable(config ControlConfig) bool {
	return config.HTTP != nil && strings.TrimSpace(config.Server) != ""
}

func validateControlConfig(config ControlConfig) error {
	if config.Context == nil {
		return fmt.Errorf("context is required")
	}
	if config.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if strings.TrimSpace(config.WorkerSessionID) == "" {
		return newCLIError("WORKER_SESSION_CONTROL_INVALID", "Worker Session identity is required", nil)
	}
	if !validControlAction(config.Action) {
		return newCLIError("WORKER_SESSION_CONTROL_INVALID", "Worker Session control action is invalid", nil)
	}
	if config.Remote && config.HTTP == nil {
		return newCLIError("WORKER_SESSION_HTTP_UNAVAILABLE", "remote Worker Session control requires a CLI HTTP protocol", nil)
	}
	if !config.Remote && config.Local == nil {
		return newCLIError("WORKER_SESSION_LOCAL_UNAVAILABLE", "local Worker Sessions control boundary is unavailable", nil)
	}
	return nil
}

func validControlAction(action workersessions.ControlAction) bool {
	switch action {
	case workersessions.ControlActionPause, workersessions.ControlActionResume,
		workersessions.ControlActionCancel, workersessions.ControlActionTerminate:
		return true
	default:
		return false
	}
}

// controlLocal applies the control through the in-process Worker Sessions root
// and, when that root does not own the addressed session, re-addresses the
// exact same stable Worker Session ID to the configured factory server.
//
// The inspection commands (list, show, read, stream) are server-addressed only,
// so every Worker Session an operator can observe is owned by that server while
// the CLI process builds its own empty root per invocation. Resolving a control
// solely against the local root therefore reported NOT_FOUND for every
// observable session, including a RUNNING one, regardless of whether it had
// published a Provider Session yet.
//
// This is deliberately the opposite direction from the remote path, which never
// falls back to the local boundary: a remote answer is authoritative and must
// not be masked by a different root. A local ErrSessionNotFound is not an
// answer about the session at all -- it only reports that this process never
// ran it -- so continuing to its owner resolves the same identity rather than
// substituting a different one.
//
// Re-addressing may only replace the local answer when a factory server
// actually answered. --server always defaults to the shared local URI, so an
// address exists even when nothing is listening there; a direct-mode process
// owns its own Worker Sessions, and an unknown identity is genuinely NOT_FOUND
// rather than a transport failure. When the re-addressed request reaches no
// server, the local owner's answer therefore stands.
func controlLocal(config ControlConfig, jsonOutput bool) error {
	result, localErr := applyLocalControl(config)
	if localErr == nil {
		return writeControlResult(config, jsonOutput, controlResultFromService(result))
	}
	if !errors.Is(localErr, workersessions.ErrSessionNotFound) || !controlOwnerAddressable(config) {
		return emitControlCLIError(config, jsonOutput, mapControlServiceError(localErr))
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions control request placement=local-miss action=%s workerSessionID=%s re-addressing=server",
		config.Action, config.WorkerSessionID)
	remoteResult, remoteErr := requestRemoteControl(config)
	if remoteErr == nil {
		return writeControlResult(config, jsonOutput, remoteResult)
	}
	if errors.Is(remoteErr, errControlOwnerUnreachable) {
		clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
			"worker sessions control request placement=local-miss action=%s workerSessionID=%s owner=unreachable keeping=local-not-found",
			config.Action, config.WorkerSessionID)
		return emitControlCLIError(config, jsonOutput, mapControlServiceError(localErr))
	}
	return emitControlCLIError(config, jsonOutput, remoteErr)
}

func applyLocalControl(config ControlConfig) (workersessions.ControlResult, error) {
	request := workersessions.ControlRequest{ID: strings.TrimSpace(config.WorkerSessionID)}
	switch config.Action {
	case workersessions.ControlActionPause:
		return config.Local.Pause(config.Context, request)
	case workersessions.ControlActionResume:
		return config.Local.Resume(config.Context, request)
	case workersessions.ControlActionCancel:
		return config.Local.Cancel(config.Context, request)
	case workersessions.ControlActionTerminate:
		return config.Local.Terminate(config.Context, request)
	default:
		return workersessions.ControlResult{}, workersessions.ErrInvalidState
	}
}

func controlRemote(config ControlConfig, jsonOutput bool) error {
	result, err := requestRemoteControl(config)
	if err != nil {
		return emitControlCLIError(config, jsonOutput, err)
	}
	return writeControlResult(config, jsonOutput, result)
}

// requestRemoteControl applies one control through the factory server and
// returns the server's snapshot or a classified failure. It reports rather
// than renders the failure so a re-addressed local miss can tell an answering
// server apart from one that was never reached.
func requestRemoteControl(config ControlConfig) (factoryapi.WorkerSessionControlResponse, error) {
	endpoint, err := controlEndpoint(config.Server, config.WorkerSessionID, config.Action)
	if err != nil {
		return factoryapi.WorkerSessionControlResponse{}, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "remote Worker Session control endpoint is invalid", err)
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions control request placement=remote action=%s endpointPath=%s workerSessionID=%s",
		config.Action, endpoint.Path, config.WorkerSessionID)
	request, err := http.NewRequestWithContext(config.Context, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return factoryapi.WorkerSessionControlResponse{}, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "failed to build remote Worker Session control request", err)
	}
	request.Header.Set("Accept", "application/json")
	response, requestErr := config.HTTP.Execute(request)
	if requestErr != nil {
		return factoryapi.WorkerSessionControlResponse{}, remoteControlTransportError(config, requestErr)
	}
	if response.HTTP == nil {
		return factoryapi.WorkerSessionControlResponse{}, newCLIError("FACTORY_UNREACHABLE",
			"remote Worker Session control returned no HTTP response", errControlOwnerUnreachable)
	}
	defer response.HTTP.Body.Close()
	if response.HTTP.StatusCode != http.StatusOK {
		return factoryapi.WorkerSessionControlResponse{}, remoteControlHTTPError(config.Action, response.HTTP)
	}
	var apiResult factoryapi.WorkerSessionControlResponse
	if err := json.NewDecoder(response.HTTP.Body).Decode(&apiResult); err != nil {
		return factoryapi.WorkerSessionControlResponse{}, newCLIError("WORKER_SESSION_CONTROL_FAILED", "remote Worker Session control response is invalid", err)
	}
	if err := validateRemoteControlResult(apiResult, config.WorkerSessionID, config.Action); err != nil {
		return factoryapi.WorkerSessionControlResponse{}, err
	}
	return apiResult, nil
}

func controlEndpoint(server, workerSessionID string, action workersessions.ControlAction) (*url.URL, error) {
	var path string
	switch action {
	case workersessions.ControlActionPause:
		path = sessionpath.TopLevelWorkerSessionPausePath(workerSessionID)
	case workersessions.ControlActionResume:
		path = sessionpath.TopLevelWorkerSessionResumePath(workerSessionID)
	case workersessions.ControlActionCancel:
		path = sessionpath.TopLevelWorkerSessionCancelPath(workerSessionID)
	case workersessions.ControlActionTerminate:
		path = sessionpath.TopLevelWorkerSessionTerminatePath(workerSessionID)
	default:
		return nil, fmt.Errorf("unsupported Worker Session control action %q", action)
	}
	endpoint, err := cliserver.RequestURL(server, path)
	if err != nil {
		return nil, err
	}
	return parseInvokeURL(endpoint)
}

func validateRemoteControlResult(
	result factoryapi.WorkerSessionControlResponse,
	workerSessionID string,
	action workersessions.ControlAction,
) error {
	if strings.TrimSpace(result.WorkerSessionId) != strings.TrimSpace(workerSessionID) ||
		workersessions.ControlAction(result.Action) != action ||
		!validControlOutcome(workersessions.ControlOutcome(result.Outcome)) ||
		!workersessions.State(result.State).Valid() {
		return newCLIError("WORKER_SESSION_CONTROL_FAILED", "remote Worker Session control returned a mismatched result", nil)
	}
	return nil
}

func validControlOutcome(outcome workersessions.ControlOutcome) bool {
	switch outcome {
	case workersessions.ControlOutcomeApplied, workersessions.ControlOutcomeNoop,
		workersessions.ControlOutcomeUnsupported, workersessions.ControlOutcomeFailed:
		return true
	default:
		return false
	}
}

func controlResultFromService(result workersessions.ControlResult) factoryapi.WorkerSessionControlResponse {
	return factoryapi.WorkerSessionControlResponse{
		WorkerSessionId: result.Session.ID,
		Action:          factoryapi.WorkerSessionControlResponseAction(result.Action),
		Outcome:         factoryapi.WorkerSessionControlResponseOutcome(result.Outcome),
		State:           factoryapi.WorkerSessionControlResponseState(result.Session.State),
		DispatchId:      result.DispatchID,
	}
}

func mapControlServiceError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return newCLIError("WORKER_SESSION_CONTROL_INTERRUPTED", "Worker Session control was interrupted", err)
	}
	switch {
	case errors.Is(err, workersessions.ErrInvalidSessionID):
		return newCLIError("WORKER_SESSION_CONTROL_INVALID", "invalid Worker Session identity", err)
	case errors.Is(err, workersessions.ErrSessionNotFound):
		return newCLIError("NOT_FOUND", "Worker Session not found", err)
	case errors.Is(err, workersessions.ErrInvalidState),
		errors.Is(err, workersessions.ErrProviderSessionAssociationAttemptMismatch),
		errors.Is(err, workersessions.ErrProviderSessionAssociationNotAvailable),
		errors.Is(err, workersessions.ErrProviderSessionAssociationMissing),
		errors.Is(err, workersessions.ErrProviderSessionAssociationConflict),
		errors.Is(err, workersessions.ErrInvalidProviderSessionAssociation):
		return newCLIError("WORKER_SESSION_CONTROL_CONFLICT", "Worker Session control conflicts with current state", err)
	default:
		return newCLIError("WORKER_SESSION_CONTROL_FAILED", "failed to apply Worker Session control", err)
	}
}

func remoteControlTransportError(config ControlConfig, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(config.Context.Err(), context.Canceled) || errors.Is(config.Context.Err(), context.DeadlineExceeded) {
		return newCLIError("WORKER_SESSION_CONTROL_INTERRUPTED", "remote Worker Session control was interrupted", err)
	}
	return newCLIError("FACTORY_UNREACHABLE", fmt.Sprintf("factory not reachable at %s", safeRemoteServer(config.Server)),
		errors.Join(errControlOwnerUnreachable, err))
}

func remoteControlHTTPError(action workersessions.ControlAction, response *http.Response) error {
	if apiError, ok := clihttp.DecodeAPIError(response); ok {
		code := strings.TrimSpace(string(apiError.Code))
		if code == "" {
			code = controlHTTPErrorCode(response.StatusCode)
		} else if code == "INTERNAL_ERROR" {
			code = "WORKER_SESSION_CONTROL_FAILED"
		}
		return newCLIError(code, apiError.Message, nil)
	}
	return newCLIError(controlHTTPErrorCode(response.StatusCode), fmt.Sprintf("remote Worker Session %s failed (%d)", strings.ToLower(string(action)), response.StatusCode), nil)
}

func controlHTTPErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "WORKER_SESSION_CONTROL_INVALID"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "WORKER_SESSION_CONTROL_CONFLICT"
	case http.StatusServiceUnavailable:
		return "WORKER_SESSION_CONTROL_FAILED"
	case http.StatusInternalServerError:
		return "WORKER_SESSION_CONTROL_FAILED"
	default:
		return "WORKER_SESSION_CONTROL_FAILED"
	}
}

func writeControlResult(config ControlConfig, jsonOutput bool, result factoryapi.WorkerSessionControlResponse) error {
	if jsonOutput {
		return json.NewEncoder(config.Output).Encode(result)
	}
	if _, err := fmt.Fprintf(config.Output,
		"Worker Session %s %s\nWorker Session ID:\t%s\nAction:\t%s\nOutcome:\t%s\nState:\t%s\nDispatch ID:\t%s\n",
		strings.ToLower(string(result.Outcome)), strings.ToLower(string(result.Action)),
		result.WorkerSessionId, result.Action, result.Outcome, result.State, result.DispatchId); err != nil {
		return newCLIError("WORKER_SESSION_OUTPUT_FAILED", "failed to write Worker Session control result", err)
	}
	return nil
}

func emitControlCLIError(config ControlConfig, jsonOutput bool, err error) error {
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
	}{Code: "WORKER_SESSION_CONTROL_FAILED", Message: err.Error()}
	var typed *CLIError
	if errors.As(err, &typed) && typed != nil {
		if typed.Code != "" {
			payload.Code = typed.Code
		}
		if typed.Message != "" {
			payload.Message = typed.Message
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
