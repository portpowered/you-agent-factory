package systeminitializationwire

import bootstrapworkflow "github.com/portpowered/infinite-you/pkg/services/system_initialization/internal/workflow"

// Construction ports are owned by the workflow implementation and exposed
// here only for service composition. They are intentionally absent from the
// peer-facing system_initialization root package.
type (
	InspectPath                      = bootstrapworkflow.InspectPath
	LegacyFactoryMigrationFileSystem = bootstrapworkflow.LegacyFactoryMigrationFileSystem
	OperatorSettings                 = bootstrapworkflow.OperatorSettings
	OperatorSettingsFunctions        = bootstrapworkflow.OperatorSettingsFunctions
)
