// Package clihttp owns the injected HTTP protocol used by CLI commands.
package clihttp

import (
	"context"
	"encoding/json"
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
	return &APIError{
		StatusCode:     statusCode,
		Response:       response,
		DisplayMessage: strings.TrimSpace(displayMessage),
		Cause:          cause,
	}
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
	if err == nil && response == nil {
		return result, fmt.Errorf("HTTP doer returned a nil response")
	}
	return result, err
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
		return Response{}, fmt.Errorf("build request: %w", err)
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
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			_ = resp.Body.Close()
			return result, fmt.Errorf("parse response: %w", err)
		}
	}
	return result, nil
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
