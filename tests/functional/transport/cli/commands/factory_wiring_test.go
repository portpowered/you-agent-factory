package commands_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
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

	factoryReplaceSuccessMarker = "Replaced current factory"
)

// TestCLIFactoryInitValidateAndShow proves you factory create authors a named
// Factory, you factory config validate reports validation success, and the
// default-session-only factory show command prints observable Factory identity
// markers without asserting definitions-domain validation internals.
func testCLIFactoryInitValidateAndShow(t *testing.T, remote *sharedRemoteCLI) {
	sourceDir := support.ScaffoldFactory(t, factoryWiringFactoryConfig())
	namedFactoriesRoot := filepath.Join(t.TempDir(), "named-factories")
	sourcePath := filepath.Join(sourceDir, "factory.json")

	processHarness := remote.process
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	createCmd := processHarness.CommandContext(ctx,
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

	validateCmd := processHarness.CommandContext(ctx,
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

	// factory show is intentionally characterized against the server's named
	// default session: its public CLI contract has no --session selector.
	queryOut, err := remote.run(ctx, remote.hostFactoryDir, "", "factory", "show")
	if err != nil {
		t.Fatalf("you factory show: %v\noutput:\n%s", err, queryOut)
	}

	queryOutput := string(queryOut)
	for _, marker := range []string{
		"NAME\tKIND\tID\tFACTORY DIRECTORY",
		factoryWiringName,
		"default-root",
	} {
		if !strings.Contains(queryOutput, marker) {
			t.Fatalf("factory show output missing %q:\n%s", marker, queryOutput)
		}
	}
}

// TestCLIFactoryFlattenExpandPreservesMeaning proves you factory config expand
// materializes split layout artifacts and you factory config flatten emits
// canonical camelCase JSON whose customer-visible payload matches the original
// Factory meaning without asserting definitions-domain import/export internals.
func TestCLIFactoryFlattenExpandPreservesMeaning(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("slow CLI factory wiring")
	}

	dir := t.TempDir()
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, portableFlattenExpandFixtureJSON(), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	processHarness := newLocalReusableProcessHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	want := flattenFactoryConfigViaCLI(t, ctx, processHarness, dir)

	expandCmd := processHarness.CommandContext(ctx,
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

	got := flattenFactoryConfigViaCLI(t, ctx, processHarness, dir)
	if !reflect.DeepEqual(got, want) {
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("flatten after expand changed Factory meaning\nwant: %s\ngot:  %s", wantJSON, gotJSON)
	}
}

// TestCLIFactoryReplaceCurrentChangesSessionFactory proves you factory
// replace-current re-persists the named default-session Factory with the
// documented success marker and you factory show reports the same Factory
// identity with an advanced version after persistence without asserting
// definitions save orchestration internals.
func testCLIFactoryReplaceCurrentChangesSessionFactory(t *testing.T, remote *sharedRemoteCLI) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	factoryDir := remote.hostFactoryDir
	preReplace := queryFactoryViaCLIJSON(t, ctx, remote.process, remote.baseURL, factoryDir)
	if preReplace.Id == nil || *preReplace.Id != factoryWiringName {
		t.Fatalf("pre-replace factory id = %#v, want %q", preReplace.Id, factoryWiringName)
	}
	if preReplace.Version == nil {
		t.Fatal("pre-replace factory version missing")
	}
	preReplaceLogical := preReplace.Version.Logical.Int64()

	replaceOut, err := remote.run(ctx, factoryDir, "", "factory", "replace-current")
	if err != nil {
		t.Fatalf("you factory replace-current: %v\noutput:\n%s", err, replaceOut)
	}
	replaceOutput := string(replaceOut)
	if !strings.Contains(replaceOutput, factoryReplaceSuccessMarker) {
		t.Fatalf("replace-current output missing success marker %q:\n%s", factoryReplaceSuccessMarker, replaceOutput)
	}

	postReplace := queryFactoryViaCLIJSON(t, ctx, remote.process, remote.baseURL, factoryDir)
	if postReplace.Id == nil || *postReplace.Id != factoryWiringName {
		t.Fatalf("post-replace factory id = %#v, want %q", postReplace.Id, factoryWiringName)
	}
	if postReplace.Version == nil {
		t.Fatal("post-replace factory version missing")
	}
	if postReplace.Version.Logical.Int64() <= preReplaceLogical {
		t.Fatalf(
			"post-replace factory version logical = %d, want > pre-replace %d",
			postReplace.Version.Logical.Int64(),
			preReplaceLogical,
		)
	}
}

func queryFactoryViaCLIJSON(
	t *testing.T,
	ctx context.Context,
	processHarness *builtcliacceptance.Harness,
	serverURL, workDir string,
) factoryapi.Factory {
	t.Helper()

	queryCmd := processHarness.CommandContext(ctx,
		"--json",
		"--server", serverURL,
		"factory", "show",
	)
	queryCmd.Dir = workDir
	queryOut, err := queryCmd.Output()
	if err != nil {
		t.Fatalf("you factory show --json: %v", err)
	}

	var current factoryapi.Factory
	if err := json.Unmarshal(queryOut, &current); err != nil {
		t.Fatalf("decode factory show JSON: %v\n%s", err, string(queryOut))
	}
	return current
}

func flattenFactoryConfigViaCLI(t *testing.T, ctx context.Context, processHarness *builtcliacceptance.Harness, factoryDir string) any {
	t.Helper()

	flattenCmd := processHarness.CommandContext(ctx,
		"factory", "config", "flatten", factoryDir,
	)
	flattenCmd.Dir = factoryDir
	flattenOut, err := flattenCmd.Output()
	if err != nil {
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
