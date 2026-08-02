// Package contracts contains the private authoring-layout ports. These
// capabilities are construction details of Factory Definitions and never
// cross the public root Service.
package contracts

import (
	"context"
	"io/fs"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ReaderFileSystem is the exact read capability needed by authored layout
// readers and source discovery.
type ReaderFileSystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// WriterFileSystem is the exact filesystem capability needed to materialize
// split authored layouts.
type WriterFileSystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	ReadDir(string) ([]fs.DirEntry, error)
	RemoveAll(string) error
	WriteFile(string, []byte, fs.FileMode) error
}

// PersistenceFileSystem is the exact capability needed to stage and publish
// one named Factory directory.
type PersistenceFileSystem interface {
	MkdirTemp(string, string) (string, error)
	RemoveAll(string) error
	Rename(string, string) error
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
}

// DirectoryReplacementStore supplies atomic directory replacement mechanics.
type DirectoryReplacementStore interface {
	Commit(parentDir, targetDir, stagingDir string) (backupDir string, err error)
	Restore(targetDir, backupDir string)
}

// DefinitionDirectoryRequirer verifies that a selected Factory definition
// directory has the required authored layout.
type DefinitionDirectoryRequirer func(string) error

// LayoutValidator is the validation slice needed while preparing a layout.
type LayoutValidator interface {
	PruneLayout(
		context.Context,
		*factorydefinitions.FactoryConfig,
		factorydefinitions.PendingFactoryGraphTopology,
	) factorydefinitions.ValidationResult
}

// DefinitionValidationOperation is the one validation operation needed for
// pre-persist admission.
type DefinitionValidationOperation interface {
	ValidateDefinition(
		context.Context,
		factorydefinitions.DefinitionValidationRequest,
	) (factorydefinitions.ValidationResult, error)
}

type LayoutPayloadMapper func([]byte) (factorydefinitions.DefinitionValidationRequest, error)
type FactoryConfigJSONDecoder func([]byte) (*factorydefinitions.FactoryConfig, error)
type FactoryConfigJSONEncoder func(*factorydefinitions.FactoryConfig) ([]byte, error)
type AuthoredFactoryNormalizer func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error)
type LayoutWriter func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error
type LayoutValidatorFunc func(string) error
type LayoutFlattener func(string) ([]byte, error)
type LayoutExpander func(string) (string, factorydefinitions.LayoutExpansionReport, error)

type WorkerParser func([]byte, string) (*factorydefinitions.FactoryWorkerConfig, error)
type WorkstationParser func([]byte, string) (*factorydefinitions.FactoryWorkstationConfig, error)
type BodyParser func([]byte, string) (string, error)
type FactorySourceLoader func(string) (factorydefinitions.AuthoredFactorySource, error)

type WorkerRenderer func(factorydefinitions.FactoryWorkerConfig) ([]byte, error)
type WorkstationRenderer func(factorydefinitions.FactoryWorkstationConfig) ([]byte, error)
type BodyRenderer func(string) []byte
type AgentsWriter func(string, []byte) error
type SegmentResolver func(string, string) (string, error)
type PromptPathResolver func(string, string) (string, error)
type InputInboxSentinelEnsurer interface {
	EnsureInputInboxGitkeep(string, string) error
}

type PortableBundledFilesMaterializer func(
	string,
	*factorydefinitions.FactoryConfig,
) ([]factorydefinitions.PortableBundledFileReplacement, error)
type PortableBundledFileWritesValidator func(string, *factorydefinitions.FactoryConfig) error
type PortableBundledFilesCopier func(string, string, *factorydefinitions.FactoryConfig) error
type PortableBundledDocsPruner func(string, *factorydefinitions.FactoryConfig) error
