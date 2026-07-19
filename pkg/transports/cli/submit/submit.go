// Package submit implements agent-factory submit command behavior.
package submit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const submitRequestTimeout = 15 * time.Second

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
		Name:         &name,
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
	client := &http.Client{Timeout: submitRequestTimeout}
	var result factoryapi.SubmitWorkResponse
	resp, err := clihttp.PostJSONCreated(
		context.Background(),
		client,
		endpointURL,
		bytes.NewReader(body),
		&result,
		clihttp.RequestOptions{
			Diagnostics:  cfg.Diagnostics,
			Verbose:      cfg.Verbose,
			EndpointPath: endpointPath,
			LogLabel:     "submit",
		},
	)
	if err != nil {
		return fmt.Errorf("factory not reachable at %s: %w", endpointURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}
		return submitFailureError(resp.StatusCode, respBody)
	}

	responseBytes := 0
	if encoded, marshalErr := json.Marshal(result); marshalErr == nil {
		responseBytes = len(encoded)
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit response endpointPath=%s status=%d durationMillis=%d responseBytes=%d traceId=%s", endpointPath, resp.StatusCode, time.Since(started).Milliseconds(), responseBytes, result.TraceId)

	if cfg.JSON {
		return writeJSONSubmitSuccess(
			cfg.Output,
			result,
			endpointPath,
			name,
			cfg.WorkTypeName,
			clidiag.SessionLabel(cfg.SessionID),
		)
	}
	return writeHumanSubmitSuccess(cfg.Output, result, name, cfg.WorkTypeName)
}
