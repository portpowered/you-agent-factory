package cliinputs_test

import (
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
)

type productionParserParityCase struct {
	name             string
	commandPath      string
	argv             []string
	flagLong         string
	relationshipID   string
	argumentPosition int
	wantParseErr     bool
	errContains      string
	verify           func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult)
}

// pkgmaintcheck:ignore-cyclomatic-complexity table-driven parser parity keeps inventory lookup, parse, and per-case verify hooks in one harness.
func TestProductionParserParity_RepresentativeCommands(t *testing.T) {
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production CLI: %v", err)
	}
	inv := observation.Snapshot.Inputs

	cases := productionParserParityCases()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if record := findFlagRecord(t, inv, tc.commandPath, tc.flagLong); tc.flagLong != "" && record == nil {
				t.Fatalf("missing inventory flag %q on %s", tc.flagLong, tc.commandPath)
			}
			if tc.relationshipID != "" && findRelationshipRecord(t, inv, tc.commandPath, tc.relationshipID) == nil {
				t.Fatalf("missing inventory relationship %q on %s", tc.relationshipID, tc.commandPath)
			}
			if tc.argumentPosition >= 0 && findArgumentRecord(t, inv, tc.commandPath, tc.argumentPosition) == nil {
				t.Fatalf("missing inventory argument position %d on %s", tc.argumentPosition, tc.commandPath)
			}

			parsedObservation, parseErr := productionCLIObservation(t, tc.argv...)
			parsed := parsedObservation.Parse
			if tc.wantParseErr {
				if parseErr == nil {
					t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = nil, want parse failure", tc.argv)
				}
				if tc.errContains != "" && !strings.Contains(parseErr.Error(), tc.errContains) {
					t.Fatalf("parse error = %q, want substring %q", parseErr.Error(), tc.errContains)
				}
				if tc.verify != nil {
					tc.verify(t, inv, parsed)
				}
				return
			}
			if parseErr != nil {
				t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = %v", tc.argv, parseErr)
			}
			if parsed.CommandPath != tc.commandPath {
				t.Fatalf("leaf command path = %q, want %q", parsed.CommandPath, tc.commandPath)
			}
			if tc.verify != nil {
				tc.verify(t, inv, parsed)
			}
		})
	}
}

func productionParserParityCases() []productionParserParityCase {
	cases := make([]productionParserParityCase, 0)
	cases = append(cases, productionParserParityStaticFamilyCases()...)
	cases = append(cases, productionParserParityRunCases()...)
	cases = append(cases, productionParserParitySubmitCases()...)
	cases = append(cases, productionParserParitySessionShowCases()...)
	cases = append(cases, productionParserParitySessionCreateCases()...)
	return cases
}

