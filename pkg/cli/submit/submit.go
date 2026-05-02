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

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// SubmitConfig holds parameters for the submit command.
type SubmitConfig struct {
	WorkTypeName string
	Payload      string
	Port         int
}

// Submit posts work to a running factory via HTTP.
func Submit(cfg SubmitConfig) error {
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
	if strings.HasSuffix(cfg.Payload, ".json") {
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
		WorkTypeName: cfg.WorkTypeName,
		Payload:      payload,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// POST to running factory.
	url := fmt.Sprintf("http://localhost:%d/work", cfg.Port)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("factory not reachable at %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
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

	fmt.Printf("Submitted work: %s\n", result.TraceId)
	return nil
}
