// Package systeminitializationwire constructs the System Bootstrap service
// from accepted collaborator roots without publishing additional peer-facing
// Bootstrap authority interfaces.
package systeminitializationwire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	bootstrapworkflow "github.com/portpowered/infinite-you/pkg/services/system_initialization/internal/workflow"
)

// NewService constructs the canonical System Bootstrap service from
// already-selected collaborators. Peers depend on the returned
// systeminitialization.Service root contract rather than the internal workflow
// package.
func NewService(
	operatorSettings OperatorSettings,
	definitions factorydefinitions.Service,
	inspectPath InspectPath,
	migrationFiles LegacyFactoryMigrationFileSystem,
) (systeminitialization.Service, error) {
	initializer, err := bootstrapworkflow.New(
		operatorSettings,
		definitions,
		inspectPath,
		migrationFiles,
	)
	if err != nil {
		return nil, err
	}
	return initializer, nil
}
