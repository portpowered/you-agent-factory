package commandregistry

import (
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// ResolvedWorkRunE executes one Work command from invocation-local resolved
// inputs. The second snapshot contains inherited root inputs.
type ResolvedWorkRunE func(
	*cobra.Command,
	resolvedinput.Inputs,
	resolvedinput.Inputs,
) error

// ResolvedWorkHandlers supplies typed handlers for the runnable Work commands.
// Construction maps these handlers through the stable IDs in the manifest.
type ResolvedWorkHandlers struct {
	List      ResolvedWorkRunE
	Show      ResolvedWorkRunE
	Move      ResolvedWorkRunE
	Visualize ResolvedWorkRunE
}
