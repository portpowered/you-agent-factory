package factory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

// ListConfig holds parameters for the factory list command.
type ListConfig struct {
	Dir     string
	JSON    bool
	Verbose bool
	Output  io.Writer
}

// List prints persisted named factories under the selected factory root.
func List(cfg ListConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		return fmt.Errorf("factory root is required")
	}

	entries, err := factoryconfig.ListNamedFactories(cfg.Dir)
	if err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(entries)
	}
	return renderFactoryList(entries, cfg.Output)
}

func renderFactoryList(entries []factoryconfig.NamedFactoryListEntry, output io.Writer) error {
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
