package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/retiredsurfaceguard"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
)

var settledRetiredCLIPaths = retiredsurfaceguard.SettledRetiredCLIPaths()

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

var settledRetiredDocsTopics = retiredsurfaceguard.SettledRetiredDocsTopics()

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

func TestRetiredCLICommands_AbsentFromProductionCommandTree(t *testing.T) {
	root := newLegacyTestRootCommand()

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
	root := newLegacyTestRootCommand()

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
	root := newLegacyTestRootCommand()

	for _, path := range canonicalCLIReplacementPaths {
		if _, _, err := root.Find(path); err != nil {
			t.Fatalf("find canonical replacement %v: %v", path, err)
		}
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

func TestRetiredDocsTopics_AbsentFromRegistry(t *testing.T) {
	supported := make(map[string]struct{}, len(docscli.SupportedTopics()))
	for _, topic := range docscli.SupportedTopics() {
		supported[topic] = struct{}{}
	}
	commands := make(map[string]struct{}, len(docscli.SupportedTopicCommands()))
	for _, command := range docscli.SupportedTopicCommands() {
		commands[command] = struct{}{}
	}

	for _, topic := range settledRetiredDocsTopics {
		if _, stillSupported := supported[topic]; stillSupported {
			t.Fatalf("retired topic %q is still listed in SupportedTopics()", topic)
		}
		if _, stillAccepted := commands[topic]; stillAccepted {
			t.Fatalf("retired topic %q is still accepted by SupportedTopicCommands()", topic)
		}
	}
}

func TestRetiredDocsTopics_NoCompatibilityAliases(t *testing.T) {
	retired := make(map[string]struct{}, len(settledRetiredDocsTopics))
	for _, topic := range settledRetiredDocsTopics {
		retired[topic] = struct{}{}
	}

	for _, entry := range docscli.TopicIndexEntries() {
		if _, isRetired := retired[entry.Name]; isRetired {
			t.Fatalf("retired topic %q is registered as a canonical docs topic", entry.Name)
		}
		for _, alias := range entry.Aliases {
			if _, isRetired := retired[alias]; isRetired {
				t.Fatalf("compatibility alias %q on topic %q reintroduces retired docs topic", alias, entry.Name)
			}
		}
	}

	for _, topic := range settledRetiredDocsTopics {
		got, err := docscli.Markdown(topic)
		if err == nil || got != "" {
			t.Fatalf("Markdown(%q) = %q, %v; want unsupported-topic error", topic, got, err)
		}
		if !strings.Contains(err.Error(), `unsupported docs topic "`+topic+`"`) {
			t.Fatalf("Markdown(%q) error = %v, want unsupported-topic error", topic, err)
		}
	}
}

func TestRetiredDocsTopics_AbsentFromDocsCommandValidArgs(t *testing.T) {
	root := newLegacyTestRootCommand()
	docsCmd, _, err := root.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("find docs command: %v", err)
	}

	validArgs := make(map[string]struct{}, len(docsCmd.ValidArgs))
	for _, arg := range docsCmd.ValidArgs {
		validArgs[arg] = struct{}{}
	}

	for _, topic := range settledRetiredDocsTopics {
		if _, stillAccepted := validArgs[topic]; stillAccepted {
			t.Fatalf("retired topic %q is still listed in docs command ValidArgs", topic)
		}
	}
}

func TestRetiredDocsTopics_AbsentFromDocsIndex(t *testing.T) {
	index := docscli.IndexMarkdown("you")
	for _, topic := range settledRetiredDocsTopics {
		if strings.Contains(index, "`"+topic+"`") {
			t.Fatalf("docs index still lists retired topic %q:\n%s", topic, index)
		}
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

func TestRetiredSurfaceGuards_ProductionTreePasses(t *testing.T) {
	root := newLegacyTestRootCommand()
	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("walk command tree: %v", err)
	}

	cliInventory := retiredsurfaceguard.CLIInventory{
		Commands: make([]retiredsurfaceguard.CLICommandRecord, 0, len(inventory.Commands)),
	}
	for _, record := range inventory.Commands {
		cliInventory.Commands = append(cliInventory.Commands, retiredsurfaceguard.CLICommandRecord{
			Path:              record.Path,
			Aliases:           append([]string(nil), record.Aliases...),
			Visibility:        record.Visibility,
			Lifecycle:         record.Lifecycle,
			DeprecatedMessage: record.DeprecatedMessage,
		})
	}

	indexEntries := make([]retiredsurfaceguard.DocsTopicEntry, 0, len(docscli.TopicIndexEntries()))
	for _, entry := range docscli.TopicIndexEntries() {
		indexEntries = append(indexEntries, retiredsurfaceguard.DocsTopicEntry{
			Name:    entry.Name,
			Aliases: append([]string(nil), entry.Aliases...),
		})
	}
	docsRegistry := retiredsurfaceguard.DocsRegistry{
		SupportedTopics:   docscli.SupportedTopics(),
		SupportedCommands: docscli.SupportedTopicCommands(),
		IndexEntries:      indexEntries,
	}

	if violations := retiredsurfaceguard.ScanCLIReintroductionViolations(cliInventory); len(violations) != 0 {
		t.Fatalf("CLI guard violations = %#v", violations)
	}
	if violations := retiredsurfaceguard.ScanDocsReintroductionViolations(docsRegistry); len(violations) != 0 {
		t.Fatalf("docs guard violations = %#v", violations)
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
