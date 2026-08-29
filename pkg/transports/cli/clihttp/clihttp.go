// Package clihttp owns the injected HTTP protocol used by CLI commands.
package clihttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Doer is the external HTTP effect consumed by the CLI protocol.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Clock is the wall-clock role used only to observe request duration.
type Clock interface {
	Now() time.Time
}

// Response carries the HTTP response and the protocol-observed duration.
type Response struct {
	HTTP     *http.Response
	Duration time.Duration
}

// APIError preserves one decoded server ErrorResponse across the CLI command
// boundary. DisplayMessage keeps the command's existing contextual error text
// for direct callers, while the CLI diagnostic contract renders the original
// server message and response family/code.
type APIError struct {
	StatusCode     int
	Response       factoryapi.ErrorResponse
	DisplayMessage string
	Cause          error
	RequestMethod  string
	RequestURL     string
}

// NewAPIError constructs a safe typed failure from one decoded server
// response. The caller supplies any operation-specific context and optional
// sentinel cause separately from the server-owned diagnostic fields.
func NewAPIError(
	statusCode int,
	response factoryapi.ErrorResponse,
	displayMessage string,
	cause error,
) *APIError {
	return NewAPIErrorWithRequest(nil, statusCode, response, displayMessage, cause)
}

// NewAPIErrorWithRequest preserves the request metadata needed by the shared
// CLI debug renderer while keeping it out of the normal ErrorResponse.
func NewAPIErrorWithRequest(
	request *http.Request,
	statusCode int,
	response factoryapi.ErrorResponse,
	displayMessage string,
	cause error,
) *APIError {
	method, requestURL := requestMetadata(request)
	return &APIError{
		StatusCode:     statusCode,
		Response:       response,
		DisplayMessage: strings.TrimSpace(displayMessage),
		Cause:          cause,
		RequestMethod:  method,
		RequestURL:     requestURL,
	}
}

// NewAPIErrorFromResponse builds a structured API failure directly from the
// HTTP response that carried it, retaining the response's request metadata.
func NewAPIErrorFromResponse(
	response *http.Response,
	errResponse factoryapi.ErrorResponse,
	displayMessage string,
	cause error,
) *APIError {
	statusCode := 0
	if response != nil {
		statusCode = response.StatusCode
	}
	var request *http.Request
	if response != nil {
		request = response.Request
	}
	return NewAPIErrorWithRequest(request, statusCode, errResponse, displayMessage, cause)
}

func (err *APIError) Error() string {
	if err == nil {
		return ""
	}
	if message := strings.TrimSpace(err.DisplayMessage); message != "" {
		return message
	}
	if message := strings.TrimSpace(err.Response.Message); message != "" {
		return message
	}
	if err.StatusCode != 0 {
		return fmt.Sprintf("HTTP request failed (%d)", err.StatusCode)
	}
	return "HTTP request failed"
}

func (err *APIError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// CLIErrorCode, CLIErrorFamily, and CLIErrorMessage form the shared central
// diagnostic contract without coupling command-specific packages to the
// renderer's concrete implementation.
func (err *APIError) CLIErrorCode() string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(string(err.Response.Code))
}

func (err *APIError) CLIErrorFamily() factoryapi.ErrorFamily {
	if err == nil {
		return ""
	}
	return err.Response.Family
}

func (err *APIError) CLIErrorMessage() string {
	if err == nil {
		return ""
	}
	if message := strings.TrimSpace(err.Response.Message); message != "" {
		return message
	}
	return strings.TrimSpace(err.DisplayMessage)
}

// CLIErrorResponse exposes the complete already-decoded public response so
// optional server-provided diagnostic details survive the CLI boundary too.
func (err *APIError) CLIErrorResponse() factoryapi.ErrorResponse {
	if err == nil {
		return factoryapi.ErrorResponse{}
	}
	return err.Response
}

func (err *APIError) CLIHTTPMethod() string {
	if err == nil {
		return ""
	}
	return err.RequestMethod
}

func (err *APIError) CLIHTTPURL() string {
	if err == nil {
		return ""
	}
	return err.RequestURL
}

func (err *APIError) CLIHTTPStatus() int {
	if err == nil {
		return 0
	}
	return err.StatusCode
}

// HTTPError retains request metadata for transport, malformed-response, and
// otherwise unclassified HTTP failures. Its Error method intentionally keeps
// the wrapped command text unchanged for existing callers.
type HTTPError struct {
	Method     string
	URL        string
	StatusCode int
	Cause      error
}

func (err *HTTPError) Error() string {
	if err == nil {
		return ""
	}
	if err.Cause != nil {
		return err.Cause.Error()
	}
	return "HTTP request failed"
}

