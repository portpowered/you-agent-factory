package commandidentity

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/spf13/cobra"
)

type commandTreeNode struct {
	Name    string
	Aliases []string
	Hidden  bool
	Short   string
	Long    string
	Example string
}

type commandTreeState struct {
	Commands map[string]commandTreeNode
}

func captureCommandTreeState(root *cobra.Command) commandTreeState {
	state := commandTreeState{Commands: make(map[string]commandTreeNode)}
	captureCommandTreeNode(root, state)
	return state
}

func captureCommandTreeNode(cmd *cobra.Command, state commandTreeState) {
	path := cmd.CommandPath()

	aliases := cmd.Aliases
	if aliases == nil {
		aliases = []string{}
	} else {
		aliases = append([]string(nil), aliases...)
	}
	sort.Strings(aliases)

	state.Commands[path] = commandTreeNode{
		Name:    cmd.Name(),
		Aliases: aliases,
		Hidden:  cmd.Hidden,
		Short:   cmd.Short,
		Long:    cmd.Long,
		Example: cmd.Example,
	}

	children := cmd.Commands()
	sort.Slice(children, func(i, j int) bool {
		return children[i].CommandPath() < children[j].CommandPath()
	})
	for _, child := range children {
		captureCommandTreeNode(child, state)
	}
}

func commandTreeStatesEqual(before, after commandTreeState) error {
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
		if beforeNode.Name != afterNode.Name {
			return fmt.Errorf("command %q name changed from %q to %q", path, beforeNode.Name, afterNode.Name)
		}
		if !reflect.DeepEqual(beforeNode.Aliases, afterNode.Aliases) {
			return fmt.Errorf("command %q aliases changed from %#v to %#v", path, beforeNode.Aliases, afterNode.Aliases)
		}
		if beforeNode.Hidden != afterNode.Hidden {
			return fmt.Errorf("command %q visibility changed from hidden=%t to hidden=%t", path, beforeNode.Hidden, afterNode.Hidden)
		}
		if beforeNode.Short != afterNode.Short {
			return fmt.Errorf("command %q short help changed", path)
		}
		if beforeNode.Long != afterNode.Long {
			return fmt.Errorf("command %q long help changed", path)
		}
		if beforeNode.Example != afterNode.Example {
			return fmt.Errorf("command %q example help changed", path)
		}
	}

	return nil
}
