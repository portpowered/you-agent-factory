package climanifestparity_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
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

	cases := productionManifestParsingParityCases(rootRecord, sessionShowRecord)
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
