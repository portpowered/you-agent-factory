package cliinputs

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type flagDefinitionState struct {
	Shorthand   string
	DefValue    string
	NoOptDefVal string
	Hidden      bool
	Deprecated  string
	Annotations map[string][]string
}

type commandInputsNode struct {
	Name                 string
	Aliases              []string
	ChildNames           []string
	Hidden               bool
	Short                string
	Long                 string
	Example              string
	Use                  string
	ValidArgs            []string
	HasArgsValidator     bool
	HasValidArgsFunction bool
	LocalFlags           map[string]flagDefinitionState
	PersistentFlags      map[string]flagDefinitionState
}

type commandInputsTreeState struct {
	Commands map[string]commandInputsNode
}

func captureCommandInputsTreeState(root *cobra.Command) commandInputsTreeState {
	state := commandInputsTreeState{Commands: make(map[string]commandInputsNode)}
	captureCommandInputsNode(root, state)
	return state
}

func captureCommandInputsNode(cmd *cobra.Command, state commandInputsTreeState) {
	path := cmd.CommandPath()

	aliases := cmd.Aliases
	if aliases == nil {
		aliases = []string{}
	} else {
		aliases = append([]string(nil), aliases...)
	}
	sort.Strings(aliases)

	validArgs := normalizedValidArgs(cmd)
	if validArgs == nil {
		validArgs = []string{}
	} else {
		validArgs = append([]string(nil), validArgs...)
	}

	children := cmd.Commands()
	childNames := make([]string, len(children))
	for i, child := range children {
		childNames[i] = child.Name()
	}

	state.Commands[path] = commandInputsNode{
		Name:                 cmd.Name(),
		Aliases:              aliases,
		ChildNames:           childNames,
		Hidden:               cmd.Hidden,
		Short:                cmd.Short,
		Long:                 cmd.Long,
		Example:              cmd.Example,
		Use:                  cmd.Use,
		ValidArgs:            validArgs,
		HasArgsValidator:     cmd.Args != nil,
		HasValidArgsFunction: cmd.ValidArgsFunction != nil,
		LocalFlags:           captureFlagDefinitions(cmd.LocalFlags()),
		PersistentFlags:      captureFlagDefinitions(cmd.PersistentFlags()),
	}

	for _, child := range children {
		captureCommandInputsNode(child, state)
	}
}

func captureFlagDefinitions(flagSet *pflag.FlagSet) map[string]flagDefinitionState {
	if flagSet == nil {
		return map[string]flagDefinitionState{}
	}

	definitions := make(map[string]flagDefinitionState)
	flagSet.VisitAll(func(flag *pflag.Flag) {
		definitions[flag.Name] = flagDefinitionState{
			Shorthand:   flag.Shorthand,
			DefValue:    flag.DefValue,
			NoOptDefVal: flag.NoOptDefVal,
			Hidden:      flag.Hidden,
			Deprecated:  flag.Deprecated,
			Annotations: copyFlagAnnotations(flag.Annotations),
		}
	})
	return definitions
}

func copyFlagAnnotations(annotations map[string][]string) map[string][]string {
	if len(annotations) == 0 {
		return map[string][]string{}
	}

	copied := make(map[string][]string, len(annotations))
	keys := make([]string, 0, len(annotations))
	for key := range annotations {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		values := append([]string(nil), annotations[key]...)
		sort.Strings(values)
		copied[key] = values
	}
	return copied
}

func commandInputsTreeStatesEqual(before, after commandInputsTreeState) error {
	if len(before.Commands) != len(after.Commands) {
		return fmt.Errorf(
			"command tree command set changed: before %d commands, after %d commands",
			len(before.Commands),
			len(after.Commands),
		)
	}

	paths := make([]string, 0, len(before.Commands))
	for path := range before.Commands {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		beforeNode, ok := before.Commands[path]
		if !ok {
			return fmt.Errorf("command tree missing path %q after walk", path)
		}
		afterNode, ok := after.Commands[path]
		if !ok {
			return fmt.Errorf("command tree gained path %q after walk", path)
		}
		if err := compareCommandInputsNode(path, beforeNode, afterNode); err != nil {
			return err
		}
	}

	return nil
}

