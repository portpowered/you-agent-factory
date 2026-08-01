package workflow

import (
	"io/fs"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
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

// OperatorSettings is an alias for the complete unary root retained only for
// the existing service-local wire composition alias. System Initialization
// does not define a narrowed Settings adapter contract.
type OperatorSettings = operatorsettings.Service

// OperatorSettingsFunctions remains a source-compatible type alias for old
// service-local composition callers; it is the complete Settings root and no
// longer adapts individual function ports.
type OperatorSettingsFunctions = operatorsettings.Service
