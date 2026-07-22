package factory

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ListConfig holds parameters for the factory list command.
type ListConfig struct {
	Dir     string
	JSON    bool
	Verbose bool
	Output  io.Writer
}

// NewList binds the Factory Definitions catalog to the CLI representation.
func NewList(
	catalog factorydefinitions.NamedFactoryCatalog,
) func(ListConfig) error {
	return func(cfg ListConfig) error {
		return List(catalog, cfg)
	}
}

// List prints persisted named factories under the selected factory root.
func List(catalog factorydefinitions.NamedFactoryCatalog, cfg ListConfig) error {
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		return fmt.Errorf("factory root is required")
	}
	if catalog == nil {
		return fmt.Errorf("Factory Definitions named-factory catalog is required")
	}

	entries, err := catalog.ListNamedFactories(cfg.Dir)
	if err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(entries)
	}
	return renderFactoryList(entries, cfg.Output)
}

func renderFactoryList(entries []factorydefinitions.NamedFactoryListEntry, output io.Writer) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(output, "No factories found.")
		return err
	}

	if _, err := fmt.Fprintln(output, "NAME\tFACTORY DIRECTORY\tCURRENT"); err != nil {
		return err
	}
	for _, entry := range entries {
		current := ""
		if entry.Current {
			current = "yes"
		}
		if _, err := fmt.Fprintf(output, "%s\t%s\t%s\n", entry.Name, entry.FactoryDir, current); err != nil {
			return err
		}
	}
	return nil
}
