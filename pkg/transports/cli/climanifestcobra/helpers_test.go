package climanifestcobra_test

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/spf13/cobra"
)

func findCommandByPath(root *cobra.Command, path string) (*cobra.Command, error) {
	parts := strings.Fields(path)
	if len(parts) == 0 || parts[0] != root.Name() {
		return nil, fmt.Errorf("path %q does not start at root %q", path, root.Name())
	}

	current := root
	for _, segment := range parts[1:] {
		found := false
		for _, child := range current.Commands() {
			if child.Name() == segment {
				current = child
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("command segment %q not found under %q", segment, current.CommandPath())
		}
	}
	return current, nil
}

func withActiveFlagLifecycle(flags map[string]climanifest.Flag) {
	for id, flag := range flags {
		flag.Lifecycle = activeLifecycle(id)
		if flag.Completion == "" {
			flag.Completion = "none"
		}
		if flag.DefaultValue != nil || flag.NoOptionValue != nil || flag.Kind != "" {
			flag.Kind = "named"
			flag.MinCardinality = 0
			if flag.Required {
				flag.MinCardinality = 1
			}
			flag.MaxCardinality = 1
			if flag.Repeatable {
				flag.MaxCardinality = -1
			}
			flag.AcceptedSources = []string{"cli"}
			if flag.DefaultValue != nil {
				flag.AcceptedSources = append(flag.AcceptedSources, "manifest-default")
			}
			flag.HandlerBindingID = flag.ID + ".binding"
			if flag.Scope == "inherited" {
				flag.HandlerBindingID = flag.InheritedFromID + ".binding"
			}
		}
		flags[id] = flag
	}
}

func withNoneArgumentCompletion(arguments map[string]climanifest.Argument) {
	for id, argument := range arguments {
		argument.Completion = "none"
		argument.DoubleDash = "terminates-flags"
		if argument.DefaultValue == nil {
			argument.Channels = []string{"cli"}
		} else {
			argument = canonicalTestArgument(argument)
		}
		arguments[id] = argument
	}
}

func canonicalTestArgument(argument climanifest.Argument) climanifest.Argument {
	argument.Channels = nil
	argument.Scope = "local"
	argument.AcceptedSources = []string{"cli"}
	if argument.DefaultValue != nil {
		argument.AcceptedSources = append(argument.AcceptedSources, "manifest-default")
	}
	argument.HandlerBindingID = argument.ID + ".binding"
	argument.Visibility = "visible"
	argument.Lifecycle = activeLifecycle(argument.ID)
	return argument
}
