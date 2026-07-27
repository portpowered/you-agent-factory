package commands_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	factoryWiringName     = "cli-factory-wiring"
	factoryWiringWorkType = "task"
)

// TestCLIFactoryInitValidateAndQuery proves you factory create authors a named
// Factory, you factory config validate reports validation success, and you
// factory query against a running session prints observable Factory identity
// markers without asserting definitions-domain validation internals.
func TestCLIFactoryInitValidateAndQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory wiring")
	}

	sourceDir := support.ScaffoldFactory(t, factoryWiringFactoryConfig())
	namedFactoriesRoot := filepath.Join(t.TempDir(), "named-factories")
	sourcePath := filepath.Join(sourceDir, "factory.json")

	binaryPath := buildYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	createCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"factory", "create", factoryWiringName,
		"--from", sourcePath,
		"--dir", namedFactoriesRoot,
	)
	createCmd.Dir = sourceDir
	createOut, err := createCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("you factory create: %v\noutput:\n%s", err, createOut)
	}
	createOutput := string(createOut)
	for _, marker := range []string{
		"Created factory " + factoryWiringName,
		"Directory:",
	} {
		if !strings.Contains(createOutput, marker) {
			t.Fatalf("factory create output missing %q:\n%s", marker, createOutput)
		}
	}

	createdFactoryDir := filepath.Join(namedFactoriesRoot, factoryWiringName)
	createdFactoryPath := filepath.Join(createdFactoryDir, "factory.json")
	if _, err := os.Stat(createdFactoryPath); err != nil {
		t.Fatalf("created factory.json missing at %s: %v", createdFactoryPath, err)
	}

	validateCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"factory", "config", "validate", createdFactoryPath,
	)
	validateCmd.Dir = sourceDir
	validateOut, err := validateCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("you factory config validate: %v\noutput:\n%s", err, validateOut)
	}
	if !strings.Contains(string(validateOut), "Factory validation passed.") {
		t.Fatalf("factory validate output missing success marker:\n%s", validateOut)
	}

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     createdFactoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	queryCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--server", server.URL(),
		"factory", "query",
	)
	queryCmd.Dir = createdFactoryDir
	queryOut, err := queryCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("you factory query: %v\noutput:\n%s", err, queryOut)
	}

	queryOutput := string(queryOut)
	for _, marker := range []string{
		"NAME\tKIND\tID\tFACTORY DIRECTORY",
		factoryWiringName,
		"default-root",
	} {
		if !strings.Contains(queryOutput, marker) {
			t.Fatalf("factory query output missing %q:\n%s", marker, queryOutput)
		}
	}
}

func factoryWiringFactoryConfig() map[string]any {
	return map[string]any{
		"name": factoryWiringName,
		"workTypes": []map[string]any{
			{
				"name": factoryWiringWorkType,
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": factoryWiringWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": factoryWiringWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": factoryWiringWorkType, "state": "failed"}},
			},
		},
	}
}
