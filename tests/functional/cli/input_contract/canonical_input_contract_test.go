package inputcontract

import (
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
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
