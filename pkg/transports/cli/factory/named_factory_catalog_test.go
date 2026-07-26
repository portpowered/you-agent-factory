package factory

import (
	"context"
	"io/fs"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type namedFactoryCatalogFake struct {
	list    func(string) ([]factorydefinitions.NamedFactoryListEntry, error)
	delete  func(string, string) error
	resolve func(string, string, string) (*factorydefinitions.NamedFactoryResolution, error)
}

func (catalog namedFactoryCatalogFake) ListNamedFactories(root string) ([]factorydefinitions.NamedFactoryListEntry, error) {
	if catalog.list == nil {
		return nil, nil
	}
	return catalog.list(root)
}

func (catalog namedFactoryCatalogFake) DeleteNamedFactory(root, name string) error {
	if catalog.delete == nil {
		return nil
	}
	return catalog.delete(root, name)
}

func (catalog namedFactoryCatalogFake) ResolveNamedFactoryAcrossRoots(
	projectRoot,
	globalRoot,
	name string,
) (*factorydefinitions.NamedFactoryResolution, error) {
	if catalog.resolve == nil {
		return nil, nil
	}
	return catalog.resolve(projectRoot, globalRoot, name)
}

var testNamedFactoryCatalog factorydefinitions.NamedFactoryCatalog = namedFactoryCatalogFake{}

func useNamedFactoryCatalogFake(t *testing.T, catalog namedFactoryCatalogFake) {
	t.Helper()
	previous := testNamedFactoryCatalog
	testNamedFactoryCatalog = catalog
	t.Cleanup(func() { testNamedFactoryCatalog = previous })
}

func testList(config ListConfig) error {
	if config.Context == nil {
		config.Context = context.Background()
	}
	var listed []factorydefinitions.NamedFactoryListEntry
	effective := func(
		_ context.Context,
		request factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		var err error
		listed, err = testNamedFactoryCatalog.ListNamedFactories(request.ProjectRoot)
		if err != nil {
			return factorydefinitions.ListEffectiveFactoriesResult{}, err
		}
		entries := make([]factorydefinitions.EffectiveFactoryCatalogEntry, 0, len(listed))
		for _, entry := range listed {
			location := entry.FactoryDir
			entries = append(entries, factorydefinitions.EffectiveFactoryCatalogEntry{
				Name: entry.Name, Location: &location, Definition: &factorydefinitions.FactoryConfig{Name: entry.Name},
			})
		}
		return factorydefinitions.ListEffectiveFactoriesResult{Entries: entries}, nil
	}
	readCurrent := func(string) (string, error) {
		for _, entry := range listed {
			if entry.Current {
				return entry.Name, nil
			}
		}
		return "", fs.ErrNotExist
	}
	return List(effective, readCurrent, config)
}

func testDelete(config DeleteConfig) error {
	return Delete(testNamedFactoryCatalog, config)
}
