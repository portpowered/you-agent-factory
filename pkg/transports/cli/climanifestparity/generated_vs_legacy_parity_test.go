package climanifestparity_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
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
