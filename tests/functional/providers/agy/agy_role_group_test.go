package agy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// assertAgyPackagedFactoryInstalled performs the same public missing-Factory
// bootstrap probe as support.InstallPackagedFactory, but through the caller's
// reusable root-built process instead of constructing a second throwaway
// process per case. The home, environment, and working directory remain
// invocation-local.
func assertAgyPackagedFactoryInstalled(
	t *testing.T,
	process support.Process,
	env []string,
	workingDirectory string,
	name string,
) {
	t.Helper()
	namedFactoriesRoot := support.InitializeCustomerHomeWithProcess(t, process, env, workingDirectory)
	factoryDir := filepath.Join(namedFactoriesRoot, filepath.FromSlash(name))
	if _, err := os.Stat(filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatalf("initializer omitted packaged Factory %q at %s: %v", name, factoryDir, err)
	}
}
