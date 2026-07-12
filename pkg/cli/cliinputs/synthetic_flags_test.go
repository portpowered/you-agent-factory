package cliinputs_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/cliinputs"
	"github.com/spf13/cobra"
)

type syntheticFlagCase struct {
	commandPath        string
	commandIDCandidate string
	idCandidate        string
	long               string
	shorthand          string
	aliases            []string
	scope              string
	valueType          string
	required           bool
	defaultValue       string
	changedDefault     bool
	noOptionDefault    string
	repeatable         bool
	normalization      string
	completionKind     string
	binding            string
	visibility         string
	deprecated         bool
	deprecatedMessage  string
}

func syntheticFlagCases() []syntheticFlagCase {
	return []syntheticFlagCase{
		{
			commandPath:        "synth child-local",
			commandIDCandidate: "synth.child-local",
			idCandidate:        "synth.child-local.flag.shared",
			long:               "shared",
			shorthand:          "s",
			aliases:            []string{},
			scope:              "inherited",
			valueType:          "bool",
			required:           false,
			defaultValue:       "false",
			changedDefault:     false,
			noOptionDefault:    "true",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth child-local",
			commandIDCandidate: "synth.child-local",
			idCandidate:        "synth.child-local.flag.local-only",
			long:               "local-only",
			shorthand:          "l",
			aliases:            []string{},
			scope:              "local",
			valueType:          "string",
			required:           false,
			defaultValue:       "",
			changedDefault:     false,
			noOptionDefault:    "",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth",
			commandIDCandidate: "synth",
			idCandidate:        "synth.flag.shared",
			long:               "shared",
			shorthand:          "s",
			aliases:            []string{},
			scope:              "persistent",
			valueType:          "bool",
			required:           false,
			defaultValue:       "false",
			changedDefault:     false,
			noOptionDefault:    "true",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth child-inherit",
			commandIDCandidate: "synth.child-inherit",
			idCandidate:        "synth.child-inherit.flag.shared",
			long:               "shared",
			shorthand:          "s",
			aliases:            []string{},
			scope:              "inherited",
			valueType:          "bool",
			required:           false,
			defaultValue:       "false",
			changedDefault:     false,
			noOptionDefault:    "true",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth child-shadow",
			commandIDCandidate: "synth.child-shadow",
			idCandidate:        "synth.child-shadow.flag.shared",
			long:               "shared",
			shorthand:          "",
			aliases:            []string{},
			scope:              "local",
			valueType:          "bool",
			required:           false,
			defaultValue:       "true",
			changedDefault:     false,
			noOptionDefault:    "true",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth no-option",
			commandIDCandidate: "synth.no-option",
			idCandidate:        "synth.no-option.flag.shared",
			long:               "shared",
			shorthand:          "s",
			aliases:            []string{},
			scope:              "inherited",
			valueType:          "bool",
			required:           false,
			defaultValue:       "false",
			changedDefault:     false,
			noOptionDefault:    "true",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth no-option",
			commandIDCandidate: "synth.no-option",
			idCandidate:        "synth.no-option.flag.enable",
			long:               "enable",
			shorthand:          "",
			aliases:            []string{},
			scope:              "local",
			valueType:          "bool",
			required:           false,
			defaultValue:       "false",
			changedDefault:     false,
			noOptionDefault:    "true",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth flag-meta",
			commandIDCandidate: "synth.flag-meta",
			idCandidate:        "synth.flag-meta.flag.shared",
			long:               "shared",
			shorthand:          "s",
			aliases:            []string{},
			scope:              "inherited",
			valueType:          "bool",
			required:           false,
			defaultValue:       "false",
			changedDefault:     false,
			noOptionDefault:    "true",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth flag-meta",
			commandIDCandidate: "synth.flag-meta",
			idCandidate:        "synth.flag-meta.flag.required-opt",
			long:               "required-opt",
			shorthand:          "",
			aliases:            []string{},
			scope:              "local",
			valueType:          "string",
			required:           true,
			defaultValue:       "",
			changedDefault:     false,
			noOptionDefault:    "",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth flag-meta",
			commandIDCandidate: "synth.flag-meta",
			idCandidate:        "synth.flag-meta.flag.tags",
			long:               "tags",
			shorthand:          "",
			aliases:            []string{},
			scope:              "local",
			valueType:          "stringArray",
			required:           false,
			defaultValue:       "[]",
			changedDefault:     false,
			noOptionDefault:    "",
			repeatable:         true,
			normalization:      "",
			completionKind:     "dynamic",
			binding:            "",
			visibility:         "visible",
			deprecated:         false,
			deprecatedMessage:  "",
		},
		{
			commandPath:        "synth flag-meta",
			commandIDCandidate: "synth.flag-meta",
			idCandidate:        "synth.flag-meta.flag.legacy",
			long:               "legacy",
			shorthand:          "",
			aliases:            []string{},
			scope:              "local",
			valueType:          "bool",
			required:           false,
			defaultValue:       "false",
			changedDefault:     false,
			noOptionDefault:    "true",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "hidden",
			deprecated:         true,
			deprecatedMessage:  "use --required-opt instead",
		},
		{
			commandPath:        "synth flag-meta",
			commandIDCandidate: "synth.flag-meta",
			idCandidate:        "synth.flag-meta.flag.secret",
			long:               "secret",
			shorthand:          "",
			aliases:            []string{},
			scope:              "local",
			valueType:          "string",
			required:           false,
			defaultValue:       "",
			changedDefault:     false,
			noOptionDefault:    "",
			repeatable:         false,
			normalization:      "",
			completionKind:     "none",
			binding:            "",
			visibility:         "hidden",
			deprecated:         false,
			deprecatedMessage:  "",
		},
	}
}

