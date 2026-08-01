package workflow

import (
	"io/fs"

	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// InspectPath is the workflow-local filesystem port used to inspect paths
// while initializing the system configuration.
type InspectPath func(string) (fs.FileInfo, error)

// LegacyFactoryMigrationFileSystem is the workflow-local filesystem port used
// to migrate customer-owned Factories from the retired global catalog.
type LegacyFactoryMigrationFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadFile(string) ([]byte, error)
	ReadDir(string) ([]fs.DirEntry, error)
	MkdirAll(string, fs.FileMode) error
	Rename(string, string) error
}

// OperatorSettings is the workflow-local adapter port used to load and
// initialize the operator configuration.
type OperatorSettings interface {
	LoadFileConfig(string) (operatorconfig.Config, error)
	EnsureLocalBackendScope(string) (operatorconfig.ResolvedBackendScope, error)
}

// OperatorSettingsFunctions adapts function values to OperatorSettings at the
// service-owned construction boundary.
type OperatorSettingsFunctions struct {
	Load   func(string) (operatorconfig.Config, error)
	Ensure func(string) (operatorconfig.ResolvedBackendScope, error)
}

func (functions OperatorSettingsFunctions) LoadFileConfig(path string) (operatorconfig.Config, error) {
	return functions.Load(path)
}

func (functions OperatorSettingsFunctions) EnsureLocalBackendScope(path string) (operatorconfig.ResolvedBackendScope, error) {
	return functions.Ensure(path)
}
