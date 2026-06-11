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

// EventsConfig holds CLI inputs for one durable Factory Session event poll read.
type EventsConfig struct {
	SessionID          string
	AfterEventID       string
	AfterSequence      *int
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

	service, err := resolveInspectionService(cfg.Service, cfg.FixtureCatalogPath)
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
