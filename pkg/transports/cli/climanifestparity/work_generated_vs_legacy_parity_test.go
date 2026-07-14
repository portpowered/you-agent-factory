package climanifestparity_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
)

func TestGeneratedVsLegacyParityMatrix_WorkFamily(t *testing.T) {
	workFamilyCases := []struct {
		commandID string
		path      string
	}{
		{commandID: "you.work", path: "you work"},
		{commandID: "you.work.list", path: "you work list"},
		{commandID: "you.work.show", path: "you work show"},
		{commandID: "you.work.move", path: "you work move"},
		{commandID: "you.work.visualize", path: "you work visualize"},
	}

	t.Run("identity", func(t *testing.T) {
		legacyRoot, generatedRoot := mustWorkConstructorRoots(t)
		for _, tc := range workFamilyCases {
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
		for _, tc := range workFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustWorkConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorHelpParity(tc.commandID, legacyRoot, generatedRoot, tc.path)
				if err != nil {
					t.Fatalf("CompareConstructorHelpParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("completion", func(t *testing.T) {
		for _, tc := range workFamilyCases {
			t.Run(tc.commandID, func(t *testing.T) {
				legacyRoot, generatedRoot := mustWorkConstructorRoots(t)
				mismatches, err := climanifestparity.CompareConstructorCompletionInventoryParity(tc.commandID, tc.path, legacyRoot, generatedRoot)
				if err != nil {
					t.Fatalf("CompareConstructorCompletionInventoryParity(%q) error = %v", tc.path, err)
				}
				assertNoConstructorMismatches(t, mismatches)
			})
		}
	})

	t.Run("parsing", func(t *testing.T) {
		for _, tc := range workGeneratedVsLegacyParsingCases() {
			t.Run(tc.name, func(t *testing.T) {
				legacyRoot, generatedRoot := mustWorkConstructorRoots(t)
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

type workGeneratedVsLegacyParsingCase struct {
	name         string
	commandID    string
	argv         []string
	wantParseErr bool
	errContains  string
	flagChecks   []string
	checkPreRun  bool
}

func workGeneratedVsLegacyParsingCases() []workGeneratedVsLegacyParsingCase {
	return []workGeneratedVsLegacyParsingCase{
		{
			name:      "work list accepts filter and pagination flags",
			commandID: "you.work.list",
			argv:      []string{"work", "list", "--session", "session-beta", "--max-results", "10", "--name", "prd"},
			flagChecks: []string{
				"session", "max-results", "name",
			},
		},
		{
			name:       "work list inherited json flag is parseable",
			commandID:  "you.work.list",
			argv:       []string{"--json", "work", "list"},
			flagChecks: []string{"json"},
		},
		{
			name:       "work list inherited server flag accepts explicit value",
			commandID:  "you.work.list",
			argv:       []string{"--server", "http://127.0.0.1:9090", "work", "list"},
			flagChecks: []string{"server"},
		},
		{
			name:        "work list rejects deprecated port flag",
			commandID:   "you.work.list",
			argv:        []string{"--port", "9090", "work", "list"},
			errContains: "--port is no longer supported",
			checkPreRun: true,
		},
		{
			name:      "work show required positional accepts work id",
			commandID: "you.work.show",
			argv:      []string{"work", "show", "work-review-1"},
		},
		{
			name:         "work show rejects missing required positional",
			commandID:    "you.work.show",
			argv:         []string{"work", "show"},
			wantParseErr: true,
			errContains:  "accepts 1 arg",
		},
		{
			name:       "work show inherited json flag is parseable",
			commandID:  "you.work.show",
			argv:       []string{"--json", "work", "show", "work-review-1"},
			flagChecks: []string{"json"},
		},
		{
			name:      "work move required positionals accept work id and state",
			commandID: "you.work.move",
			argv:      []string{"work", "move", "work-move-1", "complete"},
		},
		{
			name:         "work move rejects missing state positional",
			commandID:    "you.work.move",
			argv:         []string{"work", "move", "work-move-1"},
			wantParseErr: true,
			errContains:  "accepts 2 arg",
		},
		{
			name:       "work move local request-id flag is parseable",
			commandID:  "you.work.move",
			argv:       []string{"work", "move", "work-move-1", "complete", "--request-id", "req-move-1"},
			flagChecks: []string{"request-id"},
		},
		{
			name:      "work visualize required batch path accepts one value",
			commandID: "you.work.visualize",
			argv:      []string{"work", "visualize", "batch.json"},
		},
		{
			name:         "work visualize rejects missing batch path",
			commandID:    "you.work.visualize",
			argv:         []string{"work", "visualize"},
			wantParseErr: true,
			errContains:  "accepts 1 arg",
		},
		{
			name:       "work visualize format keeps contract default",
			commandID:  "you.work.visualize",
			argv:       []string{"work", "visualize", "batch.json"},
			flagChecks: []string{"format"},
		},
		{
			name:       "work visualize format accepts markdown-mermaid",
			commandID:  "you.work.visualize",
			argv:       []string{"work", "visualize", "--format", "markdown-mermaid", "batch.json"},
			flagChecks: []string{"format"},
		},
	}
}

func mustWorkConstructorRoots(t *testing.T) (*cobra.Command, *cobra.Command) {
	t.Helper()
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      parityNoopRunE,
		ShowRunE:      parityNoopRunE,
		MoveRunE:      parityNoopRunE,
		VisualizeRunE: parityNoopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	legacyRoot, generatedRoot, err := cli.NewWorkFamilyParityRoots(registry, workParityBindings())
	if err != nil {
		t.Fatalf("NewWorkFamilyParityRoots() error = %v", err)
	}
	return legacyRoot, generatedRoot
}

func workParityBindings() climanifestcobra.WorkFamilyBindings {
	listCfg := workcli.ListConfig{}
	showCfg := workcli.ShowConfig{}
	moveCfg := workcli.MoveConfig{}
	format := "mermaid"
	return climanifestcobra.WorkFamilyBindings{
		ListConfig:      &listCfg,
		ShowConfig:      &showCfg,
		MoveConfig:      &moveCfg,
		VisualizeFormat: &format,
		FlagUsages:      workFamilyParityFlagUsages(),
	}
}

func workFamilyParityFlagUsages() map[string]string {
	// Mirror handwritten work flag help from root_work.go for generated-vs-legacy help parity.
	return map[string]string{
		"state-name":     "filter by current state name",
		"state-type":     "filter by current state type (INITIAL, PROCESSING, TERMINAL, FAILED)",
		"name":           "filter by case-insensitive substring of work name (applied before pagination)",
		"work-type-name": "filter by exact workTypeName (applied before pagination)",
		"trace-id":       "filter by exact traceId or currentChainingTraceId (applied before pagination)",
		"sort-by":        "sort returned work by field (state.type)",
		"max-results":    "maximum work items to return per page after server-side filters",
		"next-token":     "pagination cursor returned by a previous work list response",
		"session":        "target one live factory session; omit to use the default compatibility session",
		"request-id":     "optional client idempotency key for operator moves",
		"format":         "output format: mermaid or markdown-mermaid",
	}
}
