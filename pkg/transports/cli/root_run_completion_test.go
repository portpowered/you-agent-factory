package cli

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
	"github.com/spf13/cobra"
)

func TestRunCanonicalNamedFlagRegistersFactoryNameCompletion(t *testing.T) {
	var gotRequest cobracompletion.FactoryNamesRequest
	options := CommandFactory{
		homeDir: func() (string, error) { return "customer-home", nil },
		resolveNamedFactoryRoots: func(home, workingDirectory string) (
			interfaces.NamedFactoryRoots,
			error,
		) {
			if home != "customer-home" || workingDirectory != "customer-repo" {
				t.Fatalf("root inputs = (%q, %q)", home, workingDirectory)
			}
			return interfaces.NamedFactoryRoots{
				Project: "project-root",
				Global:  "global-root",
			}, nil
		},
		completeFactoryNames: func(
			_ context.Context,
			request cobracompletion.FactoryNamesRequest,
		) ([]cobra.Completion, cobra.ShellCompDirective) {
			gotRequest = request
			return []cobra.Completion{"alpha", "alpine"}, cobra.ShellCompDirectiveNoFileComp
		},
	}

	commands, err := buildRunServerProductionCommands(
		&cliGlobalOptions{},
		&cliDiagnosticsOptions{},
		&cliOperatorDefaultsOptions{},
		options,
	)
	if err != nil {
		t.Fatalf("buildRunServerProductionCommands() error = %v", err)
	}
	if commands.Run.Flags().Lookup(cobracompletion.SelectedFactoryFlagName) == nil {
		t.Fatal("canonical --named flag is missing")
	}
	completion, exists := commands.Run.GetFlagCompletionFunc(
		cobracompletion.SelectedFactoryFlagName,
	)
	if !exists {
		t.Fatal("canonical --named flag has no completion callback")
	}
	commands.Run.SetContext(startupcli.WithWorkingDirectory(t.Context(), "customer-repo"))

	got, directive := completion(commands.Run, nil, "al")
	if !reflect.DeepEqual(got, []cobra.Completion{"alpha", "alpine"}) ||
		directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("completion = (%#v, %v)", got, directive)
	}
	if gotRequest != (cobracompletion.FactoryNamesRequest{
		ProjectRoot:   "project-root",
		GlobalRoot:    "global-root",
		EnteredPrefix: "al",
	}) {
		t.Fatalf("completion request = %#v", gotRequest)
	}
}

func TestRunCanonicalNamedSelectionCompletesSignatureInputs(t *testing.T) {
	var gotRequest cobracompletion.SelectedFactorySignatureRequest
	options := CommandFactory{
		homeDir: func() (string, error) { return "customer-home", nil },
		resolveNamedFactoryRoots: func(home, workingDirectory string) (
			interfaces.NamedFactoryRoots,
			error,
		) {
			if home != "customer-home" || workingDirectory != "customer-repo" {
				t.Fatalf("root inputs = (%q, %q)", home, workingDirectory)
			}
			return interfaces.NamedFactoryRoots{
				Project: "project-root",
				Global:  "global-root",
			}, nil
		},
		completeSelectedFactorySignature: func(
			_ context.Context,
			request cobracompletion.SelectedFactorySignatureRequest,
		) cobracompletion.SelectedFactorySignatureResult {
			gotRequest = request
			return cobracompletion.SelectedFactorySignatureResult{
				Completions: []cobra.Completion{
					cobra.CompletionWithDesc("--output-format", "output format"),
				},
				Directive: cobra.ShellCompDirectiveNoFileComp,
			}
		},
	}

	commands, err := buildRunServerProductionCommands(
		&cliGlobalOptions{},
		&cliDiagnosticsOptions{},
		&cliOperatorDefaultsOptions{},
		options,
	)
	if err != nil {
		t.Fatalf("buildRunServerProductionCommands() error = %v", err)
	}
	commands.Run.SetContext(startupcli.WithWorkingDirectory(t.Context(), "customer-repo"))

	got, directive := commands.Run.ValidArgsFunction(
		commands.Run,
		[]string{"--named", "alpha"},
		"--out",
	)
	want := []cobra.Completion{
		cobra.CompletionWithDesc("--output-format", "output format"),
	}
	if !reflect.DeepEqual(got, want) ||
		directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("completion = (%#v, %v), want (%#v, no-file)", got, directive, want)
	}
	if gotRequest.ProjectRoot != "project-root" ||
		gotRequest.GlobalRoot != "global-root" ||
		gotRequest.FactoryName != "alpha" ||
		gotRequest.Target != "flags" ||
		gotRequest.EnteredPrefix != "--out" {
		t.Fatalf("completion request = %#v", gotRequest)
	}

	var output bytes.Buffer
	root := &cobra.Command{Use: "you"}
	root.SetOut(&output)
	root.SetErr(io.Discard)
	root.SetContext(startupcli.WithWorkingDirectory(t.Context(), "customer-repo"))
	root.AddCommand(commands.Run)
	root.SetArgs([]string{"__complete", "run", "--named", "alpha", "--out"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute Cobra completion protocol: %v", err)
	}
	if !strings.Contains(output.String(), "--output-format\toutput format") ||
		!strings.Contains(output.String(), ":4") {
		t.Fatalf("Cobra completion output = %q", output.String())
	}
}

func TestRunCompletionPreservesPositionalInputAfterFlagTerminator(t *testing.T) {
	dynamicCalls := 0
	options := CommandFactory{
		completeFactoryNames: func(
			context.Context,
			cobracompletion.FactoryNamesRequest,
		) ([]cobra.Completion, cobra.ShellCompDirective) {
			dynamicCalls++
			return []cobra.Completion{"dynamic-factory"}, cobra.ShellCompDirectiveNoFileComp
		},
		completeSelectedFactorySignature: func(
			context.Context,
			cobracompletion.SelectedFactorySignatureRequest,
		) cobracompletion.SelectedFactorySignatureResult {
			dynamicCalls++
			return cobracompletion.SelectedFactorySignatureResult{
				Completions: []cobra.Completion{"--dynamic-signature"},
				Directive:   cobra.ShellCompDirectiveNoFileComp,
			}
		},
	}
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "factory-looking positional input",
			args: []string{"__complete", "run", "--", "--named", "@you/"},
		},
		{
			name: "signature-looking positional input",
			args: []string{
				"__complete", "run", "--", "--named", "does-not-exist", "--m",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commands, err := buildRunServerProductionCommands(
				&cliGlobalOptions{},
				&cliDiagnosticsOptions{},
				&cliOperatorDefaultsOptions{},
				options,
			)
			if err != nil {
				t.Fatalf("buildRunServerProductionCommands() error = %v", err)
			}
			var output bytes.Buffer
			root := &cobra.Command{Use: "you"}
			root.SetOut(&output)
			root.SetErr(io.Discard)
			root.AddCommand(commands.Run)
			root.SetArgs(test.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute Cobra completion protocol: %v", err)
			}
			if strings.Contains(output.String(), "dynamic-") ||
				!strings.Contains(output.String(), ":0") {
				t.Fatalf("Cobra completion output = %q, want static default", output.String())
			}
		})
	}
	if dynamicCalls != 0 {
		t.Fatalf("dynamic completion calls = %d, want zero after terminator", dynamicCalls)
	}
}

// Legacy mutable delegates remain test-only while older command tests migrate
// to CommandFactory. Production command construction has no mutable
// package-level service bindings.