func compareCommandInputsNode(path string, before, after commandInputsNode) error {
	if before.Name != after.Name {
		return fmt.Errorf("command %q name changed from %q to %q", path, before.Name, after.Name)
	}
	if !reflect.DeepEqual(before.Aliases, after.Aliases) {
		return fmt.Errorf("command %q aliases changed from %#v to %#v", path, before.Aliases, after.Aliases)
	}
	if !reflect.DeepEqual(before.ChildNames, after.ChildNames) {
		return fmt.Errorf(
			"command %q child registration order changed from %#v to %#v",
			path,
			before.ChildNames,
			after.ChildNames,
		)
	}
	if before.Hidden != after.Hidden {
		return fmt.Errorf("command %q visibility changed from hidden=%t to hidden=%t", path, before.Hidden, after.Hidden)
	}
	if before.Short != after.Short {
		return fmt.Errorf("command %q short help changed", path)
	}
	if before.Long != after.Long {
		return fmt.Errorf("command %q long help changed", path)
	}
	if before.Example != after.Example {
		return fmt.Errorf("command %q example help changed", path)
	}
	if before.Use != after.Use {
		return fmt.Errorf("command %q use line changed from %q to %q", path, before.Use, after.Use)
	}
	if !reflect.DeepEqual(before.ValidArgs, after.ValidArgs) {
		return fmt.Errorf("command %q valid args changed from %#v to %#v", path, before.ValidArgs, after.ValidArgs)
	}
	if before.HasArgsValidator != after.HasArgsValidator {
		return fmt.Errorf("command %q args validator presence changed", path)
	}
	if before.HasValidArgsFunction != after.HasValidArgsFunction {
		return fmt.Errorf("command %q valid args function presence changed", path)
	}
	if err := compareFlagDefinitions(path, "local", before.LocalFlags, after.LocalFlags); err != nil {
		return err
	}
	if err := compareFlagDefinitions(path, "persistent", before.PersistentFlags, after.PersistentFlags); err != nil {
		return err
	}
	return nil
}

func compareFlagDefinitions(path, scope string, before, after map[string]flagDefinitionState) error {
	if len(before) != len(after) {
		return fmt.Errorf(
			"command %q %s flag set size changed from %d to %d",
			path,
			scope,
			len(before),
			len(after),
		)
	}

	names := make([]string, 0, len(before))
	for name := range before {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		beforeFlag, ok := before[name]
		if !ok {
			return fmt.Errorf("command %q %s flag %q missing before walk", path, scope, name)
		}
		afterFlag, ok := after[name]
		if !ok {
			return fmt.Errorf("command %q %s flag %q missing after walk", path, scope, name)
		}
		if beforeFlag.Shorthand != afterFlag.Shorthand {
			return fmt.Errorf("command %q %s flag %q shorthand changed", path, scope, name)
		}
		if beforeFlag.DefValue != afterFlag.DefValue {
			return fmt.Errorf("command %q %s flag %q default changed", path, scope, name)
		}
		if beforeFlag.NoOptDefVal != afterFlag.NoOptDefVal {
			return fmt.Errorf("command %q %s flag %q no-option default changed", path, scope, name)
		}
		if beforeFlag.Hidden != afterFlag.Hidden {
			return fmt.Errorf("command %q %s flag %q visibility changed", path, scope, name)
		}
		if beforeFlag.Deprecated != afterFlag.Deprecated {
			return fmt.Errorf("command %q %s flag %q deprecation changed", path, scope, name)
		}
		if !reflect.DeepEqual(beforeFlag.Annotations, afterFlag.Annotations) {
			return fmt.Errorf(
				"command %q %s flag %q annotations changed from %#v to %#v",
				path,
				scope,
				name,
				beforeFlag.Annotations,
				afterFlag.Annotations,
			)
		}
	}

	return nil
}
