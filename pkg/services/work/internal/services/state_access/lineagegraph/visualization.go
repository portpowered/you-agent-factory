package lineagegraph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

const (
	VisualizationFormatMermaid         = "mermaid"
	VisualizationFormatMarkdownMermaid = "markdown-mermaid"
)

type VisualizationFileSystem interface {
	ReadFile(string) ([]byte, error)
}

type VisualizationRequest struct {
	BatchFile string
	Format    string
}

type VisualizationOperation func(VisualizationRequest) (string, error)

// BatchRequestParser decodes canonical batch JSON into the lineagegraph batch shape.
type BatchRequestParser func([]byte) (BatchRequest, error)

// NewVisualizationOperation binds the exact filesystem read edge to Work's
// batch dependency visualization policy.
func NewVisualizationOperation(filesystem VisualizationFileSystem, parse BatchRequestParser) VisualizationOperation {
	return func(request VisualizationRequest) (string, error) {
		if filesystem == nil {
			return "", fmt.Errorf("Work visualization filesystem is required")
		}
		path := strings.TrimSpace(request.BatchFile)
		if path == "" {
			return "", fmt.Errorf("batch file path is required")
		}
		format := strings.TrimSpace(strings.ToLower(request.Format))
		if format == "" {
			format = VisualizationFormatMermaid
		}
		if format != VisualizationFormatMermaid && format != VisualizationFormatMarkdownMermaid {
			return "", fmt.Errorf(
				"unsupported format %q (supported: %s, %s)",
				format,
				VisualizationFormatMermaid,
				VisualizationFormatMarkdownMermaid,
			)
		}

		data, err := filesystem.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("batch file not found: %s", path)
			}
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		if len(data) == 0 {
			return "", fmt.Errorf("batch input is empty")
		}
		if !json.Valid(data) {
			return "", fmt.Errorf("invalid JSON")
		}

		if parse == nil {
			return "", fmt.Errorf("Work visualization batch parser is required")
		}
		workRequest, err := parse(data)
		if err != nil {
			return "", err
		}
		graph, err := DeriveFromBatchRequest(workRequest)
		if err != nil {
			return "", err
		}
		switch format {
		case VisualizationFormatMermaid:
			return RenderMermaidFlowchart(graph), nil
		case VisualizationFormatMarkdownMermaid:
			return RenderMarkdownMermaid(graph), nil
		default:
			return "", fmt.Errorf("unsupported format %q", format)
		}
	}
}
