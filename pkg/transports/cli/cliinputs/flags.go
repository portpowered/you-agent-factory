package cliinputs

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	completionKindFilename = "filename"

	flagBindingNone = ""
)

func collectFlagRecords(root *cobra.Command) []FlagRecord {
	records := []FlagRecord{}
	collectCommandFlags(root, &records)
	return records
}

func collectCommandFlags(cmd *cobra.Command, records *[]FlagRecord) {
	*records = append(*records, flagRecordsForCommand(cmd)...)

	children := append([]*cobra.Command(nil), cmd.Commands()...)
	sort.Slice(children, func(i, j int) bool {
		return children[i].CommandPath() < children[j].CommandPath()
	})
	for _, child := range children {
		collectCommandFlags(child, records)
	}
}

func flagRecordsForCommand(cmd *cobra.Command) []FlagRecord {
	flags := effectiveFlagsForCommand(cmd)
	if len(flags) == 0 {
		return nil
	}

	join := commandJoin(cmd)
	records := make([]FlagRecord, 0, len(flags))
	for _, flag := range flags {
		records = append(records, newFlagRecord(cmd, join, flag))
	}
	return records
}

func effectiveFlagsForCommand(cmd *cobra.Command) []*pflag.Flag {
	byName := map[string]*pflag.Flag{}
	visit := func(flagSet *pflag.FlagSet) {
		if flagSet == nil {
			return
		}
		flagSet.VisitAll(func(flag *pflag.Flag) {
			byName[flag.Name] = flag
		})
	}
	visit(cmd.InheritedFlags())
	visit(cmd.Flags())

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	flags := make([]*pflag.Flag, 0, len(names))
	for _, name := range names {
		flags = append(flags, byName[name])
	}
	return flags
}

func newFlagRecord(cmd *cobra.Command, join CommandJoin, flag *pflag.Flag) FlagRecord {
	aliases := flagAliases(flag)
	if aliases == nil {
		aliases = []string{}
	}

	return FlagRecord{
		CommandJoin:       join,
		IDCandidate:       flagIDCandidate(join.CommandPath, flag.Name),
		Long:              flag.Name,
		Shorthand:         flag.Shorthand,
		Aliases:           aliases,
		Scope:             flagScope(cmd, flag.Name),
		ValueType:         flagValueType(flag),
		Required:          flagRequired(flag),
		Default:           flag.DefValue,
		ChangedDefault:    flag.Changed,
		NoOptionDefault:   flag.NoOptDefVal,
		Repeatable:        flagRepeatable(flag),
		Normalization:     flagNormalization(cmd, flag),
		CompletionKind:    flagCompletionKind(cmd, flag),
		Binding:           flagBinding(flag),
		Visibility:        flagVisibility(flag),
		Deprecated:        flag.Deprecated != "",
		DeprecatedMessage: flag.Deprecated,
	}
}

func flagIDCandidate(commandPath, longName string) string {
	return fmt.Sprintf("%s.flag.%s", commandIDCandidate(commandPath), longName)
}

func flagScope(cmd *cobra.Command, name string) string {
	if cmd.LocalFlags().Lookup(name) != nil {
		if cmd.PersistentFlags().Lookup(name) != nil {
			return flagScopePersistent
		}
		return flagScopeLocal
	}
	return flagScopeInherited
}

func flagValueType(flag *pflag.Flag) string {
	if flag.Value == nil {
		return valueTypeString
	}
	return flag.Value.Type()
}

func flagRequired(flag *pflag.Flag) bool {
	if flag.Annotations == nil {
		return false
	}
	if values, ok := flag.Annotations["infinite-you/required"]; ok && len(values) > 0 && values[0] == "true" {
		return true
	}
	_, ok := flag.Annotations[cobra.BashCompOneRequiredFlag]
	return ok
}

func flagRepeatable(flag *pflag.Flag) bool {
	return strings.HasSuffix(flagValueType(flag), "Array")
}

func flagNormalization(cmd *cobra.Command, flag *pflag.Flag) string {
	if cmd.GlobalNormalizationFunc() != nil {
		return "global"
	}
	return flagBindingNone
}

func flagCompletionKind(cmd *cobra.Command, flag *pflag.Flag) string {
	if _, ok := cmd.GetFlagCompletionFunc(flag.Name); ok {
		return completionKindDynamic
	}
	if flag.Annotations != nil {
		if _, ok := flag.Annotations[cobra.BashCompFilenameExt]; ok {
			return completionKindFilename
		}
	}
	return completionKindNone
}

func flagBinding(flag *pflag.Flag) string {
	if flag.Annotations == nil {
		return flagBindingNone
	}
	if values, ok := flag.Annotations["cobra_annotation_viper_bind"]; ok && len(values) > 0 {
		return values[0]
	}
	return flagBindingNone
}

func flagVisibility(flag *pflag.Flag) string {
	if flag.Hidden {
		return visibilityHidden
	}
	return visibilityVisible
}

func flagAliases(flag *pflag.Flag) []string {
	if flag.Annotations == nil {
		return nil
	}
	values, ok := flag.Annotations["cobra_annotation_flag_aliases"]
	if !ok || len(values) == 0 {
		return nil
	}
	aliases := make([]string, 0, len(values))
	for _, alias := range values {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
