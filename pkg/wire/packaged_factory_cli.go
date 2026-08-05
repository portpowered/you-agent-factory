package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
)

func provideInstallPackagedFactoryOperation(
	packaging factorydefinitions.Packaging,
) factorydefinitions.InstallPackagedFactoryOperation {
	if packaging == nil {
		return nil
	}
	return packaging.InstallPackagedFactory
}

func providePackagedFactoryNameCompletionOperation(
	packaging factorydefinitions.Packaging,
) cobracompletion.PackagedFactoryNamesOperation {
	return cobracompletion.NewPackagedFactoryNames(packaging)
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
