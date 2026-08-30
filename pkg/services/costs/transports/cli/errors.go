package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	// DefaultRequestTimeout is the client completion bound for one metrics
	// costs request. Wire uses the same value for the generated HTTP client.
	DefaultRequestTimeout = 10 * time.Second

	// CostsRequestTimeoutCode identifies a client-side request deadline.
	CostsRequestTimeoutCode = "COSTS_REQUEST_TIMEOUT"
	// CostsRequestCanceledCode identifies an operator- or caller-canceled
	// request before the server returned a report.
	CostsRequestCanceledCode = "COSTS_REQUEST_CANCELED"
	// CostsNetworkFailureCode identifies a request that failed before an HTTP
	// response was received.
	CostsNetworkFailureCode = "COSTS_NETWORK_FAILURE"
	// CostsHTTPFailureCode identifies an HTTP failure without a typed API code.
	CostsHTTPFailureCode = "COSTS_HTTP_FAILURE"

	metricsCostsEndpoint = "/metrics/costs"

	internalErrorFamily = factoryapi.ErrorFamilyInternalServerError
)

// CostsError is the safe CLI diagnostic contract for one metrics costs
// failure. Message is suitable for stderr; Cause remains available to tests
// and callers through errors.Is/errors.As without being rendered centrally.
type CostsError struct {
	Code    string
	Family  factoryapi.ErrorFamily
	Message string
	Cause   error
}

func (err *CostsError) Error() string {
	if err == nil {
		return ""
	}
	code := strings.TrimSpace(err.Code)
	if code == "" {
		code = CostsHTTPFailureCode
	}
	message := strings.TrimSpace(err.Message)
	if message == "" {
		message = "metrics costs request failed"
	}
	return fmt.Sprintf("%s: %s", code, message)
}

func (err *CostsError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// CLIErrorCode exposes the stable code to the central CLI diagnostic writer.
func (err *CostsError) CLIErrorCode() string {
	if err == nil || strings.TrimSpace(err.Code) == "" {
		return CostsHTTPFailureCode
	}
	return strings.TrimSpace(err.Code)
}

// CLIErrorMessage exposes only the safe, actionable message to the central
// CLI diagnostic writer.
func (err *CostsError) CLIErrorMessage() string {
	if err == nil || strings.TrimSpace(err.Message) == "" {
		return "metrics costs request failed"
	}
	return strings.TrimSpace(err.Message)
}

// CLIErrorFamily preserves the API family for typed server failures.
func (err *CostsError) CLIErrorFamily() factoryapi.ErrorFamily {
	if err == nil || err.Family == "" {
		return internalErrorFamily
	}
	return err.Family
}

func newCostsError(code string, family factoryapi.ErrorFamily, message string, cause error) *CostsError {
	return &CostsError{Code: code, Family: family, Message: message, Cause: cause}
}

func normalizeRequestTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return DefaultRequestTimeout
	}
	return timeout
}

func newCostsTransportError(server string, timeout time.Duration, cause error) error {
	endpoint := safeServerEndpoint(server)
	switch {
	case errors.Is(cause, context.DeadlineExceeded), isNetworkTimeout(cause):
		return newCostsError(
			CostsRequestTimeoutCode,
			internalErrorFamily,
			fmt.Sprintf("GET %s at %s timed out within the configured %s request timeout; retry or narrow the request with --session", metricsCostsEndpoint, endpoint, normalizeRequestTimeout(timeout)),
			cause,
		)
	case errors.Is(cause, context.Canceled):
		return newCostsError(
			CostsRequestCanceledCode,
			internalErrorFamily,
			fmt.Sprintf("GET %s at %s was canceled before a cost report was returned; retry the command", metricsCostsEndpoint, endpoint),
			cause,
		)
	default:
		return newCostsError(
			CostsNetworkFailureCode,
			internalErrorFamily,
			fmt.Sprintf("GET %s at %s failed before a response; check --server and confirm the Factory API is running", metricsCostsEndpoint, endpoint),
			cause,
		)
	}
}

func isNetworkTimeout(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

func safeServerEndpoint(server string) string {
	trimmed := strings.TrimSpace(server)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "<configured server>"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	endpoint := strings.TrimRight(parsed.String(), "/")
	if endpoint == "" {
		return "<configured server>"
	}
	return endpoint
}

func candidateFamily(family factoryapi.ErrorFamily) factoryapi.ErrorFamily {
	if family == "" {
		return internalErrorFamily
	}
	return family
}

func familyForStatus(status int) factoryapi.ErrorFamily {
	if status == http.StatusBadRequest {
		return factoryapi.ErrorFamilyBadRequest
	}
	if status == http.StatusNotFound {
		return factoryapi.ErrorFamilyNotFound
	}
	return internalErrorFamily
}
