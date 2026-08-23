// Package serverstop implements the loopback-only `you server stop` command.
package serverstop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	// DefaultRequestTimeout bounds the typed shutdown request itself.
	DefaultRequestTimeout = 10 * time.Second
	// DefaultObservationTimeout bounds confirmation that the selected listener
	// has stopped after its accepted acknowledgment.
	DefaultObservationTimeout = 5 * time.Second

	InvalidTargetCode      = "SERVER_STOP_INVALID_TARGET"
	RequestTimeoutCode     = "SERVER_STOP_REQUEST_TIMEOUT"
	CanceledCode           = "SERVER_STOP_CANCELED"
	UnreachableCode        = "SERVER_STOP_UNREACHABLE"
	HTTPFailureCode        = "SERVER_STOP_HTTP_FAILURE"
	ObservationTimeoutCode = "SERVER_STOP_OBSERVATION_TIMEOUT"
	ControlRejectedCode    = "SHUTDOWN_CONTROL_REJECTED"
	ControlUnavailableCode = "SHUTDOWN_CONTROL_UNAVAILABLE"
)

// Client is the narrow generated response-aware HTTP capability required by
// the command. No fallback request path is available through this interface.
type Client interface {
	ShutdownServerWithResponse(
		context.Context,
		...generatedclient.RequestEditorFn,
	) (*generatedclient.ShutdownServerClientResponse, error)
}

// ClientFactory constructs one generated client for the selected server.
type ClientFactory func(string) (Client, error)

// StopObserver observes whether the selected listener still accepts TCP
// connections after the server acknowledges shutdown. The platform HTTP host
// supplies the lifecycle implementation; this transport only coordinates it.
type StopObserver interface {
	Wait(context.Context, string, time.Duration) error
}

// StopObserverFunc adapts a function to StopObserver for focused tests and
// other explicit edge bindings.
type StopObserverFunc func(context.Context, string, time.Duration) error

func (observer StopObserverFunc) Wait(ctx context.Context, address string, timeout time.Duration) error {
	if observer == nil {
		return errors.New("stop server: listener observation is unavailable")
	}
	return observer(ctx, address, timeout)
}

// Operation is the injected command operation for one server-stop request.
type Operation func(context.Context, Config) error

// Config contains resolved invocation inputs and output policy.
type Config struct {
	Server             string
	JSON               bool
	Output             io.Writer
	RequestTimeout     time.Duration
	ObservationTimeout time.Duration
}

// NewOperation binds the generated client factory to the platform listener
// observation path.
func NewOperation(factory ClientFactory, observer StopObserver) Operation {
	return NewOperationWithDependencies(factory, observer, DefaultRequestTimeout, DefaultObservationTimeout)
}

// NewOperationWithDependencies exposes only the network effects needed by
// focused contract tests while preserving the production operation boundary.
func NewOperationWithDependencies(
	factory ClientFactory,
	observer StopObserver,
	requestTimeout time.Duration,
	observationTimeout time.Duration,
) Operation {
	return func(ctx context.Context, config Config) error {
		return run(ctx, config, factory, observer, requestTimeout, observationTimeout)
	}
}

func run(
	ctx context.Context,
	config Config,
	factory ClientFactory,
	observer StopObserver,
	defaultRequestTimeout time.Duration,
	defaultObservationTimeout time.Duration,
) error {
	if ctx == nil {
		return newError(CanceledCode, factoryapi.ErrorFamilyInternalServerError, "stop server: context is required", nil)
	}
	if config.Output == nil {
		return newError(HTTPFailureCode, factoryapi.ErrorFamilyInternalServerError, "stop server: output writer is required", nil)
	}
	base, target, err := resolveTarget(config.Server)
	if err != nil {
		return err
	}
	if factory == nil {
		return newError(UnreachableCode, factoryapi.ErrorFamilyInternalServerError, clientFailureMessage(base.String()), nil)
	}
	client, err := factory(base.String())
	if err != nil || client == nil {
		return newError(UnreachableCode, factoryapi.ErrorFamilyInternalServerError, clientFailureMessage(base.String()), err)
	}

	requestTimeout := normalizeTimeout(config.RequestTimeout, defaultRequestTimeout)
	// The generated client is bound to a bounded HTTP client by Wire. Preserve
	// the invocation context here so caller cancellation remains authoritative;
	// requestTimeout is retained for stable diagnostics and test bindings.
	response, requestErr := client.ShutdownServerWithResponse(ctx)
	if requestErr != nil {
		return mapRequestError(base.String(), requestTimeout, requestErr)
	}
	if err := acceptedResponse(response, base.String()); err != nil {
		return err
	}

	observationTimeout := normalizeTimeout(config.ObservationTimeout, defaultObservationTimeout)
	if err := waitForStopped(ctx, target, observer, observationTimeout); err != nil {
		return err
	}
	return writeSuccess(config.Output, config.JSON, base.String())
}

func resolveTarget(server string) (cliserver.Base, cliserver.LocalBindTarget, error) {
	base, err := cliserver.ResolveBase(server)
	if err != nil {
		return cliserver.Base{}, cliserver.LocalBindTarget{}, newError(
			InvalidTargetCode, factoryapi.ErrorFamilyBadRequest,
			fmt.Sprintf("stop server: %v", err), err,
		)
	}
	target, err := cliserver.LocalBindTargetFromBase(base)
	if err != nil {
		return cliserver.Base{}, cliserver.LocalBindTarget{}, newError(
			InvalidTargetCode, factoryapi.ErrorFamilyBadRequest,
			fmt.Sprintf("stop server: %v", err), err,
		)
	}
	if target.Port == 0 {
		return cliserver.Base{}, cliserver.LocalBindTarget{}, newError(
			InvalidTargetCode, factoryapi.ErrorFamilyBadRequest,
			"stop server: server port must be between 1 and 65535", nil,
		)
	}
	return base, target, nil
}

