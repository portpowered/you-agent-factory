package sessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// StatusConfig holds CLI inputs for one durable Factory Session status read.
type StatusConfig struct {
	SessionID          string
	JSON               bool
	Output             io.Writer
	Service            factorysessionexecution.Service
	FixtureCatalogPath string
}

// RunStatus loads one durable Factory Session read model through the shared execution
// service and renders deterministic human or JSON output.
func RunStatus(ctx context.Context, cfg StatusConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		return writeRunError(cfg.Output, cfg.JSON, newExecutionError(
			ErrorCodeValidation,
			"workflow status requires a factory session id",
			"sessionId",
		))
	}

	service, err := resolveStatusService(cfg)
	if err != nil {
		return err
	}

	read, err := service.GetSession(ctx, sessionID)
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}

	mapped := factorysession.SessionReadResponseToAPI(read)
	if cfg.JSON {
		encoded, marshalErr := json.Marshal(mapped)
		if marshalErr != nil {
			return fmt.Errorf("marshal status response: %w", marshalErr)
		}
		_, err = fmt.Fprintln(cfg.Output, string(encoded))
		return err
	}
	return renderStatusHuman(cfg.Output, mapped)
}

func resolveStatusService(cfg StatusConfig) (factorysessionexecution.Service, error) {
	runCfg := RunConfig{
		Service:            cfg.Service,
		FixtureCatalogPath: cfg.FixtureCatalogPath,
	}
	return resolveExecutionService(runCfg)
}

func renderStatusHuman(output io.Writer, result factoryapi.FactorySessionDurableReadModel) error {
	if _, err := fmt.Fprintf(
		output,
		"Factory session %s is %s.\n",
		result.SessionId,
		result.Status,
	); err != nil {
		return err
	}
	if result.Phase != nil {
		if err := writeOptionalTrimmedLine(output, "Phase", *result.Phase); err != nil {
			return err
		}
	}
	if summary := formatProgressSummary(result.Progress); summary != "" {
		if _, err := fmt.Fprintf(output, "%s\n", summary); err != nil {
			return err
		}
	}
	if result.ResultSummary != nil {
		if _, err := fmt.Fprintf(
			output,
			"Result availability: %s\n",
			result.ResultSummary.ResultStatus,
		); err != nil {
			return err
		}
	}
	if result.ArtifactRefs != nil && len(*result.ArtifactRefs) > 0 {
		if _, err := fmt.Fprintf(output, "Artifacts: %d\n", len(*result.ArtifactRefs)); err != nil {
			return err
		}
	}
	return writeExecutionLinksHuman(output, result.Links)
}

func formatProgressSummary(progress *factoryapi.FactorySessionDurableProgressCounts) string {
	if progress == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if progress.TotalDispatches != nil {
		parts = append(parts, fmt.Sprintf("total dispatches %d", *progress.TotalDispatches))
	}
	if progress.CompletedDispatches != nil {
		parts = append(parts, fmt.Sprintf("completed %d", *progress.CompletedDispatches))
	}
	if progress.InFlightDispatches != nil {
		parts = append(parts, fmt.Sprintf("in flight %d", *progress.InFlightDispatches))
	}
	if progress.FailedDispatches != nil && *progress.FailedDispatches > 0 {
		parts = append(parts, fmt.Sprintf("failed %d", *progress.FailedDispatches))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Progress: " + strings.Join(parts, ", ")
}

func defaultOutputWriter() io.Writer {
	return os.Stdout
}
