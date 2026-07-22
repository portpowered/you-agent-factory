package factory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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

// NewDelete binds the Factory Definitions catalog to the CLI representation.
func NewDelete(
	catalog factorydefinitions.NamedFactoryCatalog,
) func(DeleteConfig) error {
	return func(cfg DeleteConfig) error {
		return Delete(catalog, cfg)
	}
}

// Delete removes a persisted named factory from disk.
func Delete(catalog factorydefinitions.NamedFactoryCatalog, cfg DeleteConfig) error {
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return fmt.Errorf("factory name is required")
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		return fmt.Errorf("factory root is required")
	}
	if catalog == nil {
		return fmt.Errorf("Factory Definitions named-factory catalog is required")
	}

	if err := catalog.DeleteNamedFactory(cfg.Dir, name); err != nil {
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
	if errors.Is(err, factorydefinitions.ErrNamedFactoryIsCurrent) {
		return fmt.Errorf("cannot delete current factory: %w", err)
	}
	return err
}
