// Package work implements composition-facing facades over the Work-owned CLI adapter.
package work

import (
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli"
)

type VisualizeConfig = workcli.VisualizeConfig

func NewVisualize(visualize workdomain.VisualizationOperation) func(VisualizeConfig) error {
	return workcli.BindVisualize(visualize)
}

func Visualize(visualize workdomain.VisualizationOperation, cfg VisualizeConfig) error {
	return workcli.Visualize(visualize, cfg)
}
