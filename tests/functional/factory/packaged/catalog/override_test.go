package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	factorynamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalFactoryName        = "@you/goal"
	localGoalOverrideDescription   = "customer local override for packaged goal"
	invalidOverrideLeakProbe       = "broken-local-override-secret"
	invalidGoalOverrideFactoryJSON = `{"` + invalidOverrideLeakProbe + `":"do-not-expose"`
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

// TestInvalidLocalOverrideDoesNotFallBackSilently proves that when a customer
// publishes a broken local Factory under the same name as a packaged built-in,
// the public factory list and named-selection catalog surface the
// misconfiguration instead of silently resolving to the packaged Factory.
func TestInvalidLocalOverrideDoesNotFallBackSilently(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	globalRoot := filepath.Join(home, ".you-agent-factory", "factories")
	writeInvalidScopedFactory(t, globalRoot, packagedGoalFactoryName, []byte(invalidGoalOverrideFactoryJSON))

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
	for _, entry := range entries {
		if entry.Name != packagedGoalFactoryName {
			continue
		}
		t.Fatalf(
			"factory list entry %s = %#v, want no silent packaged fallback for shadowed name",
			packagedGoalFactoryName,
			entry,
		)
	}
	if !slices.ContainsFunc(entries, func(entry listEntry) bool {
		return strings.HasPrefix(entry.Name, "@you/") &&
			entry.Name != packagedGoalFactoryName &&
			entry.FactoryDirectory == "-"
	}) {
		t.Fatalf(
			"entries = %#v, want other packaged Factories still visible alongside invalid override",
			entries,
		)
	}

	diagnostics := inputs.Stderr()
	if !strings.Contains(diagnostics, packagedGoalFactoryName) ||
		!strings.Contains(diagnostics, "malformed") {
		t.Fatalf(
			"diagnostics = %q, want customer-visible misconfiguration for %s",
			diagnostics,
			packagedGoalFactoryName,
		)
	}
	if strings.Contains(diagnostics, invalidOverrideLeakProbe) ||
		strings.Contains(diagnostics, "do-not-expose") {
		t.Fatalf("diagnostics leaked invalid override payload: %q", diagnostics)
	}

	completion := executeNamedFactoryCompletion(t, home, workingDirectory, packagedGoalFactoryName)
	if strings.Contains(completion, packagedGoalFactoryName) {
		t.Fatalf(
			"named selection completion = %q, want %s absent instead of packaged fallback",
			completion,
			packagedGoalFactoryName,
		)
	}
}

func writeInvalidScopedFactory(t *testing.T, root, name string, payload []byte) string {
	t.Helper()

	factoryDirectory, err := factorynamedpaths.MapDir(root, name)
	if err != nil {
		t.Fatalf("MapDir(%q, %q): %v", root, name, err)
	}
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDirectory, "factory.json"), payload, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return factoryDirectory
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