func productionParserParityStaticFamilyCases() []productionParserParityCase {
	return []productionParserParityCase{
		{
			name:             "docs retains optional topic parsing",
			commandPath:      "you docs",
			argv:             []string{"docs", "run"},
			argumentPosition: 0,
		},
		{
			name:             "models retains required model and local invoke flags",
			commandPath:      "you models invoke",
			argv:             []string{"models", "invoke", "voice-model", "--text", "hello"},
			flagLong:         "text",
			argumentPosition: 0,
		},
		{
			name:             "mcp retains local runtime parsing",
			commandPath:      "you mcp serve",
			argv:             []string{"mcp", "serve", "--runtime"},
			flagLong:         "runtime",
			argumentPosition: -1,
		},
		{
			name:             "factory retains local directory parsing",
			commandPath:      "you factory list",
			argv:             []string{"factory", "list", "--dir", "factory"},
			flagLong:         "dir",
			argumentPosition: -1,
		},
		{
			name:             "init retains local provider parsing",
			commandPath:      "you init",
			argv:             []string{"init", "--provider", "codex"},
			flagLong:         "provider",
			argumentPosition: -1,
		},
		{
			name:        "work retains local state filtering",
			commandPath: "you work list",
			argv:        []string{"work", "list", "--state-name", "ready"},
			flagLong:    "state-name",
		},
		{
			name:             "server global remains parseable after deep descendant",
			commandPath:      "you factory list",
			argv:             []string{"factory", "list", "--server", "https://factory.example"},
			flagLong:         "server",
			argumentPosition: -1,
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you factory list", "server")
				if record.Scope != "inherited" {
					t.Fatalf("inventory server scope = %q, want inherited", record.Scope)
				}
				flag := parsedFlag(parsed, "server")
				if flag == nil || !flag.Changed || flag.Value != "https://factory.example" {
					t.Fatalf("parsed server = changed %v value %q, want explicit URI", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
		{
			name:         "unknown local input fails before dispatch",
			commandPath:  "you work list",
			argv:         []string{"work", "list", "--missing-local"},
			wantParseErr: true,
			errContains:  "unknown flag: --missing-local",
		},
		{
			name:         "unknown global input fails before dispatch",
			commandPath:  "you docs",
			argv:         []string{"--missing-global", "docs"},
			wantParseErr: true,
			errContains:  "unknown flag: --missing-global",
		},
	}
}

func productionParserParityRunCases() []productionParserParityCase {
	cases := make([]productionParserParityCase, 0)
	cases = append(cases, productionParserParityRunVariadicCases()...)
	cases = append(cases, productionParserParityRunFlagCases()...)
	return cases
}

func productionParserParityRunVariadicCases() []productionParserParityCase {
	return []productionParserParityCase{
		{
			name:             "run variadic positional accepts zero trailing args",
			commandPath:      "you run",
			argv:             []string{"run", "--no-record"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you run", 0)
				if !arg.Variadic || arg.MinCardinality != 0 {
					t.Fatalf("inventory arg = %+v, want optional variadic tail", arg)
				}
				if len(parsed.Positionals) != 0 {
					t.Fatalf("positionals = %v, want empty when only flags are provided", parsed.Positionals)
				}
			},
		},
		{
			name:             "run variadic positional retains tokens after custom parse",
			commandPath:      "you run",
			argv:             []string{"run", "--no-record", "prompt-one", "prompt-two"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you run", 0)
				if !arg.Variadic {
					t.Fatalf("inventory arg = %+v, want variadic", arg)
				}
				if len(parsed.Positionals) != 2 || parsed.Positionals[0] != "prompt-one" || parsed.Positionals[1] != "prompt-two" {
					t.Fatalf("positionals = %v, want [prompt-one prompt-two]", parsed.Positionals)
				}
			},
		},
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity run parser parity cases keep inline verify closures beside argv fixtures for reviewer readability.
func productionParserParityRunFlagCases() []productionParserParityCase {
	return []productionParserParityCase{
		{
			name:        "run inherited verbose flag is exposed and parseable",
			commandPath: "you run",
			argv:        []string{"--verbose", "run", "--no-record"},
			flagLong:    "verbose",
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you run", "verbose")
				if record.Scope != "inherited" {
					t.Fatalf("inventory verbose scope = %q, want inherited", record.Scope)
				}
				flag := parsedFlag(parsed, "verbose")
				if flag == nil || !flag.Changed || flag.Value != "true" {
					t.Fatalf("parsed verbose = changed %v value %q, want changed true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
		{
			name:        "run no-option with-mock-workers applies inventory default",
			commandPath: "you run",
			argv:        []string{"run", "--with-mock-workers"},
			flagLong:    "with-mock-workers",
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you run", "with-mock-workers")
				if record.NoOptionDefault == "" {
					t.Fatal("inventory with-mock-workers missing noOptionDefault")
				}
				flag := parsedFlag(parsed, "with-mock-workers")
				if flag == nil || !flag.Changed {
					t.Fatal("expected --with-mock-workers to parse without a value")
				}
				if flag.Value != record.NoOptionDefault {
					t.Fatalf("parsed value = %q, want inventory noOptionDefault %q", flag.Value, record.NoOptionDefault)
				}
				if len(parsed.Positionals) != 0 {
					t.Fatalf("positionals = %v, want empty after no-option flag", parsed.Positionals)
				}
			},
		},
		{
			name:        "run no-option bool flag parses as true",
			commandPath: "you run",
			argv:        []string{"run", "--no-record"},
			flagLong:    "no-record",
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you run", "no-record")
				if record.NoOptionDefault != "true" {
					t.Fatalf("inventory no-record noOptionDefault = %q, want true", record.NoOptionDefault)
				}
				flag := parsedFlag(parsed, "no-record")
				if flag == nil || !flag.Changed || flag.Value != "true" {
					t.Fatalf("parsed no-record = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
		{
			name:             "run double-dash leaves following tokens positional",
			commandPath:      "you run",
			argv:             []string{"run", "--no-record", "--", "--dir", "prompt"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you run", 0)
				if arg.DoubleDashHandling != "terminates-flags" {
					t.Fatalf("inventory doubleDashHandling = %q, want terminates-flags", arg.DoubleDashHandling)
				}
				if len(parsed.Positionals) != 2 || parsed.Positionals[0] != "--dir" || parsed.Positionals[1] != "prompt" {
					t.Fatalf("positionals = %v, want [--dir prompt]", parsed.Positionals)
				}
			},
		},
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity submit parser parity cases keep inline verify closures beside argv fixtures for reviewer readability.
func productionParserParitySubmitCases() []productionParserParityCase {
	return []productionParserParityCase{
		{
			name:             "submit inherited json flag is parseable from root",
			commandPath:      "you submit",
			argv:             []string{"--json", "submit", "--name", "work-a", "--work-type-name", "task", "--payload", "payload.md"},
			flagLong:         "json",
			argumentPosition: -1,
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you submit", "json")
				if record.Scope != "inherited" {
					t.Fatalf("inventory json scope = %q, want inherited", record.Scope)
				}
				flag := parsedFlag(parsed, "json")
				if flag == nil || !flag.Changed || flag.Value != "true" {
					t.Fatalf("parsed json = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
		{
			name:        "submit batch dry-run no-option bool parses without value",
			commandPath: "you submit batch",
			argv:        []string{"submit", "batch", "--dry-run", "batch.json"},
			flagLong:    "dry-run",
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you submit batch", "dry-run")
				if record.NoOptionDefault != "true" {
					t.Fatalf("inventory dry-run noOptionDefault = %q, want true", record.NoOptionDefault)
				}
				flag := parsedFlag(parsed, "dry-run")
				if flag == nil || !flag.Changed || flag.Value != "true" {
					t.Fatalf("parsed dry-run = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
				if len(parsed.Positionals) != 1 || parsed.Positionals[0] != "batch.json" {
					t.Fatalf("positionals = %v, want [batch.json]", parsed.Positionals)
				}
			},
		},
		{
			name:             "submit batch optional positional accepts flag-only argv",
			commandPath:      "you submit batch",
			argv:             []string{"submit", "batch", "--dry-run"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you submit batch", 0)
				if arg.Variadic || arg.MinCardinality != 0 || arg.MaxCardinality != 1 {
					t.Fatalf("inventory arg = %+v, want one optional positional slot", arg)
				}
				if len(parsed.Positionals) != 0 {
					t.Fatalf("positionals = %v, want empty", parsed.Positionals)
				}
			},
		},
		{
			name:        "submit batch inherited verbose flag is parseable",
			commandPath: "you submit batch",
			argv:        []string{"--verbose", "submit", "batch", "--dry-run"},
			flagLong:    "verbose",
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you submit batch", "verbose")
				if record.Scope != "inherited" {
					t.Fatalf("inventory verbose scope = %q, want inherited", record.Scope)
				}
				flag := parsedFlag(parsed, "verbose")
				if flag == nil || !flag.Changed || flag.Value != "true" {
					t.Fatalf("parsed verbose = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
	}
}

func productionParserParitySessionShowCases() []productionParserParityCase {
	return []productionParserParityCase{
		{
			name:             "session show optional positional accepts omission",
			commandPath:      "you session show",
			argv:             []string{"session", "show"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you session show", 0)
				if arg.Required || arg.MaxCardinality != 1 || arg.Variadic {
					t.Fatalf("inventory arg = %+v, want optional single positional", arg)
				}
				if len(parsed.Positionals) != 0 {
					t.Fatalf("positionals = %v, want empty", parsed.Positionals)
				}
			},
		},
		{
			name:             "session show optional positional accepts one value",
			commandPath:      "you session show",
			argv:             []string{"session", "show", "session-beta"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you session show", 0)
				if arg.Name != "session-id" {
					t.Fatalf("inventory arg name = %q, want session-id", arg.Name)
				}
				if len(parsed.Positionals) != 1 || parsed.Positionals[0] != "session-beta" {
					t.Fatalf("positionals = %v, want [session-beta]", parsed.Positionals)
				}
			},
		},
		{
			name:             "session show rejects excess positionals",
			commandPath:      "you session show",
			argv:             []string{"session", "show", "one", "two"},
			argumentPosition: 0,
			wantParseErr:     true,
			errContains:      "accepts at most 1 arg",
		},
		{
			name:        "session show inherited json flag is parseable",
			commandPath: "you session show",
			argv:        []string{"--json", "session", "show", "session-beta"},
			flagLong:    "json",
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you session show", "json")
				if record.Scope != "inherited" {
					t.Fatalf("inventory json scope = %q, want inherited", record.Scope)
				}
				flag := parsedFlag(parsed, "json")
				if flag == nil || !flag.Changed || flag.Value != "true" {
					t.Fatalf("parsed json = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
	}
}

func productionParserParitySessionCreateCases() []productionParserParityCase {
	return []productionParserParityCase{
		{
			name:        "session create inherits root json",
			commandPath: "you session create",
			argv:        []string{"session", "create", "--dir", "/tmp/factory", "--json"},
			flagLong:    "json",
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you session create", "json")
				if record.Scope != "inherited" {
					t.Fatalf("inventory json scope = %q, want inherited", record.Scope)
				}
				showRecord := findFlagRecord(t, inv, "you session show", "json")
				if showRecord.Scope != "inherited" {
					t.Fatalf("session show json scope = %q, want inherited", showRecord.Scope)
				}
				flag := parsedFlag(parsed, "json")
				if flag == nil || !flag.Changed || flag.Value != "true" {
					t.Fatalf("parsed json = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
		{
			name:         "session create required dir inventory matches missing-flag rejection",
			commandPath:  "you session create",
			argv:         []string{"session", "create"},
			flagLong:     "dir",
			wantParseErr: true,
			errContains:  `required flag(s) "dir" not set`,
			verify: func(t *testing.T, inv cliinputs.Inventory, _ platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you session create", "dir")
				if !record.Required {
					t.Fatalf("inventory dir required = false, want true")
				}
			},
		},
		{
			name:           "session create mutex inventory matches conflicting flags rejection",
			commandPath:    "you session create",
			argv:           []string{"session", "create", "--dir", "/tmp/factory", "--init-new-factory", "--validate-only"},
			relationshipID: "you.session.create.rel.mutex.init-new-factory-validate-only",
			wantParseErr:   true,
			errContains:    `if any flags in the group [init-new-factory validate-only] are set none of the others can be`,
			verify: func(t *testing.T, inv cliinputs.Inventory, _ platformprocess.CLIParseResult) {
				t.Helper()
				rel := findRelationshipRecord(t, inv, "you session create", "you.session.create.rel.mutex.init-new-factory-validate-only")
				if rel.Kind != "mutually-exclusive" {
					t.Fatalf("relationship kind = %q, want mutually-exclusive", rel.Kind)
				}
			},
		},
		{
			name:        "session create inherited verbose flag is parseable",
			commandPath: "you session create",
			argv:        []string{"--verbose", "session", "create", "--dir", "/tmp/factory"},
			flagLong:    "verbose",
			verify: func(t *testing.T, inv cliinputs.Inventory, parsed platformprocess.CLIParseResult) {
				t.Helper()
				record := findFlagRecord(t, inv, "you session create", "verbose")
				if record.Scope != "inherited" {
					t.Fatalf("inventory verbose scope = %q, want inherited", record.Scope)
				}
				flag := parsedFlag(parsed, "verbose")
				if flag == nil || !flag.Changed || flag.Value != "true" {
					t.Fatalf("parsed verbose = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
	}
}

func findFlagRecord(t *testing.T, inv cliinputs.Inventory, commandPath, longName string) *cliinputs.FlagRecord {
	t.Helper()
	for i := range inv.Flags {
		record := inv.Flags[i]
		if record.CommandPath == commandPath && record.Long == longName {
			return &record
		}
	}
	return nil
}

func findArgumentRecord(t *testing.T, inv cliinputs.Inventory, commandPath string, position int) *cliinputs.ArgumentRecord {
	t.Helper()
	for i := range inv.Arguments {
		record := inv.Arguments[i]
		if record.CommandPath == commandPath && record.Position == position {
			return &record
		}
	}
	return nil
}

func findRelationshipRecord(t *testing.T, inv cliinputs.Inventory, commandPath, idCandidate string) *cliinputs.RelationshipRecord {
	t.Helper()
	for i := range inv.Relationships {
		record := inv.Relationships[i]
		if record.CommandPath == commandPath && record.IDCandidate == idCandidate {
			return &record
		}
	}
	return nil
}

func parsedFlag(result platformprocess.CLIParseResult, name string) *platformprocess.CLIParsedFlag {
	flag, ok := cliobservation.Flag(result, name)
	if !ok {
		return nil
	}
	return &flag
}

func flagValue(flag *platformprocess.CLIParsedFlag) string {
	if flag == nil {
		return ""
	}
	return flag.Value
}
