package root

import (
	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/wire"
)

// AutomationsRootFromEdges constructs the published Automations Root through the
// same AutomationFactory wiring used by BuildProcess / InjectBundle.
func AutomationsRootFromEdges(
	edges serviceedges.Edges,
	workflowID string,
	defaultFactoryDir string,
) (automations.Root, error) {
	return wire.AutomationsRootFromEdges(edges, workflowID, defaultFactoryDir)
}
