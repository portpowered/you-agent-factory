// Package authoredsource adapts the package-owned authored source tree to the
// legacy packaged Factory definition wrappers.
package authoredsource

import (
	"fmt"
	"io/fs"
	"path"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
)

const factoriesRoot = "factories"

// MustFactoryJSON reads one authored root document for a compatibility wrapper.
func MustFactoryJSON(name string) []byte {
	filename := path.Join(factoriesRoot, name, "factory.json")
	payload, err := fs.ReadFile(packagedfactories.Source(), filename)
	if err != nil {
		panic(fmt.Sprintf("read authored packaged Factory %q: %v", filename, err))
	}
	return payload
}

// MustFactoryFS scopes the authored source tree to one Factory directory.
func MustFactoryFS(name string) fs.FS {
	root := path.Join(factoriesRoot, name)
	factoryFS, err := fs.Sub(packagedfactories.Source(), root)
	if err != nil {
		panic(fmt.Sprintf("open authored packaged Factory directory %q: %v", root, err))
	}
	return factoryFS
}
