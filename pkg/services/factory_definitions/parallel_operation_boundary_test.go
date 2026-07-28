package factorydefinitions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// parallelOperationAllowedPackages may still reference deletion-only parallel
// catalog operation aliases until pkg/wire and remaining construction surfaces
// finish cutover in a later packet.
var parallelOperationAllowedPackages = []string{
	"github.com/portpowered/infinite-you/pkg/wire",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedfactories",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/testcomposition",
	"github.com/portpowered/infinite-you/pkg/transports/cli",
	"github.com/portpowered/infinite-you/pkg/transports/cli/factory",
	"github.com/portpowered/infinite-you/pkg/transports/cli/run",
	"github.com/portpowered/infinite-you/pkg/transports/cli/runconfig",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening/invocation",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/wire",
}

var parallelOperationPreferredPeerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/http",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mcp",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition",
}

func TestParallelOperationPreferredPeers_DoNotReferenceLegacyCatalogAuthority(t *testing.T) {
	t.Parallel()

	for _, packagePath := range parallelOperationPreferredPeerPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageSourceDoesNotContain(t, packagePath, "NamedFactoryCatalog")
			assertPackageSourceDoesNotContain(t, packagePath, "CurrentFactoryDirectoryResolver")
		})
	}
}

func assertPackageSourceDoesNotContain(t *testing.T, packagePath, needle string) {
	t.Helper()

	if containsAllowedParallelOperationReference(packagePath) {
		return
	}

	dir, err := packageSourceDir(packagePath)
	if err != nil {
		t.Fatalf("resolve package dir for %s: %v", packagePath, err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read package dir %s: %v", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(content), needle) {
			t.Fatalf("%s must not reference deletion-only parallel operation %q; use root Service catalog operations", path, needle)
		}
	}
}

func containsAllowedParallelOperationReference(packagePath string) bool {
	for _, allowed := range parallelOperationAllowedPackages {
		if packagePath == allowed || strings.HasPrefix(packagePath, allowed+"/") {
			return true
		}
	}
	return false
}

func packageSourceDir(packagePath string) (string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
