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

// DispatchesConfig holds CLI inputs for one durable Factory Session dispatch list read.
type DispatchesConfig struct {
	SessionID          string
	JSON               bool
	Output             io.Writer
	Service            factorysessionexecution.Service
	FixtureCatalogPath string
}

// RunDispatches loads one durable Factory Session dispatch list through the shared
// execution service and renders deterministic human or JSON output.
func RunDispatches(ctx context.Context, cfg DispatchesConfig) error {
	if cfg.Output == nil {
		cfg.Output = defaultOutputWriter()
	}

	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		return writeRunError(cfg.Output, cfg.JSON, newExecutionError(
			ErrorCodeValidation,
			"workflow dispatches requires a factory session id",
			"sessionId",
		))
	}

	service, err := resolveInspectionService(cfg.Service, cfg.FixtureCatalogPath)
	if err != nil {
		return err
	}

	listed, err := service.ListDispatches(ctx, sessionID)
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}

	mapped := factorysession.ListDispatchesResponseToAPI(listed)
	if cfg.JSON {
		encoded, marshalErr := json.Marshal(mapped)
		if marshalErr != nil {
			return fmt.Errorf("marshal dispatches response: %w", marshalErr)
		}
		_, err = fmt.Fprintln(cfg.Output, string(encoded))
		return err
	}
	return renderDispatchesHuman(cfg.Output, mapped)
}

func renderDispatchesHuman(output io.Writer, result factoryapi.ListFactorySessionDispatchesResponse) error {
	count := 0
	if result.Dispatches != nil {
		count = len(result.Dispatches)
	}
	if _, err := fmt.Fprintf(
		output,
		"Factory session %s dispatches (%d):\n",
		result.SessionId,
		count,
	); err != nil {
		return err
	}
	if result.Dispatches == nil {
		return nil
	}
	for _, dispatch := range result.Dispatches {
		line := fmt.Sprintf(
			"- %s %s %s",
			dispatch.Id,
			dispatch.Status,
			dispatch.DispatchKind,
		)
		if refs := formatProviderSessionRefs(dispatch.ProviderSessionRefs); refs != "" {
			line += " (provider: " + refs + ")"
		}
		if _, err := fmt.Fprintf(output, "%s\n", line); err != nil {
			return err
		}
	}
	return nil
}

func formatProviderSessionRefs(refs *[]factoryapi.LoadableProviderSessionRef) string {
	if refs == nil || len(*refs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*refs))
	for _, ref := range *refs {
		if trimmed := strings.TrimSpace(ref.Id); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, ", ")
}

func resolveInspectionService(
	service factorysessionexecution.Service,
	fixtureCatalogPath string,
) (factorysessionexecution.Service, error) {
	return resolveExecutionService(RunConfig{
		Service:            service,
		FixtureCatalogPath: fixtureCatalogPath,
	})
}