func (err *HTTPError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (err *HTTPError) CLIHTTPMethod() string {
	if err == nil {
		return ""
	}
	return err.Method
}

func (err *HTTPError) CLIHTTPURL() string {
	if err == nil {
		return ""
	}
	return err.URL
}

func (err *HTTPError) CLIHTTPStatus() int {
	if err == nil {
		return 0
	}
	return err.StatusCode
}

func NewHTTPError(method, requestURL string, statusCode int, cause error) error {
	if cause == nil {
		return nil
	}
	return &HTTPError{Method: method, URL: requestURL, StatusCode: statusCode, Cause: cause}
}

func NewHTTPErrorFromResponse(response *http.Response, cause error) error {
	if cause == nil {
		return nil
	}
	statusCode := 0
	var request *http.Request
	if response != nil {
		statusCode = response.StatusCode
		request = response.Request
	}
	method, requestURL := requestMetadata(request)
	return NewHTTPError(method, requestURL, statusCode, cause)
}

// WithHTTPResponse adds response metadata to an error that was classified
// after the body was consumed. Existing HTTP-aware errors pass through so
// structured server details remain the outermost diagnostic contract.
func WithHTTPResponse(response *http.Response, cause error) error {
	if cause == nil {
		return nil
	}
	var metadata interface {
		CLIHTTPMethod() string
		CLIHTTPURL() string
		CLIHTTPStatus() int
	}
	if errors.As(cause, &metadata) {
		return cause
	}
	return NewHTTPErrorFromResponse(response, cause)
}

func requestMetadata(request *http.Request) (string, string) {
	if request == nil {
		return "", ""
	}
	requestURL := ""
	if request.URL != nil {
		requestURL = request.URL.String()
	}
	return request.Method, requestURL
}

// Protocol is the single HTTP adapter consumed by handwritten CLI transports.
type Protocol interface {
	Execute(*http.Request) (Response, error)
	GetJSON(context.Context, string, any) (Response, error)
	PostJSON(context.Context, string, io.Reader, any) (Response, error)
	PostJSONCreated(context.Context, string, io.Reader, any) (Response, error)
	PutJSON(context.Context, string, io.Reader, any) (Response, error)
	PutJSONCreated(context.Context, string, io.Reader, any) (Response, error)
}

type protocol struct {
	doer  Doer
	clock Clock
}

// NewProtocol binds one HTTP effect and one clock to the CLI protocol.
func NewProtocol(doer Doer, clock Clock) (Protocol, error) {
	if doer == nil {
		return nil, fmt.Errorf("HTTP doer is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("HTTP clock is required")
	}
	return &protocol{doer: doer, clock: clock}, nil
}

func (p *protocol) Execute(request *http.Request) (Response, error) {
	started := p.clock.Now()
	response, err := p.doer.Do(request)
	result := Response{HTTP: response, Duration: p.clock.Now().Sub(started)}
	if err != nil {
		statusCode := 0
		if response != nil {
			statusCode = response.StatusCode
		}
		method, requestURL := requestMetadata(request)
		if response != nil && response.Request != nil {
			method, requestURL = requestMetadata(response.Request)
		}
		return result, NewHTTPError(method, requestURL, statusCode, err)
	}
	if response == nil {
		return result, NewHTTPErrorFromRequest(request, fmt.Errorf("HTTP doer returned a nil response"))
	}
	return result, nil
}

// GetJSON executes an HTTP GET and decodes JSON into dst for 200 OK. Other
// statuses are returned intact so the owning command can map and diagnose them.
func (p *protocol) GetJSON(ctx context.Context, url string, dst any) (Response, error) {
	return p.doJSON(ctx, http.MethodGet, url, nil, dst, http.StatusOK)
}

// PostJSON executes an HTTP POST with an optional JSON body and decodes JSON into dst when the
// response status is 200 OK.
func (p *protocol) PostJSON(ctx context.Context, url string, body io.Reader, dst any) (Response, error) {
	return p.doJSON(ctx, http.MethodPost, url, body, dst, http.StatusOK)
}

// PostJSONCreated executes an HTTP POST with an optional JSON body and decodes JSON into dst when
// the response status is 201 Created.
func (p *protocol) PostJSONCreated(ctx context.Context, url string, body io.Reader, dst any) (Response, error) {
	return p.doJSON(ctx, http.MethodPost, url, body, dst, http.StatusCreated)
}

// PutJSON executes an HTTP PUT with an optional JSON body and decodes JSON into dst when the
// response status is 200 OK.
func (p *protocol) PutJSON(ctx context.Context, url string, body io.Reader, dst any) (Response, error) {
	return p.doJSON(ctx, http.MethodPut, url, body, dst, http.StatusOK)
}

// PutJSONCreated executes an HTTP PUT with an optional JSON body and decodes JSON into dst when
// the response status is 201 Created.
func (p *protocol) PutJSONCreated(ctx context.Context, url string, body io.Reader, dst any) (Response, error) {
	return p.doJSON(ctx, http.MethodPut, url, body, dst, http.StatusCreated)
}

func (p *protocol) doJSON(
	ctx context.Context,
	method string,
	url string,
	body io.Reader,
	dst any,
	successStatus int,
) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return Response{}, NewHTTPError(method, url, 0, fmt.Errorf("build request: %w", err))
	}
	if method == http.MethodPost || method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}

	result, err := p.Execute(req)
	if err != nil {
		return result, err
	}
	resp := result.HTTP
	if resp.StatusCode != successStatus {
		return result, nil
	}
	if dst != nil {
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(dst); err != nil {
			_ = resp.Body.Close()
			return result, WithHTTPResponse(resp, fmt.Errorf("parse response: %w", err))
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			_ = resp.Body.Close()
			if err == nil {
				err = fmt.Errorf("response contains multiple JSON values")
			}
			return result, WithHTTPResponse(resp, fmt.Errorf("parse response: %w", err))
		}
	}
	return result, nil
}

func NewHTTPErrorFromRequest(request *http.Request, cause error) error {
	if cause == nil {
		return nil
	}
	method, requestURL := requestMetadata(request)
	return NewHTTPError(method, requestURL, 0, cause)
}

// DecodeAPIError decodes a factory API error response when the body includes a message.
func DecodeAPIError(resp *http.Response) (factoryapi.ErrorResponse, bool) {
	if resp == nil || resp.Body == nil {
		return factoryapi.ErrorResponse{}, false
	}
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return factoryapi.ErrorResponse{}, false
	}
	if errResp.Message == "" {
		return errResp, false
	}
	return errResp, true
}
