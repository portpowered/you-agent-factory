package session

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
)

const deleteRequestTimeout = 10 * time.Second

// DeleteConfig holds parameters for the session delete command.
type DeleteConfig struct {
	Port        int
	SessionID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// DeleteResult is the CLI JSON confirmation emitted after a successful close.
type DeleteResult struct {
	SessionID string `json:"sessionId"`
}

// Delete closes one live factory session on a running host via HTTP.
func Delete(cfg DeleteConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}

	endpoint := deleteEndpoint(cfg.Port, sessionID)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session delete request endpointPath=%s endpoint=%s port=%d session=%s",
		endpoint.Path,
		endpoint.String(),
		cfg.Port,
		sessionID,
	)

	client := &http.Client{Timeout: deleteRequestTimeout}
	started := time.Now()
	req, err := http.NewRequest(http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build delete factory session request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session delete response endpointPath=%s error=unreachable durationMillis=%d", endpoint.Path, time.Since(started).Milliseconds())
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"session delete response endpointPath=%s status=%d durationMillis=%d session=%s",
			endpoint.Path,
			resp.StatusCode,
			time.Since(started).Milliseconds(),
			sessionID,
		)
		return renderDeleteSuccess(cfg, sessionID)
	case http.StatusNotFound:
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session delete response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds())
		return deleteNotFoundError(sessionID, resp)
	default:
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "session delete response endpointPath=%s status=%d durationMillis=%d", endpoint.Path, resp.StatusCode, time.Since(started).Milliseconds())
		return deleteStatusError(resp)
	}
}

func deleteEndpoint(port int, sessionID string) url.URL {
	return url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", port),
		Path:   fmt.Sprintf("/factory-sessions/%s", url.PathEscape(sessionID)),
	}
}

func renderDeleteSuccess(cfg DeleteConfig, sessionID string) error {
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(DeleteResult{SessionID: sessionID})
	}
	_, err := fmt.Fprintf(cfg.Output, "Closed factory session %s\n", sessionID)
	return err
}

func deleteNotFoundError(sessionID string, resp *http.Response) error {
	var errResp factoryapi.ErrorResponse
	if json.NewDecoder(resp.Body).Decode(&errResp) == nil && errResp.Message != "" {
		return fmt.Errorf("factory session not found (%s): %s", sessionID, errResp.Message)
	}
	return fmt.Errorf("factory session not found: %s", sessionID)
}

func deleteStatusError(resp *http.Response) error {
	var errResp factoryapi.ErrorResponse
	if json.NewDecoder(resp.Body).Decode(&errResp) == nil && errResp.Message != "" {
		return fmt.Errorf("close factory session failed (%d): %s", resp.StatusCode, errResp.Message)
	}
	return fmt.Errorf("close factory session failed (%d)", resp.StatusCode)
}
