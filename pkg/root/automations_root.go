package root

import (
	"fmt"

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
	root, err := wire.AutomationsRootFromEdges(edges, workflowID, defaultFactoryDir)
	if err != nil {
		return automations.Root{}, fmt.Errorf("build automations root: %w", err)
	}
	return root, nil
}
