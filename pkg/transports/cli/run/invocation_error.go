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
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factoryconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

const (
	InvocationErrorCodeFailed       = factoryruntimecli.InvocationErrorCodeFailed
	InvocationErrorCodeCancelled    = factoryruntimecli.InvocationErrorCodeCancelled
	InvocationErrorCodeTimeout      = factoryruntimecli.InvocationErrorCodeTimeout
	CurrentFactoryNotFoundCode      = factoryruntimecli.CurrentFactoryNotFoundCode
	CurrentFactoryInvalidCode       = factoryruntimecli.CurrentFactoryInvalidCode
	InvocationOutputConflictCode    = factoryruntimecli.InvocationOutputConflictCode
	InvocationOutputUnsupportedCode = factoryruntimecli.InvocationOutputUnsupportedCode
	RemoteLocalHostingConflictCode  = factoryruntimecli.RemoteLocalHostingConflictCode
	ServerBindFailedCode            = factoryruntimecli.ServerBindFailedCode
	InvocationOutputPrimaryResult   = factoryruntimecli.InvocationOutputPrimaryResult
	InvocationOutputResponseStream  = factoryruntimecli.InvocationOutputResponseStream
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

// MapServerFailure classifies terminal listener binding failures at the CLI
// boundary while preserving all other errors for their owning mapper.
func MapServerFailure(err error) error {
	return factoryruntimecli.MapServerFailure(err)
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
	return runtimeCLIService(cfg).ValidateInvocationOutputMode(factoryruntimecli.ValidateInvocationOutputModeRequest{
		InvocationOutputMode: cfg.InvocationOutputMode,
		Continuously:         cfg.Continuously,
		InvocationMode:       invocationMode,
	})
}

func isResponseStreamOutputMode(mode string) bool {
	return strings.TrimSpace(mode) == InvocationOutputResponseStream
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
			Code:    RemoteDurableRequestInvalidCode,
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

func waitForRemoteInvocationResult(
	ctx context.Context,
	cfg RunConfig,
	server string,
	start factoryapi.FactorySessionExecutionResponse,
	requestID string,
	operation RemoteInvocationResultOperation,
) (apisurface.FactoryInvocationResult, error) {
	if ctx == nil {
		return apisurface.FactoryInvocationResult{}, &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: "wait for remote durable result: context is required",
		}
	}
	if operation == nil {
		return apisurface.FactoryInvocationResult{}, &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: "wait for remote durable result: operation is required",
		}
	}
	for {
		result, err := operation.GetFactorySessionResult(ctx, RemoteInvocationResultRequest{
			Server:      server,
			SessionID:   start.SessionId,
			Diagnostics: cfg.Diagnostics,
			Verbose:     cfg.Verbose,
		})
		if err != nil {
			return apisurface.FactoryInvocationResult{}, err
		}
		invocationResult, ready, poll, err := remoteInvocationResultFromDurable(
			result,
			start.SessionId,
			requestID,
		)
		if err != nil {
			return apisurface.FactoryInvocationResult{}, err
		}
		if ready {
			return invocationResult, nil
		}
		if !poll {
			return apisurface.FactoryInvocationResult{}, &InvocationError{
				Code:    RemoteDurableResponseInvalidCode,
				Message: "remote durable result ended without a terminal classification",
			}
		}

		timer := time.NewTimer(remoteDurableResultPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			endpoint := safeRemoteEndpoint(server)
			return apisurface.FactoryInvocationResult{}, &InvocationError{
				Code: RemoteDurableResultCode,
				Message: fmt.Sprintf(
					"remote durable result wait canceled at %s: %v",
					endpoint,
					ctx.Err(),
				),
				Cause: ctx.Err(),
			}
		case <-timer.C:
		}
	}
}

