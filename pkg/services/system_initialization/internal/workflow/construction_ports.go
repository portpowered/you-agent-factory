package workflow

import (
	"io/fs"

	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// InspectPath is the workflow-local filesystem port used to inspect paths
// while initializing the system configuration.
type InspectPath func(string) (fs.FileInfo, error)

// OperatorSettings is the workflow-local adapter port used to load and
// initialize the operator configuration.
type OperatorSettings interface {
	LoadFileConfig(string) (operatorconfig.Config, error)
	EnsureLocalBackendScope(string) (operatorconfig.ResolvedBackendScope, error)
}
