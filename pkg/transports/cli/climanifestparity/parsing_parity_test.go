package climanifestparity_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/spf13/cobra"
)

type parsingParityCase struct {
	name         string
	commandID    string
	argv         []string
	wantParseErr bool
	errContains  string
	verify       func(t *testing.T, manifest climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, positionals []string)
}

// pkgmaintcheck:ignore-cyclomatic-complexity table-driven manifest parsing parity keeps contract lookup, parse, and per-case verify hooks in one harness.
func TestProductionManifestParsingParity_RootAndSessionShow(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		t.Fatalf("CommandByID(you) error = %v", err)
	}
	sessionShowRecord, err := manifest.CommandByID("you.session.show")
	if err != nil {
		t.Fatalf("CommandByID(you.session.show) error = %v", err)
	}

	runProductionManifestParsingParityCases(t, manifest, productionManifestParsingParityCases(rootRecord, sessionShowRecord))
}

func runProductionManifestParsingParityCases(t *testing.T, manifest climanifest.Manifest, cases []parsingParityCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertProductionManifestParsingCase(t, manifest, tc)
		})
	}
}

func assertProductionManifestParsingCase(t *testing.T, manifest climanifest.Manifest, tc parsingParityCase) {
	t.Helper()
	record, err := manifest.CommandByID(tc.commandID)
	if err != nil {
		t.Fatalf("CommandByID(%q) error = %v", tc.commandID, err)
	}

	leaf, positionals, parseErr := cli.ParseArgvForCLIInputsInventory(tc.argv)
	if tc.wantParseErr {
		if parseErr == nil {
			t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = nil, want parse failure", tc.argv)
		}
		if tc.errContains != "" && !strings.Contains(parseErr.Error(), tc.errContains) {
			t.Fatalf("parse error = %q, want substring %q", parseErr.Error(), tc.errContains)
		}
		if tc.verify != nil {
			tc.verify(t, manifest, record, leaf, positionals)
		}
		return
	}
	if parseErr != nil {
		t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = %v", tc.argv, parseErr)
	}
	if mismatch := climanifestparity.AssertLeafCommandPath(record.ID, record.Path, leaf); mismatch != nil {
		t.Fatal(mismatch.Error())
	}
	if tc.verify != nil {
		tc.verify(t, manifest, record, leaf, positionals)
	}
}

func TestProductionManifestParsingParity_ModelsFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		t.Fatalf("CommandByID(you) error = %v", err)
	}
	modelsRecord, err := manifest.CommandByID("you.models")
	if err != nil {
		t.Fatalf("CommandByID(you.models) error = %v", err)
	}
	listRecord, err := manifest.CommandByID("you.models.list")
	if err != nil {
		t.Fatalf("CommandByID(you.models.list) error = %v", err)
	}
	inspectRecord, err := manifest.CommandByID("you.models.inspect")
	if err != nil {
		t.Fatalf("CommandByID(you.models.inspect) error = %v", err)
	}
	invokeRecord, err := manifest.CommandByID("you.models.invoke")
	if err != nil {
		t.Fatalf("CommandByID(you.models.invoke) error = %v", err)
	}
	pullRecord, err := manifest.CommandByID("you.models.pull")
	if err != nil {
		t.Fatalf("CommandByID(you.models.pull) error = %v", err)
	}

	cases := productionManifestModelsParsingParityCases(rootRecord, modelsRecord, listRecord, inspectRecord, invokeRecord, pullRecord)
	runProductionManifestParsingParityCases(t, manifest, cases)
}

func productionManifestModelsParsingParityCases(
	rootRecord, modelsRecord, listRecord, inspectRecord, invokeRecord, pullRecord climanifest.Command,
) []parsingParityCase {
	cases := make([]parsingParityCase, 0)
	cases = append(cases, modelsListParsingCases(listRecord)...)
	cases = append(cases, modelsRequiredModelNameCases(inspectRecord, "inspect")...)
	cases = append(cases, modelsRequiredModelNameCases(invokeRecord, "invoke")...)
	cases = append(cases, modelsRequiredModelNameCases(pullRecord, "pull")...)
	cases = append(cases, modelsInvokeLocalFlagCases(invokeRecord)...)
	cases = append(cases, modelsListInheritedFlagCases(listRecord)...)
	cases = append(cases, modelsLeafInheritedFlagCases(inspectRecord, "inspect")...)
	cases = append(cases, modelsLocalPortCase(inspectRecord)...)
	cases = append(cases, modelsInheritedDefaultsCase(rootRecord, listRecord)...)
	_ = modelsRecord
	return cases
}

