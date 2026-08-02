// Package contracts contains the private distribution ports. These
// capabilities are construction details of Factory Definitions and never cross
// the public unary root.
package contracts

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

// PackagedFactoryCatalogOperations are the exact catalog operations used by
// distribution after the generated publication has been validated.
type PackagedFactoryCatalogOperations struct {
	List func(
		context.Context,
		factorydefinitions.ListBuiltInPackagedFactoriesRequest,
	) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error)
	Resolve func(
		context.Context,
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
	) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error)
}

func (operations PackagedFactoryCatalogOperations) ListBuiltInPackagedFactories(
	ctx context.Context,
	request factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	if operations.List == nil {
		return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, fmt.Errorf("packaged Factory catalog collaborator is required")
	}
	return operations.List(ctx, request)
}

func (operations PackagedFactoryCatalogOperations) ResolveBuiltInPackagedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	if operations.Resolve == nil {
		return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
	}
	return operations.Resolve(ctx, request)
}

// PackagedFactoryInstallationOperations are the exact packaged-write
// operations used after catalog selection has returned a detached definition.
type PackagedFactoryInstallationOperations struct {
	Install func(
		context.Context,
		factorydefinitions.PackagedFactoryInstallParams,
	) (factorydefinitions.PackagedFactoryInstallResult, error)
}

func (operations PackagedFactoryInstallationOperations) InstallPackagedFactory(
	ctx context.Context,
	params factorydefinitions.PackagedFactoryInstallParams,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	if operations.Install == nil {
		return factorydefinitions.PackagedFactoryInstallResult{}, fmt.Errorf("packaged Factory installation collaborator is required")
	}
	return operations.Install(ctx, params)
}

// Persistence is the private layout persistence capability needed by packaged
// installation.
type Persistence = factorycontracts.Persistence

// PackagedInstallationFileSystem inspects an installation target before the
// persistence policy is applied.
type PackagedInstallationFileSystem interface {
	Stat(string) (fs.FileInfo, error)
}

// ScaffoldInitializer materializes the supported default Factory scaffold.
type ScaffoldInitializer = factorycontracts.ScaffoldInitializer

// ScaffoldFileSystem is the exact filesystem capability needed by scaffold
// materialization.
type ScaffoldFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}

// ScaffoldOutput is the process output capability selected at composition.
type ScaffoldOutput interface {
	io.Writer
}

// ScaffoldFactoryNameResolver reads the authored aggregate name after
// scaffold materialization.
type ScaffoldFactoryNameResolver func(string) (string, error)
