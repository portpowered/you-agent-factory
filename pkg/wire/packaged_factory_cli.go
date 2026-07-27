package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
)

func provideInstallPackagedFactoryOperation(
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
	installer factorydefinitions.PackagedFactoryInstallationOperations,
) factorydefinitions.InstallPackagedFactoryOperation {
	return factorydefinitions.NewInstallPackagedFactoryOperation(catalog, installer)
}

func providePackagedFactoryNameCompletionOperation(
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
) cobracompletion.PackagedFactoryNamesOperation {
	return cobracompletion.NewPackagedFactoryNames(catalog)
}

func provideInstallPackagedFactoryCLI(
	install factorydefinitions.InstallPackagedFactoryOperation,
) cli.InstallPackagedFactoryOperation {
	if install == nil {
		return nil
	}
	return func(cfg factorydefinitionscli.InstallPackagedFactoryConfig) error {
		return factorydefinitionscli.InstallPackagedFactory(cfg, install)
	}
}