func modelsListParsingCases(listRecord climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "models list accepts no model positional",
			commandID: listRecord.ID,
			argv:      []string{"models", "list"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, positionals []string) {
				t.Helper()
				if len(record.Arguments) != 0 {
					t.Fatalf("contract arguments = %d, want none for list", len(record.Arguments))
				}
				if len(positionals) != 0 {
					t.Fatalf("positionals = %v, want empty", positionals)
				}
			},
		},
	}
}

func modelsRequiredModelNameCases(record climanifest.Command, leafName string) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      leafName + " requires one model-name positional",
			commandID: record.ID,
			argv:      []string{"models", leafName, "OMNIVOICE_Q4_K_M"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg, err := record.RequireArgumentAt(0)
				if err != nil {
					t.Fatal(err)
				}
				if !arg.Required || arg.Name != "model-name" || arg.MaxCardinality != 1 {
					t.Fatalf("contract arg = %+v, want required single model-name positional", arg)
				}
				if mismatch := climanifestparity.CompareArgumentCardinality(record.ID, arg, positionals); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
				if len(positionals) != 1 || positionals[0] != "OMNIVOICE_Q4_K_M" {
					t.Fatalf("positionals = %v, want [OMNIVOICE_Q4_K_M]", positionals)
				}
			},
		},
		{
			name:         leafName + " rejects missing model-name positional",
			commandID:    record.ID,
			argv:         []string{"models", leafName},
			wantParseErr: true,
			errContains:  "accepts 1 arg(s), received 0",
		},
	}
}