func remoteInvocationResultFromDurable(
	result factoryapi.FactorySessionResult,
	expectedSessionID string,
	requestID string,
) (apisurface.FactoryInvocationResult, bool, bool, error) {
	expectedSessionID = strings.TrimSpace(expectedSessionID)
	actualSessionID := strings.TrimSpace(result.SessionId)
	if expectedSessionID == "" || actualSessionID == "" || actualSessionID != expectedSessionID {
		return apisurface.FactoryInvocationResult{}, false, false, &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: "remote durable result returned a session identity different from the accepted session",
		}
	}
	if strings.TrimSpace(string(result.ResultStatus)) == "" {
		return apisurface.FactoryInvocationResult{}, false, false, &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: fmt.Sprintf("remote durable result for session %s has no result status", actualSessionID),
		}
	}

	parts := contentcontract.PartsFromGenerated(result.PrimaryResult)
	status, code, message, terminalFailure := remoteDurableFailureClassification(result)
	if terminalFailure {
		return remoteDurableInvocationFailure(
			requestID,
			actualSessionID,
			status,
			code,
			message,
			parts,
		), true, false, nil
	}

	if result.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
		if len(parts) == 0 {
			return remoteDurableInvocationFailure(
				requestID,
				actualSessionID,
				interfaces.InvocationTerminalStatusFailed,
				string(factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED),
				remoteDurableResultMessage(result, "primary result could not be resolved"),
				nil,
			), true, false, nil
		}
		return apisurface.FactoryInvocationResult{
			RequestID:     strings.TrimSpace(requestID),
			Status:        interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: parts,
			SessionID:     actualSessionID,
		}, true, false, nil
	}

	if remoteDurableResultShouldPoll(result) {
		return apisurface.FactoryInvocationResult{}, false, true, nil
	}

	return remoteDurableInvocationFailure(
		requestID,
		actualSessionID,
		interfaces.InvocationTerminalStatusFailed,
		string(factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED),
		remoteDurableResultMessage(result, "primary result could not be resolved"),
		parts,
	), true, false, nil
}

func remoteDurableInvocationFailure(
	requestID string,
	sessionID string,
	status interfaces.InvocationTerminalStatus,
	code string,
	message string,
	parts []work.WorkContentPart,
) apisurface.FactoryInvocationResult {
	return apisurface.FactoryInvocationResult{
		RequestID:     strings.TrimSpace(requestID),
		Status:        status,
		PrimaryResult: parts,
		ErrorCode:     strings.TrimSpace(code),
		Message:       strings.TrimSpace(message),
		SessionID:     strings.TrimSpace(sessionID),
	}
}

func remoteDurableFailureClassification(
	result factoryapi.FactorySessionResult,
) (interfaces.InvocationTerminalStatus, string, string, bool) {
	lifecycle := remoteDurableLifecycleStatus(result.SessionStatus)
	switch lifecycle {
	case string(factoryapi.FactorySessionDurableLifecycleStatusAwaitingApproval):
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONNEEDSHUMAN),
			remoteDurableResultMessage(result, "factory session is awaiting human approval"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusPaused):
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONPAUSED),
			remoteDurableResultMessage(result, "factory session is paused; resume the session to continue waiting for the primary result"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusTimedOut):
		return interfaces.InvocationTerminalStatusTimedOut,
			string(factoryapi.INVOCATIONTIMEDOUT),
			remoteDurableResultMessage(result, "invocation timed out while waiting for primary result"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusCanceled):
		return interfaces.InvocationTerminalStatusCanceled,
			string(factoryapi.INVOCATIONCANCELED),
			remoteDurableResultMessage(result, "invocation was canceled while waiting for primary result"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusInterrupted), string(factoryapi.FactorySessionDurableLifecycleStatusTerminated):
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONINTERRUPTED),
			remoteDurableResultMessage(result, "invocation was interrupted before the primary result was available"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusFailed):
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONRUNTIMEFAILURE),
			remoteDurableResultMessage(result, "remote Factory Session failed before the primary result was available"),
			true
	}

	for _, rawReason := range []string{remoteDurableAvailabilityReason(result), remoteDurableFailureReason(result)} {
		reason := strings.ToUpper(strings.TrimSpace(rawReason))
		switch {
		case strings.Contains(reason, "BLOCKED"):
			return interfaces.InvocationTerminalStatusFailed,
				string(factoryapi.INVOCATIONBLOCKED),
				remoteDurableResultMessage(result, "invocation was blocked before the primary result was available"),
				true
		case strings.Contains(reason, "NEEDS_HUMAN"), strings.Contains(reason, "NEEDS-HUMAN"), strings.Contains(reason, "APPROVAL"):
			return interfaces.InvocationTerminalStatusFailed,
				string(factoryapi.INVOCATIONNEEDSHUMAN),
				remoteDurableResultMessage(result, "factory session is awaiting human approval"),
				true
		case strings.Contains(reason, "PAUSED"):
			return interfaces.InvocationTerminalStatusFailed,
				string(factoryapi.INVOCATIONPAUSED),
				remoteDurableResultMessage(result, "factory session is paused; resume the session to continue waiting for the primary result"),
				true
		case strings.Contains(reason, "TIMEOUT"), strings.Contains(reason, "TIMED_OUT"):
			return interfaces.InvocationTerminalStatusTimedOut,
				string(factoryapi.INVOCATIONTIMEDOUT),
				remoteDurableResultMessage(result, "invocation timed out while waiting for primary result"),
				true
		case strings.Contains(reason, "CANCELED"), strings.Contains(reason, "CANCELLED"):
			return interfaces.InvocationTerminalStatusCanceled,
				string(factoryapi.INVOCATIONCANCELED),
				remoteDurableResultMessage(result, "invocation was canceled while waiting for primary result"),
				true
		case strings.Contains(reason, "INTERRUPT"), strings.Contains(reason, "TERMINAT"):
			return interfaces.InvocationTerminalStatusFailed,
				string(factoryapi.INVOCATIONINTERRUPTED),
				remoteDurableResultMessage(result, "invocation was interrupted before the primary result was available"),
				true
		}
	}

	if result.ResultStatus == factoryapi.FactorySessionResultStatusFailedWithPartial {
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONRUNTIMEFAILURE),
			remoteDurableResultMessage(result, "remote Factory Session failed before the primary result was available"),
			true
	}
	if result.ResultStatus == factoryapi.FactorySessionResultStatusPartial && lifecycle == string(factoryapi.FactorySessionDurableLifecycleStatusSucceeded) {
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED),
			remoteDurableResultMessage(result, "primary result could not be resolved"),
			true
	}
	if result.ResultStatus == factoryapi.FactorySessionResultStatusUnavailable && !remoteDurableResultShouldPoll(result) {
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED),
			remoteDurableResultMessage(result, "primary result could not be resolved"),
			true
	}
	return "", "", "", false
}

