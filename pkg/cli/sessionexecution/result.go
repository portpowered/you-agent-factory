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

// ResultConfig holds CLI inputs for one durable Factory Session result read.
type ResultConfig struct {
	SessionID          string
	Mode               string
	IncludeArtifacts   bool
	JSON               bool
	Output             io.Writer
	Service            factorysessionexecution.Service
	FixtureCatalogPath string
}

// ResultOutcomeError signals a rendered durable result that should exit non-zero
// for CLI automation without using the missing-session error contract.
type ResultOutcomeError struct {
	Status factoryapi.FactorySessionResultStatus
}

func (e *ResultOutcomeError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("factory session result status is %s", e.Status)
}

// RunResult loads one durable Factory Session result through the shared execution
// service and renders deterministic human or JSON output.
func RunResult(ctx context.Context, cfg ResultConfig) error {
	if cfg.Output == nil {
		cfg.Output = defaultOutputWriter()
	}

	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		return writeRunError(cfg.Output, cfg.JSON, newExecutionError(
			ErrorCodeValidation,
			"workflow result requires a factory session id",
			"sessionId",
		))
	}

	normalized, err := factorysession.ResultRequestFromCLI(factorysession.CLIResultInput{
		Mode:             cfg.Mode,
		IncludeArtifacts: cfg.IncludeArtifacts,
	})
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}

	service, err := resolveResultService(cfg)
	if err != nil {
		return err
	}

	read, err := service.GetResult(ctx, sessionID, normalized)
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}

	mapped := factorysession.ResultResponseToAPI(read)
	if cfg.JSON {
		encoded, marshalErr := json.Marshal(mapped)
		if marshalErr != nil {
			return fmt.Errorf("marshal result response: %w", marshalErr)
		}
		if _, err = fmt.Fprintln(cfg.Output, string(encoded)); err != nil {
			return err
		}
	} else if err := renderResultHuman(cfg.Output, mapped); err != nil {
		return err
	}
	return resultOutcomeError(mapped.ResultStatus)
}

func resolveResultService(cfg ResultConfig) (factorysessionexecution.Service, error) {
	runCfg := RunConfig{
		Service:            cfg.Service,
		FixtureCatalogPath: cfg.FixtureCatalogPath,
	}
	return resolveExecutionService(runCfg)
}

func resultOutcomeError(status factoryapi.FactorySessionResultStatus) error {
	switch status {
	case factoryapi.FactorySessionResultStatusFinal,
		factoryapi.FactorySessionResultStatusPartial:
		return nil
	case factoryapi.FactorySessionResultStatusNotReady,
		factoryapi.FactorySessionResultStatusUnavailable,
		factoryapi.FactorySessionResultStatusFailedWithPartial:
		return &ResultOutcomeError{Status: status}
	default:
		if status == "" {
			return nil
		}
		return &ResultOutcomeError{Status: status}
	}
}

func renderResultHuman(output io.Writer, result factoryapi.FactorySessionResult) error {
	if _, err := fmt.Fprintf(
		output,
		"Factory session %s result is %s.\n",
		result.SessionId,
		result.ResultStatus,
	); err != nil {
		return err
	}
	if result.SessionStatus != nil {
		if _, err := fmt.Fprintf(output, "Session status: %s\n", *result.SessionStatus); err != nil {
			return err
		}
	}
	if summary := resultDisplaySummary(&result); summary != "" {
		if _, err := fmt.Fprintf(output, "Primary result: %s\n", summary); err != nil {
			return err
		}
	}
	if result.Availability != nil {
		if reason := result.Availability.Reason; reason != nil && strings.TrimSpace(*reason) != "" {
			if _, err := fmt.Fprintf(output, "Availability reason: %s\n", strings.TrimSpace(*reason)); err != nil {
				return err
			}
		}
		if message := result.Availability.Message; message != nil && strings.TrimSpace(*message) != "" {
			if _, err := fmt.Fprintf(output, "Availability message: %s\n", strings.TrimSpace(*message)); err != nil {
				return err
			}
		}
		if retryable := result.Availability.Retryable; retryable != nil && *retryable {
			if _, err := fmt.Fprintf(output, "Retryable: true\n"); err != nil {
				return err
			}
		}
	}
	if result.Failure != nil {
		if reason := result.Failure.Reason; reason != nil && strings.TrimSpace(*reason) != "" {
			if _, err := fmt.Fprintf(output, "Failure reason: %s\n", strings.TrimSpace(*reason)); err != nil {
				return err
			}
		}
		if message := result.Failure.Message; message != nil && strings.TrimSpace(*message) != "" {
			if _, err := fmt.Fprintf(output, "Failure message: %s\n", strings.TrimSpace(*message)); err != nil {
				return err
			}
		}
	}
	return nil
}

func resultDisplaySummary(result *factoryapi.FactorySessionResult) string {
	if result == nil || result.PrimaryResult == nil {
		return ""
	}
	if summary := primaryResultSummary(result); summary != "" {
		return summary
	}
	for _, part := range *result.PrimaryResult {
		jsonPart, err := part.AsWorkJsonContentPart()
		if err != nil {
			continue
		}
		if jsonPart.Json == nil {
			continue
		}
		encoded, err := json.Marshal(jsonPart.Json)
		if err != nil {
			continue
		}
		return string(encoded)
	}
	return ""
}