func modelsInvokeLocalFlagCases(invokeRecord climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "models invoke local flags match contract defaults and parsed values",
			commandID: invokeRecord.ID,
			argv:      []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello", "--output", "speech.wav"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				for _, flagLong := range []string{"operation", "text", "output"} {
					contract, err := record.RequireFlagByLong(flagLong)
					if err != nil {
						t.Fatal(err)
					}
					if contract.Scope != "local" {
						t.Fatalf("contract %s scope = %q, want local", flagLong, contract.Scope)
					}
				}
				operation, err := record.RequireFlagByLong("operation")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, operation, climanifestparity.LiveFlag(leaf, "operation"), true, "TTS"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
				text, err := record.RequireFlagByLong("text")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, text, climanifestparity.LiveFlag(leaf, "text"), true, "hello"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
				output, err := record.RequireFlagByLong("output")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, output, climanifestparity.LiveFlag(leaf, "output"), true, "speech.wav"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func modelsListInheritedFlagCases(record climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "list inherited json flag is parseable",
			commandID: record.ID,
			argv:      []string{"--json", "models", "list"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("json")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "json"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      "list inherited server flag keeps contract default until changed",
			commandID: record.ID,
			argv:      []string{"models", "list"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("server")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagDefault(record.ID, contract, climanifestparity.LiveFlag(leaf, "server")); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      "list inherited verbose no-option applies contract default",
			commandID: record.ID,
			argv:      []string{"--verbose", "models", "list"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("verbose")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "verbose"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func modelsLeafInheritedFlagCases(record climanifest.Command, leafName string) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      leafName + " inherited json flag is parseable",
			commandID: record.ID,
			argv:      []string{"--json", "models", leafName, "OMNIVOICE_Q4_K_M"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("json")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "json"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      leafName + " inherited server flag accepts explicit value",
			commandID: record.ID,
			argv:      []string{"--server", "http://127.0.0.1:9090", "models", leafName, "OMNIVOICE_Q4_K_M"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("server")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "server"), true, "http://127.0.0.1:9090"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      leafName + " inherited verbose no-option applies contract default",
			commandID: record.ID,
			argv:      []string{"--verbose", "models", leafName, "OMNIVOICE_Q4_K_M"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("verbose")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "verbose"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      leafName + " inherited debug no-option applies contract default",
			commandID: record.ID,
			argv:      []string{"--debug", "models", leafName, "OMNIVOICE_Q4_K_M"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("debug")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "debug"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func modelsLocalPortCase(record climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "models inspect local hidden port keeps contract default",
			commandID: record.ID,
			argv:      []string{"models", "inspect", "OMNIVOICE_Q4_K_M"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("port")
				if err != nil {
					t.Fatal(err)
				}
				if contract.Scope != "local" || contract.Visibility != "hidden" || contract.Default != "0" {
					t.Fatalf("contract port = %+v, want local hidden default 0", contract)
				}
				if mismatch := climanifestparity.CompareFlagDefault(record.ID, contract, climanifestparity.LiveFlag(leaf, "port")); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func modelsInheritedDefaultsCase(rootRecord, leafRecord climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "models list inherited flags match root persistent defaults",
			commandID: leafRecord.ID,
			argv:      []string{"models", "list"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				mismatches := climanifestparity.CompareInheritedFlagDefaultsAgainstRoot(rootRecord, record, leaf)
				if len(mismatches) == 0 {
					return
				}
				t.Fatalf("contract vs live inherited flag default drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
			},
		},
	}
}

func productionManifestParsingParityCases(rootRecord, sessionShow climanifest.Command) []parsingParityCase {
	cases := make([]parsingParityCase, 0)
	cases = append(cases, sessionShowPositionalCases(sessionShow)...)
	cases = append(cases, sessionShowInheritedFlagCases(sessionShow)...)
	cases = append(cases, sessionShowLocalPortCase(sessionShow)...)
	cases = append(cases, sessionShowInheritedDefaultsCase(rootRecord, sessionShow)...)
	return cases
}

func sessionShowPositionalCases(sessionShow climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "session show optional positional accepts omission",
			commandID: sessionShow.ID,
			argv:      []string{"session", "show"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg, err := record.RequireArgumentAt(0)
				if err != nil {
					t.Fatal(err)
				}
				if arg.Required || arg.MaxCardinality != 1 || arg.Variadic || arg.Name != "session-id" {
					t.Fatalf("contract arg = %+v, want optional single session-id positional", arg)
				}
				if mismatch := climanifestparity.CompareArgumentCardinality(record.ID, arg, positionals); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
				if len(positionals) != 0 {
					t.Fatalf("positionals = %v, want empty", positionals)
				}
			},
		},
		{
			name:      "session show optional positional accepts one value",
			commandID: sessionShow.ID,
			argv:      []string{"session", "show", "session-beta"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg, err := record.RequireArgumentAt(0)
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareArgumentCardinality(record.ID, arg, positionals); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
				if len(positionals) != 1 || positionals[0] != "session-beta" {
					t.Fatalf("positionals = %v, want [session-beta]", positionals)
				}
			},
		},
		{
			name:         "session show rejects excess positionals",
			commandID:    sessionShow.ID,
			argv:         []string{"session", "show", "one", "two"},
			wantParseErr: true,
			errContains:  "accepts at most 1 arg",
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, _ []string) {
				t.Helper()
				arg, err := record.RequireArgumentAt(0)
				if err != nil {
					t.Fatal(err)
				}
				if arg.MaxCardinality != 1 {
					t.Fatalf("contract maxCardinality = %d, want 1", arg.MaxCardinality)
				}
			},
		},
	}
}

func sessionShowInheritedFlagCases(sessionShow climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "session show inherited json flag is parseable",
			commandID: sessionShow.ID,
			argv:      []string{"--json", "session", "show", "session-beta"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("json")
				if err != nil {
					t.Fatal(err)
				}
				if contract.Scope != "inherited" {
					t.Fatalf("contract json scope = %q, want inherited", contract.Scope)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "json"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      "session show inherited server flag keeps contract default until changed",
			commandID: sessionShow.ID,
			argv:      []string{"session", "show"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("server")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagDefault(record.ID, contract, climanifestparity.LiveFlag(leaf, "server")); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      "session show inherited server flag accepts explicit value",
			commandID: sessionShow.ID,
			argv:      []string{"--server", "http://127.0.0.1:9090", "session", "show"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("server")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "server"), true, "http://127.0.0.1:9090"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      "session show inherited verbose no-option applies contract default",
			commandID: sessionShow.ID,
			argv:      []string{"--verbose", "session", "show"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("verbose")
				if err != nil {
					t.Fatal(err)
				}
				if contract.NoOptionDefault != "true" {
					t.Fatalf("contract verbose noOptionDefault = %q, want true", contract.NoOptionDefault)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "verbose"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      "session show inherited debug no-option applies contract default",
			commandID: sessionShow.ID,
			argv:      []string{"--debug", "session", "show"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("debug")
				if err != nil {
					t.Fatal(err)
				}
				if contract.NoOptionDefault != "true" {
					t.Fatalf("contract debug noOptionDefault = %q, want true", contract.NoOptionDefault)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "debug"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func sessionShowLocalPortCase(sessionShow climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "session show local hidden port keeps contract default",
			commandID: sessionShow.ID,
			argv:      []string{"session", "show"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("port")
				if err != nil {
					t.Fatal(err)
				}
				if contract.Scope != "local" || contract.Visibility != "hidden" || contract.Default != "0" {
					t.Fatalf("contract port = %+v, want local hidden default 0", contract)
				}
				if mismatch := climanifestparity.CompareFlagDefault(record.ID, contract, climanifestparity.LiveFlag(leaf, "port")); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func sessionShowInheritedDefaultsCase(rootRecord, sessionShow climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "session show inherited flags match root persistent defaults",
			commandID: sessionShow.ID,
			argv:      []string{"session", "show"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				mismatches := climanifestparity.CompareInheritedFlagDefaultsAgainstRoot(rootRecord, record, leaf)
				if len(mismatches) == 0 {
					return
				}
				t.Fatalf("contract vs live inherited flag default drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
			},
		},
	}
}

func TestProductionManifestParsingParity_DocsFamily(t *testing.T) {
	manifestPath := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(manifestPath)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		t.Fatalf("CommandByID(you) error = %v", err)
	}
	docsRecord, err := manifest.CommandByID("you.docs")
	if err != nil {
		t.Fatalf("CommandByID(you.docs) error = %v", err)
	}

	cases := productionManifestDocsParsingParityCases(rootRecord, docsRecord)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record, err := manifest.CommandByID(tc.commandID)
			if err != nil {
				t.Fatalf("CommandByID(%q) error = %v", tc.commandID, err)
			}

			leaf, positionals, parseErr := cli.ParseArgvForCLIInputsInventory(tc.argv)
			if tc.wantParseErr {
				if parseErr == nil {
					t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = nil, want parse failure", tc.argv)
				}
				if tc.errContains != "" && !strings.Contains(parseErr.Error(), tc.errContains) {
					t.Fatalf("parse error = %q, want substring %q", parseErr.Error(), tc.errContains)
				}
				if tc.verify != nil {
					tc.verify(t, manifest, record, leaf, positionals)
				}
				return
			}
			if parseErr != nil {
				t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = %v", tc.argv, parseErr)
			}
			if mismatch := climanifestparity.AssertLeafCommandPath(record.ID, record.Path, leaf); mismatch != nil {
				t.Fatal(mismatch.Error())
			}
			if tc.verify != nil {
				tc.verify(t, manifest, record, leaf, positionals)
			}
		})
	}
}

func productionManifestDocsParsingParityCases(rootRecord, docsRecord climanifest.Command) []parsingParityCase {
	cases := make([]parsingParityCase, 0)
	cases = append(cases, docsTopicPositionalCases(docsRecord)...)
	cases = append(cases, docsInheritedFlagCases(docsRecord)...)
	cases = append(cases, docsInheritedDefaultsCase(rootRecord, docsRecord)...)
	return cases
}

func docsTopicPositionalCases(docsRecord climanifest.Command) []parsingParityCase {
	cases := make([]parsingParityCase, 0, 4)
	cases = append(cases, docsTopicOmissionCase(docsRecord))
	cases = append(cases, docsTopicSingleValueCase(docsRecord))
	cases = append(cases, docsTopicExcessPositionalCase(docsRecord))
	cases = append(cases, docsTopicEnumCase(docsRecord))
	return cases
}

func docsTopicOmissionCase(docsRecord climanifest.Command) parsingParityCase {
	return parsingParityCase{
		name:      "docs optional topic accepts omission",
		commandID: docsRecord.ID,
		argv:      []string{"docs"},
		verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, positionals []string) {
			t.Helper()
			arg, err := record.RequireArgumentAt(0)
			if err != nil {
				t.Fatal(err)
			}
			if arg.Required || arg.MaxCardinality != 1 || arg.Variadic || arg.Name != "topic" {
				t.Fatalf("contract arg = %+v, want optional single topic positional", arg)
			}
			if mismatch := climanifestparity.CompareArgumentCardinality(record.ID, arg, positionals); mismatch != nil {
				t.Fatal(mismatch.Error())
			}
			if len(positionals) != 0 {
				t.Fatalf("positionals = %v, want empty", positionals)
			}
		},
	}
}

func docsTopicSingleValueCase(docsRecord climanifest.Command) parsingParityCase {
	return parsingParityCase{
		name:      "docs optional topic accepts one value",
		commandID: docsRecord.ID,
		argv:      []string{"docs", "config"},
		verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, positionals []string) {
			t.Helper()
			arg, err := record.RequireArgumentAt(0)
			if err != nil {
				t.Fatal(err)
			}
			if mismatch := climanifestparity.CompareArgumentCardinality(record.ID, arg, positionals); mismatch != nil {
				t.Fatal(mismatch.Error())
			}
			if len(positionals) != 1 || positionals[0] != "config" {
				t.Fatalf("positionals = %v, want [config]", positionals)
			}
		},
	}
}

func docsTopicExcessPositionalCase(docsRecord climanifest.Command) parsingParityCase {
	return parsingParityCase{
		name:         "docs rejects excess positionals",
		commandID:    docsRecord.ID,
		argv:         []string{"docs", "config", "extra"},
		wantParseErr: true,
		errContains:  "accepts at most 1 arg",
		verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, _ *cobra.Command, _ []string) {
			t.Helper()
			arg, err := record.RequireArgumentAt(0)
			if err != nil {
				t.Fatal(err)
			}
			if arg.MaxCardinality != 1 {
				t.Fatalf("contract maxCardinality = %d, want 1", arg.MaxCardinality)
			}
		},
	}
}

func docsTopicEnumCase(docsRecord climanifest.Command) parsingParityCase {
	return parsingParityCase{
		name:      "docs topic enum matches live ValidArgs",
		commandID: docsRecord.ID,
		argv:      []string{"docs", "agents"},
		verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
			t.Helper()
			arg, err := record.RequireArgumentAt(0)
			if err != nil {
				t.Fatal(err)
			}
			if mismatch := climanifestparity.CompareArgumentEnum(record.ID, arg, leaf.ValidArgs); mismatch != nil {
				t.Fatal(mismatch.Error())
			}
		},
	}
}

func docsInheritedFlagCases(docsRecord climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "docs inherited verbose no-option applies contract default",
			commandID: docsRecord.ID,
			argv:      []string{"--verbose", "docs", "config"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("verbose")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "verbose"), true, "true"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
		{
			name:      "docs inherited server flag accepts explicit value",
			commandID: docsRecord.ID,
			argv:      []string{"--server", "http://127.0.0.1:9090", "docs", "config"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				contract, err := record.RequireFlagByLong("server")
				if err != nil {
					t.Fatal(err)
				}
				if mismatch := climanifestparity.CompareFlagParsed(record.ID, contract, climanifestparity.LiveFlag(leaf, "server"), true, "http://127.0.0.1:9090"); mismatch != nil {
					t.Fatal(mismatch.Error())
				}
			},
		},
	}
}

func docsInheritedDefaultsCase(rootRecord, docsRecord climanifest.Command) []parsingParityCase {
	return []parsingParityCase{
		{
			name:      "docs inherited flags match root persistent defaults",
			commandID: docsRecord.ID,
			argv:      []string{"docs"},
			verify: func(t *testing.T, _ climanifest.Manifest, record climanifest.Command, leaf *cobra.Command, _ []string) {
				t.Helper()
				mismatches := climanifestparity.CompareInheritedFlagDefaultsAgainstRoot(rootRecord, record, leaf)
				if len(mismatches) == 0 {
					return
				}
				t.Fatalf("contract vs live inherited flag default drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
			},
		},
	}
}
