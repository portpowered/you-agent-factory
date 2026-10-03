package named_lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	namedLifecycleFactoryName = "cli-named-lifecycle"
	listMembershipFactoryName = "cli-list-membership"
	deleteMissingFactoryName  = "cli-delete-missing"
	namedLifecycleWorkType    = "task"
	namedLifecycleUpdatedType = "updated-task"
	listMembershipWorkType    = "membership-task"
)

// Parallel invocations share the production graph but own homes, catalogs,
// settings and writers. Rebinding a handler to one profile must not affect peers.
func TestFactoryListAndInitKeepConcurrentProfilesSeparate(t *testing.T) {
	for _, name := range []string{"profile-alpha", "profile-beta"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home, working := t.TempDir(), t.TempDir()
			roots, err := factorydefinitions.ResolveNamedFactoryRoots(home, working)
			if err != nil {
				t.Fatal(err)
			}
			source := support.ScaffoldFactory(t, namedLifecycleFactoryConfig("profile-task"))
			execute := func(args ...string) *support.CapturedInputs {
				t.Helper()
				inputs := support.FakeInputs(t.Context(), args)
				inputs.Input.Env = []string{"USERPROFILE=" + home, "HOME=" + home}
				inputs.Input.WorkingDirectory = working
				if err := namedLifecycleProcess.Execute(inputs.Input); err != nil {
					t.Fatalf("Execute(%v): %v; stderr=%s", args, err, inputs.Stderr())
				}
				return inputs
			}
			execute("you", "factory", "create", name, "--from", filepath.Join(source, "factory.json"), "--dir", roots.Global)
			execute("you", "init", "--provider", "codex", "--model", name)
			listed := execute("you", "--json", "factory", "list")
			var entries []struct {
				Name      string `json:"name"`
				Directory string `json:"factoryDirectory"`
			}
			if err := json.Unmarshal([]byte(listed.Stdout()), &entries); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name, "profile-") {
					if entry.Name != name || entry.Directory != filepath.Join(roots.Global, name) {
						t.Fatalf("profile catalog leaked: %#v", entry)
					}
					found = true
				}
			}
			if !found {
				t.Fatalf("list omitted own factory: %s", listed.Stdout())
			}
			// Omitting the model retains and reports the previously persisted
			// default through the public command, rather than reading its store.
			reloaded := execute("you", "init", "--provider", "codex")
			if !strings.Contains(reloaded.Stdout(), "Configured default provider codex and model "+name) {
				t.Fatalf("profile defaults did not survive a new invocation: %q", reloaded.Stdout())
			}
		})
	}
}

func TestMissingHomeRejectsFactoryEffectsButKeepsHelpAvailable(t *testing.T) {
	t.Parallel()
	working := t.TempDir()
	for _, test := range []struct {
		args   []string
		prefix string
	}{
		{[]string{"you", "factory", "list"}, "resolve factory list home: "},
		{[]string{"you", "init", "--package", "@you/goal", "--dir", filepath.Join(working, "installed")}, "resolve init home directory: "},
	} {
		inputs := support.FakeInputs(t.Context(), test.args)
		inputs.Input.Env = []string{}
		inputs.Input.WorkingDirectory = working
		err := namedLifecycleProcess.Execute(inputs.Input)
		want := "CLI_COMMAND_FAILED: command failed: " + test.prefix + "home directory is not defined in the supplied environment"
		if err == nil || err.Error() != want || inputs.Stdout() != "" {
			t.Fatalf("Execute(%v): error=%v stdout=%q, want %q and no success", test.args, err, inputs.Stdout(), want)
		}
		inputs = support.FakeInputs(t.Context(), append(append([]string{}, test.args...), "--help"))
		inputs.Input.Env = []string{}
		inputs.Input.WorkingDirectory = working
		if err := namedLifecycleProcess.Execute(inputs.Input); err != nil || !strings.Contains(inputs.Stdout(), "Usage:") {
			t.Fatalf("help error=%v stdout=%q", err, inputs.Stdout())
		}
	}
	entries, err := os.ReadDir(working)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed commands wrote working directory: %v error=%v", entries, err)
	}
}

