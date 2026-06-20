package session

import (
	"context"
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
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

const lifecycleControlRequestTimeout = 10 * time.Second

// LifecycleControlConfig holds parameters for session pause and resume commands.
type LifecycleControlConfig struct {
	Server      string
	SessionID   string
	Operation   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// Pause requests POST /factory-sessions/{session_id}/pause on a running host.
func Pause(cfg LifecycleControlConfig) error {
	cfg.Operation = "pause"
	return lifecycleControl(cfg)
}

// Resume requests POST /factory-sessions/{session_id}/resume on a running host.
func Resume(cfg LifecycleControlConfig) error {
	cfg.Operation = "resume"
	return lifecycleControl(cfg)
}

func lifecycleControl(cfg LifecycleControlConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	operation := strings.TrimSpace(cfg.Operation)
	if operation == "" {
		return fmt.Errorf("lifecycle control operation is required")
	}

	endpoint, err := lifecycleControlEndpoint(cfg)
	if err != nil {
		return err
	}
	sessionID := resolvedSessionID(cfg.SessionID)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session %s request endpointPath=%s endpoint=%s server=%s session=%s",
		operation,
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(sessionID),
	)

	client := &http.Client{Timeout: lifecycleControlRequestTimeout}
	started := time.Now()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint.String(), strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("build session %s request: %w", operation, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"session %s response endpointPath=%s error=unreachable durationMillis=%d",
			operation,
			endpoint.Path,
			time.Since(started).Milliseconds(),
		)
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	var result factoryapi.FactorySessionLifecycleControlResponse
	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted:
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("parse session %s response: %w", operation, err)
		}
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"session %s response endpointPath=%s status=%d durationMillis=%d sessionId=%s outcome=%s status=%s",
			operation,
			endpoint.Path,
			resp.StatusCode,
			time.Since(started).Milliseconds(),
			result.SessionId,
			result.Outcome,
			result.Status,
		)
		if cfg.JSON {
			return json.NewEncoder(cfg.Output).Encode(result)
		}
		return renderLifecycleControlSuccess(cfg.Output, operation, result)
	case http.StatusNotFound:
		return lifecycleControlNotFoundError(sessionID, resp)
	case http.StatusConflict:
		return lifecycleControlConflictError(operation, resp)
	default:
		return lifecycleControlStatusError(operation, resp)
	}
}

func lifecycleControlEndpoint(cfg LifecycleControlConfig) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("/"+strings.TrimSpace(cfg.Operation), cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse session %s endpoint: %w", cfg.Operation, err)
	}
	return *endpoint, nil
}

func renderLifecycleControlSuccess(
	output io.Writer,
	operation string,
	result factoryapi.FactorySessionLifecycleControlResponse,
) error {
	verb := lifecycleControlVerb(operation, result.Outcome)
	_, err := fmt.Fprintf(
		output,
		"%s factory session %s (outcome=%s status=%s)\n",
		verb,
		result.SessionId,
		result.Outcome,
		result.Status,
	)
	return err
}

func lifecycleControlVerb(operation string, outcome factoryapi.FactorySessionLifecycleControlOutcome) string {
	switch outcome {
	case factoryapi.FactorySessionLifecycleControlOutcomeNoOp:
		return "Factory session already " + lifecycleControlStateLabel(operation)
	default:
		return lifecycleControlAppliedLabel(operation)
	}
}

func lifecycleControlAppliedLabel(operation string) string {
	switch strings.TrimSpace(operation) {
	case "resume":
		return "Resumed"
	default:
		return "Paused"
	}
}

func lifecycleControlStateLabel(operation string) string {
	switch strings.TrimSpace(operation) {
	case "resume":
		return "running"
	default:
		return "paused"
	}
}

func lifecycleControlNotFoundError(sessionID string, resp *http.Response) error {
	if errResp, ok := clihttp.DecodeAPIError(resp); ok {
		return fmt.Errorf("factory session %q not found: %s", sessionID, errResp.Message)
	}
	return fmt.Errorf("factory session %q not found", sessionID)
}

func lifecycleControlConflictError(operation string, resp *http.Response) error {
	var controlResp factoryapi.FactorySessionLifecycleControlResponse
	if json.NewDecoder(resp.Body).Decode(&controlResp) == nil && controlResp.SessionId != "" {
		detail := ""
		if controlResp.Detail != nil {
			detail = strings.TrimSpace(*controlResp.Detail)
		}
		if detail != "" {
			return fmt.Errorf("session %s rejected (%s): %s", operation, controlResp.Outcome, detail)
		}
		return fmt.Errorf(
			"session %s rejected (%s): session %s is %s",
			operation,
			controlResp.Outcome,
			controlResp.SessionId,
			controlResp.Status,
		)
	}
	if errResp, ok := clihttp.DecodeAPIError(resp); ok {
		return fmt.Errorf("session %s failed (409): %s", operation, errResp.Message)
	}
	return fmt.Errorf("session %s failed (409)", operation)
}

func lifecycleControlStatusError(operation string, resp *http.Response) error {
	if errResp, ok := clihttp.DecodeAPIError(resp); ok {
		return fmt.Errorf("session %s failed (%d): %s", operation, resp.StatusCode, errResp.Message)
	}
	return fmt.Errorf("session %s failed (%d)", operation, resp.StatusCode)
}
