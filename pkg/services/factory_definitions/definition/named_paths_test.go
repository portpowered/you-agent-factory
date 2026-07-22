package factorydefinition

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

var definitionTestNamedPaths = func() *factorynamedpaths.Resolver {
	resolver, err := factorynamedpaths.New(platformfilesystem.Local{})
	if err != nil {
		panic(err)
	}
	return resolver
}()