// TestCLIFactoryNamedCreateListUpdateDelete proves the public you factory
// create, list, update, and delete commands succeed as one named Factory
// lifecycle through root.BuildProcess + Process.Execute, persisting and
// revising factory.json under the named catalog root before removal.
func TestCLIFactoryNamedCreateListUpdateDelete(t *testing.T) {
	t.Parallel()
	workingDirectory := t.TempDir()
	namedFactoriesRoot := filepath.Join(t.TempDir(), "named-factories")
	initialSourceDir := support.ScaffoldFactory(t, namedLifecycleFactoryConfig(namedLifecycleWorkType))
	updatedSourceDir := support.ScaffoldFactory(t, namedLifecycleFactoryConfig(namedLifecycleUpdatedType))
	initialSourcePath := filepath.Join(initialSourceDir, "factory.json")
	updatedSourcePath := filepath.Join(updatedSourceDir, "factory.json")

	process := namedLifecycleProcess

	create := support.FakeInputs(t.Context(), []string{
		"you",
		"factory", "create", namedLifecycleFactoryName,
		"--from", initialSourcePath,
		"--dir", namedFactoriesRoot,
	})
	create.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(create.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory create) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			create.Stdout(),
			create.Stderr(),
		)
	}
	for _, marker := range []string{
		"Created factory " + namedLifecycleFactoryName,
		"Directory:",
	} {
		if !strings.Contains(create.Stdout(), marker) {
			t.Fatalf("factory create output missing %q:\n%s", marker, create.Stdout())
		}
	}

	factoryDir := filepath.Join(namedFactoriesRoot, namedLifecycleFactoryName)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if _, err := os.Stat(factoryPath); err != nil {
		t.Fatalf("created factory.json missing at %s: %v", factoryPath, err)
	}
	assertFactoryListIncludes(t, process, workingDirectory, namedFactoriesRoot, namedLifecycleFactoryName)

	update := support.FakeInputs(t.Context(), []string{
		"you",
		"factory", "update", namedLifecycleFactoryName,
		"--from", updatedSourcePath,
		"--dir", namedFactoriesRoot,
	})
	update.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(update.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory update) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			update.Stdout(),
			update.Stderr(),
		)
	}
	if !strings.Contains(update.Stdout(), "Updated factory "+namedLifecycleFactoryName) {
		t.Fatalf("factory update output missing success marker:\n%s", update.Stdout())
	}
	assertPersistedWorkType(t, factoryPath, namedLifecycleUpdatedType)

	deleteInputs := support.FakeInputs(t.Context(), []string{
		"you",
		"factory", "delete", namedLifecycleFactoryName,
		"--dir", namedFactoriesRoot,
	})
	deleteInputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(deleteInputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory delete) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			deleteInputs.Stdout(),
			deleteInputs.Stderr(),
		)
	}
	if !strings.Contains(deleteInputs.Stdout(), "Deleted factory "+namedLifecycleFactoryName) {
		t.Fatalf("factory delete output missing success marker:\n%s", deleteInputs.Stdout())
	}
	if _, err := os.Stat(factoryPath); !os.IsNotExist(err) {
		t.Fatalf("factory.json still present after delete at %s: %v", factoryPath, err)
	}
}