func acceptedResponse(response *generatedclient.ShutdownServerClientResponse, endpoint string) error {
	if response == nil {
		return newError(HTTPFailureCode, factoryapi.ErrorFamilyInternalServerError, responseFailureMessage(endpoint, 0), nil)
	}
	if response.JSON202 != nil && response.StatusCode() == 202 {
		return nil
	}
	switch {
	case response.JSON403 != nil:
		return typedResponseError(ControlRejectedCode, factoryapi.ErrorFamilyBadRequest, response.JSON403.Message, endpoint, response.StatusCode())
	case response.JSON503 != nil:
		return typedResponseError(ControlUnavailableCode, factoryapi.ErrorFamilyInternalServerError, response.JSON503.Message, endpoint, response.StatusCode())
	case response.JSON500 != nil:
		return typedResponseError(string(response.JSON500.Code), factoryapi.ErrorFamily(response.JSON500.Family), response.JSON500.Message, endpoint, response.StatusCode())
	default:
		return newError(HTTPFailureCode, factoryapi.ErrorFamilyInternalServerError, responseFailureMessage(endpoint, response.StatusCode()), nil)
	}
}

func typedResponseError(code string, family factoryapi.ErrorFamily, message, endpoint string, status int) error {
	if strings.TrimSpace(code) == "" {
		code = HTTPFailureCode
	}
	if strings.TrimSpace(message) == "" {
		message = responseFailureMessage(endpoint, status)
	}
	return newError(code, family, message, nil)
}

func mapRequestError(endpoint string, timeout time.Duration, err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return newError(CanceledCode, factoryapi.ErrorFamilyInternalServerError, fmt.Sprintf("POST /shutdown at %s was canceled; retry the command", endpoint), err)
	case errors.Is(err, context.DeadlineExceeded), isNetworkTimeout(err):
		return newError(RequestTimeoutCode, factoryapi.ErrorFamilyInternalServerError, fmt.Sprintf("POST /shutdown at %s timed out within %s; confirm the Factory API is running and retry", endpoint, timeout), err)
	default:
		return newError(UnreachableCode, factoryapi.ErrorFamilyInternalServerError, clientFailureMessage(endpoint), err)
	}
}

func waitForStopped(
	ctx context.Context,
	target cliserver.LocalBindTarget,
	observer StopObserver,
	timeout time.Duration,
) error {
	if observer == nil {
		return newError(HTTPFailureCode, factoryapi.ErrorFamilyInternalServerError, "stop server: listener observation is unavailable", nil)
	}
	address := net.JoinHostPort(target.Host, fmt.Sprintf("%d", target.Port))
	if err := observer.Wait(ctx, address, timeout); err != nil {
		return observationError(err, address, timeout)
	}
	return nil
}

func observationError(err error, address string, timeout time.Duration) error {
	if errors.Is(err, context.Canceled) {
		return newError(CanceledCode, factoryapi.ErrorFamilyInternalServerError, fmt.Sprintf("stop server observation for %s was canceled", address), err)
	}
	return newError(ObservationTimeoutCode, factoryapi.ErrorFamilyInternalServerError, fmt.Sprintf("server at %s did not stop within %s after accepting shutdown; verify the process and retry", address, timeout), err)
}

func writeSuccess(output io.Writer, jsonOutput bool, endpoint string) error {
	if jsonOutput {
		return json.NewEncoder(output).Encode(struct {
			Server string `json:"server"`
			Status string `json:"status"`
		}{Server: endpoint, Status: "stopped"})
	}
	_, err := fmt.Fprintf(output, "Server stopped: %s\n", endpoint)
	return err
}

func normalizeTimeout(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	if fallback > 0 {
		return fallback
	}
	return time.Second
}

func clientFailureMessage(endpoint string) string {
	return fmt.Sprintf("cannot reach Factory API at %s for POST /shutdown; confirm the selected server is running and retry", endpoint)
}

func responseFailureMessage(endpoint string, status int) string {
	if status == 0 {
		return fmt.Sprintf("POST /shutdown at %s returned no response", endpoint)
	}
	return fmt.Sprintf("POST /shutdown at %s returned HTTP %d", endpoint, status)
}

func isNetworkTimeout(err error) bool {
	var networkErr net.Error
	return errors.As(err, &networkErr) && networkErr.Timeout()
}

// ServerStopError is the sanitized diagnostic crossing the CLI boundary.
type ServerStopError struct {
	Code    string
	Family  factoryapi.ErrorFamily
	Message string
	Cause   error
}

func (err *ServerStopError) Error() string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", err.CLIErrorCode(), err.CLIErrorMessage())
}

func (err *ServerStopError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (err *ServerStopError) CLIErrorCode() string {
	if err == nil || strings.TrimSpace(err.Code) == "" {
		return HTTPFailureCode
	}
	return strings.TrimSpace(err.Code)
}

func (err *ServerStopError) CLIErrorMessage() string {
	if err == nil || strings.TrimSpace(err.Message) == "" {
		return "server stop failed"
	}
	return strings.TrimSpace(err.Message)
}

func (err *ServerStopError) CLIErrorFamily() factoryapi.ErrorFamily {
	if err == nil || err.Family == "" {
		return factoryapi.ErrorFamilyInternalServerError
	}
	return err.Family
}

func newError(code string, family factoryapi.ErrorFamily, message string, cause error) *ServerStopError {
	return &ServerStopError{Code: code, Family: family, Message: message, Cause: cause}
}
