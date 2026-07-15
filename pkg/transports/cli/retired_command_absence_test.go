package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
)

// settledRetiredCLIPaths are hard-retired CLI surfaces from S11/S12/B08 closeout.
// Keep aligned with docs/reference/config.md and repository guards in story 005.
var settledRetiredCLIPaths = []string{
	"you config validate",
	"you config flatten",
	"you config expand",
	"you factory save",
	"you factory validate",
}

var settledRetiredCLIInvocations = []struct {
	name string
	args []string
}{
	{name: "config validate", args: []string{"config", "validate", "./factory.json"}},
	{name: "config flatten", args: []string{"config", "flatten", "./factory"}},
	{name: "config expand", args: []string{"config", "expand", "./factory.json"}},
	{name: "factory save with args", args: []string{"factory", "save", "staging", "--from", "./factory.json"}},
	{name: "factory save bare", args: []string{"factory", "save"}},
	{name: "factory save name only", args: []string{"factory", "save", "staging"}},
	{name: "factory validate", args: []string{"factory", "validate", "./factory.json"}},
}

var canonicalCLIReplacementPaths = [][]string{
	{"config", "init"},
	{"factory", "config", "validate"},
	{"factory", "config", "flatten"},
	{"factory", "config", "expand"},
	{"factory", "create"},
	{"factory", "update"},
	{"factory", "replace-current"},
}

func TestRetiredCLICommands_RejectUnknownAtRuntime(t *testing.T) {
	originalValidate := validateFactory
	originalFlatten := flattenFactoryConfig
	originalExpand := expandFactoryConfig
	originalCreate := createFactoryFromFile
	originalReplace := replaceFactoryCurrent
	defer func() {
		validateFactory = originalValidate
		flattenFactoryConfig = originalFlatten
		expandFactoryConfig = originalExpand
		createFactoryFromFile = originalCreate
		replaceFactoryCurrent = originalReplace
	}()

	validateCalled := false
	flattenCalled := false
	expandCalled := false
	createCalled := false
	replaceCalled := false

	validateFactory = func(factorycli.ValidateConfig) error {
		validateCalled = true
		return nil
	}
	flattenFactoryConfig = func(configcli.FactoryConfigFlattenConfig) error {
		flattenCalled = true
		return nil
	}
	expandFactoryConfig = func(configcli.FactoryConfigExpandConfig) error {
		expandCalled = true
		return nil
	}
	createFactoryFromFile = func(factorycli.CreateFromFileConfig) error {
		createCalled = true
		return nil
	}
	replaceFactoryCurrent = func(factorycli.ReplaceCurrentConfig) error {
		replaceCalled = true
		return nil
	}

	for _, tc := range settledRetiredCLIInvocations {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			root := NewRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(tc.args)

			err := root.Execute()
			if err == nil {
				t.Fatalf("expected retired invocation %v to fail", tc.args)
			}
			if !strings.Contains(err.Error(), "unknown command") {
				t.Fatalf("execute %v: got %v, want unknown-command error", tc.args, err)
			}
		})
	}

	if validateCalled {
		t.Fatal("retired CLI paths must not invoke factory config validate")
	}
	if flattenCalled {
		t.Fatal("retired CLI paths must not invoke factory config flatten")
	}
	if expandCalled {
		t.Fatal("retired CLI paths must not invoke factory config expand")
	}
	if createCalled {
		t.Fatal("retired CLI paths must not invoke factory create persistence")
	}
	if replaceCalled {
		t.Fatal("retired CLI paths must not invoke factory replace-current persistence")
	}
}

func TestRetiredCLICommands_AbsentFromProductionCommandTree(t *testing.T) {
	root := NewRootCommand()

	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("walk command tree: %v", err)
	}

	registered := make(map[string]struct{}, len(inventory.Commands))
	for _, record := range inventory.Commands {
		registered[record.Path] = struct{}{}
	}

	for _, path := range settledRetiredCLIPaths {
		if _, stillRegistered := registered[path]; stillRegistered {
			t.Fatalf("retired path %q is still registered in the production command tree", path)
		}
	}
}

func TestRetiredCLICommands_NoHiddenDeprecatedOrAliasWrappers(t *testing.T) {
	root := NewRootCommand()

	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("walk command tree: %v", err)
	}

	retired := make(map[string]struct{}, len(settledRetiredCLIPaths))
	for _, path := range settledRetiredCLIPaths {
		retired[path] = struct{}{}
	}

	for _, record := range inventory.Commands {
		if _, stillRegistered := retired[record.Path]; stillRegistered {
			t.Fatalf("retired path %q is still registered", record.Path)
		}
		if record.Visibility == "hidden" || record.Lifecycle == "deprecated" {
			for path := range retired {
				if record.Path == path {
					t.Fatalf("%s command %q reintroduces retired path", record.Visibility, record.Path)
				}
			}
		}
		for _, alias := range record.Aliases {
			for _, retiredPath := range settledRetiredCLIPaths {
				retiredSegments := strings.Fields(strings.TrimPrefix(retiredPath, "you "))
				if len(retiredSegments) < 2 {
					continue
				}
				retiredLeaf := retiredSegments[len(retiredSegments)-1]
				recordSegments := strings.Fields(strings.TrimPrefix(record.Path, "you "))
				if len(recordSegments) < 2 {
					continue
				}
				if alias == retiredLeaf && strings.Join(recordSegments[:len(recordSegments)-1], " ") == strings.Join(retiredSegments[:len(retiredSegments)-1], " ") {
					t.Fatalf("alias %q on %q would reintroduce retired path %q", alias, record.Path, retiredPath)
				}
			}
		}
	}
}

func TestRetiredCLICommands_CanonicalReplacementsRemainReachable(t *testing.T) {
	root := NewRootCommand()

	for _, path := range canonicalCLIReplacementPaths {
		if _, _, err := root.Find(path); err != nil {
			t.Fatalf("find canonical replacement %v: %v", path, err)
		}
	}
}