func remoteDurableResultShouldPoll(result factoryapi.FactorySessionResult) bool {
	if result.Availability != nil && result.Availability.Retryable != nil && !*result.Availability.Retryable {
		return false
	}
	lifecycle := remoteDurableLifecycleStatus(result.SessionStatus)
	switch lifecycle {
	case "", string(factoryapi.FactorySessionDurableLifecycleStatusQueued), string(factoryapi.FactorySessionDurableLifecycleStatusRunning), string(factoryapi.FactorySessionDurableLifecycleStatusResuming), string(factoryapi.FactorySessionDurableLifecycleStatusCanceling):
		return result.ResultStatus == factoryapi.FactorySessionResultStatusNotReady || result.ResultStatus == factoryapi.FactorySessionResultStatusPartial || result.ResultStatus == factoryapi.FactorySessionResultStatusUnavailable
	default:
		return false
	}
}

func remoteDurableResultMessage(result factoryapi.FactorySessionResult, fallback string) string {
	if result.FailureDetail != nil && strings.TrimSpace(result.FailureDetail.Message) != "" {
		return strings.TrimSpace(result.FailureDetail.Message)
	}
	if result.Availability != nil && result.Availability.Message != nil && strings.TrimSpace(*result.Availability.Message) != "" {
		return strings.TrimSpace(*result.Availability.Message)
	}
	return fallback
}

func remoteDurableAvailabilityReason(result factoryapi.FactorySessionResult) string {
	if result.Availability == nil || result.Availability.Reason == nil {
		return ""
	}
	return *result.Availability.Reason
}

func remoteDurableFailureReason(result factoryapi.FactorySessionResult) string {
	if result.FailureDetail == nil {
		return ""
	}
	return string(result.FailureDetail.Reason)
}

func remoteDurableLifecycleStatus(status *factoryapi.FactorySessionDurableLifecycleStatus) string {
	if status == nil {
		return ""
	}
	return string(*status)
}

func writeRemoteInvocationResult(cfg RunConfig, result apisurface.FactoryInvocationResult) error {
	if cfg.Output == nil {
		cfg.Output = cfg.StartupOutput
	}
	if isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		if cfg.JSONOutput {
			if err := writeRemoteInvocationNDJSON(cfg.Output, result); err != nil {
				return err
			}
			if result.Status != interfaces.InvocationTerminalStatusCompleted {
				return invocationResultFailure(result)
			}
			return nil
		}
		if result.Status != interfaces.InvocationTerminalStatusCompleted {
			if err := writeRemoteInvocationHumanFailure(cfg.Output, result); err != nil {
				return err
			}
			return invocationResultFailure(result)
		}
	}
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return writeInvocationFailure(cfg, result, nil)
	}
	return writeInvocationSuccess(cfg, result, nil)
}