func TestWalk_SyntheticTreeRecordsFlags(t *testing.T) {
	root := newSyntheticFlagTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	cases := syntheticFlagCases()
	if len(inv.Flags) != len(cases) {
		t.Fatalf("Flags len = %d, want %d", len(inv.Flags), len(cases))
	}

	byID := indexFlagsByID(t, inv.Flags)
	for _, tc := range cases {
		record, ok := byID[tc.idCandidate]
		if !ok {
			t.Fatalf("missing flag record %q", tc.idCandidate)
		}
		assertSyntheticFlagRecord(t, tc, record)
	}
}

func TestWalk_SyntheticTreeInheritedFlagsAppearOnChild(t *testing.T) {
	root := newSyntheticFlagTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	byID := indexFlagsByID(t, inv.Flags)
	inherited, ok := byID["synth.child-inherit.flag.shared"]
	if !ok {
		t.Fatal("missing inherited shared flag on child-inherit")
	}
	if inherited.Scope != "inherited" {
		t.Fatalf("inherited scope = %q, want inherited", inherited.Scope)
	}
	if inherited.Default != "false" {
		t.Fatalf("inherited default = %q, want root persistent default false", inherited.Default)
	}
}

func TestWalk_SyntheticTreeShadowedFlagUsesChildDefinition(t *testing.T) {
	root := newSyntheticFlagTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	byID := indexFlagsByID(t, inv.Flags)
	shadowed, ok := byID["synth.child-shadow.flag.shared"]
	if !ok {
		t.Fatal("missing shadowed shared flag on child-shadow")
	}
	if shadowed.Scope != "local" {
		t.Fatalf("shadowed scope = %q, want local", shadowed.Scope)
	}
	if shadowed.Default != "true" {
		t.Fatalf("shadowed default = %q, want child local default true", shadowed.Default)
	}

	shadowCount := 0
	for _, record := range inv.Flags {
		if record.CommandPath == "synth child-shadow" && record.Long == "shared" {
			shadowCount++
		}
	}
	if shadowCount != 1 {
		t.Fatalf("child-shadow shared flag count = %d, want 1 effective record", shadowCount)
	}
}

func TestWalk_SyntheticTreeDoesNotRecordArgumentsAsFlags(t *testing.T) {
	root := newSyntheticArgumentTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	for _, record := range inv.Flags {
		if record.Long == "topic" || record.Long == "seed" {
			t.Fatalf("positional name %q was recorded as a flag on %q", record.Long, record.CommandPath)
		}
	}
}

func TestWalk_SyntheticTreeDoesNotInventFlags(t *testing.T) {
	root := newSyntheticArgumentTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	for _, record := range inv.Flags {
		if record.CommandPath == "synth" {
			t.Fatalf("invented flag %q for command %q with no flags", record.IDCandidate, record.CommandPath)
		}
	}
}

