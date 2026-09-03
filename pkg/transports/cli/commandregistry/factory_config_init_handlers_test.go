package commandregistry_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
	configcli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/config"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli/initsetup"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestFactoryConfigInitCommandHandlerMapsEffectiveListRootsAndOutputs(t *testing.T) {
	workingDirectory := t.TempDir()
	home := t.TempDir()
	var got factorycli.ListConfig
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			ListFactories: func(cfg factorycli.ListConfig) error {
				got = cfg
				return nil
			},
			HomeDir: func() (string, error) { return home, nil },
			ResolveFactoryRoots: func(gotHome, gotWorking string) (factorydefinitions.NamedFactoryRoots, error) {
				if gotHome != home || gotWorking != workingDirectory {
					t.Fatalf("root inputs = (%q, %q)", gotHome, gotWorking)
				}
				return factorydefinitions.NamedFactoryRoots{
					Project: filepath.Join(workingDirectory, "factory"),
					Global:  filepath.Join(home, "factories"),
				}, nil
			},
		},
	)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{
			id: "you.factory.list.flag.dir", source: resolvedinput.SourceCLIFlag,
			value: resolvedinput.StringValue("alternate"),
		},
	)
	var output strings.Builder
	var diagnostics strings.Builder
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	cmd.SetErr(&diagnostics)
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), workingDirectory))

	if err := handler.FactoryList(cmd, inputs, resolvedFactoryGlobals(t, true, false, false)); err != nil {
		t.Fatalf("FactoryList() error = %v", err)
	}
	if got.Context != cmd.Context() ||
		got.ProjectRoot != filepath.Join(workingDirectory, "alternate") ||
		got.GlobalRoot != filepath.Join(home, "factories") ||
		!got.JSON || got.Output != &output || got.Diagnostics != &diagnostics {
		t.Fatalf("list config = %#v", got)
	}
}

func TestFactoryConfigInitCommandHandlerReportsEffectiveListBoundaryFailures(t *testing.T) {
	workingDirectory := t.TempDir()
	homeErr := errors.New("home unavailable")
	rootsErr := errors.New("roots unavailable")
	validInputs := resolvedTestInputs(t, resolvedTestValue{
		id: "you.factory.list.flag.dir", source: resolvedinput.SourceManifestDefault,
		value: resolvedinput.StringValue("factory"),
	})
	validGlobals := resolvedFactoryGlobals(t, false, false, false)
	cases := []struct {
		name     string
		services commandregistry.FactoryConfigInitServices
		context  context.Context
		inputs   resolvedinput.Inputs
		want     string
	}{
		{
			name: "missing list service", context: startupcli.WithWorkingDirectory(t.Context(), workingDirectory),
			want: "factory list service is required",
		},
		{
			name: "missing dir input",
			services: commandregistry.FactoryConfigInitServices{
				ListFactories: func(factorycli.ListConfig) error { return nil },
			},
			context: startupcli.WithWorkingDirectory(t.Context(), workingDirectory),
			inputs:  resolvedinput.Inputs{},
			want:    "you.factory.list.flag.dir",
		},
		{
			name: "missing home resolver",
			services: commandregistry.FactoryConfigInitServices{
				ListFactories: func(factorycli.ListConfig) error { return nil },
			},
			context: startupcli.WithWorkingDirectory(t.Context(), workingDirectory),
			inputs:  validInputs,
			want:    "home-directory resolver is required",
		},
		{
			name: "home failure",
			services: commandregistry.FactoryConfigInitServices{
				ListFactories: func(factorycli.ListConfig) error { return nil },
				HomeDir:       func() (string, error) { return "", homeErr },
			},
			context: startupcli.WithWorkingDirectory(t.Context(), workingDirectory),
			inputs:  validInputs,
			want:    homeErr.Error(),
		},
		{
			name: "missing working directory",
			services: commandregistry.FactoryConfigInitServices{
				ListFactories: func(factorycli.ListConfig) error { return nil },
				HomeDir:       func() (string, error) { return "home", nil },
			},
			context: t.Context(), inputs: validInputs,
			want: "process working directory is required",
		},
		{
			name: "missing roots resolver",
			services: commandregistry.FactoryConfigInitServices{
				ListFactories: func(factorycli.ListConfig) error { return nil },
				HomeDir:       func() (string, error) { return "home", nil },
			},
			context: startupcli.WithWorkingDirectory(t.Context(), workingDirectory),
			inputs:  validInputs,
			want:    "Factory Definitions root resolver is required",
		},
		{
			name: "roots failure",
			services: commandregistry.FactoryConfigInitServices{
				ListFactories: func(factorycli.ListConfig) error { return nil },
				HomeDir:       func() (string, error) { return "home", nil },
				ResolveFactoryRoots: func(string, string) (factorydefinitions.NamedFactoryRoots, error) {
					return factorydefinitions.NamedFactoryRoots{}, rootsErr
				},
			},
			context: startupcli.WithWorkingDirectory(t.Context(), workingDirectory),
			inputs:  validInputs,
			want:    rootsErr.Error(),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			handler := commandregistry.NewFactoryConfigInitCommandHandler(test.services)
			cmd := &cobra.Command{}
			cmd.SetContext(test.context)
			err := handler.FactoryList(cmd, test.inputs, validGlobals)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("FactoryList() error = %v, want %q", err, test.want)
			}
		})
	}
}

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

