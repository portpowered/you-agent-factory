// Package clihttp owns the injected HTTP protocol used by CLI commands.
package clihttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
