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

	effective, diagnostics, err := climanifest.ComposeRunInputs(manifest, "you.run", &signature)
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

func TestGenericRepresentativeProjectionIsObservableThroughApplicationRoot(t *testing.T) {
	var observation cliobservation.Result
	process := support.BuildProcess(t, serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&observation),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--server",
		"http://localhost:9090",
		"--json",
		"session",
		"show",
		"session-customer",
	})

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(session show observation) error = %v", err)
	}
	if observation.Snapshot.Commands.RootPath != "you" {
		t.Fatalf("observed root path = %q, want you", observation.Snapshot.Commands.RootPath)
	}
	show, found := observedCommand(observation, "you.session.show")
	if !found {
		t.Fatal("application-root observation omitted stable command you.session.show")
	}
	if show.Path != "you session show" || !show.Runnable || !show.HandlerPresent {
		t.Fatalf("observed session show command = %#v, want runnable stable-ID generic handler projection", show)
	}
	server, found := cliobservation.Flag(observation.Parse, "server")
	if !found || !server.Changed || server.Value != "http://localhost:9090" {
		t.Fatalf("observed --server parse = %#v found=%v", server, found)
	}
	jsonOutput, found := cliobservation.Flag(observation.Parse, "json")
	if !found || !jsonOutput.Changed || jsonOutput.Value != "true" {
		t.Fatalf("observed --json parse = %#v found=%v", jsonOutput, found)
	}
	if observation.Parse.CommandPath != "you session show" ||
		len(observation.Parse.Positionals) != 1 ||
		observation.Parse.Positionals[0] != "session-customer" {
		t.Fatalf("observed session show parse = %#v", observation.Parse)
	}

	help := support.FakeInputs(t.Context(), []string{"you", "session", "show", "--help"})
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(help.Input); err != nil {
		t.Fatalf("Process.Execute(session show help) error = %v", err)
	}
	for _, marker := range []string{
		"Show one live factory session",
		"you session show [session-id]",
		"--server",
		"--json",
	} {
		if !strings.Contains(help.Stdout(), marker) {
			t.Fatalf("session show help omitted %q:\n%s", marker, help.Stdout())
		}
	}
}

func TestGenericSessionProjectionEnforcesProductionInputContracts(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "required positional",
			args: []string{"you", "session", "dispatches"},
			want: "requires at least 1 arg",
		},
		{
			name: "required flag",
			args: []string{"you", "session", "create"},
			want: `required flag(s) "--dir" not set`,
		},
		{
			name: "relationship",
			args: []string{"you", "session", "create", "--dir", ".", "--init-new-factory", "--validate-only"},
			want: "cannot be used together",
		},
		{
			name: "enumerated choice",
			args: []string{"you", "session", "list", "--scope", "unknown"},
			want: "scope must be live, persisted, or all",
		},
		{
			name: "deprecated input before dispatch",
			args: []string{"you", "session", "show", "--port", "9090"},
			want: "--port is no longer supported",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := support.FakeInputs(t.Context(), test.args)
			err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Process.Execute(%v) error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

func TestGenericSessionProjectionParsesVariadicInputsThroughApplicationRoot(t *testing.T) {
	var observation cliobservation.Result
	process := support.BuildProcess(t, serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&observation),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "session", "list", "workspace-a", "workspace-b", "--scope", "all",
	})

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(session list observation) error = %v", err)
	}
	if observation.Parse.CommandPath != "you session list" ||
		len(observation.Parse.Positionals) != 2 ||
		observation.Parse.Positionals[0] != "workspace-a" ||
		observation.Parse.Positionals[1] != "workspace-b" {
		t.Fatalf("observed session list parse = %#v", observation.Parse)
	}
	scope, found := cliobservation.Flag(observation.Parse, "scope")
	if !found || !scope.Changed || scope.Value != "all" {
		t.Fatalf("observed --scope parse = %#v found=%v", scope, found)
	}
}

func TestGenericSessionProjectionCoversProductionCommandShapes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{
			name: "root boolean shorthand and string options",
			args: []string{
				"you", "-v", "--debug", "--default-worker-model-provider", "openai",
				"--default-worker-model", "gpt-test", "session", "list",
			},
			path: "you session list",
		},
		{
			name: "list default scope",
			args: []string{"you", "session", "list"},
			path: "you session list",
		},
		{
			name: "dispatch filters",
			args: []string{
				"you", "session", "dispatches", "missing-session",
				"--phase", "queued", "--status", "active",
			},
			path: "you session dispatches",
		},
		{
			name: "delete required id",
			args: []string{"you", "session", "delete", "missing-session"},
			path: "you session delete",
		},
		{
			name: "pause optional id",
			args: []string{"you", "session", "pause", "missing-session"},
			path: "you session pause",
		},
		{
			name: "resume optional id",
			args: []string{"you", "session", "resume", "missing-session"},
			path: "you session resume",
		},
		{
			name: "show omitted optional id",
			args: []string{"you", "session", "show"},
			path: "you session show",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var observation cliobservation.Result
			process := support.BuildProcess(t, serviceedges.Edges{
				CLIObserver: cliobservation.Capture(&observation),
			})
			inputs := support.FakeInputs(t.Context(), test.args)
			_ = process.Execute(inputs.Input)
			if observation.Parse.CommandPath != test.path {
				t.Fatalf("observed command path = %q, want %q", observation.Parse.CommandPath, test.path)
			}
		})
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
