// Package clihttp provides shared JSON HTTP transport helpers for CLI commands.
package clihttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
)

// RequestOptions configures diagnostics for JSON HTTP transport helpers.
type RequestOptions struct {
	Diagnostics  io.Writer
	Verbose      bool
	EndpointPath string
	LogLabel     string
}

// GetJSON executes an HTTP GET and decodes JSON into dst when the response status is 200 OK.
// On transport failure it logs an unreachable response diagnostic and returns a non-nil error.
// On other HTTP statuses it logs a status response diagnostic and returns the response with a nil
// error so callers can map command-specific errors (for example work show 404).
func GetJSON(ctx context.Context, client *http.Client, url string, dst any, opts RequestOptions) (*http.Response, error) {
	return doJSON(ctx, client, http.MethodGet, url, nil, dst, http.StatusOK, opts)
}

// PostJSON executes an HTTP POST with an optional JSON body and decodes JSON into dst when the
// response status is 200 OK.
func PostJSON(ctx context.Context, client *http.Client, url string, body io.Reader, dst any, opts RequestOptions) (*http.Response, error) {
	return doJSON(ctx, client, http.MethodPost, url, body, dst, http.StatusOK, opts)
}

// PutJSON executes an HTTP PUT with an optional JSON body and decodes JSON into dst when the
// response status is 200 OK.
func PutJSON(ctx context.Context, client *http.Client, url string, body io.Reader, dst any, opts RequestOptions) (*http.Response, error) {
	return doJSON(ctx, client, http.MethodPut, url, body, dst, http.StatusOK, opts)
}

func doJSON(
	ctx context.Context,
	client *http.Client,
	method string,
	url string,
	body io.Reader,
	dst any,
	successStatus int,
	opts RequestOptions,
) (*http.Response, error) {
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if method == http.MethodPost || method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	durationMillis := time.Since(started).Milliseconds()
	if err != nil {
		logUnreachableResponse(opts, durationMillis)
		return nil, err
	}
	if resp.StatusCode != successStatus {
		logStatusResponse(opts, resp.StatusCode, durationMillis)
		return resp, nil
	}
	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("parse response: %w", err)
		}
	}
	return resp, nil
}

func logUnreachableResponse(opts RequestOptions, durationMillis int64) {
	clidiag.Printf(
		opts.Diagnostics,
		opts.Verbose,
		"%s response endpointPath=%s error=unreachable durationMillis=%d",
		opts.LogLabel,
		opts.EndpointPath,
		durationMillis,
	)
}

func logStatusResponse(opts RequestOptions, status int, durationMillis int64) {
	clidiag.Printf(
		opts.Diagnostics,
		opts.Verbose,
		"%s response endpointPath=%s status=%d durationMillis=%d",
		opts.LogLabel,
		opts.EndpointPath,
		status,
		durationMillis,
	)
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
