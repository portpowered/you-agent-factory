package scaffoldfacts_test

import (
	"os"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionscaffoldfacts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/scaffoldfacts"
)

func TestLocalFactoryNameResolverReadsScaffoldedName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
	if err := os.WriteFile(path, []byte(`{"name":"alpha"}`), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	resolve := distributionscaffoldfacts.LocalFactoryNameResolver()
	name, err := resolve(dir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if name != "alpha" {
		t.Fatalf("name = %q, want alpha", name)
	}
}
