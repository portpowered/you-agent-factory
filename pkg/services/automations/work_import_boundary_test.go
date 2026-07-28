package automations_test

import (
	"os/exec"
	"strings"
	"testing"
)

const workRootImportPath = "github.com/portpowered/infinite-you/pkg/services/work"

// forbiddenWorkImportRoots are Work surfaces Automations production code must
// not depend on. Automations may import only the Work service root contract.
var forbiddenWorkImportRoots = []string{
	workRootImportPath + "/service",
	workRootImportPath + "/internal",
	"github.com/portpowered/infinite-you/pkg/work",
}

func TestProductionPackagesImportWorkOnlyThroughRoot(t *testing.T) {
	t.Parallel()

	cmd := exec.Command(
		"go",
		"list",
		"-f",
		"{{if .GoFiles}}{{.ImportPath}} {{range .Imports}}{{.}} {{end}}{{end}}",
		"github.com/portpowered/infinite-you/pkg/services/automations/...",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list automations production packages: %v\n%s", err, output)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		packagePath := fields[0]
		for _, importPath := range fields[1:] {
			if forbiddenWorkImport(importPath) {
				t.Fatalf(
					"%s must import Work only through %s; found forbidden import %s",
					packagePath,
					workRootImportPath,
					importPath,
				)
			}
		}
	}
}

func forbiddenWorkImport(importPath string) bool {
	if importPath == workRootImportPath {
		return false
	}
	for _, forbidden := range forbiddenWorkImportRoots {
		if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
			return true
		}
	}
	workSubpackagePrefix := workRootImportPath + "/"
	if strings.HasPrefix(importPath, workSubpackagePrefix) {
		return true
	}
	legacyPrefix := "github.com/portpowered/infinite-you/pkg/work"
	return importPath == legacyPrefix || strings.HasPrefix(importPath, legacyPrefix+"/")
}
