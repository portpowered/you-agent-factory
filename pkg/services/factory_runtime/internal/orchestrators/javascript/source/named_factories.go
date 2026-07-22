package workflowsource

import (
	"fmt"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type namedJavaScriptFactory struct {
	Name       string
	FactoryDir string
}

func listNamedJavaScriptFactories(files fileSystem, rootDir string) ([]namedJavaScriptFactory, error) {
	children, err := files.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}
	var factories []namedJavaScriptFactory
	for _, child := range children {
		if !child.IsDir() || strings.HasPrefix(child.Name(), ".") {
			continue
		}
		childDir := filepath.Join(rootDir, child.Name())
		if hasFactoryDefinition(files, childDir) {
			factories = append(factories, namedJavaScriptFactory{Name: child.Name(), FactoryDir: childDir})
			continue
		}
		if !strings.HasPrefix(child.Name(), "@") {
			continue
		}
		scoped, readErr := files.ReadDir(childDir)
		if readErr != nil {
			continue
		}
		for _, leaf := range scoped {
			leafDir := filepath.Join(childDir, leaf.Name())
			if !leaf.IsDir() || strings.HasPrefix(leaf.Name(), ".") || !hasFactoryDefinition(files, leafDir) {
				continue
			}
			name, nameErr := interfaces.NameFromPathSegments([]string{child.Name(), leaf.Name()})
			if nameErr == nil {
				factories = append(factories, namedJavaScriptFactory{Name: name, FactoryDir: leafDir})
			}
		}
	}
	return factories, nil
}

func resolveNamedJavaScriptFactoryDir(files fileSystem, projectRoot, globalRoot, name string) (string, error) {
	for _, root := range []string{projectRoot, globalRoot} {
		factoryDir, err := interfaces.MapDir(root, name)
		if err != nil {
			return "", err
		}
		if hasFactoryDefinition(files, factoryDir) {
			return factoryDir, nil
		}
	}
	return "", fmt.Errorf("Factory Definition %q was not found", name)
}

func hasFactoryDefinition(files fileSystem, factoryDir string) bool {
	info, err := files.Stat(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	return err == nil && !info.IsDir()
}
