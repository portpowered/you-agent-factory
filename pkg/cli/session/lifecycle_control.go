package session

import (
	"bytes"
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

const lifecycleControlRequestTimeout = 15 * time.Second

// LifecycleControlConfig holds parameters for session pause and resume commands.
type LifecycleControlConfig struct {
	Server      string
	SessionID   string
	RequestID   string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// Pause pauses one live factory session via POST /factory-sessions/{session_id}/pause.
func Pause(cfg LifecycleControlConfig) error {
	return postLifecycleControl(cfg, "pause")
}

// Resume resumes one paused live factory session via POST /factory-sessions/{session_id}/resume.
func Resume(cfg LifecycleControlConfig) error {
	return postLifecycleControl(cfg, "resume")
}

func postLifecycleControl(cfg LifecycleControlConfig, operation string) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	endpoint, err := lifecycleControlEndpoint(cfg, operation)
	if err != nil {
		return err
	}

	requestBody, err := json.Marshal(factoryapi.FactorySessionLifecycleControlRequest{
		RequestId: optionalLifecycleRequestID(cfg.RequestID),
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"session %s request endpointPath=%s endpoint=%s server=%s session=%s requestIdPresent=%t",
		operation,
		endpoint.Path,
		endpoint.String(),
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		strings.TrimSpace(cfg.RequestID) != "",
	)

	client := &http.Client{Timeout: lifecycleControlRequestTimeout}
	started := time.Now()
	var response factoryapi.FactorySessionLifecycleControlResponse
	resp, err := clihttp.PostJSON(
		context.Background(),
		client,
		endpoint.String(),
		bytes.NewReader(requestBody),
		&response,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpoint.Path,
			LogLabel:     "session " + operation,
		},
	)
	if err != nil {
		return fmt.Errorf("factory sessions endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		clidiag.Printf(
			cfg.Diagnostics,
			cfg.Verbose,
			"session %s response endpointPath=%s status=%d durationMillis=%d sessionId=%s outcome=%s status=%s",
			operation,
			endpoint.Path,
			resp.StatusCode,
			time.Since(started).Milliseconds(),
			response.SessionId,
			response.Outcome,
			response.Status,
		)
		if cfg.JSON {
			return json.NewEncoder(cfg.Output).Encode(response)
		}
		return renderLifecycleControlSuccess(cfg.Output, response, operation)
	case http.StatusNotFound:
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("factory session %q not found: %s", resolvedSessionID(cfg.SessionID), errResp.Message)
		}
		return fmt.Errorf("factory session %q not found", resolvedSessionID(cfg.SessionID))
	case http.StatusConflict:
		var conflict factoryapi.FactorySessionLifecycleControlResponse
		if err := json.NewDecoder(resp.Body).Decode(&conflict); err != nil {
			return fmt.Errorf("%s factory session failed (%d): parse conflict response", operation, resp.StatusCode)
		}
		return lifecycleControlRejectedError(conflict, operation)
	default:
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			return fmt.Errorf("%s factory session failed (%d): %s", operation, resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("%s factory session failed (%d)", operation, resp.StatusCode)
	}
}

func lifecycleControlEndpoint(cfg LifecycleControlConfig, operation string) (url.URL, error) {
	endpointPath := sessionpath.ScopedPath("/"+operation, cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse session %s endpoint: %w", operation, err)
	}
	return *endpoint, nil
}

func renderLifecycleControlSuccess(
	output io.Writer,
	response factoryapi.FactorySessionLifecycleControlResponse,
	operation string,
) error {
	sessionID := strings.TrimSpace(response.SessionId)
	if sessionID == "" {
		sessionID = resolvedSessionID("")
	}

	switch response.Outcome {
	case factoryapi.FactorySessionLifecycleControlOutcomeAccepted:
		switch operation {
		case "pause":
			_, err := fmt.Fprintf(output, "Paused factory session %s\n", sessionID)
			return err
		case "resume":
			_, err := fmt.Fprintf(output, "Resumed factory session %s\n", sessionID)
			return err
		default:
			_, err := fmt.Fprintf(output, "Applied %s to factory session %s\n", operation, sessionID)
			return err
		}
	case factoryapi.FactorySessionLifecycleControlOutcomeNoOp:
		switch operation {
		case "pause":
			_, err := fmt.Fprintf(output, "Factory session %s is already paused\n", sessionID)
			return err
		case "resume":
			_, err := fmt.Fprintf(output, "Factory session %s is already running\n", sessionID)
			return err
		default:
			_, err := fmt.Fprintf(output, "Factory session %s already matches %s\n", sessionID, operation)
			return err
		}
	default:
		return lifecycleControlRejectedError(response, operation)
	}
}

func lifecycleControlRejectedError(
	response factoryapi.FactorySessionLifecycleControlResponse,
	operation string,
) error {
	sessionID := strings.TrimSpace(response.SessionId)
	if sessionID == "" {
		sessionID = "factory session"
	}
	message := strings.TrimSpace(stringOrEmpty(response.Detail))
	if message != "" {
		return fmt.Errorf("%s rejected for %s: %s", operation, sessionID, message)
	}
	return fmt.Errorf(
		"%s rejected for %s (outcome=%s status=%s)",
		operation,
		sessionID,
		response.Outcome,
		response.Status,
	)
}

func optionalLifecycleRequestID(requestID string) *string {
	trimmed := strings.TrimSpace(requestID)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
