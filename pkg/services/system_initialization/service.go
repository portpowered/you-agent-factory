// Package systeminitialization defines process-level system initialization.
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

// Service initializes operator configuration and packaged Factories.
type Service interface {
	Initialize(context.Context, Request) (Result, error)
}

// InspectPath is the exact filesystem observation required to distinguish
// missing, file, and directory paths during system initialization.
type InspectPath func(string) (fs.FileInfo, error)

// LegacyFactoryMigrationFileSystem is the exact filesystem effect required to
// move customer-owned Factories from the retired global catalog root.
type LegacyFactoryMigrationFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadFile(string) ([]byte, error)
	ReadDir(string) ([]fs.DirEntry, error)
	MkdirAll(string, fs.FileMode) error
	Rename(string, string) error
}

type OperatorSettings interface {
	LoadFileConfig(string) (operatorconfig.Config, error)
	EnsureLocalBackendScope(string) (operatorconfig.ResolvedBackendScope, error)
}

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
