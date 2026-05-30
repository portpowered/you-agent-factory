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

const submitErrorDetailPreviewSize = 200

// SubmitConfirmation is the CLI JSON object emitted after a successful submit.
type SubmitConfirmation struct {
	WorkID       string `json:"workId"`
	Name         string `json:"name"`
	WorkTypeName string `json:"workTypeName"`
	TraceID      string `json:"traceId"`
	SessionID    string `json:"sessionId"`
	EndpointPath string `json:"endpointPath"`
}

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

	payload, data, payloadType, err := readSubmitPayload(cfg.Payload)
	if err != nil {
		return err
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
		return submitRequestError(resp.StatusCode, respBody)
	}

	var result factoryapi.SubmitWorkResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit response endpointPath=%s status=%d durationMillis=%d responseBytes=%d traceId=%s", endpointPath, resp.StatusCode, time.Since(started).Milliseconds(), len(respBody), result.TraceId)

	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(submitConfirmation(cfg, endpointPath, result))
	}
	return renderHumanSubmitConfirmation(cfg.Output, cfg, result)
}

func submitConfirmation(cfg SubmitConfig, endpointPath string, result factoryapi.SubmitWorkResponse) SubmitConfirmation {
	name := optionalString(result.Name)
	if name == "" {
		name = strings.TrimSpace(cfg.Name)
	}
	workTypeName := optionalString(result.WorkTypeName)
	if workTypeName == "" {
		workTypeName = cfg.WorkTypeName
	}
	return SubmitConfirmation{
		WorkID:       optionalString(result.WorkId),
		Name:         name,
		WorkTypeName: workTypeName,
		TraceID:      result.TraceId,
		SessionID:    clidiag.SessionLabel(cfg.SessionID),
		EndpointPath: endpointPath,
	}
}

func renderHumanSubmitConfirmation(output io.Writer, cfg SubmitConfig, result factoryapi.SubmitWorkResponse) error {
	name := optionalString(result.Name)
	if name == "" {
		name = strings.TrimSpace(cfg.Name)
	}
	workTypeName := optionalString(result.WorkTypeName)
	if workTypeName == "" {
		workTypeName = cfg.WorkTypeName
	}
	workID := optionalString(result.WorkId)

	if _, err := fmt.Fprintln(output, "Submitted work"); err != nil {
		return err
	}
	lines := []struct {
		label string
		value string
	}{
		{label: "name", value: name},
		{label: "workTypeName", value: workTypeName},
		{label: "traceId", value: result.TraceId},
	}
	if workID != "" {
		lines = append(lines, struct {
			label string
			value string
		}{label: "workId", value: workID})
	}
	for _, line := range lines {
		if line.value == "" {
			continue
		}
		if _, err := fmt.Fprintf(output, "%s: %s\n", line.label, line.value); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(output, "%s\n", submitVerifyHint(workID, name))
	return err
}

func submitVerifyHint(workID, name string) string {
	if workID != "" {
		return "Verify with: you work show " + workID
	}
	return "Verify with: you work list --name " + name
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func submitRequestError(statusCode int, body []byte) error {
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
		detail := boundSubmitErrorDetail(errResp.Message)
		if workID := submitErrorWorkID(body); workID != "" {
			detail = fmt.Sprintf("%s (workId: %s)", detail, workID)
		}
		return fmt.Errorf("submission failed (%d): %s", statusCode, detail)
	}
	preview := boundSubmitErrorDetail(strings.TrimSpace(string(body)))
	if preview == "" {
		return fmt.Errorf("submission failed (%d)", statusCode)
	}
	return fmt.Errorf("submission failed (%d): %s", statusCode, preview)
}

func boundSubmitErrorDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > submitErrorDetailPreviewSize {
		return detail[:submitErrorDetailPreviewSize] + "..."
	}
	return detail
}

func submitErrorWorkID(body []byte) string {
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return ""
	}
	for _, key := range []string{"workId", "work_id"} {
		value, ok := raw[key]
		if !ok {
			continue
		}
		var workID string
		if json.Unmarshal(value, &workID) != nil {
			continue
		}
		workID = strings.TrimSpace(workID)
		if workID != "" {
			return workID
		}
	}
	return ""
}
