package cli_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestProvidersCommandHandlerListTransformsInheritedInputs(t *testing.T) {
	t.Parallel()

	root := &recordingProvidersRoot{listResult: providers.ListProvidersResult{Providers: []providers.Descriptor{{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	}}}}
	service := providerscli.New(root)
	var diagnostics bytes.Buffer
	handler := providerscli.NewCommandHandler(service, func(*cobra.Command) io.Writer { return &diagnostics })

	var output bytes.Buffer
	command := &cobra.Command{Use: "list"}
	command.SetContext(context.Background())
	command.SetOut(&output)
	if err := handler.List(command, resolvedinput.Inputs{}, resolvedProviderInheritedInputs(t, true, true, true)); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !strings.Contains(output.String(), `"id":"codex"`) {
		t.Fatalf("command output = %q, want JSON provider result", output.String())
	}
	if !strings.Contains(diagnostics.String(), "providers list request") || !strings.Contains(diagnostics.String(), "providers list complete") {
		t.Fatalf("diagnostics = %q, want request and completion entries", diagnostics.String())
	}
	if listCalls, _, _ := root.callCounts(); listCalls != 1 {
		t.Fatalf("ListProviders calls = %d, want 1", listCalls)
	}
}

func TestProvidersCommandHandlerListValidatesDependenciesAndInputs(t *testing.T) {
	t.Parallel()

	service := providerscli.New(&recordingProvidersRoot{})
	command := &cobra.Command{Use: "list"}
	command.SetContext(context.Background())
	command.SetOut(&bytes.Buffer{})
	allInputs := resolvedProviderInheritedInputs(t, true, true, true)
	cases := []struct {
		name      string
		handler   *providerscli.CommandHandler
		command   *cobra.Command
		inherited resolvedinput.Inputs
		want      string
	}{
		{name: "nil handler", handler: nil, command: command, inherited: allInputs, want: "service is required"},
		{name: "nil service", handler: providerscli.NewCommandHandler(nil, nil), command: command, inherited: allInputs, want: "service is required"},
		{name: "nil command", handler: providerscli.NewCommandHandler(service, nil), want: "command is required"},
		{name: "missing json", handler: providerscli.NewCommandHandler(service, nil), command: command, inherited: resolvedProviderInheritedInputs(t, false, true, true), want: "JSON input"},
		{name: "missing verbose", handler: providerscli.NewCommandHandler(service, nil), command: command, inherited: resolvedProviderInheritedInputs(t, true, false, true), want: "verbose input"},
		{name: "missing debug", handler: providerscli.NewCommandHandler(service, nil), command: command, inherited: resolvedProviderInheritedInputs(t, true, true, false), want: "debug input"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := testCase.handler.List(testCase.command, resolvedinput.Inputs{}, testCase.inherited)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("List() error = %v, want text %q", err, testCase.want)
			}
		})
	}
}

func resolvedProviderInheritedInputs(t *testing.T, jsonOutput, verbose, debug bool) resolvedinput.Inputs {
	t.Helper()
	definitions := []resolvedinput.Definition{
		{ID: "you.flag.json", Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
		{ID: "you.flag.verbose", Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
		{ID: "you.flag.debug", Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
	}
	candidates := make([]resolvedinput.Candidate, 0, len(definitions))
	if jsonOutput {
		candidates = append(candidates, resolvedinput.Candidate{InputID: "you.flag.json", Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(true)})
	}
	if verbose {
		candidates = append(candidates, resolvedinput.Candidate{InputID: "you.flag.verbose", Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(true)})
	}
	if debug {
		candidates = append(candidates, resolvedinput.Candidate{InputID: "you.flag.debug", Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(true)})
	}
	inputs, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return inputs
}
