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

// ArtifactsConfig holds CLI inputs for one durable Factory Session artifact list read.
type ArtifactsConfig struct {
	SessionID          string
	JSON               bool
	Output             io.Writer
	Service            factorysessionexecution.Service
	FixtureCatalogPath string
}

// RunArtifacts loads one durable Factory Session artifact list through the shared
// execution service and renders deterministic human or JSON output.
func RunArtifacts(ctx context.Context, cfg ArtifactsConfig) error {
	if cfg.Output == nil {
		cfg.Output = defaultOutputWriter()
	}

	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		return writeRunError(cfg.Output, cfg.JSON, newExecutionError(
			ErrorCodeValidation,
			"workflow artifacts requires a factory session id",
			"sessionId",
		))
	}

	service, err := resolveInspectionService(cfg.Service, cfg.FixtureCatalogPath)
	if err != nil {
		return err
	}

	listed, err := service.ListArtifacts(ctx, sessionID)
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}

	mapped := factorysession.ListArtifactsResponseToAPI(listed)
	if cfg.JSON {
		encoded, marshalErr := json.Marshal(mapped)
		if marshalErr != nil {
			return fmt.Errorf("marshal artifacts response: %w", marshalErr)
		}
		_, err = fmt.Fprintln(cfg.Output, string(encoded))
		return err
	}
	return renderArtifactsHuman(cfg.Output, mapped)
}

func renderArtifactsHuman(output io.Writer, result factoryapi.ListFactorySessionArtifactsResponse) error {
	count := 0
	if result.Artifacts != nil {
		count = len(result.Artifacts)
	}
	if _, err := fmt.Fprintf(
		output,
		"Factory session %s artifacts (%d):\n",
		result.SessionId,
		count,
	); err != nil {
		return err
	}
	if result.Artifacts == nil {
		return nil
	}
	for _, artifact := range result.Artifacts {
		name := artifact.Id
		if artifact.Label != nil && strings.TrimSpace(*artifact.Label) != "" {
			name = strings.TrimSpace(*artifact.Label)
		}
		line := fmt.Sprintf("- %s %s %s", artifact.Id, name, artifact.Kind)
		if artifact.RetrievalRef != nil && strings.TrimSpace(artifact.RetrievalRef.Href) != "" {
			line += " (" + strings.TrimSpace(artifact.RetrievalRef.Href) + ")"
		}
		if _, err := fmt.Fprintf(output, "%s\n", line); err != nil {
			return err
		}
	}
	return nil
}