func TestFactoryConfigInitCommandHandlerReportsEachMissingCreateInput(t *testing.T) {
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			CreateFactoryFromFile: func(factorycli.CreateFromFileConfig) error { return nil },
		},
	)
	name := resolvedTestValue{id: "you.factory.create.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("staging")}
	dir := resolvedTestValue{id: "you.factory.create.flag.dir", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("factories")}
	from := resolvedTestValue{id: "you.factory.create.flag.from", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("factory.json")}
	setCurrent := resolvedTestValue{id: "you.factory.create.flag.set-current", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(true)}
	cases := []struct {
		name      string
		inputs    resolvedinput.Inputs
		inherited resolvedinput.Inputs
		wantID    string
	}{
		{name: "dir", inputs: resolvedTestInputs(t, name), inherited: resolvedFactoryGlobals(t, false, false, false), wantID: "you.factory.create.flag.dir"},
		{name: "from", inputs: resolvedTestInputs(t, name, dir), inherited: resolvedFactoryGlobals(t, false, false, false), wantID: "you.factory.create.flag.from"},
		{name: "set-current", inputs: resolvedTestInputs(t, name, dir, from), inherited: resolvedFactoryGlobals(t, false, false, false), wantID: "you.factory.create.flag.set-current"},
		{name: "globals", inputs: resolvedTestInputs(t, name, dir, from, setCurrent), inherited: resolvedinput.Inputs{}, wantID: "you.flag.server"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := handler.FactoryCreate(&cobra.Command{}, test.inputs, test.inherited)
			var accessErr *resolvedinput.AccessError
			if !errors.As(err, &accessErr) || accessErr.InputID != test.wantID {
				t.Fatalf("FactoryCreate() error = %v, want AccessError for %s", err, test.wantID)
			}
		})
	}
}

func TestFactoryConfigInitCommandHandlerMapsUpdateStableInputs(t *testing.T) {
	var got factorycli.UpdateFromFileConfig
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			UpdateFactoryFromFile: func(cfg factorycli.UpdateFromFileConfig) error {
				got = cfg
				return nil
			},
		},
	)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.factory.update.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("staging")},
		resolvedTestValue{id: "you.factory.update.flag.dir", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("factories")},
		resolvedTestValue{id: "you.factory.update.flag.from", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("factory.json")},
	)
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	if err := handler.FactoryUpdate(cmd, inputs, resolvedFactoryGlobals(t, true, false, false)); err != nil {
		t.Fatalf("FactoryUpdate() error = %v", err)
	}
	if got.Name != "staging" || got.Dir != "factories" || got.From != "factory.json" || !got.JSON {
		t.Fatalf("update config = %#v, want stable-ID values", got)
	}
}

func TestFactoryConfigInitCommandHandlerReportsEachMissingUpdateInput(t *testing.T) {
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			UpdateFactoryFromFile: func(factorycli.UpdateFromFileConfig) error { return nil },
		},
	)
	name := resolvedTestValue{id: "you.factory.update.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("staging")}
	dir := resolvedTestValue{id: "you.factory.update.flag.dir", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("factories")}
	from := resolvedTestValue{id: "you.factory.update.flag.from", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("factory.json")}
	cases := []struct {
		name      string
		inputs    resolvedinput.Inputs
		inherited resolvedinput.Inputs
		wantID    string
	}{
		{name: "dir", inputs: resolvedTestInputs(t, name), inherited: resolvedFactoryGlobals(t, false, false, false), wantID: "you.factory.update.flag.dir"},
		{name: "from", inputs: resolvedTestInputs(t, name, dir), inherited: resolvedFactoryGlobals(t, false, false, false), wantID: "you.factory.update.flag.from"},
		{name: "globals", inputs: resolvedTestInputs(t, name, dir, from), inherited: resolvedinput.Inputs{}, wantID: "you.flag.server"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := handler.FactoryUpdate(&cobra.Command{}, test.inputs, test.inherited)
			var accessErr *resolvedinput.AccessError
			if !errors.As(err, &accessErr) || accessErr.InputID != test.wantID {
				t.Fatalf("FactoryUpdate() error = %v, want AccessError for %s", err, test.wantID)
			}
		})
	}
}

