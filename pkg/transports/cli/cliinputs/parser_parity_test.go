package cliinputs_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	verify           func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, positionals []string)
}

// pkgmaintcheck:ignore-cyclomatic-complexity table-driven parser parity keeps inventory lookup, parse, and per-case verify hooks in one harness.
func TestProductionParserParity_RepresentativeCommands(t *testing.T) {
	root := cli.NewRootCommand()
	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk(production root) error = %v", err)
	}

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

			leaf, positionals, parseErr := cli.ParseArgvForCLIInputsInventory(tc.argv)
			if tc.wantParseErr {
				if parseErr == nil {
					t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = nil, want parse failure", tc.argv)
				}
				if tc.errContains != "" && !strings.Contains(parseErr.Error(), tc.errContains) {
					t.Fatalf("parse error = %q, want substring %q", parseErr.Error(), tc.errContains)
				}
				if tc.verify != nil {
					tc.verify(t, inv, leaf, positionals)
				}
				return
			}
			if parseErr != nil {
				t.Fatalf("ParseArgvForCLIInputsInventory(%v) error = %v", tc.argv, parseErr)
			}
			if leaf == nil {
				t.Fatal("expected leaf command")
			}
			if leaf.CommandPath() != tc.commandPath {
				t.Fatalf("leaf command path = %q, want %q", leaf.CommandPath(), tc.commandPath)
			}
			if tc.verify != nil {
				tc.verify(t, inv, leaf, positionals)
			}
		})
	}
}

