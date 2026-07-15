package contracts_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
)

var javascriptRuntimePackages = []string{
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/childcontract",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/callbehavior",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/symbolidentity",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/store",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation",
}

var forbiddenContractToolingImports = []string{
	"github.com/portpowered/infinite-you/internal/contractvalidator",
	"github.com/portpowered/infinite-you/internal/contractstaging",
	"github.com/portpowered/infinite-you/contracts",
}

func TestJavaScriptRuntimePackagesDoNotImportContractTooling(t *testing.T) {
	t.Parallel()

	for _, pkg := range javascriptRuntimePackages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()

			cmd := exec.Command(
				"go",
				"list",
				"-deps",
				"-f",
				"{{if not .Standard}}{{.ImportPath}}{{end}}",
				pkg,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go list dependencies: %v\n%s", err, output)
			}

			for _, dependency := range strings.Fields(string(output)) {
				for _, forbidden := range forbiddenContractToolingImports {
					if dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/") {
						t.Fatalf(
							"runtime package %s must not depend on contract tooling %s; found %s",
							pkg,
							forbidden,
							dependency,
						)
					}
				}
			}
		})
	}
}

func TestJavaScriptAuthoredCatalogBoundary(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoRootFromCaller(t, 0)
	authoredSchema := testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript", "runtime-manifest.schema.json")
	authoredCatalog := testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript", "runtime-api.json")

	if _, err := os.Stat(authoredSchema); err != nil {
		t.Fatalf("authored runtime-manifest schema missing: %v", err)
	}
	if _, err := os.Stat(authoredCatalog); err != nil {
		t.Fatalf("authored JavaScript runtime API catalog missing: %v", err)
	}

	allowed := map[string]struct{}{
		"runtime-manifest.schema.json": {},
		"runtime-api.json":             {},
	}
	entries, err := os.ReadDir(testpath.MustRepoPathFromCaller(t, 0, "contracts", "javascript"))
	if err != nil {
		t.Fatalf("read contracts/javascript: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("contracts/javascript must contain only authored contract files, found directory %s", entry.Name())
		}
		if _, ok := allowed[entry.Name()]; !ok {
			t.Fatalf("unexpected authored javascript contract %s under %s", entry.Name(), repositoryRoot)
		}
	}
}

func TestJavaScriptStagedRuntimeAPIRemainsInventorySourced(t *testing.T) {
	t.Parallel()

	const (
		wantSource = "pkg/orchestrators/javascript/runtime/javascript-runtime-symbols.json"
		wantTarget = "packages/api/generated/javascript/runtime-api.json"
	)

	found := false
	for _, artifact := range contractstaging.RawArtifacts() {
		if artifact.Target != wantTarget {
			continue
		}
		found = true
		if artifact.Source != wantSource {
			t.Fatalf("staged runtime-api source = %q, want inventory %q", artifact.Source, wantSource)
		}
	}
	if !found {
		t.Fatalf("missing staged runtime-api projection in RawArtifacts()")
	}
}
