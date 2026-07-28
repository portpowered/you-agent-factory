package runtimetests

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

var testNamedFactoryCatalog = factorydefinitioncomposition.NamedFactoryCatalog()
var namedFactoryCatalog = testNamedFactoryCatalog

func ResolveNamedFactoryAcrossRoots(
	projectRoot string,
	globalRoot string,
	name string,
) (*factorydefinitions.NamedFactoryResolution, error) {
	return testNamedFactoryCatalog.ResolveNamedFactoryAcrossRoots(
		projectRoot,
		globalRoot,
		name,
	)
}

func ResolveNamedFactoryDirAcrossRoots(
	projectRoot string,
	globalRoot string,
	name string,
) (string, error) {
	resolution, err := ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, name)
	if err != nil {
		return "", err
	}
	return resolution.FactoryDir, nil
}
