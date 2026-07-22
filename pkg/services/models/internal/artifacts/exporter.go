// Package artifacts owns model-invocation artifact materialization.
package artifacts

import (
	"fmt"
	"io"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

type Exporter struct {
	filesystem models.InvocationArtifactFileSystem
}

func NewExporter(filesystem models.InvocationArtifactFileSystem) (*Exporter, error) {
	if filesystem == nil {
		return nil, fmt.Errorf("construct model invocation artifact exporter: filesystem is required")
	}
	return &Exporter{filesystem: filesystem}, nil
}

func (e *Exporter) ExportInvocationArtifact(sourcePath, destinationPath string) error {
	if e == nil || e.filesystem == nil {
		return fmt.Errorf("model invocation artifact exporter is required")
	}
	input, err := e.filesystem.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open streamed invocation output: %w", err)
	}
	defer input.Close()

	output, err := e.filesystem.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

var _ models.InvocationArtifactExporter = (*Exporter)(nil)
