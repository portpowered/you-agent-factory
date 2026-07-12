package factory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
)

// SaveCurrentConfig holds parameters for persisting the live current factory.
type SaveCurrentConfig struct {
	Server      string
	SessionID   string
	JSON        bool
	Verbose     bool
	Output      io.Writer
	Diagnostics io.Writer
}

// SaveCurrent reads the session current factory and persists it with PUT.
func SaveCurrent(cfg SaveCurrentConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	saved, err := replaceCurrentFactory(replaceCurrentOptions{
		Server:      cfg.Server,
		SessionID:   cfg.SessionID,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
		labels:      replaceCurrentLegacySaveLabels,
	})
	if err != nil {
		return renderSaveCurrentError(err)
	}

	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(saved)
	}

	return renderSaveCurrentSuccess(saved, cfg.SessionID, cfg.Output)
}

func renderSaveCurrentSuccess(saved factoryapi.Factory, sessionID string, output io.Writer) error {
	_, err := fmt.Fprintf(
		output,
		"Saved factory %s\nSession: %s\n",
		saved.Name,
		clidiag.SessionLabel(sessionID),
	)
	return err
}

func renderSaveCurrentError(err error) error {
	if errors.Is(err, ErrCurrentFactoryNotFound) {
		return fmt.Errorf("running service has no active current factory; start a factory or activate a named factory: %w", err)
	}
	return err
}
