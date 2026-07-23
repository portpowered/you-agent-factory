package inputcontract

import (
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCanonicalRunInputCompositionAndResolution(t *testing.T) {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	signature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{{
		Name:          "customer-topic",
		ExternalName:  "customer-topic",
		DefaultValue:  "manifest contracts",
		DefaultValues: []string{"manifest contracts"},
		Bindings: []work.InvocationParameterBindingConfig{{
			Kind: work.InvocationParameterBindingKindNamed,
		}},
	}}}

	effective, diagnostics, err := climanifest.ComposeRunInputs(manifest, "you.run", signature)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("ComposeRunInputs() err=%v diagnostics=%#v", err, diagnostics)
	}
	if effective.CommandID != "you.run" || len(effective.StaticInputs) == 0 {
		t.Fatalf("effective static schema = %#v, want canonical run inputs", effective)
	}
	if len(effective.FactoryParameters) != 1 || effective.FactoryParameters[0].BindingID != "customer-topic" {
		t.Fatalf("effective Factory parameters = %#v", effective.FactoryParameters)
	}

	manifestDefault := "manifest contracts"
	explicitCLI := "runtime contracts"
	resolved, found, err := climanifest.ResolveInputValue(
		climanifest.CanonicalPrecedence(),
		[]string{climanifest.SourceCLI, climanifest.SourceFactorySignatureDefault},
		false,
		[]climanifest.ResolutionCandidate{
			{Source: climanifest.SourceFactorySignatureDefault, BindingID: "customer-topic", Value: climanifest.InputValue{String: &manifestDefault}},
			{Source: climanifest.SourceCLI, BindingID: "customer-topic", Value: climanifest.InputValue{String: &explicitCLI}},
		},
	)
	if err != nil || !found {
		t.Fatalf("ResolveInputValue() found=%v err=%v", found, err)
	}
	if resolved.Source != climanifest.SourceCLI || resolved.Value.String == nil || *resolved.Value.String != explicitCLI {
		t.Fatalf("resolved input = %#v, want explicit CLI provenance and value", resolved)
	}
}

func TestCanonicalFactoryGroupRejectsUnknownSubcommand(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{"you", "factory", "not-a-command"})

	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), `unknown command "not-a-command" for "you factory"`) {
		t.Fatalf("Process.Execute(factory unknown) error = %v, want stable unknown-command diagnostic", err)
	}
}

func TestCanonicalSchemaProjectionIsObservableThroughApplicationRoot(t *testing.T) {
	var observation cliobservation.Result
	process := support.BuildProcess(t, serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&observation),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"run",
		"--factory",
		"factory.json",
		"--output",
		"response-stream",
	})

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(run observation) error = %v", err)
	}
	if observation.Snapshot.Commands.RootPath != "you" {
		t.Fatalf("observed root path = %q, want you", observation.Snapshot.Commands.RootPath)
	}
	run, found := observedCommand(observation, "you.run")
	if !found {
		t.Fatal("application-root observation omitted stable command you.run")
	}
	if run.Path != "you run" || !run.Runnable || !run.HandlerPresent {
		t.Fatalf("observed run command = %#v, want runnable stable-ID handler projection", run)
	}
	factory, found := cliobservation.Flag(observation.Parse, "factory")
	if !found || !factory.Changed || factory.Value != "factory.json" {
		t.Fatalf("observed --factory parse = %#v found=%v", factory, found)
	}
	output, found := cliobservation.Flag(observation.Parse, "output")
	if !found || !output.Changed || output.Value != "response-stream" {
		t.Fatalf("observed --output parse = %#v found=%v", output, found)
	}
	if observation.Parse.CommandPath != "you run" {
		t.Fatalf("observed parse command path = %q, want you run", observation.Parse.CommandPath)
	}
}

func observedCommand(
	observation cliobservation.Result,
	id string,
) (commandidentity.CommandRecord, bool) {
	for _, command := range observation.Snapshot.Commands.Commands {
		if command.IDCandidate == id {
			return command, true
		}
	}
	return commandidentity.CommandRecord{}, false
}
