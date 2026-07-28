// Package systeminitialization publishes the singular System Bootstrap root
// contract for cross-service peers. Peers depend on the one named Service
// interface and Bootstrap-owned request, result, value, and typed-error
// contracts for initialize intent, customer-visible created/skipped outcomes,
// and partial-failure rollback facts. Concrete initialize/rollback workflow
// ownership lives under pkg/services/system_initialization/internal/workflow
// behind the service-local wire constructor. Filesystem collaborator ports
// (InspectPath, LegacyFactoryMigrationFileSystem) and Operator Settings adapter
// helpers (OperatorSettings, OperatorSettingsFunctions) are workflow/collaborator
// surfaces for the canonical implementer; they are not additional peer-facing
// Bootstrap authority interfaces for the published initialize or rollback slices.
package systeminitialization

import (
	"context"
	"io/fs"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// SystemConfigOutcome reports whether initialization created or preserved the
// operator configuration.
type SystemConfigOutcome string

const (
	SystemConfigCreated SystemConfigOutcome = "created"
	SystemConfigSkipped SystemConfigOutcome = "skipped"
)

// PackagedFactoryOutcome reports whether initialization created or preserved a
// packaged Factory.
type PackagedFactoryOutcome string

const (
	PackagedFactoryCreated PackagedFactoryOutcome = "created"
	PackagedFactorySkipped PackagedFactoryOutcome = "skipped"
)

// PackagedFactoryResult summarizes one packaged Factory installation.
type PackagedFactoryResult struct {
	Name       string
	FactoryDir string
	Outcome    PackagedFactoryOutcome
}

// Request contains the caller-controlled inputs for one initialization.
type Request struct {
	HomeDir string
}

// Result summarizes a successful system initialization.
type Result struct {
	HomeDir             string
	ConfigPath          string
	NamedFactoriesRoot  string
	SystemConfigOutcome SystemConfigOutcome
	PackagedFactories   []PackagedFactoryResult
}

// Service is the singular System Bootstrap root contract for cross-service
// peers. Published slices (initialize request/result and partial-failure
// rollback facts) are additive methods on this one named interface and use
// plain Bootstrap-owned request, result, value, and typed-error contracts.
// Peers depend on Service rather than Initializer construction, filesystem
// collaborator ports, Operator Settings adapter helpers, Factory Definitions
// implementation subpackages, or pkg/initializer lifecycle types.
type Service interface {
	// Initialize applies Bootstrap-owned initialization intent and returns
	// customer-visible created/skipped outcomes. Missing or blank home-directory
	// intent fails with ErrMissingHomeDir; cancelled context fails with
	// ErrInitializeCancelled. Partial failures fail with ErrInitializePartialFailure
	// and carry Bootstrap-owned InitializePartialFailure rollback facts peers
	// inspect with errors.As. Bootstrap owns ordering, idempotency, and rollback
	// reporting; Operator Settings and Factory Definitions retain their own
	// transactional store boundaries. Validation and cancellation failures do not
	// invent rollback work facts. Rollback facts are additive typed outcomes on
	// this same Service rather than through a second peer-facing Bootstrap
	// interface.
	Initialize(context.Context, Request) (Result, error)
}

// InspectPath is a filesystem collaborator port for the canonical Initializer
// implementer. It is not a peer-facing Bootstrap authority interface.
type InspectPath func(string) (fs.FileInfo, error)

// LegacyFactoryMigrationFileSystem is a filesystem collaborator port for the
// canonical Initializer implementer. It is not a peer-facing Bootstrap
// authority interface.
type LegacyFactoryMigrationFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadFile(string) ([]byte, error)
	ReadDir(string) ([]fs.DirEntry, error)
	MkdirAll(string, fs.FileMode) error
	Rename(string, string) error
}

// OperatorSettings is an Operator Settings adapter helper for the canonical
// Initializer implementer. It is not a peer-facing Bootstrap authority
// interface.
type OperatorSettings interface {
	LoadFileConfig(string) (operatorconfig.Config, error)
	EnsureLocalBackendScope(string) (operatorconfig.ResolvedBackendScope, error)
}

// OperatorSettingsFunctions adapts function values to OperatorSettings for
// Initializer construction. It is not a peer-facing Bootstrap authority
// interface.
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

type PackagedFactoryInstaller = factorydefinitions.PackagedFactoryInstaller
type PackagedFactoryCatalogOperations = factorydefinitions.PackagedFactoryCatalogOperations