// TestCLIFactoryListReflectsCreateAndDelete proves public factory list membership
// includes a named Factory after create and omits it after delete through
// root.BuildProcess + Process.Execute.
func TestCLIFactoryListReflectsCreateAndDelete(t *testing.T) {
	t.Parallel()
	workingDirectory := t.TempDir()
	namedFactoriesRoot := filepath.Join(t.TempDir(), "named-factories")
	sourceDir := support.ScaffoldFactory(t, listMembershipFactoryConfig(listMembershipWorkType))
	sourcePath := filepath.Join(sourceDir, "factory.json")

	process := namedLifecycleProcess

	create := support.FakeInputs(t.Context(), []string{
		"you",
		"factory", "create", listMembershipFactoryName,
		"--from", sourcePath,
		"--dir", namedFactoriesRoot,
	})
	create.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(create.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory create) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			create.Stdout(),
			create.Stderr(),
		)
	}

	assertFactoryListIncludes(t, process, workingDirectory, namedFactoriesRoot, listMembershipFactoryName)
	assertHumanFactoryListIncludes(t, process, workingDirectory, namedFactoriesRoot, listMembershipFactoryName)

	deleteInputs := support.FakeInputs(t.Context(), []string{
		"you",
		"factory", "delete", listMembershipFactoryName,
		"--dir", namedFactoriesRoot,
	})
	deleteInputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(deleteInputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory delete) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			deleteInputs.Stdout(),
			deleteInputs.Stderr(),
		)
	}

	assertFactoryListExcludes(t, process, workingDirectory, namedFactoriesRoot, listMembershipFactoryName)
	assertHumanFactoryListExcludes(t, process, workingDirectory, namedFactoriesRoot, listMembershipFactoryName)
}

// TestCLIFactoryDeleteMissingReturnsActionableFailure proves deleting a named
// Factory that is not in the catalog fails with an actionable public diagnostic
// through root.BuildProcess + Process.Execute without reporting delete success
// or creating catalog entries.
func TestCLIFactoryDeleteMissingReturnsActionableFailure(t *testing.T) {
	t.Parallel()
	workingDirectory := t.TempDir()
	namedFactoriesRoot := filepath.Join(t.TempDir(), "named-factories")

	process := namedLifecycleProcess

	deleteInputs := support.FakeInputs(t.Context(), []string{
		"you",
		"factory", "delete", deleteMissingFactoryName,
		"--dir", namedFactoriesRoot,
	})
	deleteInputs.Input.WorkingDirectory = workingDirectory
	err := process.Execute(deleteInputs.Input)
	if err == nil {
		t.Fatalf(
			"Process.Execute(factory delete missing) error = nil, want failure; stdout:\n%s\nstderr:\n%s",
			deleteInputs.Stdout(),
			deleteInputs.Stderr(),
		)
	}
	if !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf(
			"factory delete missing error = %v, want actionable not-found diagnostic; stdout:\n%s\nstderr:\n%s",
			err,
			deleteInputs.Stdout(),
			deleteInputs.Stderr(),
		)
	}
	if strings.Contains(deleteInputs.Stdout(), "Deleted factory "+deleteMissingFactoryName) {
		t.Fatalf("factory delete missing reported success:\n%s", deleteInputs.Stdout())
	}

	assertFactoryListExcludes(t, process, workingDirectory, namedFactoriesRoot, deleteMissingFactoryName)

	factoryPath := filepath.Join(namedFactoriesRoot, deleteMissingFactoryName, "factory.json")
	if _, statErr := os.Stat(factoryPath); !os.IsNotExist(statErr) {
		t.Fatalf("delete missing created factory.json at %s: %v", factoryPath, statErr)
	}
}

func assertFactoryListIncludes(
	t *testing.T,
	process support.Process,
	workingDirectory, namedFactoriesRoot, name string,
) {
	t.Helper()

	entries := executeJSONFactoryList(t, process, workingDirectory, namedFactoriesRoot)
	for _, entry := range entries {
		if entry.Name == name {
			return
		}
	}
	t.Fatalf("factory list %#v missing named Factory %q", entries, name)
}

