package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	defaultRemoteInvocationSessionID  = sessionpath.DefaultFactorySessionID
	invalidRemoteEndpointLabel        = "<invalid endpoint>"
)

// RemoteInvocationRequest is the normalized CLI transport request sent to a
// selected You server. Placement is resolved by the caller; this type carries
// only the endpoint and the already-normalized operation request.
type RemoteInvocationRequest struct {
	Server      string
	SessionID   string
	Request     factoryapi.InvocationRequest
	Diagnostics io.Writer
	Verbose     bool
}

// RemoteInvocationOperation is the injected HTTP adapter for one remote
// invocation. It deliberately does not expose a local service dependency.
type RemoteInvocationOperation interface {
	InvokeFactory(context.Context, RemoteInvocationRequest) (factoryapi.InvocationResponse, error)
}

type remoteInvocationClient struct {
	transport clihttp.Protocol
}

// NewRemoteInvocation binds the CLI HTTP protocol to the remote invocation
// operation. The protocol is the only external effect owned by this adapter.
func NewRemoteInvocation(transport clihttp.Protocol) RemoteInvocationOperation {
	return remoteInvocationClient{transport: transport}
}

func (client remoteInvocationClient) InvokeFactory(
	ctx context.Context,
	cfg RemoteInvocationRequest,
) (factoryapi.InvocationResponse, error) {
	if ctx == nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("remote invocation: context is required")
	}
	if client.transport == nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("remote invocation: CLI HTTP protocol is required")
	}
	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		sessionID = defaultRemoteInvocationSessionID
	}
	endpointPath := sessionpath.ScopedPath("/invocations", sessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf(
			"remote invocation endpoint %q is invalid",
			safeRemoteEndpoint(cfg.Server),
		)
	}
	body, err := json.Marshal(cfg.Request)
	if err != nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("marshal remote invocation request: %w", err)
	}
	endpointLabel := safeRemoteEndpoint(endpointURL)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"remote invocation request endpointPath=%s endpoint=%s session=%s requestBytes=%d",
		endpointPath,
		endpointLabel,
		clidiag.SessionLabel(sessionID),
		len(body),
	)

	var result factoryapi.InvocationResponse
	response, err := client.transport.PostJSON(
		ctx,
		endpointURL,
		bytes.NewReader(body),
		&result,
	)
	if err != nil {
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"remote invocation response endpointPath=%s error=unreachable durationMillis=%d",
			endpointPath,
			response.Duration.Milliseconds(),
		)
		return factoryapi.InvocationResponse{}, fmt.Errorf(
			"remote invocation failed at %s: %w",
			endpointLabel,
			err,
		)
	}
	if response.HTTP == nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("remote invocation failed at %s: HTTP response is unavailable", endpointLabel)
	}
	if response.HTTP.Body != nil {
		defer response.HTTP.Body.Close()
	}
	if response.HTTP.StatusCode != http.StatusOK {
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"remote invocation response endpointPath=%s status=%d durationMillis=%d",
			endpointPath,
			response.HTTP.StatusCode,
			response.Duration.Milliseconds(),
		)
		if apiError, ok := clihttp.DecodeAPIError(response.HTTP); ok {
			return factoryapi.InvocationResponse{}, fmt.Errorf(
				"remote invocation failed at %s (%d): %s",
				endpointLabel,
				response.HTTP.StatusCode,
				apiError.Message,
			)
		}
		return factoryapi.InvocationResponse{}, fmt.Errorf(
			"remote invocation failed at %s (%d)",
			endpointLabel,
			response.HTTP.StatusCode,
		)
	}

	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"remote invocation response endpointPath=%s status=%d durationMillis=%d requestId=%s",
		endpointPath,
		response.HTTP.StatusCode,
		response.Duration.Milliseconds(),
		result.RequestId,
	)
	return result, nil
}

// ResolveFactoryInvocationRequest exposes the same request projection used by
// the local run operation. Callers must prepare the RunConfig through the
// shared Work input preparation boundary before invoking this helper.
func ResolveFactoryInvocationRequest(cfg RunConfig) (*factoryapi.InvocationRequest, bool, error) {
	return resolveFactoryInvocationRequest(cfg)
}

// RunRemoteInvocation dispatches one already-prepared run invocation through
// the selected remote adapter and renders its terminal result with the same
// response contract used by local invocation.
func RunRemoteInvocation(
	ctx context.Context,
	cfg RunConfig,
	server string,
	remote RemoteInvocationOperation,
) error {
	if remote == nil {
		return fmt.Errorf("run remote invocation: operation is required")
	}
	request, invocationMode, err := ResolveFactoryInvocationRequest(cfg)
	if err != nil {
		return err
	}
	if !invocationMode || request == nil {
		return &InvocationError{
			Code:    RemoteInvocationInputRequiredCode,
			Message: "--remote run requires invocation input; provide a prompt or use a remote-supported command",
		}
	}
	response, err := remote.InvokeFactory(ctx, RemoteInvocationRequest{
		Server: server, SessionID: defaultRemoteInvocationSessionID,
		Request: *request, Diagnostics: cfg.Diagnostics, Verbose: cfg.Verbose,
	})
	if err != nil {
		return err
	}
	result := invocationResultFromRemoteResponse(response)
	if result.Status == "" {
		return fmt.Errorf("remote invocation failed: response has no terminal status")
	}
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return writeInvocationFailure(cfg, result, nil)
	}
	return writeInvocationSuccess(cfg, result, nil)
}

func invocationResultFromRemoteResponse(response factoryapi.InvocationResponse) apisurface.FactoryInvocationResult {
	result := apisurface.FactoryInvocationResult{
		RequestID:     response.RequestId,
		TraceID:       response.TraceId,
		Status:        string(response.Status),
		PrimaryResult: contentcontract.PartsFromGenerated(response.PrimaryResult),
	}
	if response.ErrorCode != nil {
		result.ErrorCode = string(*response.ErrorCode)
	}
	if response.Message != nil {
		result.Message = *response.Message
	}
	if response.SessionId != nil {
		result.SessionID = *response.SessionId
	}
	if response.WorkId != nil {
		result.WorkID = *response.WorkId
	}
	if response.WorkName != nil {
		result.WorkName = *response.WorkName
	}
	if response.WorkState != nil {
		result.WorkState = *response.WorkState
	}
	return result
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
