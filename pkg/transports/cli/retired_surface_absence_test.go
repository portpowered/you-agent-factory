package cli

import (
	"io"
	"strings"
	"testing"

	configcli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/config"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
)

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
	{name: "factory query", args: []string{"factory", "query"}},
	{name: "work visualize", args: []string{"work", "visualize"}},
	{name: "session dispatches", args: []string{"session", "dispatches", "session-customer"}},
	{name: "serve acp", args: []string{"serve", "acp"}},
	{name: "mcp serve", args: []string{"mcp", "serve"}},
}

var settledRetiredDocsTopics = []string{"packaged-fusion", "packaged-goal", "packaged-tts", "mcp-hosts"}

var canonicalDocsTopicSamples = []string{
	"agents",
	"authoring-factories",
	"run",
	"config",
	"mcp",
	"javascript-workflows",
}

var canonicalDocsTopicAliases = []string{
	"batch-work",
	"workstation",
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
			root := newLegacyTestRootCommand()
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

func TestRetiredDocsTopics_RejectUnsupportedAtRuntime(t *testing.T) {
	for _, topic := range settledRetiredDocsTopics {
		topic := topic
		t.Run(topic, func(t *testing.T) {
			var stdout strings.Builder
			root := newLegacyTestRootCommand()
			root.SetOut(&stdout)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"docs", topic})

			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), `unsupported docs topic "`+topic+`"`) {
				t.Fatalf("execute docs %s error = %v, want unsupported-topic error", topic, err)
			}
			if got := stdout.String(); got != "" {
				t.Fatalf("retired docs topic %s wrote stdout %q", topic, got)
			}
		})
	}
}

func TestRetiredDocsTopics_CanonicalTopicsRemainResolvable(t *testing.T) {
	for _, topic := range docscli.SupportedTopics() {
		topic := topic
		t.Run("markdown/"+topic, func(t *testing.T) {
			got, err := docscli.Markdown(topic)
			if err != nil {
				t.Fatalf("Markdown(%q): %v", topic, err)
			}
			if strings.TrimSpace(got) == "" {
				t.Fatalf("Markdown(%q) returned empty body", topic)
			}
		})
	}

	for _, topic := range canonicalDocsTopicSamples {
		topic := topic
		t.Run("cli/"+topic, func(t *testing.T) {
			var stdout strings.Builder
			root := newLegacyTestRootCommand()
			root.SetOut(&stdout)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"docs", topic})

			if err := root.Execute(); err != nil {
				t.Fatalf("execute docs %s: %v", topic, err)
			}
			if strings.TrimSpace(stdout.String()) == "" {
				t.Fatalf("execute docs %s returned empty body", topic)
			}
		})
	}

	for _, alias := range canonicalDocsTopicAliases {
		alias := alias
		t.Run("alias/"+alias, func(t *testing.T) {
			got, err := docscli.Markdown(alias)
			if err != nil {
				t.Fatalf("Markdown(%q): %v", alias, err)
			}
			if strings.TrimSpace(got) == "" {
				t.Fatalf("Markdown(%q) returned empty body", alias)
			}
		})
	}
}

func TestRetiredSurfaceResidue_FactorySaveDoesNotInvokeOwningPersistence(t *testing.T) {
	originalCreate := createFactoryFromFile
	originalReplace := replaceFactoryCurrent
	defer func() {
		createFactoryFromFile = originalCreate
		replaceFactoryCurrent = originalReplace
	}()

	createCalled := false
	replaceCalled := false
	createFactoryFromFile = func(factorycli.CreateFromFileConfig) error {
		createCalled = true
		return nil
	}
	replaceFactoryCurrent = func(factorycli.ReplaceCurrentConfig) error {
		replaceCalled = true
		return nil
	}

	root := newLegacyTestRootCommand()
	var output strings.Builder
	root.SetOut(&output)
	root.SetErr(&output)
	for _, args := range [][]string{
		{"factory", "save", "staging", "--from", "./factory.json"},
		{"factory", "save"},
	} {
		output.Reset()
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("args %v: expected unknown command error", args)
		}
	}

	if createCalled {
		t.Fatal("removed factory save must not invoke create persistence")
	}
	if replaceCalled {
		t.Fatal("removed factory save must not invoke replace-current persistence")
	}
}
