package climanifestparity_test

import (
	"fmt"
	"strings"
	"testing"

	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	"github.com/spf13/cobra"
)

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
