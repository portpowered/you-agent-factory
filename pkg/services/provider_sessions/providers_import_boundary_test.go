package providersessions_test

import (
	"os/exec"
	"strings"
	"testing"
)

// productionPackages are Provider Sessions packages that ship production
// behavior. Boundary tests scan their dependency closure so non-root Providers
// or Workers-owned transitional provider packages cannot re-enter through a
// nested import.
var productionPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/wire",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/transports/http",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader/wire",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader/internal/service",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/wire",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/internal/service",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/internal/cursor",
}

var forbiddenProvidersConsumerRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/providers/internal",
	"github.com/portpowered/infinite-you/pkg/services/providers/wire",
	"github.com/portpowered/infinite-you/pkg/services/workers/provider",
}

const providersServiceRoot = "github.com/portpowered/infinite-you/pkg/services/providers"

func TestProductionPackages_ImportProvidersOnlyThroughServiceRoot(t *testing.T) {
	t.Parallel()

	for _, packagePath := range productionPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageDepsForbidden(t, packagePath, forbiddenProvidersConsumerRoots)
		})
	}
}

func TestProductionPackages_DirectProvidersImportsResolveToServiceRoot(t *testing.T) {
	t.Parallel()

	for _, packagePath := range productionPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			for _, importPath := range listDirectImports(t, packagePath) {
				if !strings.HasPrefix(importPath, "github.com/portpowered/infinite-you/pkg/services/providers") {
					continue
				}
				if importPath != providersServiceRoot {
					t.Fatalf(
						"%s must import Providers only through %s; found direct import %s",
						packagePath,
						providersServiceRoot,
						importPath,
					)
				}
			}
		})
	}
}

func assertPackageDepsForbidden(t *testing.T, packagePath string, forbiddenRoots []string) {
	t.Helper()

	for _, dep := range listTransitiveDeps(t, packagePath) {
		for _, forbidden := range forbiddenRoots {
			if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
				t.Fatalf(
					"%s must not depend on forbidden Providers consumer path %s; found dependency %s",
					packagePath,
					forbidden,
					dep,
				)
			}
		}
	}
}

func listDirectImports(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func listTransitiveDeps(t *testing.T, packagePath string) []string {
	t.Helper()

	cmd := exec.Command(
		"go",
		"list",
		"-deps",
		"-f",
		"{{if not .Standard}}{{.ImportPath}}{{end}}",
		packagePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps for %s: %v\n%s", packagePath, err, output)
	}
	return strings.Fields(string(output))
}