func assertSyntheticFlagRecord(t *testing.T, tc syntheticFlagCase, record cliinputs.FlagRecord) {
	t.Helper()

	if record.CommandPath != tc.commandPath {
		t.Fatalf("%s commandPath = %q, want %q", tc.idCandidate, record.CommandPath, tc.commandPath)
	}
	if record.CommandIDCandidate != tc.commandIDCandidate {
		t.Fatalf("%s commandIdCandidate = %q, want %q", tc.idCandidate, record.CommandIDCandidate, tc.commandIDCandidate)
	}
	if record.IDCandidate != tc.idCandidate {
		t.Fatalf("%s idCandidate = %q, want %q", tc.idCandidate, record.IDCandidate, tc.idCandidate)
	}
	if record.Long != tc.long {
		t.Fatalf("%s long = %q, want %q", tc.idCandidate, record.Long, tc.long)
	}
	if record.Shorthand != tc.shorthand {
		t.Fatalf("%s shorthand = %q, want %q", tc.idCandidate, record.Shorthand, tc.shorthand)
	}
	if !reflect.DeepEqual(record.Aliases, tc.aliases) {
		t.Fatalf("%s aliases = %#v, want %#v", tc.idCandidate, record.Aliases, tc.aliases)
	}
	if record.Scope != tc.scope {
		t.Fatalf("%s scope = %q, want %q", tc.idCandidate, record.Scope, tc.scope)
	}
	if record.ValueType != tc.valueType {
		t.Fatalf("%s valueType = %q, want %q", tc.idCandidate, record.ValueType, tc.valueType)
	}
	if record.Required != tc.required {
		t.Fatalf("%s required = %t, want %t", tc.idCandidate, record.Required, tc.required)
	}
	if record.Default != tc.defaultValue {
		t.Fatalf("%s default = %q, want %q", tc.idCandidate, record.Default, tc.defaultValue)
	}
	if record.ChangedDefault != tc.changedDefault {
		t.Fatalf("%s changedDefault = %t, want %t", tc.idCandidate, record.ChangedDefault, tc.changedDefault)
	}
	if record.NoOptionDefault != tc.noOptionDefault {
		t.Fatalf("%s noOptionDefault = %q, want %q", tc.idCandidate, record.NoOptionDefault, tc.noOptionDefault)
	}
	if record.Repeatable != tc.repeatable {
		t.Fatalf("%s repeatable = %t, want %t", tc.idCandidate, record.Repeatable, tc.repeatable)
	}
	if record.Normalization != tc.normalization {
		t.Fatalf("%s normalization = %q, want %q", tc.idCandidate, record.Normalization, tc.normalization)
	}
	if record.CompletionKind != tc.completionKind {
		t.Fatalf("%s completionKind = %q, want %q", tc.idCandidate, record.CompletionKind, tc.completionKind)
	}
	if record.Binding != tc.binding {
		t.Fatalf("%s binding = %q, want %q", tc.idCandidate, record.Binding, tc.binding)
	}
	if record.Visibility != tc.visibility {
		t.Fatalf("%s visibility = %q, want %q", tc.idCandidate, record.Visibility, tc.visibility)
	}
	if record.Deprecated != tc.deprecated {
		t.Fatalf("%s deprecated = %t, want %t", tc.idCandidate, record.Deprecated, tc.deprecated)
	}
	if record.DeprecatedMessage != tc.deprecatedMessage {
		t.Fatalf("%s deprecatedMessage = %q, want %q", tc.idCandidate, record.DeprecatedMessage, tc.deprecatedMessage)
	}
}

func newSyntheticFlagTree() *cobra.Command {
	root := &cobra.Command{
		Use:   "synth",
		Short: "synthetic flag inventory root",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	root.PersistentFlags().BoolP("shared", "s", false, "persistent shared flag")

	childLocal := &cobra.Command{
		Use:   "child-local",
		Short: "command with a local flag only",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	childLocal.Flags().StringP("local-only", "l", "", "local string flag")

	childInherit := &cobra.Command{
		Use:   "child-inherit",
		Short: "command that inherits persistent flags",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	childShadow := &cobra.Command{
		Use:   "child-shadow",
		Short: "command that shadows an inherited flag",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	childShadow.Flags().Bool("shared", true, "shadowed shared flag")

	noOption := &cobra.Command{
		Use:   "no-option",
		Short: "command with a no-option bool flag",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	noOption.Flags().Bool("enable", false, "enable without value")
	noOption.Flags().Lookup("enable").NoOptDefVal = "true"

	flagMeta := &cobra.Command{
		Use:   "flag-meta",
		Short: "command with required, repeatable, hidden, and deprecated flags",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	flagMeta.Flags().String("required-opt", "", "required string flag")
	_ = flagMeta.MarkFlagRequired("required-opt")
	flagMeta.Flags().StringArray("tags", nil, "repeatable tags")
	_ = flagMeta.RegisterFlagCompletionFunc("tags", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"alpha", "beta"}, cobra.ShellCompDirectiveNoFileComp
	})
	flagMeta.Flags().Bool("legacy", false, "deprecated bool flag")
	_ = flagMeta.Flags().MarkDeprecated("legacy", "use --required-opt instead")
	flagMeta.Flags().String("secret", "", "hidden string flag")
	_ = flagMeta.Flags().MarkHidden("secret")

	root.AddCommand(childLocal, childInherit, childShadow, noOption, flagMeta)
	return root
}

func indexFlagsByID(t *testing.T, flags []cliinputs.FlagRecord) map[string]cliinputs.FlagRecord {
	t.Helper()

	index := make(map[string]cliinputs.FlagRecord, len(flags))
	for _, record := range flags {
		if _, exists := index[record.IDCandidate]; exists {
			t.Fatalf("duplicate flag idCandidate in inventory: %q", record.IDCandidate)
		}
		index[record.IDCandidate] = record
	}
	return index
}