type remoteInvocationNDJSONRecord struct {
	RecordType string                        `json:"recordType"`
	Response   factoryapi.InvocationResponse `json:"response"`
}

func writeRemoteInvocationNDJSON(output io.Writer, result apisurface.FactoryInvocationResult) error {
	if output == nil {
		return fmt.Errorf("write remote invocation response stream: process output is required")
	}
	encoded, err := json.Marshal(remoteInvocationNDJSONRecord{
		RecordType: "invocation_result",
		Response:   apisurface.InvocationResponseFromResult(result),
	})
	if err != nil {
		return fmt.Errorf("marshal remote invocation terminal record: %w", err)
	}
	_, err = fmt.Fprintln(output, string(encoded))
	return err
}

func writeRemoteInvocationHumanFailure(output io.Writer, result apisurface.FactoryInvocationResult) error {
	if output == nil {
		return fmt.Errorf("write remote invocation outcome: process output is required")
	}
	if _, err := fmt.Fprintln(output, "--- invocation outcome ---"); err != nil {
		return err
	}
	lines := []string{"status: " + string(result.Status)}
	if code := strings.TrimSpace(result.ErrorCode); code != "" {
		lines = append(lines, "error: "+code)
	}
	if message := strings.TrimSpace(result.Message); message != "" {
		lines = append(lines, "message: "+message)
	}
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" {
		lines = append(lines, "session: "+sessionID)
	}
	if workID := strings.TrimSpace(result.WorkID); workID != "" {
		lines = append(lines, "workId: "+workID)
	}
	if workName := strings.TrimSpace(result.WorkName); workName != "" {
		lines = append(lines, "workName: "+workName)
	}
	if workState := strings.TrimSpace(result.WorkState); workState != "" {
		lines = append(lines, "workState: "+workState)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(output, line); err != nil {
			return err
		}
	}
	return nil
}

// ResolveFactoryInvocationRequest exposes the same request projection used by
// the local run operation. Callers must prepare the RunConfig through the
// shared Work input preparation boundary before invoking this helper.
func ResolveFactoryInvocationRequest(cfg RunConfig) (*factoryapi.InvocationRequest, bool, error) {
	return resolveFactoryInvocationRequest(cfg)
}

// RunRemoteInvocation starts one server-owned durable Factory Session through
// the selected remote adapter. It does not open local runtime state or use the
// live-session compatibility invocation route.
func RunRemoteInvocation(
	ctx context.Context,
	cfg RunConfig,
	server string,
	remote RemoteInvocationOperation,
) error {
	if remote == nil {
		return fmt.Errorf("run remote durable start: operation is required")
	}
	request, invocationMode, err := ResolveFactoryInvocationRequest(cfg)
	if err != nil {
		return err
	}
	if !invocationMode || request == nil {
		return &InvocationError{
			Code:    RemoteInvocationInputRequiredCode,
			Message: "--remote run requires invocation input; provide a normalized Factory target and invocation arguments",
		}
	}
	executionRequest, err := remoteDurableRequestFromRunConfig(cfg, *request)
	if err != nil {
		return err
	}
	response, err := remote.StartFactorySession(ctx, RemoteInvocationRequest{
		Server: server, Request: executionRequest, Diagnostics: cfg.Diagnostics, Verbose: cfg.Verbose,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(response.SessionId) == "" || strings.TrimSpace(string(response.Status)) == "" {
		return &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: "remote durable start returned no canonical session identity",
		}
	}
	resultOperation, ok := remote.(RemoteInvocationResultOperation)
	if !ok {
		// Keep the narrow injected start seam usable for placement-only callers;
		// the production HTTP adapter implements result retrieval and therefore
		// always follows the accepted session to an authoritative terminal fact.
		return writeRemoteDurableStartResult(cfg, response)
	}
	result, err := waitForRemoteInvocationResult(
		ctx,
		cfg,
		server,
		response,
		executionRequest.RequestId,
		resultOperation,
	)
	if err != nil {
		return err
	}
	return writeRemoteInvocationResult(cfg, result)
}

func safeRemoteEndpoint(raw string) string {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return invalidRemoteEndpointLabel
	}
	parsed.User = nil
	base, err := cliserver.ResolveBase(parsed.String())
	if err != nil {
		return invalidRemoteEndpointLabel
	}
	return base.String()
}
