package work

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/workgraph"
)

const visualizeFormatMermaid = "mermaid"

// VisualizeConfig holds parameters for the work visualize command.
type VisualizeConfig struct {
	BatchFile string
	Format    string
	Output    io.Writer
}

// Visualize reads a batch file and writes a Mermaid dependency graph to stdout.
func Visualize(cfg VisualizeConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	format, err := normalizeVisualizeFormat(cfg.Format)
	if err != nil {
		return err
	}
	if format != visualizeFormatMermaid {
		return fmt.Errorf("unsupported format %q (supported: %s)", format, visualizeFormatMermaid)
	}

	path := strings.TrimSpace(cfg.BatchFile)
	if path == "" {
		return fmt.Errorf("batch file path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("batch file not found: %s", path)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	graph, err := workgraph.DeriveFromJSON(data)
	if err != nil {
		return err
	}

	_, err = io.WriteString(cfg.Output, workgraph.RenderMermaidFlowchart(graph))
	return err
}

func normalizeVisualizeFormat(format string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(format))
	if normalized == "" {
		return visualizeFormatMermaid, nil
	}
	return normalized, nil
}
