package identityinventory

import documentidentityinventory "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/identityinventory"

// ProjectInputInventory builds the deterministic system-config input inventory
// from committed fixtures and documented loader outcomes.
func ProjectInputInventory() InputInventory {
	return documentidentityinventory.ProjectInputInventory()
}
