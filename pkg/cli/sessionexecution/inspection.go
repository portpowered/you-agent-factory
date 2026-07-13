package sessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
)

// DispatchesConfig holds CLI inputs for one durable Factory Session dispatch list read.
type DispatchesConfig struct {
	SessionID string
	ExecutionBackendConfig
	JSON               bool
	Output             io.Writer
	Service            factorysessionexecution.Service
	FixtureCatalogPath string
	Phase              string
	Status             string
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

	service, err := resolveInspectionService(cfg.Service, cfg.ExecutionBackendConfig, cfg.FixtureCatalogPath)
	if err != nil {
		return err
	}

	listed, err := factorysessionexecution.QueryDispatches(ctx, service, sessionID, factorysessionexecution.DispatchFilters{
		Phase: cfg.Phase, Status: factorysessionexecution.DispatchStatus(cfg.Status),
	})
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
		if provider := optionalString(dispatch.Provider); provider != "" {
			line += " provider=" + provider
		}
		if refs := formatProviderSessionRefs(dispatch.ProviderSessionRefs); refs != "" {
			line += " (provider session: " + refs + ")"
		}
		if ids := formatStringSlice(dispatch.OutputArtifactIds); ids != "" {
			line += " artifacts=" + ids
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

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func formatStringSlice(values *[]string) string {
	if values == nil || len(*values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*values))
	for _, value := range *values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, ", ")
}

// ArtifactsConfig holds CLI inputs for one durable Factory Session artifact list read.
type ArtifactsConfig struct {
	SessionID string
	ExecutionBackendConfig
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

	service, err := resolveInspectionService(cfg.Service, cfg.ExecutionBackendConfig, cfg.FixtureCatalogPath)
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
		if dispatchID := optionalString(artifact.DispatchId); dispatchID != "" {
			line += " dispatch=" + dispatchID
		}
		if artifact.RetrievalRef != nil && strings.TrimSpace(artifact.RetrievalRef.Href) != "" {
			line += " (" + strings.TrimSpace(artifact.RetrievalRef.Href) + ")"
		}
		if _, err := fmt.Fprintf(output, "%s\n", line); err != nil {
			return err
		}
	}
	return nil
}

// EventsConfig holds CLI inputs for one durable Factory Session event poll read.
type EventsConfig struct {
	SessionID     string
	AfterEventID  string
	AfterSequence *int
	ExecutionBackendConfig
	JSON               bool
	Output             io.Writer
	Service            factorysessionexecution.Service
	FixtureCatalogPath string
}

// RunEvents loads ordered durable Factory Session events through the shared execution
// service and renders deterministic human or JSON output.
func RunEvents(ctx context.Context, cfg EventsConfig) error {
	if cfg.Output == nil {
		cfg.Output = defaultOutputWriter()
	}

	sessionID := strings.TrimSpace(cfg.SessionID)
	if sessionID == "" {
		return writeRunError(cfg.Output, cfg.JSON, newExecutionError(
			ErrorCodeValidation,
			"workflow events requires a factory session id",
			"sessionId",
		))
	}

	reconnect, err := factorysession.EventReconnectRequestFromCLI(factorysession.CLIEventReconnectInput{
		AfterEventID:  cfg.AfterEventID,
		AfterSequence: cfg.AfterSequence,
	})
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}

	service, err := resolveInspectionService(cfg.Service, cfg.ExecutionBackendConfig, cfg.FixtureCatalogPath)
	if err != nil {
		return err
	}

	read, err := service.ReadEvents(ctx, sessionID, reconnect)
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}

	events := factorysession.EventReadResponseToAPI(read)
	if cfg.JSON {
		encoded, marshalErr := json.Marshal(events)
		if marshalErr != nil {
			return fmt.Errorf("marshal events response: %w", marshalErr)
		}
		_, err = fmt.Fprintln(cfg.Output, string(encoded))
		return err
	}
	return renderEventsHuman(cfg.Output, sessionID, events)
}

func renderEventsHuman(output io.Writer, sessionID string, events []factoryapi.FactoryEvent) error {
	if _, err := fmt.Fprintf(
		output,
		"Factory session %s events (%d):\n",
		sessionID,
		len(events),
	); err != nil {
		return err
	}
	for _, event := range events {
		sequence := event.Context.Sequence
		if _, err := fmt.Fprintf(
			output,
			"- %s %s (sequence %d)\n",
			event.Type,
			event.Id,
			sequence,
		); err != nil {
			return err
		}
	}
	return nil
}

func resolveInspectionService(
	service factorysessionexecution.Service,
	backend ExecutionBackendConfig,
	fixtureCatalogPath string,
) (factorysessionexecution.Service, error) {
	return resolveExecutionService(RunConfig{
		ExecutionBackendConfig: backend,
		Service:                service,
		FixtureCatalogPath:     fixtureCatalogPath,
	})
}
