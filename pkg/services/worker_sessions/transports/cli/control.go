package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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

func controlLocal(config ControlConfig, jsonOutput bool) error {
	request := workersessions.ControlRequest{ID: strings.TrimSpace(config.WorkerSessionID)}
	var (
		result workersessions.ControlResult
		err    error
	)
	switch config.Action {
	case workersessions.ControlActionPause:
		result, err = config.Local.Pause(config.Context, request)
	case workersessions.ControlActionResume:
		result, err = config.Local.Resume(config.Context, request)
	case workersessions.ControlActionCancel:
		result, err = config.Local.Cancel(config.Context, request)
	case workersessions.ControlActionTerminate:
		result, err = config.Local.Terminate(config.Context, request)
	}
	if err != nil {
		return emitControlCLIError(config, jsonOutput, mapControlServiceError(err))
	}
	return writeControlResult(config, jsonOutput, controlResultFromService(result))
}

func controlRemote(config ControlConfig, jsonOutput bool) error {
	endpoint, err := controlEndpoint(config.Server, config.WorkerSessionID, config.Action)
	if err != nil {
		return emitControlCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "remote Worker Session control endpoint is invalid", err))
	}
	clidiag.Printf(config.Diagnostics, config.Verbose || config.Debug,
		"worker sessions control request placement=remote action=%s endpointPath=%s workerSessionID=%s",
		config.Action, endpoint.Path, config.WorkerSessionID)
	request, err := http.NewRequestWithContext(config.Context, http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return emitControlCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_ENDPOINT_INVALID", "failed to build remote Worker Session control request", err))
	}
	request.Header.Set("Accept", "application/json")
	response, requestErr := config.HTTP.Execute(request)
	if requestErr != nil {
		return emitControlCLIError(config, jsonOutput, remoteControlTransportError(config, requestErr))
	}
	if response.HTTP == nil {
		return emitControlCLIError(config, jsonOutput, newCLIError("FACTORY_UNREACHABLE", "remote Worker Session control returned no HTTP response", nil))
	}
	defer response.HTTP.Body.Close()
	if response.HTTP.StatusCode != http.StatusOK {
		return emitControlCLIError(config, jsonOutput, remoteControlHTTPError(config.Action, response.HTTP))
	}
	var apiResult factoryapi.WorkerSessionControlResponse
	if err := json.NewDecoder(response.HTTP.Body).Decode(&apiResult); err != nil {
		return emitControlCLIError(config, jsonOutput, newCLIError("WORKER_SESSION_CONTROL_FAILED", "remote Worker Session control response is invalid", err))
	}
	if err := validateRemoteControlResult(apiResult, config.WorkerSessionID, config.Action); err != nil {
		return emitControlCLIError(config, jsonOutput, err)
	}
	return writeControlResult(config, jsonOutput, apiResult)
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
	return newCLIError("FACTORY_UNREACHABLE", fmt.Sprintf("factory not reachable at %s", safeRemoteServer(config.Server)), err)
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
