package contracts_test

import (
	"os/exec"
	"strings"
	"testing"
)

var runtimePackagesForbiddenFromConverterTooling = []string{
	"github.com/portpowered/infinite-you/pkg/config",
	"github.com/portpowered/infinite-you/pkg/service",
	"github.com/portpowered/infinite-you/pkg/transports/http",
	"github.com/portpowered/infinite-you/pkg/transports/cli",
	"github.com/portpowered/infinite-you/pkg/workers",
	"github.com/portpowered/infinite-you/pkg/factory",
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime",
	"github.com/portpowered/infinite-you/cmd/factory",
}

var forbiddenConverterToolingImports = []string{
	"github.com/portpowered/infinite-you/internal/contractopenapiconverter",
	"github.com/portpowered/infinite-you/internal/contractstaging",
	"github.com/portpowered/infinite-you/internal/contractopenapidiff",
}

func TestRuntimePackagesDoNotImportConverterTooling(t *testing.T) {
	t.Parallel()

	for _, pkg := range runtimePackagesForbiddenFromConverterTooling {
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
				for _, forbidden := range forbiddenConverterToolingImports {
					if dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/") {
						t.Fatalf(
							"runtime package %s must not depend on converter tooling %s; found %s",
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
