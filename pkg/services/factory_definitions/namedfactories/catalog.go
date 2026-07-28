// Package namedfactories provides the concrete persisted named-Factory
// catalog selected by the application composition root.
package namedfactories

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	internalcatalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedfactories"
)

type catalog struct {
	paths      factorydefinitions.NamedPathResolver
	fileSystem factorydefinitions.NamedFactoryCatalogFileSystem
}

// New constructs the stateless persisted named-Factory catalog.
func New(
	paths factorydefinitions.NamedPathResolver,
	fileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
) (factorydefinitions.NamedFactoryCatalog, error) {
	if paths == nil {
		return nil, fmt.Errorf("named Factory path resolver is required")
	}
	if fileSystem == nil {
		return nil, fmt.Errorf("named Factory catalog filesystem is required")
	}
	return catalog{paths: paths, fileSystem: fileSystem}, nil
}

// ResolveCurrent resolves the active Factory definition under rootDir.
func ResolveCurrent(paths factorydefinitions.NamedPathResolver, rootDir string) (string, error) {
	return internalcatalog.ResolveCurrent(paths, rootDir)
}

func (c catalog) ListNamedFactories(
	rootDir string,
) ([]factorydefinitions.NamedFactoryListEntry, error) {
	return internalcatalog.List(c.paths, c.fileSystem, rootDir)
}

func (c catalog) DeleteNamedFactory(rootDir, name string) error {
	return internalcatalog.Delete(c.paths, c.fileSystem, rootDir, name)
}

func (c catalog) ResolveNamedFactoryAcrossRoots(
	projectRoot string,
	globalRoot string,
	name string,
) (*factorydefinitions.NamedFactoryResolution, error) {
	return internalcatalog.ResolveAcrossRoots(c.paths, projectRoot, globalRoot, name)
}
