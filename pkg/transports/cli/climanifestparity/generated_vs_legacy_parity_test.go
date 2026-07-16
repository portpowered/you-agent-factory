package climanifestparity_test

import (
	"fmt"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	"github.com/spf13/cobra"
)

func TestGeneratedVsLegacyParityMatrix_RepresentativeFamily(t *testing.T) {
	representativeFamilyCases := []struct {
		commandID string
		path      string
	}{
		{commandID: "you", path: "you"},
		{commandID: "you.session", path: "you session"},
		{commandID: "you.session.show", path: "you session show"},
	}

	t.Run("identity", func(t *testing.T) {
		legacyRoot, generatedRoot := mustRepresentativeConstructorRoots(t)
		for _, tc := range representativeFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyCmd, err := climanifestparity.FindCommandByPath(legacyRoot, tc.path)
				if err != nil {
					t.Fatalf("legacy FindCommandByPath(%q) error = %v", tc.path, err)
				}
				generatedCmd, err := climanifestparity.FindCommandByPath(generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("generated FindCommandByPath(%q) error = %v", tc.path, err)
				}
				mismatches := climanifestparity.CompareConstructorIdentityParity(tc.commandID, legacyCmd, generatedCmd)
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("help", func(t *testing.T) {
		for _, tc := range representativeFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustRepresentativeConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorHelpParity(tc.commandID, legacyRoot, generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("CompareConstructorHelpParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("completion", func(t *testing.T) {
		for _, tc := range representativeFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustRepresentativeConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorCompletionInventoryParity(tc.commandID, tc.path, legacyRoot, generatedRoot)
				if err != nil {
					t.Fatalf("CompareConstructorCompletionInventoryParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("parsing", func(t *testing.T) {
		for _, tc := range generatedVsLegacyParsingCases() {
			t.Run(tc.name, func(t *testing.T) {
				legacyRoot, generatedRoot := mustRepresentativeConstructorRoots(t)
				mismatches := climanifestparity.CompareConstructorParseParity(
					tc.commandID,
					legacyRoot,
					generatedRoot,
					tc.argv,
					tc.wantParseErr,
					tc.errContains,
				)
				assertNoConstructorMismatches(t, mismatches)

				if tc.wantParseErr {
					return
				}

				legacyLeaf, _, legacyErr := climanifestparity.ParseArgvOnRoot(legacyRoot, tc.argv)
				if legacyErr != nil {
					t.Fatalf("legacy ParseArgvOnRoot(%v) error = %v", tc.argv, legacyErr)
				}
				generatedLeaf, _, generatedErr := climanifestparity.ParseArgvOnRoot(generatedRoot, tc.argv)
				if generatedErr != nil {
					t.Fatalf("generated ParseArgvOnRoot(%v) error = %v", tc.argv, generatedErr)
				}
				for _, flagLong := range tc.flagChecks {
					mismatches := climanifestparity.CompareConstructorFlagParity(tc.commandID, flagLong, legacyLeaf, generatedLeaf)
					assertNoConstructorMismatches(t, mismatches)
				}
				if tc.checkPreRun {
					mismatches := climanifestparity.CompareConstructorPreRunParity(tc.commandID, legacyLeaf, generatedLeaf, tc.errContains)
					assertNoConstructorMismatches(t, mismatches)
				}
			})
		}
	})
}

func TestGeneratedVsLegacyParityMatrix_ModelsFamily(t *testing.T) {
	modelsFamilyCases := []struct {
		commandID string
		path      string
	}{
		{commandID: "you.models", path: "you models"},
		{commandID: "you.models.list", path: "you models list"},
		{commandID: "you.models.inspect", path: "you models inspect"},
		{commandID: "you.models.invoke", path: "you models invoke"},
		{commandID: "you.models.pull", path: "you models pull"},
	}

	t.Run("identity", func(t *testing.T) {
		legacyRoot, generatedRoot := mustModelsConstructorRoots(t)
		for _, tc := range modelsFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyCmd, err := climanifestparity.FindCommandByPath(legacyRoot, tc.path)
				if err != nil {
					t.Fatalf("legacy FindCommandByPath(%q) error = %v", tc.path, err)
				}
				generatedCmd, err := climanifestparity.FindCommandByPath(generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("generated FindCommandByPath(%q) error = %v", tc.path, err)
				}
				mismatches := climanifestparity.CompareConstructorIdentityParity(tc.commandID, legacyCmd, generatedCmd)
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("help", func(t *testing.T) {
		for _, tc := range modelsFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustModelsConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorHelpParity(tc.commandID, legacyRoot, generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("CompareConstructorHelpParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("completion", func(t *testing.T) {
		for _, tc := range modelsFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustModelsConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorCompletionInventoryParity(tc.commandID, tc.path, legacyRoot, generatedRoot)
				if err != nil {
					t.Fatalf("CompareConstructorCompletionInventoryParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("parsing", func(t *testing.T) {
		for _, tc := range generatedVsLegacyModelsParsingCases() {
			t.Run(tc.name, func(t *testing.T) {
				legacyRoot, generatedRoot := mustModelsConstructorRoots(t)
				mismatches := climanifestparity.CompareConstructorParseParity(
					tc.commandID,
					legacyRoot,
					generatedRoot,
					tc.argv,
					tc.wantParseErr,
					tc.errContains,
				)
				assertNoConstructorMismatches(t, mismatches)

				if tc.wantParseErr {
					return
				}

				legacyLeaf, _, legacyErr := climanifestparity.ParseArgvOnRoot(legacyRoot, tc.argv)
				if legacyErr != nil {
					t.Fatalf("legacy ParseArgvOnRoot(%v) error = %v", tc.argv, legacyErr)
				}
				generatedLeaf, _, generatedErr := climanifestparity.ParseArgvOnRoot(generatedRoot, tc.argv)
				if generatedErr != nil {
					t.Fatalf("generated ParseArgvOnRoot(%v) error = %v", tc.argv, generatedErr)
				}
				for _, flagLong := range tc.flagChecks {
					mismatches := climanifestparity.CompareConstructorFlagParity(tc.commandID, flagLong, legacyLeaf, generatedLeaf)
					assertNoConstructorMismatches(t, mismatches)
				}
				if tc.checkPreRun {
					mismatches := climanifestparity.CompareConstructorPreRunParity(tc.commandID, legacyLeaf, generatedLeaf, tc.errContains)
					assertNoConstructorMismatches(t, mismatches)
				}
			})
		}
	})
}

func TestGeneratedVsLegacyParityMatrix_DocsFamily(t *testing.T) {
	docsFamilyCases := []struct {
		commandID string
		path      string
	}{
		{commandID: "you.docs", path: "you docs"},
	}

	t.Run("identity", func(t *testing.T) {
		legacyRoot, generatedRoot := mustDocsConstructorRoots(t)
		for _, tc := range docsFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyCmd, err := climanifestparity.FindCommandByPath(legacyRoot, tc.path)
				if err != nil {
					t.Fatalf("legacy FindCommandByPath(%q) error = %v", tc.path, err)
				}
				generatedCmd, err := climanifestparity.FindCommandByPath(generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("generated FindCommandByPath(%q) error = %v", tc.path, err)
				}
				mismatches := climanifestparity.CompareConstructorIdentityParity(tc.commandID, legacyCmd, generatedCmd)
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("help", func(t *testing.T) {
		for _, tc := range docsFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustDocsConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorHelpParity(tc.commandID, legacyRoot, generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("CompareConstructorHelpParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("completion", func(t *testing.T) {
		for _, tc := range docsFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustDocsConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorCompletionInventoryParity(tc.commandID, tc.path, legacyRoot, generatedRoot)
				if err != nil {
					t.Fatalf("CompareConstructorCompletionInventoryParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("parsing", func(t *testing.T) {
		for _, tc := range generatedVsLegacyDocsParsingCases() {
			t.Run(tc.name, func(t *testing.T) {
				legacyRoot, generatedRoot := mustDocsConstructorRoots(t)
				mismatches := climanifestparity.CompareConstructorParseParity(
					tc.commandID,
					legacyRoot,
					generatedRoot,
					tc.argv,
					tc.wantParseErr,
					tc.errContains,
				)
				assertNoConstructorMismatches(t, mismatches)

				if tc.wantParseErr {
					return
				}

				legacyLeaf, _, legacyErr := climanifestparity.ParseArgvOnRoot(legacyRoot, tc.argv)
				if legacyErr != nil {
					t.Fatalf("legacy ParseArgvOnRoot(%v) error = %v", tc.argv, legacyErr)
				}
				generatedLeaf, _, generatedErr := climanifestparity.ParseArgvOnRoot(generatedRoot, tc.argv)
				if generatedErr != nil {
					t.Fatalf("generated ParseArgvOnRoot(%v) error = %v", tc.argv, generatedErr)
				}
				for _, flagLong := range tc.flagChecks {
					mismatches := climanifestparity.CompareConstructorFlagParity(tc.commandID, flagLong, legacyLeaf, generatedLeaf)
					assertNoConstructorMismatches(t, mismatches)
				}
			})
		}
	})
}

type generatedVsLegacyParsingCase struct {
	name         string
	commandID    string
	argv         []string
	wantParseErr bool
	errContains  string
	flagChecks   []string
	checkPreRun  bool
}

func generatedVsLegacyParsingCases() []generatedVsLegacyParsingCase {
	return []generatedVsLegacyParsingCase{
		{
			name:      "session show optional positional accepts omission",
			commandID: "you.session.show",
			argv:      []string{"session", "show"},
		},
		{
			name:      "session show optional positional accepts one value",
			commandID: "you.session.show",
			argv:      []string{"session", "show", "session-beta"},
		},
		{
			name:         "session show rejects excess positionals",
			commandID:    "you.session.show",
			argv:         []string{"session", "show", "one", "two"},
			wantParseErr: true,
			errContains:  "accepts at most 1 arg",
		},
		{
			name:       "session show inherited json flag is parseable",
			commandID:  "you.session.show",
			argv:       []string{"--json", "session", "show", "session-beta"},
			flagChecks: []string{"json"},
		},
		{
			name:       "session show inherited server flag keeps contract default until changed",
			commandID:  "you.session.show",
			argv:       []string{"session", "show"},
			flagChecks: []string{"server"},
		},
		{
			name:       "session show inherited server flag accepts explicit value",
			commandID:  "you.session.show",
			argv:       []string{"--server", "http://127.0.0.1:9090", "session", "show"},
			flagChecks: []string{"server"},
		},
		{
			name:       "session show inherited verbose no-option applies contract default",
			commandID:  "you.session.show",
			argv:       []string{"--verbose", "session", "show"},
			flagChecks: []string{"verbose"},
		},
		{
			name:       "session show inherited debug no-option applies contract default",
			commandID:  "you.session.show",
			argv:       []string{"--debug", "session", "show"},
			flagChecks: []string{"debug"},
		},
		{
			name:       "session show local hidden port keeps contract default",
			commandID:  "you.session.show",
			argv:       []string{"session", "show"},
			flagChecks: []string{"port"},
		},
		{
			name:        "session show rejects deprecated port flag",
			commandID:   "you.session.show",
			argv:        []string{"--port", "9090", "session", "show"},
			errContains: "--port is no longer supported",
			checkPreRun: true,
		},
	}
}

func generatedVsLegacyModelsParsingCases() []generatedVsLegacyParsingCase {
	return []generatedVsLegacyParsingCase{
		{
			name:      "models list accepts no model positional",
			commandID: "you.models.list",
			argv:      []string{"models", "list"},
		},
		{
			name:      "models inspect requires one model-name positional",
			commandID: "you.models.inspect",
			argv:      []string{"models", "inspect", "OMNIVOICE_Q4_K_M"},
		},
		{
			name:         "models inspect rejects missing model-name positional",
			commandID:    "you.models.inspect",
			argv:         []string{"models", "inspect"},
			wantParseErr: true,
			errContains:  "accepts 1 arg(s), received 0",
		},
		{
			name:      "models invoke accepts model-name and local flags",
			commandID: "you.models.invoke",
			argv:      []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello", "--output", "speech.wav"},
			flagChecks: []string{
				"operation", "text", "output",
			},
		},
		{
			name:         "models invoke rejects missing model-name positional",
			commandID:    "you.models.invoke",
			argv:         []string{"models", "invoke"},
			wantParseErr: true,
			errContains:  "accepts 1 arg(s), received 0",
		},
		{
			name:      "models pull requires one model-name positional",
			commandID: "you.models.pull",
			argv:      []string{"models", "pull", "OMNIVOICE_Q4_K_M"},
		},
		{
			name:         "models pull rejects missing model-name positional",
			commandID:    "you.models.pull",
			argv:         []string{"models", "pull"},
			wantParseErr: true,
			errContains:  "accepts 1 arg(s), received 0",
		},
		{
			name:       "models list inherited json flag is parseable",
			commandID:  "you.models.list",
			argv:       []string{"--json", "models", "list"},
			flagChecks: []string{"json"},
		},
		{
			name:       "models inspect inherited server flag keeps contract default until changed",
			commandID:  "you.models.inspect",
			argv:       []string{"models", "inspect", "OMNIVOICE_Q4_K_M"},
			flagChecks: []string{"server"},
		},
		{
			name:       "models inspect inherited server flag accepts explicit value",
			commandID:  "you.models.inspect",
			argv:       []string{"--server", "http://127.0.0.1:9090", "models", "inspect", "OMNIVOICE_Q4_K_M"},
			flagChecks: []string{"server"},
		},
		{
			name:       "models list inherited verbose no-option applies contract default",
			commandID:  "you.models.list",
			argv:       []string{"--verbose", "models", "list"},
			flagChecks: []string{"verbose"},
		},
		{
			name:       "models pull inherited debug no-option applies contract default",
			commandID:  "you.models.pull",
			argv:       []string{"--debug", "models", "pull", "OMNIVOICE_Q4_K_M"},
			flagChecks: []string{"debug"},
		},
		{
			name:       "models inspect local hidden port keeps contract default",
			commandID:  "you.models.inspect",
			argv:       []string{"models", "inspect", "OMNIVOICE_Q4_K_M"},
			flagChecks: []string{"port"},
		},
		{
			name:        "models list rejects deprecated port flag",
			commandID:   "you.models.list",
			argv:        []string{"--port", "9090", "models", "list"},
			errContains: "--port is no longer supported",
			checkPreRun: true,
		},
	}
}

func generatedVsLegacyDocsParsingCases() []generatedVsLegacyParsingCase {
	return []generatedVsLegacyParsingCase{
		{
			name:      "docs optional topic accepts omission",
			commandID: "you.docs",
			argv:      []string{"docs"},
		},
		{
			name:      "docs optional topic accepts one value",
			commandID: "you.docs",
			argv:      []string{"docs", "config"},
		},
		{
			name:         "docs rejects excess positionals",
			commandID:    "you.docs",
			argv:         []string{"docs", "config", "extra"},
			wantParseErr: true,
			errContains:  "accepts at most 1 arg",
		},
		{
			name:       "docs inherited verbose no-option applies contract default",
			commandID:  "you.docs",
			argv:       []string{"--verbose", "docs", "config"},
			flagChecks: []string{"verbose"},
		},
		{
			name:       "docs inherited server flag accepts explicit value",
			commandID:  "you.docs",
			argv:       []string{"--server", "http://127.0.0.1:9090", "docs", "config"},
			flagChecks: []string{"server"},
		},
	}
}

func mustDocsConstructorRoots(t *testing.T) (*cobra.Command, *cobra.Command) {
	t.Helper()
	legacyRoot := cli.NewLegacyDocsFamilyCommand()
	generatedRoot, err := cli.NewGeneratedDocsFamilyParityCommand(mustDocsParityRegistry(t), modelsParityInvokeFlagBindings())
	if err != nil {
		t.Fatalf("NewGeneratedDocsFamilyParityCommand() error = %v", err)
	}
	return legacyRoot, generatedRoot
}

func mustModelsConstructorRoots(t *testing.T) (*cobra.Command, *cobra.Command) {
	t.Helper()
	legacyRoot := cli.NewLegacyModelsFamilyCommand()
	generatedRoot, err := cli.NewGeneratedModelsFamilyParityCommand(
		mustModelsParityRegistry(t),
		modelsParityInvokeFlagBindings(),
	)
	if err != nil {
		t.Fatalf("NewGeneratedModelsFamilyParityCommand() error = %v", err)
	}
	return legacyRoot, generatedRoot
}

func mustModelsParityRegistry(t *testing.T) *commandregistry.Registry {
	t.Helper()
	registry, err := commandregistry.NewModelsDocsRegistry(commandregistry.ModelsDocsHandlers{
		DocsRunE:          parityNoopRunE,
		ModelsListRunE:    parityNoopRunE,
		ModelsInspectRunE: parityNoopRunE,
		ModelsInvokeRunE:  parityNoopRunE,
		ModelsPullRunE:    parityNoopRunE,
	})
	if err != nil {
		t.Fatalf("NewModelsDocsRegistry() error = %v", err)
	}
	return registry
}

func modelsParityInvokeFlagBindings() climanifestcobra.ModelsInvokeFlagBindings {
	operation := "TTS"
	text := ""
	output := ""
	return climanifestcobra.ModelsInvokeFlagBindings{
		Operation:  &operation,
		Text:       &text,
		OutputPath: &output,
		FlagUsages: map[string]string{
			"operation": "uppercase provider-agnostic operation name",
			"text":      "text input for direct invocation",
			"output":    "output file path for streamed audio responses",
		},
	}
}

func mustRepresentativeConstructorRoots(t *testing.T) (*cobra.Command, *cobra.Command) {
	t.Helper()
	legacyRoot := cli.NewLegacyRepresentativeFamilyCommand()
	generatedRoot, err := climanifestcobra.NewRepresentativeFamilyCommand(
		mustRepresentativeParityRegistry(t),
		representativeParityBindings(),
	)
	if err != nil {
		t.Fatalf("NewRepresentativeFamilyCommand() error = %v", err)
	}
	return legacyRoot, generatedRoot
}

func mustRepresentativeParityRegistry(t *testing.T) *commandregistry.Registry {
	t.Helper()
	registry, err := commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE:        parityNoopRunE,
		SessionShowRunE: parityNoopRunE,
	})
	if err != nil {
		t.Fatalf("NewRepresentativeRegistry() error = %v", err)
	}
	return registry
}

func representativeParityBindings() climanifestcobra.PersistentFlagBindings {
	var verbose bool
	var debug bool
	server := "http://localhost:7437"
	var json bool
	defaultWorkerModelProvider := ""
	defaultWorkerModel := ""
	return climanifestcobra.PersistentFlagBindings{
		Verbose:                    &verbose,
		Debug:                      &debug,
		Server:                     &server,
		JSON:                       &json,
		DefaultWorkerModelProvider: &defaultWorkerModelProvider,
		DefaultWorkerModel:         &defaultWorkerModel,
		FlagUsages: map[string]string{
			"verbose": "emit concise command diagnostics to stderr",
			"debug":   "emit lower-level command diagnostics where supported (implies --verbose)",
			"server":  "factory API base URI (http:// or https://); HTTP client commands target this URI and you run binds locally to its host and port",
			"json":    "emit structured JSON on stdout for supported commands; diagnostics remain on stderr",
			"default-worker-model-provider": fmt.Sprintf(
				"default worker model provider for model workers with omitted modelProvider (%s; DEFAULT resolves through lower-precedence concrete provider)",
				interfaces.AcceptedPublicWorkerModelProviderSummary(),
			),
			"default-worker-model": "default worker model for model workers with omitted model",
		},
	}
}

func parityNoopRunE(cmd *cobra.Command, args []string) error {
	return nil
}

func assertNoConstructorMismatches(t *testing.T, mismatches []climanifestparity.Mismatch) {
	t.Helper()
	if len(mismatches) == 0 {
		return
	}
	report := climanifestparity.FormatMismatchReport(mismatches)
	if !strings.Contains(report, "mismatch") {
		t.Fatalf("generated vs legacy parity drift detected:\n%s", report)
	}
	t.Fatalf("generated vs legacy parity drift detected:\n%s", report)
}

func TestGeneratedVsLegacyParityMatrix_FactoryConfigInitFamily(t *testing.T) {
	familyCases := factoryConfigInitFamilyParityCases(t)

	t.Run("identity", func(t *testing.T) {
		legacyRoot, generatedRoot := mustFactoryConfigInitConstructorRoots(t)
		for _, tc := range familyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyCmd, err := climanifestparity.FindCommandByPath(legacyRoot, tc.path)
				if err != nil {
					t.Fatalf("legacy FindCommandByPath(%q) error = %v", tc.path, err)
				}
				generatedCmd, err := climanifestparity.FindCommandByPath(generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("generated FindCommandByPath(%q) error = %v", tc.path, err)
				}
				mismatches := climanifestparity.CompareConstructorIdentityParity(tc.commandID, legacyCmd, generatedCmd)
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("help", func(t *testing.T) {
		for _, tc := range familyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustFactoryConfigInitConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorHelpParity(tc.commandID, legacyRoot, generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("CompareConstructorHelpParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("completion", func(t *testing.T) {
		for _, tc := range familyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustFactoryConfigInitConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorCompletionInventoryParity(tc.commandID, tc.path, legacyRoot, generatedRoot)
				if err != nil {
					t.Fatalf("CompareConstructorCompletionInventoryParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("parsing", func(t *testing.T) {
		for _, tc := range factoryConfigInitGeneratedVsLegacyParsingCases() {
			t.Run(tc.name, func(t *testing.T) {
				legacyRoot, generatedRoot := mustFactoryConfigInitConstructorRoots(t)
				mismatches := climanifestparity.CompareConstructorParseParity(
					tc.commandID,
					legacyRoot,
					generatedRoot,
					tc.argv,
					tc.wantParseErr,
					tc.errContains,
				)
				assertNoConstructorMismatches(t, mismatches)

				if tc.wantParseErr {
					return
				}

				legacyLeaf, _, legacyErr := climanifestparity.ParseArgvOnRoot(legacyRoot, tc.argv)
				if legacyErr != nil {
					t.Fatalf("legacy ParseArgvOnRoot(%v) error = %v", tc.argv, legacyErr)
				}
				generatedLeaf, _, generatedErr := climanifestparity.ParseArgvOnRoot(generatedRoot, tc.argv)
				if generatedErr != nil {
					t.Fatalf("generated ParseArgvOnRoot(%v) error = %v", tc.argv, generatedErr)
				}
				for _, flagLong := range tc.flagChecks {
					mismatches := climanifestparity.CompareConstructorFlagParity(tc.commandID, flagLong, legacyLeaf, generatedLeaf)
					assertNoConstructorMismatches(t, mismatches)
				}
				if tc.checkPreRun {
					mismatches := climanifestparity.CompareConstructorPreRunParity(tc.commandID, legacyLeaf, generatedLeaf, tc.errContains)
					assertNoConstructorMismatches(t, mismatches)
				}
			})
		}
	})
}

type factoryConfigInitFamilyParityCase struct {
	commandID string
	path      string
}

func factoryConfigInitFamilyParityCases(t *testing.T) []factoryConfigInitFamilyParityCase {
	t.Helper()
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		t.Fatalf("FactoryConfigInitFamilyManifest() error = %v", err)
	}
	cases := make([]factoryConfigInitFamilyParityCase, 0, len(climanifestgen.FactoryConfigInitFamilyCommandIDs))
	for _, commandID := range climanifestgen.FactoryConfigInitFamilyCommandIDs {
		record, lookupErr := manifest.CommandByID(commandID)
		if lookupErr != nil {
			t.Fatalf("CommandByID(%q) error = %v", commandID, lookupErr)
		}
		cases = append(cases, factoryConfigInitFamilyParityCase{
			commandID: commandID,
			path:      record.Path,
		})
	}
	return cases
}

type factoryConfigInitGeneratedVsLegacyParsingCase struct {
	name         string
	commandID    string
	argv         []string
	wantParseErr bool
	errContains  string
	flagChecks   []string
	checkPreRun  bool
}

func factoryConfigInitGeneratedVsLegacyParsingCases() []factoryConfigInitGeneratedVsLegacyParsingCase {
	cases := make([]factoryConfigInitGeneratedVsLegacyParsingCase, 0)
	cases = append(cases, factoryConfigInitGeneratedVsLegacyParsingCasesFactory()...)
	cases = append(cases, factoryConfigInitGeneratedVsLegacyParsingCasesConfigInit()...)
	return cases
}

func factoryConfigInitGeneratedVsLegacyParsingCasesFactory() []factoryConfigInitGeneratedVsLegacyParsingCase {
	return []factoryConfigInitGeneratedVsLegacyParsingCase{
		{
			name:      "factory query optional positional accepts omission",
			commandID: "you.factory.query",
			argv:      []string{"factory", "query"},
		},
		{
			name:       "factory query inherited json flag is parseable",
			commandID:  "you.factory.query",
			argv:       []string{"--json", "factory", "query"},
			flagChecks: []string{"json"},
		},
		{
			name:       "factory query inherited server flag keeps contract default until changed",
			commandID:  "you.factory.query",
			argv:       []string{"factory", "query"},
			flagChecks: []string{"server"},
		},
		{
			name:       "factory query inherited server flag accepts explicit value",
			commandID:  "you.factory.query",
			argv:       []string{"--server", "http://127.0.0.1:9090", "factory", "query"},
			flagChecks: []string{"server"},
		},
		{
			name:       "factory query inherited verbose no-option applies contract default",
			commandID:  "you.factory.query",
			argv:       []string{"--verbose", "factory", "query"},
			flagChecks: []string{"verbose"},
		},
		{
			name:       "factory query inherited debug no-option applies contract default",
			commandID:  "you.factory.query",
			argv:       []string{"--debug", "factory", "query"},
			flagChecks: []string{"debug"},
		},
		{
			name:       "factory query local hidden port keeps contract default",
			commandID:  "you.factory.query",
			argv:       []string{"factory", "query"},
			flagChecks: []string{"port"},
		},
		{
			name:        "factory query rejects deprecated port flag",
			commandID:   "you.factory.query",
			argv:        []string{"factory", "query", "--port", "9090"},
			errContains: "--port is no longer supported",
			checkPreRun: true,
		},
		{
			name:         "factory create rejects missing required from flag",
			commandID:    "you.factory.create",
			argv:         []string{"factory", "create", "staging"},
			wantParseErr: true,
			errContains:  "required flag(s)",
		},
	}
}

func factoryConfigInitGeneratedVsLegacyParsingCasesConfigInit() []factoryConfigInitGeneratedVsLegacyParsingCase {
	return []factoryConfigInitGeneratedVsLegacyParsingCase{
		{
			name:      "config init optional positional accepts omission",
			commandID: "you.config.init",
			argv:      []string{"config", "init"},
		},
		{
			name:       "config init inherited json flag is parseable",
			commandID:  "you.config.init",
			argv:       []string{"--json", "config", "init"},
			flagChecks: []string{"json"},
		},
		{
			name:       "config init inherited server flag keeps contract default until changed",
			commandID:  "you.config.init",
			argv:       []string{"config", "init"},
			flagChecks: []string{"server"},
		},
		{
			name:      "init optional positional accepts omission",
			commandID: "you.init",
			argv:      []string{"init"},
		},
		{
			name:       "init inherited json flag is parseable",
			commandID:  "you.init",
			argv:       []string{"--json", "init"},
			flagChecks: []string{"json"},
		},
		{
			name:       "init local dir flag keeps contract default",
			commandID:  "you.init",
			argv:       []string{"init"},
			flagChecks: []string{"dir"},
		},
		{
			name:       "init local type flag keeps contract default",
			commandID:  "you.init",
			argv:       []string{"init"},
			flagChecks: []string{"type"},
		},
		{
			name:       "init local executor flag keeps contract default",
			commandID:  "you.init",
			argv:       []string{"init"},
			flagChecks: []string{"executor"},
		},
		{
			name:       "init local dir flag accepts explicit value",
			commandID:  "you.init",
			argv:       []string{"init", "--dir", "my-factory"},
			flagChecks: []string{"dir"},
		},
	}
}

func mustFactoryConfigInitConstructorRoots(t *testing.T) (*cobra.Command, *cobra.Command) {
	t.Helper()
	legacyRoot := cli.NewLegacyFactoryConfigInitFamilyCommand()
	generatedRoot, err := cli.NewGeneratedFactoryConfigInitFamilyCommandForParity(
		mustFactoryConfigInitParityRegistry(t),
		factoryConfigInitParityBindings(),
	)
	if err != nil {
		t.Fatalf("NewGeneratedFactoryConfigInitFamilyCommandForParity() error = %v", err)
	}
	return legacyRoot, generatedRoot
}

func mustFactoryConfigInitParityRegistry(t *testing.T) *commandregistry.Registry {
	t.Helper()
	registry, err := commandregistry.NewFactoryConfigInitRegistry(factoryConfigInitParityNoopHandlers())
	if err != nil {
		t.Fatalf("NewFactoryConfigInitRegistry() error = %v", err)
	}
	return registry
}

func factoryConfigInitParityBindings() climanifestcobra.FactoryConfigInitFlagBindings {
	listDir := defaultcmd.FactoryDir
	createDir := defaultcmd.FactoryDir
	updateDir := defaultcmd.FactoryDir
	deleteDir := defaultcmd.FactoryDir
	createFrom := ""
	createSetCurrent := false
	updateFrom := ""
	replaceSessionID := ""
	initDir := defaultcmd.FactoryDir
	initType := string(initcmd.DefaultScaffoldType)
	initExecutor := initcmd.DefaultStarterExecutor
	return climanifestcobra.FactoryConfigInitFlagBindings{
		FactoryListDir:          &listDir,
		FactoryCreateDir:        &createDir,
		FactoryUpdateDir:        &updateDir,
		FactoryDeleteDir:        &deleteDir,
		FactoryCreateFrom:       &createFrom,
		FactoryCreateSetCurrent: &createSetCurrent,
		FactoryUpdateFrom:       &updateFrom,
		FactoryReplaceSessionID: &replaceSessionID,
		InitDir:                 &initDir,
		InitType:                &initType,
		InitExecutor:            &initExecutor,
		FlagUsages: map[string]string{
			"dir":         "factory root directory containing named factories",
			"from":        "path to an existing factory.json payload (required)",
			"set-current": "update .current-factory to the created name",
			"session":     "target one live factory session; omit to use the default compatibility session",
			"type":        "scaffold type to generate (supported: default, ralph)",
			"executor": fmt.Sprintf(
				"starter scaffold to generate (%s)",
				strings.Join(initcmd.SupportedStarterExecutors(), ", "),
			),
		},
	}
}

func factoryConfigInitParityNoopHandlers() commandregistry.FactoryConfigInitHandlers {
	return commandregistry.FactoryConfigInitHandlers{
		FactoryQueryRunE:          parityNoopRunE,
		FactoryListRunE:           parityNoopRunE,
		FactoryCreateRunE:         parityNoopRunE,
		FactoryUpdateRunE:         parityNoopRunE,
		FactoryDeleteRunE:         parityNoopRunE,
		FactoryReplaceCurrentRunE: parityNoopRunE,
		FactoryConfigValidateRunE: parityNoopRunE,
		FactoryConfigFlattenRunE:  parityNoopRunE,
		FactoryConfigExpandRunE:   parityNoopRunE,
		ConfigInitRunE:            parityNoopRunE,
		InitRunE:                  parityNoopRunE,
	}
}
