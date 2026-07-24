package factory_visualization

import "context"

// Root is the singular peer-facing Factory Visualization contract.
//
// Cross-service consumers depend on this named root for request-activated
// lifecycle, live projection, and presentation/drain slices. Collaborator
// ports and legacy presentation helpers are not additional Visualization
// authority interfaces for those published slices.
type Root interface {
	// Start activates retained-then-live Factory event projection.
	Start(context.Context) error
	// Stop cancels the live subscription and emits one final projected view.
	Stop(context.Context) error
	// Wait joins the live subscription. Calling Wait before Start returns a
	// not-started failure while the root remains inert.
	Wait(context.Context) error
}

// Compile-time proof that the existing lifecycle Service remains reachable
// through the singular Root seam.
var _ Root = (*Service)(nil)
