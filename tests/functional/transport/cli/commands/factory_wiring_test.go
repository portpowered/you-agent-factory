package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	factoryWiringName     = "cli-factory-wiring"
	factoryWiringWorkType = "task"

	factoryFlattenExpandName         = "portable-flatten-expand-factory"
	factoryFlattenExpandWorker       = "executor"
	factoryFlattenExpandWorkstation  = "execute-task"
	factoryFlattenExpandExpandMarker = "Expanded factory config into"

	factoryReplaceInitialName = "cli-replace-initial"
	factoryReplaceTargetName    = "cli-replace-target"
	factoryReplaceSuccessMarker = "Replaced current factory"
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

// TestCLIFactoryFlattenExpandPreservesMeaning proves you factory config expand
// materializes split layout artifacts and you factory config flatten emits
// canonical camelCase JSON whose customer-visible payload matches the original
// Factory meaning without asserting definitions-domain import/export internals.
func TestCLIFactoryFlattenExpandPreservesMeaning(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory wiring")
	}

	dir := t.TempDir()
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, portableFlattenExpandFixtureJSON(), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	binaryPath := buildYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	want := flattenFactoryConfigViaCLI(t, ctx, binaryPath, dir)

	expandCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"factory", "config", "expand", factoryPath,
	)
	expandCmd.Dir = dir
	expandOut, err := expandCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("you factory config expand: %v\noutput:\n%s", err, expandOut)
	}
	if !strings.Contains(string(expandOut), factoryFlattenExpandExpandMarker) {
		t.Fatalf("factory expand output missing success marker %q:\n%s", factoryFlattenExpandExpandMarker, expandOut)
	}

	for _, relPath := range []string{
		filepath.Join("workers", factoryFlattenExpandWorker, "AGENTS.md"),
		filepath.Join("workstations", factoryFlattenExpandWorkstation, "AGENTS.md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, relPath)); err != nil {
			t.Fatalf("expected expand to materialize %s: %v", relPath, err)
		}
	}

	got := flattenFactoryConfigViaCLI(t, ctx, binaryPath, dir)
	if !reflect.DeepEqual(got, want) {
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("flatten after expand changed Factory meaning\nwant: %s\ngot:  %s", wantJSON, gotJSON)
	}
}

// TestCLIFactoryReplaceCurrentChangesSessionFactory proves you factory
// replace-current persists the live session Factory and you factory query
// reports identity markers for the replaced Factory that differ from the
// pre-replace session Factory without asserting definitions save internals.
func TestCLIFactoryReplaceCurrentChangesSessionFactory(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory wiring")
	}

	initialSourceDir := support.ScaffoldFactory(t, factoryReplaceFactoryConfig(factoryReplaceInitialName))
	namedFactoriesRoot := filepath.Join(t.TempDir(), "named-factories")
	initialSourcePath := filepath.Join(initialSourceDir, "factory.json")

	binaryPath := buildYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	initialFactoryDir := createNamedFactoryViaCLI(
		t, ctx, binaryPath, initialSourceDir,
		factoryReplaceInitialName, initialSourcePath, namedFactoriesRoot,
	)

	replacementSourceDir := support.ScaffoldFactory(t, factoryReplaceFactoryConfig(factoryReplaceTargetName))
	replacementSourcePath := filepath.Join(replacementSourceDir, "factory.json")
	createNamedFactoryViaCLI(
		t, ctx, binaryPath, replacementSourceDir,
		factoryReplaceTargetName, replacementSourcePath, namedFactoriesRoot,
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     initialFactoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	preReplace := queryFactoryViaCLIJSON(t, ctx, binaryPath, server.URL(), initialFactoryDir)
	if preReplace.Id == nil || *preReplace.Id != factoryReplaceInitialName {
		t.Fatalf("pre-replace factory id = %#v, want %q", preReplace.Id, factoryReplaceInitialName)
	}

	activateNamedFactoryOverHTTPForWiring(
		t, server.URL(), filepath.Join(namedFactoriesRoot, factoryReplaceTargetName),
	)

	replaceCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--server", server.URL(),
		"factory", "replace-current",
	)
	replaceCmd.Dir = initialFactoryDir
	replaceOut, err := replaceCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("you factory replace-current: %v\noutput:\n%s", err, replaceOut)
	}
	replaceOutput := string(replaceOut)
	if !strings.Contains(replaceOutput, factoryReplaceSuccessMarker+" "+factoryReplaceTargetName) {
		t.Fatalf("replace-current output missing success marker for %q:\n%s", factoryReplaceTargetName, replaceOutput)
	}

	postReplace := queryFactoryViaCLIJSON(t, ctx, binaryPath, server.URL(), initialFactoryDir)
	if postReplace.Name != factoryReplaceTargetName {
		t.Fatalf("post-replace factory name = %q, want %q", postReplace.Name, factoryReplaceTargetName)
	}
	if postReplace.Id == nil || *postReplace.Id != factoryReplaceTargetName {
		t.Fatalf("post-replace factory id = %#v, want %q", postReplace.Id, factoryReplaceTargetName)
	}
	if preReplace.Name == postReplace.Name && preReplace.Id != nil && postReplace.Id != nil && *preReplace.Id == *postReplace.Id {
		t.Fatalf("post-replace identity should differ from pre-replace name/id")
	}
}

