package sessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
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
	if trimmed := strings.TrimSpace(requestID); trimmed != "" {
		if _, err := fmt.Fprintf(output, "Request id: %s\n", trimmed); err != nil {
			return err
		}
	}
	if result.SourceHash != nil && strings.TrimSpace(*result.SourceHash) != "" {
		if _, err := fmt.Fprintf(output, "Source hash: %s\n", strings.TrimSpace(*result.SourceHash)); err != nil {
			return err
		}
	} else if ref := result.ResolvedSource.SourceRef; ref != nil && strings.TrimSpace(*ref) != "" {
		if _, err := fmt.Fprintf(output, "Source ref: %s\n", strings.TrimSpace(*ref)); err != nil {
			return err
		}
	}
	if result.Links != nil {
		if result.Links.Status != nil && strings.TrimSpace(*result.Links.Status) != "" {
			if _, err := fmt.Fprintf(output, "Status link: %s\n", strings.TrimSpace(*result.Links.Status)); err != nil {
				return err
			}
		}
		if result.Links.Results != nil && strings.TrimSpace(*result.Links.Results) != "" {
			if _, err := fmt.Fprintf(output, "Results link: %s\n", strings.TrimSpace(*result.Links.Results)); err != nil {
				return err
			}
		}
		if result.Links.Session != nil && strings.TrimSpace(*result.Links.Session) != "" {
			if _, err := fmt.Fprintf(output, "Session link: %s\n", strings.TrimSpace(*result.Links.Session)); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(
		output,
		"Follow-up: you workflow status %s\n",
		result.SessionId,
	); err != nil {
		return err
	}
	return nil
}
