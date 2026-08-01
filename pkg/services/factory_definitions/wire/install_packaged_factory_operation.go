package wire

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

// InstallPackagedFactoryOperation installs one built-in packaged Factory through
// the Definitions-owned catalog and installation collaborators.
type InstallPackagedFactoryOperation = factorydefinitions.InstallPackagedFactoryOperation

// NewInstallPackagedFactoryOperation constructs the shared distribute operation
// used by bootstrap-adjacent callers and the Factory Definitions CLI adapter.
func NewInstallPackagedFactoryOperation(
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
	installer factorydefinitions.PackagedFactoryInstallationOperations,
) InstallPackagedFactoryOperation {
	return func(
		ctx context.Context,
		request factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		if err := ctx.Err(); err != nil {
			return factorydefinitions.InstallPackagedFactoryResult{}, err
		}
		if err := distributionwire.ValidateInstallPackagedFactoryRequest(request); err != nil {
			return factorydefinitions.InstallPackagedFactoryResult{}, err
		}
		resolved, err := catalog.ResolveBuiltInPackagedFactory(
			ctx,
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: request.Name},
		)
		if err != nil {
			return factorydefinitions.InstallPackagedFactoryResult{}, err
		}
		if installer.Install == nil {
			return factorydefinitions.InstallPackagedFactoryResult{},
				fmt.Errorf("%w: packaged Factory installation collaborator is required",
					factorydefinitions.ErrFactoryDistributeFailed)
		}
		installed, err := installer.InstallPackagedFactory(
			ctx,
			factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: request.RootDir,
				Definition:         resolved.Definition,
				Format:             request.Format,
				Replace:            request.Replace,
			},
		)
		if err != nil {
			return factorydefinitions.InstallPackagedFactoryResult{},
				fmt.Errorf("%w: %w", factorydefinitions.ErrFactoryDistributeFailed, err)
		}
		return factorydefinitions.InstallPackagedFactoryResult{
			Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
				Name:       installed.Name,
				FactoryDir: installed.FactoryDir,
			},
			Outcome: installed.Outcome,
			Format:  installed.Format,
		}, nil
	}
}