func TestFactoryConfigInitCommandHandlerReportsMissingTrailingInputs(t *testing.T) {
	handler := commandregistry.NewFactoryConfigInitCommandHandler(commandregistry.FactoryConfigInitServices{
		DeleteFactory:        func(factorycli.DeleteConfig) error { return nil },
		ValidateFactory:      func(factorycli.ValidateConfig) error { return nil },
		FlattenFactoryConfig: func(configcli.FactoryConfigFlattenConfig) error { return nil },
		ExpandFactoryConfig:  func(configcli.FactoryConfigExpandConfig) error { return nil },
	})
	deleteName := resolvedTestValue{id: "you.factory.delete.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("staging")}
	deleteDir := resolvedTestValue{id: "you.factory.delete.flag.dir", source: resolvedinput.SourceManifestDefault, value: resolvedinput.StringValue("factories")}
	validatePath := resolvedTestValue{id: "you.factory.config.validate.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("factory.json")}
	flattenPath := resolvedTestValue{id: "you.factory.config.flatten.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("factory.json")}
	expandPath := resolvedTestValue{id: "you.factory.config.expand.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("factory.json")}
	cases := []struct {
		name   string
		run    func() error
		wantID string
	}{
		{name: "delete-dir", run: func() error {
			return handler.FactoryDelete(&cobra.Command{}, resolvedTestInputs(t, deleteName), resolvedFactoryGlobals(t, false, false, false))
		}, wantID: "you.factory.delete.flag.dir"},
		{name: "delete-globals", run: func() error {
			return handler.FactoryDelete(&cobra.Command{}, resolvedTestInputs(t, deleteName, deleteDir), resolvedinput.Inputs{})
		}, wantID: "you.flag.server"},
		{name: "validate-globals", run: func() error {
			return handler.FactoryConfigValidate(&cobra.Command{}, resolvedTestInputs(t, validatePath), resolvedinput.Inputs{})
		}, wantID: "you.flag.server"},
		{name: "flatten-globals", run: func() error {
			return handler.FactoryConfigFlatten(&cobra.Command{}, resolvedTestInputs(t, flattenPath), resolvedinput.Inputs{})
		}, wantID: "you.flag.server"},
		{name: "expand-globals", run: func() error {
			return handler.FactoryConfigExpand(&cobra.Command{}, resolvedTestInputs(t, expandPath), resolvedinput.Inputs{})
		}, wantID: "you.flag.server"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var accessErr *resolvedinput.AccessError
			if err := test.run(); !errors.As(err, &accessErr) || accessErr.InputID != test.wantID {
				t.Fatalf("handler error = %v, want AccessError for %s", err, test.wantID)
			}
		})
	}
}

func TestFactoryConfigInitCommandHandlerReportsMissingServices(t *testing.T) {
	handler := commandregistry.NewFactoryConfigInitCommandHandler(commandregistry.FactoryConfigInitServices{})
	cmd := &cobra.Command{}
	globals := resolvedFactoryGlobals(t, false, false, false)
	cases := []struct {
		name string
		run  func() error
	}{
		{name: "show", run: func() error { return handler.FactoryQuery(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "list", run: func() error { return handler.FactoryList(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "create", run: func() error { return handler.FactoryCreate(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "update", run: func() error { return handler.FactoryUpdate(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "delete", run: func() error { return handler.FactoryDelete(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "replace", run: func() error { return handler.FactoryReplaceCurrent(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "validate", run: func() error { return handler.FactoryConfigValidate(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "flatten", run: func() error { return handler.FactoryConfigFlatten(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "expand", run: func() error { return handler.FactoryConfigExpand(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "init", run: func() error { return handler.Init(cmd, resolvedinput.Inputs{}, globals) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil || !strings.Contains(err.Error(), "service is required") {
				t.Fatalf("missing service error = %v", err)
			}
		})
	}
}

func TestFactoryConfigInitCommandHandlerReportsMissingRequiredInputs(t *testing.T) {
	handler := commandregistry.NewFactoryConfigInitCommandHandler(commandregistry.FactoryConfigInitServices{
		QueryFactory:          func(factorycli.QueryConfig) error { return nil },
		ListFactories:         func(factorycli.ListConfig) error { return nil },
		CreateFactoryFromFile: func(factorycli.CreateFromFileConfig) error { return nil },
		UpdateFactoryFromFile: func(factorycli.UpdateFromFileConfig) error { return nil },
		DeleteFactory:         func(factorycli.DeleteConfig) error { return nil },
		ReplaceFactoryCurrent: func(factorycli.ReplaceCurrentConfig) error { return nil },
		ValidateFactory:       func(factorycli.ValidateConfig) error { return nil },
		FlattenFactoryConfig:  func(configcli.FactoryConfigFlattenConfig) error { return nil },
		ExpandFactoryConfig:   func(configcli.FactoryConfigExpandConfig) error { return nil },
		ConfigureInit:         func(initsetup.Config) error { return nil },
		HomeDir:               func() (string, error) { return "operator-home", nil },
	})
	cmd := &cobra.Command{}
	globals := resolvedFactoryGlobals(t, false, false, false)
	cases := []struct {
		name string
		run  func() error
	}{
		{name: "list", run: func() error { return handler.FactoryList(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "create", run: func() error { return handler.FactoryCreate(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "update", run: func() error { return handler.FactoryUpdate(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "delete", run: func() error { return handler.FactoryDelete(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "replace", run: func() error { return handler.FactoryReplaceCurrent(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "validate", run: func() error { return handler.FactoryConfigValidate(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "flatten", run: func() error { return handler.FactoryConfigFlatten(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "expand", run: func() error { return handler.FactoryConfigExpand(cmd, resolvedinput.Inputs{}, globals) }},
		{name: "init", run: func() error { return handler.Init(cmd, resolvedinput.Inputs{}, globals) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var accessErr *resolvedinput.AccessError
			if err := test.run(); !errors.As(err, &accessErr) {
				t.Fatalf("missing input error = %v, want AccessError", err)
			}
		})
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

func TestFactoryConfigInitCommandHandlerMapsPackagedInstallStableInputs(t *testing.T) {
	var got factorydefinitionscli.InstallPackagedFactoryConfig
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			InstallPackagedFactory: func(cfg factorydefinitionscli.InstallPackagedFactoryConfig) error {
				got = cfg
				return nil
			},
			HomeDir: func() (string, error) { return "operator-home", nil },
		},
	)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.init.flag.package", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("@you/goal")},
		resolvedTestValue{id: "you.init.flag.dir", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("factories")},
		resolvedTestValue{id: "you.init.flag.format", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("yaml")},
		resolvedTestValue{id: "you.init.flag.replace", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(true)},
	)
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	if err := handler.Init(cmd, inputs, resolvedFactoryGlobals(t, true, true, false)); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if got.HomeDir != "operator-home" || got.Package != "@you/goal" || got.Dir != "factories" ||
		!got.DirChanged || got.Format != "yaml" || !got.FormatChanged || !got.Replace || !got.JSON || !got.Verbose {
		t.Fatalf("packaged init config = %#v, want stable-ID values", got)
	}
}

func TestFactoryConfigInitCommandHandlerRejectsJSONForProviderSetup(t *testing.T) {
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
		resolvedTestValue{id: "you.factory.show.flag.port", source: resolvedinput.SourceCLIFlag, value: resolvedinput.IntValue(9090)},
	)
	err := handler.FactoryQuery(&cobra.Command{}, inputs, resolvedFactoryGlobals(t, false, false, false))
	if err == nil || !strings.Contains(err.Error(), "--server") {
		t.Fatalf("FactoryQuery() error = %v, want --server guidance", err)
	}
	if called {
		t.Fatal("query service called after deprecated port rejection")
	}
}

func TestFactoryConfigInitCommandHandlerMapsExplicitShowSession(t *testing.T) {
	const sessionID = "session-alpha"
	var got factorycli.QueryConfig
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			QueryFactory: func(cfg factorycli.QueryConfig) error {
				got = cfg
				return nil
			},
		},
	)
	inputs := resolvedTestInputs(t,
		resolvedTestValue{id: "you.factory.show.flag.session", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue(sessionID)},
	)
	cmd := &cobra.Command{}
	if err := handler.FactoryQuery(cmd, inputs, resolvedFactoryGlobals(t, true, true, true)); err != nil {
		t.Fatalf("FactoryQuery() error = %v", err)
	}
	if got.Context != cmd.Context() || got.Server != "http://localhost:7437" || got.SessionID != sessionID ||
		!got.JSON || !got.Verbose || !got.Debug || got.Output != cmd.OutOrStdout() {
		t.Fatalf("query config = %#v, want explicit session and inherited globals", got)
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
