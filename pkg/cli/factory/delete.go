package factory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

// DeleteConfig holds parameters for deleting a persisted named factory.
type DeleteConfig struct {
	Name   string
	Dir    string
	JSON   bool
	Output io.Writer
}

// DeleteResult reports a successful factory deletion.
type DeleteResult struct {
	Name string `json:"name"`
}

// Delete removes a persisted named factory from disk.
func Delete(cfg DeleteConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return fmt.Errorf("factory name is required")
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		return fmt.Errorf("factory root is required")
	}

	if err := factoryconfig.DeleteNamedFactory(cfg.Dir, name); err != nil {
		return renderDeleteError(err)
	}

	result := DeleteResult{Name: name}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(result)
	}
	_, err := fmt.Fprintf(cfg.Output, "Deleted factory %s\n", name)
	return err
}

func renderDeleteError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("factory not found: %w", err)
	}
	if errors.Is(err, factoryconfig.ErrNamedFactoryIsCurrent) {
		return fmt.Errorf("cannot delete current factory: %w", err)
	}
	return err
}
