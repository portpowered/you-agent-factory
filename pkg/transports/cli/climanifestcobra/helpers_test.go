package climanifestcobra_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/spf13/cobra"
)

func TestNewCommandTreeRejectsInvalidCanonicalFlagValuesBeforeDispatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*climanifest.Flag)
		want   string
	}{
		{name: "default", mutate: func(flag *climanifest.Flag) {
			value := []string{" Base "}
			flag.DefaultValue = &climanifest.InputValue{StringArray: &value}
			flag.Normalization = "lowercase-trim"
		}, want: "typed default is not in declared normalized form"},
		{name: "choice", mutate: func(flag *climanifest.Flag) {
			flag.Enum = []string{"BASE"}
			flag.Normalization = "lowercase"
		}, want: "enumerated choice"},
		{name: "cardinality range", mutate: func(flag *climanifest.Flag) {
			flag.Required = true
			flag.MinCardinality = 2
			flag.MaxCardinality = 1
			flag.Repeatable = false
		}, want: "invalid cardinality"},
		{name: "default cardinality", mutate: func(flag *climanifest.Flag) {
			flag.Required = true
			flag.MinCardinality = 2
		}, want: "typed default count is outside declared cardinality"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := syntheticFlagManifest()
			updateSyntheticFlag(&manifest, "stable.alpha", "stable.alpha.flag.tags", test.mutate)
			calls := 0
			bindings := genericBindingsForManifest(manifest)
			for id := range bindings.Handlers {
				bindings.Handlers[id] = func(context.Context, map[string]any) error {
					calls++
					return nil
				}
			}
			root, err := climanifestcobra.NewCommandTree(manifest, bindings)
			if root != nil || err == nil || !strings.Contains(err.Error(), test.want) || calls != 0 {
				t.Fatalf("NewCommandTree() = (%v, %v), calls=%d; want nil, %q, zero", root, err, calls, test.want)
			}
		})
	}
}

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

func completeCanonicalCommandContract(command *climanifest.Command) {
	command.HandlerBindings = make(map[string]climanifest.HandlerBinding)
	command.SourceBindings = make(map[string]climanifest.SourceBinding)
	canonical := false
	add := func(id, bindingID, valueType string, sources []string) {
		if bindingID == "" {
			return
		}
		canonical = true
		command.HandlerBindings[bindingID] = climanifest.HandlerBinding{ID: bindingID, InputID: id}
		for _, source := range sources {
			if source != "stdin" && source != "environment" && source != "operator-config" {
				continue
			}
			bindingID := id + ".source." + source
			externalKey := ""
			if source != "stdin" {
				externalKey = strings.NewReplacer(".", "_", "-", "_").Replace(strings.ToUpper(id))
			}
			command.SourceBindings[bindingID] = climanifest.SourceBinding{
				ID: bindingID, Source: source, ExternalKey: externalKey, InputID: id,
			}
		}
	}
	for _, argument := range command.Arguments {
		add(argument.ID, argument.HandlerBindingID, argument.ValueType, argument.AcceptedSources)
	}
	for _, flag := range command.Flags {
		add(flag.ID, flag.HandlerBindingID, flag.ValueType, flag.AcceptedSources)
	}
	if !canonical {
		command.HandlerBindings = nil
		command.SourceBindings = nil
		return
	}
	command.Precedence = climanifest.Precedence{
		Order: []string{
			"cli", "stdin", "environment", "operator-config",
			"manifest-default", "factory-signature-default",
		},
		WithinTier:       climanifest.WithinTierRule{Scalar: "last", Repeated: "append"},
		AcrossTiers:      "replace",
		MultipleBindings: "reject",
	}
}
