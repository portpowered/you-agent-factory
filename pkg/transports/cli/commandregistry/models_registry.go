package commandregistry

import (
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// ModelsHandler is the Models transport-owned Cobra command surface.
type ModelsHandler interface {
	List(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	Inspect(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	Invoke(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	Pull(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
}
