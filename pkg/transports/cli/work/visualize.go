package work

import (
	"context"
	"fmt"
	"io"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
)

// VisualizeConfig holds parameters for the work visualize command.
type VisualizeConfig struct {
	Context   context.Context
	BatchFile string
	Format    string
	Output    io.Writer
}

// NewVisualize binds Work's visualization operation to CLI output encoding.
func NewVisualize(visualize workdomain.VisualizationOperation) func(VisualizeConfig) error {
	return func(cfg VisualizeConfig) error { return Visualize(visualize, cfg) }
}

// Visualize writes one Work-owned visualization result to the Cobra output.
func Visualize(visualize workdomain.VisualizationOperation, cfg VisualizeConfig) error {
	if cfg.Output == nil {
		return fmt.Errorf("work visualization output is required")
	}
	if visualize == nil {
		return fmt.Errorf("Work visualization operation is required")
	}
	if cfg.Context != nil {
		if err := cfg.Context.Err(); err != nil {
			return err
		}
	}
	output, err := visualize(workdomain.VisualizationRequest{BatchFile: cfg.BatchFile, Format: cfg.Format})
	if err != nil {
		return err
	}
	if cfg.Context != nil {
		if err := cfg.Context.Err(); err != nil {
			return err
		}
	}
	_, err = io.WriteString(cfg.Output, output)
	return err
}
