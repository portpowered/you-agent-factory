// Package submit implements agent-factory submit command behavior.
package submit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// SubmitConfig holds parameters for the submit command.
type SubmitConfig struct {
	Context      context.Context
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
	HTTP         clihttp.Protocol
}

// Submit posts work to a running factory via HTTP.
func NewSubmit(read workdomain.PayloadFileReader, transport clihttp.Protocol) func(SubmitConfig) error {
	return func(cfg SubmitConfig) error { cfg.HTTP = transport; return submit(read, cfg) }
}

func submit(read workdomain.PayloadFileReader, cfg SubmitConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
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
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}

	payload, data, payloadType, err := readSubmitPayload(read, cfg.Payload)
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
	var result factoryapi.SubmitWorkResponse
	response, err := cfg.HTTP.PostJSONCreated(
		cfg.Context,
		endpointURL,
		bytes.NewReader(body),
		&result,
	)
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit response endpointPath=%s error=unreachable durationMillis=%d", endpointPath, response.Duration.Milliseconds())
		return fmt.Errorf("factory not reachable at %s: %w", endpointURL, err)
	}
	resp := response.HTTP
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit response endpointPath=%s status=%d durationMillis=%d", endpointPath, resp.StatusCode, response.Duration.Milliseconds())
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
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "submit response endpointPath=%s status=%d durationMillis=%d responseBytes=%d traceId=%s", endpointPath, resp.StatusCode, response.Duration.Milliseconds(), responseBytes, result.TraceId)

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
