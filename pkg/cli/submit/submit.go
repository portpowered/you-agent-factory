// Package submit implements agent-factory submit command behavior.
package submit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

// SubmitConfig holds parameters for the submit command.
type SubmitConfig struct {
	Name         string
	WorkTypeName string
	Payload      string
	Server       string
	SessionID    string
	JSON         bool
	Output       io.Writer
	Verbose      bool
	Debug        bool
	Diagnostics  io.Writer
}

// Submit posts work to a running factory via HTTP.
func Submit(cfg SubmitConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if cfg.WorkTypeName == "" {
		return fmt.Errorf("--work-type-name is required")
	}
	if cfg.Payload == "" {
		return fmt.Errorf("--payload is required")
	}

	// Read the payload file.
	data, err := os.ReadFile(cfg.Payload)
	if err != nil {
		return fmt.Errorf("read payload file: %w", err)
	}

	// Build the submit request body.
	var payload json.RawMessage
	payloadType := clidiag.PayloadType(cfg.Payload)
	if payloadType == "json" {
		// JSON files are sent as-is (must be valid JSON).
		if !json.Valid(data) {
			return fmt.Errorf("payload file is not valid JSON: %s", cfg.Payload)
		}
		payload = data
	} else {
		// Non-JSON files (e.g. .md) are JSON-encoded as a string.
		encoded, err := json.Marshal(string(data))
		if err != nil {
			return fmt.Errorf("encode payload: %w", err)
		}
		payload = encoded
	}

	reqBody := factoryapi.SubmitWorkRequest{
		Name:         name,
		WorkTypeName: cfg.WorkTypeName,
		Payload:      payload,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// POST to running factory.
	endpointPath := sessionpath.ScopedPath("/work", cfg.SessionID)
	endpointURL, err := cliserver.RequestURL(cfg.Server, endpointPath)
	if err != nil {
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"submit request endpointPath=%s endpoint=%s server=%s session=%s payloadPath=%s payloadType=%s payloadBytes=%d requestName=%q workTypeName=%q requestBytes=%d",
		endpointPath,
		endpointURL,
		cfg.Server,
		clidiag.SessionLabel(cfg.SessionID),
		cfg.Payload,
		payloadType,
		len(data),
		name,
		cfg.WorkTypeName,
		len(body),
	)
	started := time.Now()
	resp, err := http.Post(endpointURL, "application/json", bytes.NewReader(body))
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit response endpointPath=%s error=unreachable durationMillis=%d", endpointPath, time.Since(started).Milliseconds())
		return fmt.Errorf("factory not reachable at %s: %w", endpointURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit response endpointPath=%s status=%d durationMillis=%d responseBytes=%d", endpointPath, resp.StatusCode, time.Since(started).Milliseconds(), len(respBody))
		var errResp factoryapi.ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return fmt.Errorf("submission failed (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("submission failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result factoryapi.SubmitWorkResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit response endpointPath=%s status=%d durationMillis=%d responseBytes=%d traceId=%s", endpointPath, resp.StatusCode, time.Since(started).Milliseconds(), len(respBody), result.TraceId)

	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	_, err = fmt.Fprintf(cfg.Output, "Submitted work: %s\n", result.TraceId)
	return err
}
