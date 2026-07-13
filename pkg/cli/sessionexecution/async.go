package sessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// RunAsync normalizes CLI inputs, executes one asynchronous durable Factory Session
// start through the shared execution service, and renders deterministic human or JSON output.
func RunAsync(ctx context.Context, cfg RunConfig) error {
	if cfg.Output == nil {
		cfg.Output = defaultOutputWriter()
	}

	normalized, mode, err := NormalizeStartRequest(cfg.StartConfig)
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}
	if mode != ExecutionModeAsync {
		return writeRunError(cfg.Output, cfg.JSON, newExecutionError(
			ErrorCodeUnsupportedMode,
			"workflow start requires async execution mode",
			"mode",
		))
	}

	service, err := resolveExecutionService(cfg)
	if err != nil {
		return err
	}

	result, err := service.StartAsync(ctx, normalized)
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}

	mapped := factorysession.AsyncStartResponseToAPI(result)
	if cfg.JSON {
		availability, availabilityErr := asyncResultAvailability(ctx, service, mapped.SessionId)
		if availabilityErr != nil {
			return writeRunError(cfg.Output, cfg.JSON, availabilityErr)
		}
		encoded, marshalErr := marshalAsyncRunJSON(mapped, normalized.RequestID, availability)
		if marshalErr != nil {
			return fmt.Errorf("marshal async start response: %w", marshalErr)
		}
		_, err = fmt.Fprintln(cfg.Output, string(encoded))
		return err
	}
	return renderAsyncRunHuman(cfg.Output, mapped, normalized.RequestID)
}

func asyncResultAvailability(
	ctx context.Context,
	service factorysessionexecution.Service,
	sessionID string,
) (factoryapi.FactorySessionResultStatus, error) {
	result, err := service.GetResult(ctx, sessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModePartial,
	})
	if err != nil {
		return "", err
	}
	return factoryapi.FactorySessionResultStatus(result.ResultStatus), nil
}

type asyncRunCLIResponse struct {
	factoryapi.FactorySessionExecutionResponse
	RequestID          string `json:"requestId,omitempty"`
	ResultAvailability string `json:"resultAvailability,omitempty"`
}

func marshalAsyncRunJSON(
	response factoryapi.FactorySessionExecutionResponse,
	requestID string,
	resultAvailability factoryapi.FactorySessionResultStatus,
) ([]byte, error) {
	payload := asyncRunCLIResponse{
		FactorySessionExecutionResponse: response,
	}
	if trimmed := strings.TrimSpace(requestID); trimmed != "" {
		payload.RequestID = trimmed
	}
	if resultAvailability != "" {
		payload.ResultAvailability = string(resultAvailability)
	}
	return json.Marshal(payload)
}

func renderAsyncRunHuman(
	output io.Writer,
	result factoryapi.FactorySessionExecutionResponse,
	requestID string,
) error {
	if _, err := fmt.Fprintf(
		output,
		"Factory session %s started (%s).\n",
		result.SessionId,
		result.Status,
	); err != nil {
		return err
	}
	if err := writeOptionalTrimmedLine(output, "Request id", requestID); err != nil {
		return err
	}
	if err := writeResolvedSourceHuman(output, result.SourceHash, result.ResolvedSource); err != nil {
		return err
	}
	if err := writeExecutionLinksHuman(output, result.Links); err != nil {
		return err
	}
	_, err := fmt.Fprintf(
		output,
		"Follow-up: you workflow status %s\n",
		result.SessionId,
	)
	return err
}
