package factorydefinitions

import (
	"context"
	"fmt"
)

// InstallPackagedFactoryOperation installs one built-in packaged Factory through
// the Definitions-owned catalog and installation collaborators.
type InstallPackagedFactoryOperation func(
	context.Context,
	InstallPackagedFactoryRequest,
) (InstallPackagedFactoryResult, error)

// NewInstallPackagedFactoryOperation constructs the shared distribute operation
// used by bootstrap-adjacent callers and the Factory Definitions CLI adapter.
func NewInstallPackagedFactoryOperation(
	catalog PackagedFactoryCatalogOperations,
	installer PackagedFactoryInstallationOperations,
) InstallPackagedFactoryOperation {
	return func(
		ctx context.Context,
		request InstallPackagedFactoryRequest,
	) (InstallPackagedFactoryResult, error) {
		if err := ctx.Err(); err != nil {
			return InstallPackagedFactoryResult{}, err
		}
		if err := ValidateInstallPackagedFactoryRequest(request); err != nil {
			return InstallPackagedFactoryResult{}, err
		}
		resolved, err := catalog.ResolveBuiltInPackagedFactory(
			ctx,
			ResolveBuiltInPackagedFactoryRequest{Name: request.Name},
		)
		if err != nil {
			return InstallPackagedFactoryResult{}, err
		}
		if installer.Install == nil {
			return InstallPackagedFactoryResult{},
				fmt.Errorf("%w: packaged Factory installation collaborator is required",
					ErrFactoryDistributeFailed)
		}
		installed, err := installer.InstallPackagedFactory(
			ctx,
			PackagedFactoryInstallParams{
				NamedFactoriesRoot: request.RootDir,
				Definition:         resolved.Definition,
				Format:             request.Format,
				Replace:            request.Replace,
			},
		)
		if err != nil {
			return InstallPackagedFactoryResult{},
				fmt.Errorf("%w: %w", ErrFactoryDistributeFailed, err)
		}
		return InstallPackagedFactoryResult{
			Definition: DistributedFactoryDefinitionFacts{
				Name:       installed.Name,
				FactoryDir: installed.FactoryDir,
			},
			Outcome: installed.Outcome,
			Format:  installed.Format,
		}, nil
	}
}
