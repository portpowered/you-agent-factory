package commandregistry_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	initcmd "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/initsetup"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

type resolvedTestValue struct {
	id     string
	source resolvedinput.Source
	value  resolvedinput.Value
}

func resolvedTestInputs(t *testing.T, values ...resolvedTestValue) resolvedinput.Inputs {
	t.Helper()
	definitions := make([]resolvedinput.Definition, 0, len(values))
	candidates := make([]resolvedinput.Candidate, 0, len(values))
	for _, item := range values {
		definitions = append(definitions, resolvedinput.Definition{
			ID: item.id, Kind: item.value.Kind(), Precedence: []resolvedinput.Source{item.source},
		})
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: item.id, Source: item.source, Value: item.value,
		})
	}
	inputs, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return inputs
}

func resolvedFactoryGlobals(t *testing.T, jsonOutput, verbose, debug bool) resolvedinput.Inputs {
	t.Helper()
	return resolvedTestInputs(t,
		resolvedTestValue{id: "you.flag.server", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("http://localhost:7437")},
		resolvedTestValue{id: "you.flag.json", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(jsonOutput)},
		resolvedTestValue{id: "you.flag.verbose", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(verbose)},
		resolvedTestValue{id: "you.flag.debug", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(debug)},
	)
}

func TestFactoryConfigInitCommandHandlerMapsCreateStableInputs(t *testing.T) {
	var got factorycli.CreateFromFileConfig
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			CreateFactoryFromFile: func(cfg factorycli.CreateFromFileConfig) error {
				got = cfg
				return nil
			},
		},
	)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.factory.create.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("staging")},
		resolvedTestValue{id: "you.factory.create.flag.dir", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("factories")},
		resolvedTestValue{id: "you.factory.create.flag.from", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("factory.json")},
		resolvedTestValue{id: "you.factory.create.flag.set-current", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(true)},
	)
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	if err := handler.FactoryCreate(cmd, inputs, resolvedFactoryGlobals(t, true, false, false)); err != nil {
		t.Fatalf("FactoryCreate() error = %v", err)
	}
	if got.Name != "staging" || got.Dir != "factories" || got.From != "factory.json" || !got.SetCurrent || !got.JSON {
		t.Fatalf("create config = %#v, want stable-ID values", got)
	}
}

func TestFactoryConfigInitCommandHandlerMapsInitStableInputs(t *testing.T) {
	var got initcmd.ScaffoldConfig
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			InitFactory: func(cfg initcmd.ScaffoldConfig) error {
				got = cfg
				return nil
			},
		},
	)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.init.flag.dir", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("custom")},
		resolvedTestValue{id: "you.init.flag.type", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("ralph")},
		resolvedTestValue{id: "you.init.flag.executor", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("claude")},
	)
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	if err := handler.Init(cmd, inputs, resolvedFactoryGlobals(t, false, true, true)); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if got.Dir != "custom" || got.Type != "ralph" || got.Executor != "claude" || !got.Verbose || !got.Debug {
		t.Fatalf("init config = %#v, want stable-ID values", got)
	}
}

func TestFactoryConfigInitCommandHandlerMapsSuppliedSetupInputs(t *testing.T) {
	var got initsetup.Config
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			ConfigureInit: func(cfg initsetup.Config) error {
				got = cfg
				return nil
			},
			HomeDir: func() (string, error) { return "operator-home", nil },
		},
	)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.init.flag.provider", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("codex")},
		resolvedTestValue{id: "you.init.flag.model", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("free-form/model")},
		resolvedTestValue{id: "you.init.flag.dir", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("factory")},
		resolvedTestValue{id: "you.init.flag.type", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("default")},
		resolvedTestValue{id: "you.init.flag.executor", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("codex")},
	)
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	ctx := startupcli.WithStdinTTY(context.Background(), true)
	ctx = startupcli.WithStdoutTTY(ctx, true)
	cmd.SetContext(ctx)
	if err := handler.Init(cmd, inputs, resolvedFactoryGlobals(t, false, false, false)); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if got.HomeDir != "operator-home" || got.Provider != "codex" ||
		got.Model == nil || *got.Model != "free-form/model" ||
		got.Input == nil || !got.Interactive {
		t.Fatalf("init setup config = %#v, want supplied stable-ID values", got)
	}
}

func TestFactoryConfigInitCommandHandlerRejectsJSONBeforeSetup(t *testing.T) {
	called := false
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			ConfigureInit: func(initsetup.Config) error {
				called = true
				return nil
			},
		},
	)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.init.flag.provider", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("codex")},
	)
	inherited := resolvedTestInputs(t,
		resolvedTestValue{id: "you.flag.server", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("http://localhost:7437")},
		resolvedTestValue{id: "you.flag.json", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(true)},
		resolvedTestValue{id: "you.flag.verbose", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
		resolvedTestValue{id: "you.flag.debug", source: resolvedinput.SourceManifestDefault, value: resolvedinput.BoolValue(false)},
	)
	err := handler.Init(&cobra.Command{}, inputs, inherited)
	if err == nil || !strings.Contains(err.Error(), "--json is not supported") {
		t.Fatalf("Init() error = %v, want JSON rejection", err)
	}
	if called {
		t.Fatal("setup service called after JSON rejection")
	}
}

func TestFactoryConfigInitCommandHandlerRejectsChangedDeprecatedPort(t *testing.T) {
	called := false
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			QueryFactory: func(factorycli.QueryConfig) error {
				called = true
				return nil
			},
		},
	)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.factory.query.flag.port", source: resolvedinput.SourceCLIFlag, value: resolvedinput.IntValue(9090)},
	)
	err := handler.FactoryQuery(&cobra.Command{}, inputs, resolvedFactoryGlobals(t, false, false, false))
	if err == nil || !strings.Contains(err.Error(), "--server") {
		t.Fatalf("FactoryQuery() error = %v, want --server guidance", err)
	}
	if called {
		t.Fatal("query service called after deprecated port rejection")
	}
}

func TestFactoryConfigInitCommandHandlerReportsMissingStableInput(t *testing.T) {
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			ListFactories: func(factorycli.ListConfig) error { return nil },
		},
	)
	err := handler.FactoryList(&cobra.Command{}, resolvedinput.Inputs{}, resolvedFactoryGlobals(t, false, false, false))
	var accessErr *resolvedinput.AccessError
	if !errors.As(err, &accessErr) || accessErr.InputID != "you.factory.list.flag.dir" {
		t.Fatalf("FactoryList() error = %v, want missing stable dir input", err)
	}
}