func productionParserParityCases() []productionParserParityCase {
	cases := make([]productionParserParityCase, 0)
	cases = append(cases, productionParserParityRunCases()...)
	cases = append(cases, productionParserParitySubmitCases()...)
	cases = append(cases, productionParserParitySessionShowCases()...)
	cases = append(cases, productionParserParitySessionCreateCases()...)
	return cases
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
			verify: func(t *testing.T, inv cliinputs.Inventory, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you run", 0)
				if !arg.Variadic || arg.MinCardinality != 0 {
					t.Fatalf("inventory arg = %+v, want optional variadic tail", arg)
				}
				if len(positionals) != 0 {
					t.Fatalf("positionals = %v, want empty when only flags are provided", positionals)
				}
			},
		},
		{
			name:             "run variadic positional retains tokens after custom parse",
			commandPath:      "you run",
			argv:             []string{"run", "--no-record", "prompt-one", "prompt-two"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you run", 0)
				if !arg.Variadic {
					t.Fatalf("inventory arg = %+v, want variadic", arg)
				}
				if len(positionals) != 2 || positionals[0] != "prompt-one" || positionals[1] != "prompt-two" {
					t.Fatalf("positionals = %v, want [prompt-one prompt-two]", positionals)
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
			verify: func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, _ []string) {
				t.Helper()
				record := findFlagRecord(t, inv, "you run", "verbose")
				if record.Scope != "inherited" {
					t.Fatalf("inventory verbose scope = %q, want inherited", record.Scope)
				}
				flag := leaf.Flag("verbose")
				if flag == nil || !flag.Changed || flag.Value.String() != "true" {
					t.Fatalf("parsed verbose = changed %v value %q, want changed true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
		{
			name:        "run no-option with-mock-workers applies inventory default",
			commandPath: "you run",
			argv:        []string{"run", "--with-mock-workers"},
			flagLong:    "with-mock-workers",
			verify: func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, positionals []string) {
				t.Helper()
				record := findFlagRecord(t, inv, "you run", "with-mock-workers")
				if record.NoOptionDefault == "" {
					t.Fatal("inventory with-mock-workers missing noOptionDefault")
				}
				flag := leaf.Flag("with-mock-workers")
				if flag == nil || !flag.Changed {
					t.Fatal("expected --with-mock-workers to parse without a value")
				}
				if flag.Value.String() != record.NoOptionDefault {
					t.Fatalf("parsed value = %q, want inventory noOptionDefault %q", flag.Value.String(), record.NoOptionDefault)
				}
				if len(positionals) != 0 {
					t.Fatalf("positionals = %v, want empty after no-option flag", positionals)
				}
			},
		},
		{
			name:        "run no-option bool flag parses as true",
			commandPath: "you run",
			argv:        []string{"run", "--no-record"},
			flagLong:    "no-record",
			verify: func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, _ []string) {
				t.Helper()
				record := findFlagRecord(t, inv, "you run", "no-record")
				if record.NoOptionDefault != "true" {
					t.Fatalf("inventory no-record noOptionDefault = %q, want true", record.NoOptionDefault)
				}
				flag := leaf.Flag("no-record")
				if flag == nil || !flag.Changed || flag.Value.String() != "true" {
					t.Fatalf("parsed no-record = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
		{
			name:             "run double-dash leaves following tokens positional",
			commandPath:      "you run",
			argv:             []string{"run", "--no-record", "--", "--dir", "prompt"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you run", 0)
				if arg.DoubleDashHandling != "terminates-flags" {
					t.Fatalf("inventory doubleDashHandling = %q, want terminates-flags", arg.DoubleDashHandling)
				}
				if len(positionals) != 2 || positionals[0] != "--dir" || positionals[1] != "prompt" {
					t.Fatalf("positionals = %v, want [--dir prompt]", positionals)
				}
			},
		},
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity submit parser parity cases keep inline verify closures beside argv fixtures for reviewer readability.
func productionParserParitySubmitCases() []productionParserParityCase {
	return []productionParserParityCase{
		{
			name:        "submit inherited json flag is parseable from root",
			commandPath: "you submit",
			argv:        []string{"--json", "submit", "--name", "work-a", "--work-type-name", "task", "--payload", "payload.md"},
			flagLong:    "json",
			verify: func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, _ []string) {
				t.Helper()
				record := findFlagRecord(t, inv, "you submit", "json")
				if record.Scope != "inherited" {
					t.Fatalf("inventory json scope = %q, want inherited", record.Scope)
				}
				flag := leaf.Flag("json")
				if flag == nil || !flag.Changed || flag.Value.String() != "true" {
					t.Fatalf("parsed json = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
		{
			name:             "submit optional variadic positional accepts flag-only argv",
			commandPath:      "you submit",
			argv:             []string{"submit", "--name", "work-a", "--work-type-name", "task", "--payload", "payload.md"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you submit", 0)
				if !arg.Variadic || arg.MinCardinality != 0 {
					t.Fatalf("inventory arg = %+v, want optional variadic slot with min 0", arg)
				}
				if len(positionals) != 0 {
					t.Fatalf("positionals = %v, want empty", positionals)
				}
			},
		},
		{
			name:        "submit batch dry-run no-option bool parses without value",
			commandPath: "you submit batch",
			argv:        []string{"submit", "batch", "--dry-run", "batch.json"},
			flagLong:    "dry-run",
			verify: func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, positionals []string) {
				t.Helper()
				record := findFlagRecord(t, inv, "you submit batch", "dry-run")
				if record.NoOptionDefault != "true" {
					t.Fatalf("inventory dry-run noOptionDefault = %q, want true", record.NoOptionDefault)
				}
				flag := leaf.Flag("dry-run")
				if flag == nil || !flag.Changed || flag.Value.String() != "true" {
					t.Fatalf("parsed dry-run = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
				if len(positionals) != 1 || positionals[0] != "batch.json" {
					t.Fatalf("positionals = %v, want [batch.json]", positionals)
				}
			},
		},
		{
			name:             "submit batch variadic positional accepts flag-only argv",
			commandPath:      "you submit batch",
			argv:             []string{"submit", "batch", "--dry-run"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you submit batch", 0)
				if !arg.Variadic || arg.MinCardinality != 0 {
					t.Fatalf("inventory arg = %+v, want optional variadic slot", arg)
				}
				if len(positionals) != 0 {
					t.Fatalf("positionals = %v, want empty", positionals)
				}
			},
		},
		{
			name:        "submit batch inherited verbose flag is parseable",
			commandPath: "you submit batch",
			argv:        []string{"--verbose", "submit", "batch", "--dry-run"},
			flagLong:    "verbose",
			verify: func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, _ []string) {
				t.Helper()
				record := findFlagRecord(t, inv, "you submit batch", "verbose")
				if record.Scope != "inherited" {
					t.Fatalf("inventory verbose scope = %q, want inherited", record.Scope)
				}
				flag := leaf.Flag("verbose")
				if flag == nil || !flag.Changed || flag.Value.String() != "true" {
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
			verify: func(t *testing.T, inv cliinputs.Inventory, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you session show", 0)
				if arg.Required || arg.MaxCardinality != 1 || arg.Variadic {
					t.Fatalf("inventory arg = %+v, want optional single positional", arg)
				}
				if len(positionals) != 0 {
					t.Fatalf("positionals = %v, want empty", positionals)
				}
			},
		},
		{
			name:             "session show optional positional accepts one value",
			commandPath:      "you session show",
			argv:             []string{"session", "show", "session-beta"},
			argumentPosition: 0,
			verify: func(t *testing.T, inv cliinputs.Inventory, _ *cobra.Command, positionals []string) {
				t.Helper()
				arg := findArgumentRecord(t, inv, "you session show", 0)
				if arg.Name != "session-id" {
					t.Fatalf("inventory arg name = %q, want session-id", arg.Name)
				}
				if len(positionals) != 1 || positionals[0] != "session-beta" {
					t.Fatalf("positionals = %v, want [session-beta]", positionals)
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
			verify: func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, _ []string) {
				t.Helper()
				record := findFlagRecord(t, inv, "you session show", "json")
				if record.Scope != "inherited" {
					t.Fatalf("inventory json scope = %q, want inherited", record.Scope)
				}
				flag := leaf.Flag("json")
				if flag == nil || !flag.Changed || flag.Value.String() != "true" {
					t.Fatalf("parsed json = changed %v value %q, want true", flag != nil && flag.Changed, flagValue(flag))
				}
			},
		},
	}
}

func productionParserParitySessionCreateCases() []productionParserParityCase {
	return []productionParserParityCase{
		{
			name:        "session create shadows root json with local flag definition",
			commandPath: "you session create",
			argv:        []string{"session", "create", "--dir", "/tmp/factory", "--json"},
			flagLong:    "json",
			verify: func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, _ []string) {
				t.Helper()
				record := findFlagRecord(t, inv, "you session create", "json")
				if record.Scope != "local" {
					t.Fatalf("inventory json scope = %q, want local shadowed definition", record.Scope)
				}
				showRecord := findFlagRecord(t, inv, "you session show", "json")
				if showRecord.Scope != "inherited" {
					t.Fatalf("session show json scope = %q, want inherited for contrast", showRecord.Scope)
				}
				flag := leaf.Flag("json")
				if flag == nil || !flag.Changed || flag.Value.String() != "true" {
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
			verify: func(t *testing.T, inv cliinputs.Inventory, _ *cobra.Command, _ []string) {
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
			verify: func(t *testing.T, inv cliinputs.Inventory, _ *cobra.Command, _ []string) {
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
			verify: func(t *testing.T, inv cliinputs.Inventory, leaf *cobra.Command, _ []string) {
				t.Helper()
				record := findFlagRecord(t, inv, "you session create", "verbose")
				if record.Scope != "inherited" {
					t.Fatalf("inventory verbose scope = %q, want inherited", record.Scope)
				}
				flag := leaf.Flag("verbose")
				if flag == nil || !flag.Changed || flag.Value.String() != "true" {
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

func flagValue(flag *pflag.Flag) string {
	if flag == nil || flag.Value == nil {
		return ""
	}
	return flag.Value.String()
}