func assertFactoryListExcludes(
	t *testing.T,
	process support.Process,
	workingDirectory, namedFactoriesRoot, name string,
) {
	t.Helper()

	entries := executeJSONFactoryList(t, process, workingDirectory, namedFactoriesRoot)
	for _, entry := range entries {
		if entry.Name == name {
			t.Fatalf("factory list %#v still includes named Factory %q after delete", entries, name)
		}
	}
}

func assertHumanFactoryListIncludes(
	t *testing.T,
	process support.Process,
	workingDirectory, namedFactoriesRoot, name string,
) {
	t.Helper()

	output := executeHumanFactoryList(t, process, workingDirectory, namedFactoriesRoot)
	if !humanFactoryListContainsName(output, name) {
		t.Fatalf("human factory list missing named Factory %q:\n%s", name, output)
	}
}

func assertHumanFactoryListExcludes(
	t *testing.T,
	process support.Process,
	workingDirectory, namedFactoriesRoot, name string,
) {
	t.Helper()

	output := executeHumanFactoryList(t, process, workingDirectory, namedFactoriesRoot)
	if humanFactoryListContainsName(output, name) {
		t.Fatalf("human factory list still includes named Factory %q after delete:\n%s", name, output)
	}
}

func executeJSONFactoryList(
	t *testing.T,
	process support.Process,
	workingDirectory, namedFactoriesRoot string,
) []factoryListEntry {
	t.Helper()

	list := support.FakeInputs(t.Context(), []string{
		"you", "--json",
		"factory", "list",
		"--dir", namedFactoriesRoot,
	})
	list.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(list.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory list) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			list.Stdout(),
			list.Stderr(),
		)
	}

	var entries []factoryListEntry
	if err := json.Unmarshal([]byte(list.Stdout()), &entries); err != nil {
		t.Fatalf("decode factory list: %v\n%s", err, list.Stdout())
	}
	return entries
}

func executeHumanFactoryList(
	t *testing.T,
	process support.Process,
	workingDirectory, namedFactoriesRoot string,
) string {
	t.Helper()

	list := support.FakeInputs(t.Context(), []string{
		"you",
		"factory", "list",
		"--dir", namedFactoriesRoot,
	})
	list.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(list.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory list) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			list.Stdout(),
			list.Stderr(),
		)
	}
	return list.Stdout()
}

func humanFactoryListContainsName(output, name string) bool {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		switch line {
		case "NAME\tFACTORY DIRECTORY\tCURRENT", "No factories found.":
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) > 0 && fields[0] == name {
			return true
		}
	}
	return false
}

func assertPersistedWorkType(t *testing.T, factoryPath, wantWorkType string) {
	t.Helper()

	data, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read persisted factory.json: %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode persisted factory.json: %v", err)
	}
	workTypes, ok := persisted["workTypes"].([]any)
	if !ok || len(workTypes) == 0 {
		t.Fatalf("persisted factory.json missing workTypes: %#v", persisted)
	}
	first, ok := workTypes[0].(map[string]any)
	if !ok {
		t.Fatalf("persisted workTypes[0] = %#v, want object", workTypes[0])
	}
	if got, _ := first["name"].(string); got != wantWorkType {
		t.Fatalf("persisted work type name = %q, want %q", got, wantWorkType)
	}
}

func listMembershipFactoryConfig(workType string) map[string]any {
	return namedLifecycleFactoryConfigWithName(listMembershipFactoryName, workType)
}

func namedLifecycleFactoryConfig(workType string) map[string]any {
	return namedLifecycleFactoryConfigWithName(namedLifecycleFactoryName, workType)
}

func namedLifecycleFactoryConfigWithName(factoryName, workType string) map[string]any {
	return map[string]any{
		"name": factoryName,
		"workTypes": []map[string]any{
			{
				"name": workType,
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
				"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": workType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": workType, "state": "failed"}},
			},
		},
	}
}

type factoryListEntry struct {
	Name             string `json:"name"`
	FactoryDirectory string `json:"factoryDirectory"`
}
