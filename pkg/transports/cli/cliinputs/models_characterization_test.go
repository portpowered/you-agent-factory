package cliinputs_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
)

// TestProductionModelsCLICharacterizesCommandGrammar pins the currently
// observable Models command shape at the process boundary. The inventory is
// live output from the production command tree, rather than a copy of the
// generated baseline fixture.
func TestProductionModelsCLICharacterizesCommandGrammar(t *testing.T) {
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production Models CLI: %v", err)
	}

	commands := []struct {
		path string
		id   string
		name bool
	}{
		{path: "you models list", id: "you.models.list"},
		{path: "you models inspect", id: "you.models.inspect", name: true},
		{path: "you models invoke", id: "you.models.invoke", name: true},
		{path: "you models pull", id: "you.models.pull", name: true},
	}

	for _, command := range commands {
		t.Run(command.path, func(t *testing.T) {
			args := modelsCharacterizationArguments(observation.Snapshot.Inputs, command.path)
			if command.name {
				want := cliinputs.ArgumentRecord{
					CommandJoin: cliinputs.CommandJoin{
						CommandPath:        command.path,
						CommandIDCandidate: command.id,
					},
					IDCandidate:        command.id + ".arg.0",
					Name:               "model-name",
					Position:           0,
					Kind:               "positional",
					ValueType:          "string",
					Required:           true,
					MinCardinality:     1,
					MaxCardinality:     1,
					Variadic:           false,
					Enum:               []string{},
					CompletionKind:     "none",
					InputChannels:      []string{"cli"},
					DoubleDashHandling: "terminates-flags",
				}
				if len(args) != 1 || !reflect.DeepEqual(args[0], want) {
					t.Fatalf("positional inputs = %#v, want exactly %#v", args, []cliinputs.ArgumentRecord{want})
				}
			} else if len(args) != 0 {
				t.Fatalf("positional inputs = %#v, want none", args)
			}

			wantFlags := []modelsCharacterizationFlag{
				{
					long: "debug", shorthand: "d", scope: "inherited", valueType: "bool",
					defaultValue: "false", noOptionDefault: "true", visibility: "visible",
				},
				{
					long: "json", scope: "inherited", valueType: "bool",
					defaultValue: "false", noOptionDefault: "true", visibility: "visible",
				},
				{
					long: "remote", scope: "inherited", valueType: "bool",
					defaultValue: "false", noOptionDefault: "true", visibility: "visible",
				},
				{
					long: "server", scope: "inherited", valueType: "string",
					defaultValue: "http://localhost:7437", visibility: "visible",
				},
				{
					long: "verbose", shorthand: "v", scope: "inherited", valueType: "bool",
					defaultValue: "false", noOptionDefault: "true", visibility: "visible",
				},
			}

			// The hidden compatibility flag is characterized, not endorsed: the
			// production pre-run guard rejects changing this legacy port input.
			wantFlags = append(wantFlags, modelsCharacterizationFlag{
				long: "port", scope: "local", valueType: "int", defaultValue: "0", visibility: "hidden",
			})
			if command.id == "you.models.invoke" {
				// The TTS-only operation surface is characterized, not endorsed: it
				// is the current public enum exposed by this CLI.
				wantFlags = append(wantFlags,
					modelsCharacterizationFlag{
						long: "operation", scope: "local", valueType: "string", defaultValue: "TTS",
						enum: []string{"TTS"}, normalization: "trim", completionKind: "static", visibility: "visible",
					},
					modelsCharacterizationFlag{
						long: "output", scope: "local", valueType: "string", normalization: "trim", visibility: "visible",
					},
					modelsCharacterizationFlag{
						long: "text", scope: "local", valueType: "string", normalization: "trim", visibility: "visible",
					},
				)
			}

			flags := modelsCharacterizationFlags(observation.Snapshot.Inputs, command.path)
			if len(flags) != len(wantFlags) {
				t.Fatalf("flag count = %d, want %d; flags = %#v", len(flags), len(wantFlags), flags)
			}
			for _, want := range wantFlags {
				got := findFlagRecord(t, observation.Snapshot.Inputs, command.path, want.long)
				if got == nil {
					t.Fatalf("missing --%s flag", want.long)
				}
				wantRecord := want.record(command.path, command.id)
				if !reflect.DeepEqual(*got, wantRecord) {
					t.Fatalf("--%s = %#v, want %#v", want.long, *got, wantRecord)
				}
			}
		})
	}
}

type modelsCharacterizationFlag struct {
	long            string
	shorthand       string
	scope           string
	valueType       string
	defaultValue    string
	noOptionDefault string
	enum            []string
	normalization   string
	completionKind  string
	visibility      string
}

func (flag modelsCharacterizationFlag) record(commandPath, commandID string) cliinputs.FlagRecord {
	completionKind := flag.completionKind
	if completionKind == "" {
		completionKind = "none"
	}
	return cliinputs.FlagRecord{
		CommandJoin: cliinputs.CommandJoin{
			CommandPath:        commandPath,
			CommandIDCandidate: commandID,
		},
		IDCandidate:       commandID + ".flag." + flag.long,
		Long:              flag.long,
		Shorthand:         flag.shorthand,
		Aliases:           []string{},
		Scope:             flag.scope,
		ValueType:         flag.valueType,
		Required:          false,
		Default:           flag.defaultValue,
		ChangedDefault:    false,
		NoOptionDefault:   flag.noOptionDefault,
		Repeatable:        false,
		Enum:              flag.enum,
		Normalization:     flag.normalization,
		CompletionKind:    completionKind,
		Binding:           "",
		Visibility:        flag.visibility,
		Deprecated:        false,
		DeprecatedMessage: "",
	}
}

func modelsCharacterizationArguments(inv cliinputs.Inventory, commandPath string) []cliinputs.ArgumentRecord {
	arguments := make([]cliinputs.ArgumentRecord, 0, 1)
	for _, argument := range inv.Arguments {
		if argument.CommandPath == commandPath {
			arguments = append(arguments, argument)
		}
	}
	return arguments
}

func modelsCharacterizationFlags(inv cliinputs.Inventory, commandPath string) []cliinputs.FlagRecord {
	flags := make([]cliinputs.FlagRecord, 0)
	for _, flag := range inv.Flags {
		if flag.CommandPath == commandPath {
			flags = append(flags, flag)
		}
	}
	return flags
}
