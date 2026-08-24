package session

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// DeleteConfig holds parameters for the session delete command.
type DeleteConfig struct {
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func NewDelete(transport clihttp.Protocol) func(DeleteConfig) error {
	return func(cfg DeleteConfig) error { cfg.HTTP = transport; return Delete(cfg) }
}

// DeleteResult is the CLI JSON confirmation emitted after a successful delete.
type DeleteResult struct {
	SessionID string `json:"sessionId"`
}

// Delete removes one already-stopped live factory session from a running host via HTTP.
func Delete(cfg DeleteConfig) error {
	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}

	endpoint, err := deleteEndpoint(cfg.Server, sessionID)
	if err != nil {
		return fmt.Errorf("resolve factory session delete endpoint: %w", err)
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session delete request endpointPath=%s endpoint=%s server=%s session=%s",
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		sessionID,
	)

	req, err := http.NewRequest(http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build delete factory session request: %w", err)
	}

	response, err := cfg.HTTP.Execute(req)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session delete response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, response.Duration.Milliseconds())
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"session delete response endpointPath=%s status=%d durationMillis=%d session=%s",
			endpoint.Path,
			resp.StatusCode,
			response.Duration.Milliseconds(),
			sessionID,
		)
		return renderDeleteSuccess(cfg, sessionID)
	case http.StatusNotFound:
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session delete response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		return deleteNotFoundError(sessionID, resp)
	default:
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session delete response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, response.Duration.Milliseconds())
		return deleteStatusError(resp)
	}
}

func deleteEndpoint(server, sessionID string) (url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, sessionpath.ScopedPath("", sessionID))
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse factory session delete endpoint: %w", err)
	}
	return *endpoint, nil
}

func renderDeleteSuccess(cfg DeleteConfig, sessionID string) error {
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(DeleteResult{SessionID: sessionID})
	}
	_, err := fmt.Fprintf(cfg.Output, "Deleted factory session %s\n", sessionID)
	return err
}

func deleteNotFoundError(sessionID string, resp *http.Response) error {
	var errResp factoryapi.ErrorResponse
	if json.NewDecoder(resp.Body).Decode(&errResp) == nil && errResp.Message != "" {
		return clihttp.NewAPIErrorFromResponse(
			resp,
			errResp,
			fmt.Sprintf("factory session not found (%s): %s", sessionID, errResp.Message),
			nil,
		)
	}
	return clihttp.WithHTTPResponse(resp, fmt.Errorf("factory session not found: %s", sessionID))
}

func deleteStatusError(resp *http.Response) error {
	var errResp factoryapi.ErrorResponse
	if json.NewDecoder(resp.Body).Decode(&errResp) == nil && errResp.Message != "" {
		return clihttp.NewAPIErrorFromResponse(
			resp,
			errResp,
			fmt.Sprintf("delete factory session failed (%d): %s", resp.StatusCode, errResp.Message),
			nil,
		)
	}
	return clihttp.WithHTTPResponse(resp, fmt.Errorf("delete factory session failed (%d)", resp.StatusCode))
}
