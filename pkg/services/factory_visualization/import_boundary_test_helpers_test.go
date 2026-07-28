package factory_visualization_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix             = "github.com/portpowered/infinite-you/"
	factoryRuntimeRoot       = modulePrefix + "pkg/services/factory_runtime"
	factoryVisualizationRoot = modulePrefix + "pkg/services/factory_visualization"
	recordingsRoot           = modulePrefix + "pkg/services/recordings"
)

func listFactoryVisualizationPackages(t *testing.T) []string {
	t.Helper()

	cmd := exec.Command("go", "list", factoryVisualizationRoot+"/...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list factory visualization packages: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func shortFactoryVisualizationPackageName(packagePath string) string {
	if strings.HasPrefix(packagePath, factoryVisualizationRoot) {
		rest := strings.TrimPrefix(packagePath, factoryVisualizationRoot)
		if rest == "" {
			return "factory_visualization"
		}
		return strings.TrimPrefix(rest, "/")
	}
	return packagePath
}
