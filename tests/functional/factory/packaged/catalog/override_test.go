package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalFactoryName    = "@you/goal"
	localGoalOverrideDescription = "customer local override for packaged goal"
)

// TestLocalFactoryOverridesPackagedFactoryWithSameName proves that when a customer
// materializes a local Factory under the same name as a packaged built-in, the
// public factory list and named-selection catalog observe the local definition
// (directory and description) instead of the unmaterialized packaged entry.
func TestLocalFactoryOverridesPackagedFactoryWithSameName(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	globalRoot := filepath.Join(home, ".you-agent-factory", "factories")
	localFactoryDir := writeScopedFactory(
		t,
		globalRoot,
		packagedGoalFactoryName,
		localGoalOverrideDescription,
	)

	entries := executeFactoryList(t, home, workingDirectory)
	assertEntry(
		t,
		entries,
		packagedGoalFactoryName,
		localFactoryDir,
		localGoalOverrideDescription,
	)

	completion := executeNamedFactoryCompletion(t, home, workingDirectory, packagedGoalFactoryName)
	if !strings.Contains(completion, localGoalOverrideDescription) {
		t.Fatalf(
			"named selection completion = %q, want local description %q",
			completion,
			localGoalOverrideDescription,
		)
	}
	if strings.Contains(completion, `"factoryDirectory":"-"`) {
		t.Fatalf(
			"named selection completion = %q, want materialized local Factory not packaged fallback",
			completion,
		)
	}
}

func writeScopedFactory(t *testing.T, root, name, description string) string {
	t.Helper()

	factoryDirectory, err := factorynamedpaths.MapDir(root, name)
	if err != nil {
		t.Fatalf("MapDir(%q, %q): %v", root, name, err)
	}
	payload, err := json.Marshal(map[string]any{
		"name": name,
		"id":   strings.TrimPrefix(strings.ReplaceAll(name, "/", "-"), "@"),
		"description": map[string]any{
			"type":  "LOCALIZABLE_ASSET",
			"value": description,
		},
		"workTypes":    []any{},
		"resources":    []any{},
		"workers":      []any{},
		"workstations": []any{},
	})
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDirectory, "factory.json"), payload, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return factoryDirectory
}

func executeFactoryList(t *testing.T, home, workingDirectory string) []listEntry {
	t.Helper()

	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "factory", "list"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory list) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	var entries []listEntry
	if err := json.Unmarshal([]byte(inputs.Stdout()), &entries); err != nil {
		t.Fatalf("decode factory list: %v\n%s", err, inputs.Stdout())
	}
	return entries
}

func executeNamedFactoryCompletion(t *testing.T, home, workingDirectory, name string) string {
	t.Helper()

	inputs := support.FakeInputs(
		t.Context(),
		[]string{"you", "__complete", "run", "--named", name},
	)
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(named completion) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout()
}
