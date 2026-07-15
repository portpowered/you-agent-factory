package contracts_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
)

var forbiddenConverterToolingImports = []string{
	"github.com/portpowered/infinite-you/internal/contractopenapiconverter",
	"github.com/portpowered/infinite-you/internal/contractstaging",
	"github.com/portpowered/infinite-you/internal/contractopenapidiff",
}

type runtimePackage struct {
	ImportPath string
	Deps       []string
}

func TestRuntimePackagesDoNotImportConverterTooling(t *testing.T) {
	cmd := exec.Command("go", "list", "-json", "./pkg/...", "./cmd/factory/...")
	cmd.Dir = testpath.MustRepoPathFromCaller(t, 0)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list runtime packages: %v\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("go list runtime packages: %v", err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(output)))
	for {
		var pkg runtimePackage
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode go list runtime package: %v", err)
		}
		if err := rejectConverterToolingImport(pkg); err != nil {
			t.Fatal(err)
		}
	}
}

func rejectConverterToolingImport(pkg runtimePackage) error {
	for _, dependency := range pkg.Deps {
		for _, forbidden := range forbiddenConverterToolingImports {
			if dependency == forbidden || strings.HasPrefix(dependency, forbidden+"/") {
				return fmt.Errorf("runtime package %s depends on forbidden converter tooling dependency %s", pkg.ImportPath, dependency)
			}
		}
	}
	return nil
}

func TestRuntimePackageConverterBoundaryNamesImporterAndForbiddenDependency(t *testing.T) {
	pkg := runtimePackage{
		ImportPath: "github.com/portpowered/infinite-you/pkg/config/example",
		Deps:       []string{"github.com/portpowered/infinite-you/internal/contractopenapiconverter"},
	}
	err := rejectConverterToolingImport(pkg)
	if err == nil || !strings.Contains(err.Error(), pkg.ImportPath) || !strings.Contains(err.Error(), pkg.Deps[0]) {
		t.Fatalf("rejectConverterToolingImport() error = %v, want importing package and forbidden dependency", err)
	}
}

func TestRuntimePackageConverterBoundaryRejectsIndirectDependency(t *testing.T) {
	pkg := runtimePackage{
		ImportPath: "github.com/portpowered/infinite-you/pkg/config/example",
		Deps: []string{
			"github.com/portpowered/infinite-you/pkg/config/intermediary",
			"github.com/portpowered/infinite-you/internal/contractstaging",
		},
	}
	err := rejectConverterToolingImport(pkg)
	if err == nil || !strings.Contains(err.Error(), pkg.ImportPath) || !strings.Contains(err.Error(), pkg.Deps[1]) {
		t.Fatalf("rejectConverterToolingImport() error = %v, want owning runtime package and indirect forbidden dependency", err)
	}
}