func activateNamedFactoryOverHTTPForWiring(t *testing.T, baseURL, factoryDir string) {
	t.Helper()

	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	payload, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read factory config %s: %v", factoryPath, err)
	}
	var factory factoryapi.Factory
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode factory config: %v", err)
	}
	factory.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(1<<62 - 1),
		Physical: time.Now().UTC().Add(time.Hour),
	}
	mode := factoryapi.FactorySaveModeUpsertNamedAndActivate
	body, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{
		Factory: factory,
		Mode:    &mode,
	})
	if err != nil {
		t.Fatalf("encode named factory activation: %v", err)
	}
	request, err := http.NewRequest(
		http.MethodPut,
		baseURL+"/factory-sessions/~default/factory",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build named factory activation request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("activate named factory over HTTP: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"activate named factory status = %d, want 200: %s",
			response.StatusCode,
			string(responseBody),
		)
	}
}

func createNamedFactoryViaCLI(
	t *testing.T,
	ctx context.Context,
	binaryPath, workDir, name, sourcePath, namedFactoriesRoot string,
) string {
	t.Helper()

	createCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"factory", "create", name,
		"--from", sourcePath,
		"--dir", namedFactoriesRoot,
	)
	createCmd.Dir = workDir
	createOut, err := createCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("you factory create %q: %v\noutput:\n%s", name, err, createOut)
	}
	createOutput := string(createOut)
	for _, marker := range []string{
		"Created factory " + name,
		"Directory:",
	} {
		if !strings.Contains(createOutput, marker) {
			t.Fatalf("factory create %q output missing %q:\n%s", name, marker, createOutput)
		}
	}

	createdFactoryDir := filepath.Join(namedFactoriesRoot, name)
	createdFactoryPath := filepath.Join(createdFactoryDir, "factory.json")
	if _, err := os.Stat(createdFactoryPath); err != nil {
		t.Fatalf("created factory.json missing at %s: %v", createdFactoryPath, err)
	}
	return createdFactoryDir
}

func queryFactoryViaCLIJSON(
	t *testing.T,
	ctx context.Context,
	binaryPath, serverURL, workDir string,
) factoryapi.Factory {
	t.Helper()

	queryCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"--server", serverURL,
		"factory", "query",
	)
	queryCmd.Dir = workDir
	queryOut, err := queryCmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("you factory query --json: %v\nstderr:\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("you factory query --json: %v", err)
	}

	var current factoryapi.Factory
	if err := json.Unmarshal(queryOut, &current); err != nil {
		t.Fatalf("decode factory query JSON: %v\n%s", err, string(queryOut))
	}
	return current
}

func flattenFactoryConfigViaCLI(t *testing.T, ctx context.Context, binaryPath, factoryDir string) any {
	t.Helper()

	flattenCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"factory", "config", "flatten", factoryDir,
	)
	flattenCmd.Dir = factoryDir
	flattenOut, err := flattenCmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("you factory config flatten: %v\nstderr:\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("you factory config flatten: %v", err)
	}

	flattenOutput := string(flattenOut)
	for _, marker := range []string{`"workTypes"`, `"workers"`, `"workstations"`} {
		if !strings.Contains(flattenOutput, marker) {
			t.Fatalf("flatten output missing camelCase marker %q:\n%s", marker, flattenOutput)
		}
	}
	if strings.Contains(flattenOutput, `"work_types"`) || strings.Contains(flattenOutput, `"model_provider"`) {
		t.Fatalf("flatten output should use camelCase keys:\n%s", flattenOutput)
	}

	var payload any
	if err := json.Unmarshal(flattenOut, &payload); err != nil {
		t.Fatalf("flatten output is not valid JSON: %v\n%s", err, flattenOutput)
	}
	return payload
}

func portableFlattenExpandFixtureJSON() []byte {
	return []byte(`{
  "name": "` + factoryFlattenExpandName + `",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "resources": [{ "name": "agent-slot", "capacity": 1 }],
  "workers": [
    {
      "name": "` + factoryFlattenExpandWorker + `",
      "type": "MODEL_WORKER",
      "model": "claude-sonnet-4-20250514",
      "modelProvider": "CLAUDE",
      "resources": [{ "name": "agent-slot", "capacity": 1 }],
      "stopToken": "COMPLETE",
      "body": "You are the portable factory executor."
    }
  ],
  "workstations": [
    {
      "id": "execute-task-id",
      "name": "` + factoryFlattenExpandWorkstation + `",
      "behavior": "STANDARD",
      "worker": "` + factoryFlattenExpandWorker + `",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{ "workType": "task", "state": "failed" }],
      "resources": [{ "name": "agent-slot", "capacity": 1 }],
      "definition": {
        "type": "MODEL_WORKSTATION",
        "worker": "` + factoryFlattenExpandWorker + `",
        "body": "Complete {{ (index .Inputs 0).WorkID }}.",
        "stopWords": ["DONE"]
      }
    }
  ]
}`)
}

func factoryReplaceFactoryConfig(name string) map[string]any {
	return map[string]any{
		"name": name,
		"id":   name,
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
