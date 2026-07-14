package work

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	workgraph "github.com/portpowered/infinite-you/pkg/work/graph"
)

const (
	visualizeFormatMermaid         = "mermaid"
	visualizeFormatMarkdownMermaid = "markdown-mermaid"
)

var supportedVisualizeFormats = []string{
	visualizeFormatMermaid,
	visualizeFormatMarkdownMermaid,
}

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
	if !isSupportedVisualizeFormat(format) {
		return fmt.Errorf("unsupported format %q (supported: %s)", format, strings.Join(supportedVisualizeFormats, ", "))
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
	if len(data) == 0 {
		return fmt.Errorf("batch input is empty")
	}
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON")
	}

	request, err := factoryrequests.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return err
	}
	graph, err := workgraph.DeriveFromWorkRequest(request)
	if err != nil {
		return err
	}

	var output string
	switch format {
	case visualizeFormatMermaid:
		output = workgraph.RenderMermaidFlowchart(graph)
	case visualizeFormatMarkdownMermaid:
		output = workgraph.RenderMarkdownMermaid(graph)
	default:
		return fmt.Errorf("unsupported format %q (supported: %s)", format, strings.Join(supportedVisualizeFormats, ", "))
	}
	_, err = io.WriteString(cfg.Output, output)
	return err
}

func isSupportedVisualizeFormat(format string) bool {
	for _, supported := range supportedVisualizeFormats {
		if format == supported {
			return true
		}
	}
	return false
}

func normalizeVisualizeFormat(format string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(format))
	if normalized == "" {
		return visualizeFormatMermaid, nil
	}
	return normalized, nil
}
