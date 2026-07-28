// Package systeminitializationwire constructs the System Bootstrap service
// from accepted collaborator roots without publishing additional peer-facing
// Bootstrap authority interfaces.
package systeminitializationwire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	bootstrapworkflow "github.com/portpowered/infinite-you/pkg/services/system_initialization/internal/workflow"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

// NewService constructs the canonical System Bootstrap service from
// already-selected collaborators. Peers depend on the returned
// systeminitialization.Service root contract rather than the internal workflow
// package.
func NewService(
	operatorSettings systeminitialization.OperatorSettings,
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstaller,
	inspectPath systeminitialization.InspectPath,
	migrationFiles systeminitialization.LegacyFactoryMigrationFileSystem,
) (systeminitialization.Service, error) {
	return bootstrapworkflow.New(
		operatorSettings,
		packagedCatalog,
		packagedInstaller,
		inspectPath,
		migrationFiles,
	)
}
