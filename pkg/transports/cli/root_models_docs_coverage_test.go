// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	submitcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/submit"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
	acpcli "github.com/portpowered/infinite-you/pkg/transports/cli/acp"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

type modelsCLIServiceFunctions struct {
	list    func(modelscli.ListConfig) error
	inspect func(modelscli.InspectConfig) error
	invoke  func(modelscli.InvokeConfig) error
	pull    func(modelscli.PullConfig) error
	remove  func(modelscli.RemoveConfig) error
}

func (service modelsCLIServiceFunctions) List(cfg modelscli.ListConfig) error {
	if service.list != nil {
		return service.list(cfg)
	}
	return nil
}
func (service modelsCLIServiceFunctions) Inspect(cfg modelscli.InspectConfig) error {
	if service.inspect != nil {
		return service.inspect(cfg)
	}
	return nil
}
func (service modelsCLIServiceFunctions) Invoke(cfg modelscli.InvokeConfig) error {
	if service.invoke != nil {
		return service.invoke(cfg)
	}
	return nil
}
func (service modelsCLIServiceFunctions) Pull(cfg modelscli.PullConfig) error {
	if service.pull != nil {
		return service.pull(cfg)
	}
	return nil
}
func (service modelsCLIServiceFunctions) Remove(cfg modelscli.RemoveConfig) error {
	if service.remove != nil {
		return service.remove(cfg)
	}
	return nil
}

func TestProductionModelsCommandWiresInjectedHandlers(t *testing.T) {
	models, err := newProductionModelsCommand(&cliGlobalOptions{}, &cliDiagnosticsOptions{}, &cliOperatorDefaultsOptions{}, CommandFactory{ModelsCLI: modelsCLIServiceFunctions{}})
	if err != nil {
		t.Fatalf("newProductionModelsCommand() error = %v", err)
	}
	for _, name := range []string{"list", "inspect", "invoke", "pull", "remove"} {
		command, _, findErr := models.Find([]string{name})
		if findErr != nil || command.RunE == nil {
			t.Fatalf("models %s handler = %v, %v", name, command, findErr)
		}
	}
}

func TestProductionDocsAndModelsCommandsBuildIndependently(t *testing.T) {
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	docs, err := newProductionDocsCommand(diagnostics)
	if err != nil {
		t.Fatalf("newProductionDocsCommand() error = %v", err)
	}
	models, err := newProductionModelsCommand(globals, diagnostics, operatorDefaults, CommandFactory{ModelsCLI: modelsCLIServiceFunctions{}})
	if err != nil {
		t.Fatalf("newProductionModelsCommand() error = %v", err)
	}
	if docs == nil || docs.RunE == nil {
		t.Fatal("generated docs command must attach handwritten RunE")
	}
	if models == nil || models.RunE != nil {
		t.Fatal("generated models parent must remain non-runnable")
	}
	if len(models.Commands()) != 5 {
		t.Fatalf("models child count = %d, want 5 generated leaves", len(models.Commands()))
	}
}

func TestProductionDocsCompletionComesFromManifestTopicChoices(t *testing.T) {
	docs, err := newProductionDocsCommand(&cliDiagnosticsOptions{})
	if err != nil {
		t.Fatalf("newProductionDocsCommand() error = %v", err)
	}
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		t.Fatalf("ModelsDocsFamilyManifest() error = %v", err)
	}
	record, err := manifest.CommandByID("you.docs")
	if err != nil {
		t.Fatal(err)
	}
	topic, err := record.RequireArgumentAt(0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(docs.ValidArgs, topic.Enum) {
		t.Fatalf("docs completion = %#v, want manifest choices %#v", docs.ValidArgs, topic.Enum)
	}
}

func TestProductionModelsInspectAndPullHonorJSONFlag(t *testing.T) {
	var inspectJSON, pullJSON bool
	root := (CommandFactory{ModelsCLI: modelsCLIServiceFunctions{
		inspect: func(cfg modelscli.InspectConfig) error {
			inspectJSON = cfg.JSON
			return nil
		},
		pull: func(cfg modelscli.PullConfig) error {
			pullJSON = cfg.JSON
			return nil
		},
	}}).NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "models", "inspect", "OMNIVOICE_Q4_K_M"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute inspect --json: %v", err)
	}
	root.SetArgs([]string{"--json", "models", "pull", "OMNIVOICE_Q4_K_M"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute pull --json: %v", err)
	}
	if !inspectJSON || !pullJSON {
		t.Fatalf("json bindings = inspect %t pull %t, want true", inspectJSON, pullJSON)
	}
}

func TestProductionModelsPullRejectsInvalidInputsBeforeService(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing model", args: []string{"models", "pull"}, want: "accepts 1 arg(s), received 0"},
		{name: "extra model", args: []string{"models", "pull", "model-a", "model-b"}, want: "accepts 1 arg(s), received 2"},
		{name: "unknown flag", args: []string{"models", "pull", "--unknown"}, want: "unknown flag: --unknown"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			called := false
			root := (CommandFactory{ModelsCLI: modelsCLIServiceFunctions{
				pull: func(modelscli.PullConfig) error {
					called = true
					return nil
				},
			}}).NewCommand(nil, nil, nil)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(testCase.args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("execute %v error = %v, want %q", testCase.args, err, testCase.want)
			}
			if called {
				t.Fatal("invalid pull input invoked Models service")
			}
		})
	}
}

func TestProductionModelsInvokeRejectsInvalidInputsBeforeService(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing model", args: []string{"models", "invoke"}, want: "accepts 1 arg(s), received 0"},
		{name: "extra model", args: []string{"models", "invoke", "model-a", "model-b"}, want: "accepts 1 arg(s), received 2"},
		{name: "unknown flag", args: []string{"models", "invoke", "model-a", "--unknown"}, want: "unknown flag: --unknown"},
		{name: "invalid operation", args: []string{"models", "invoke", "model-a", "--operation", "INVALID"}, want: "not one of the declared choices"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			called := false
			root := (CommandFactory{ModelsCLI: modelsCLIServiceFunctions{
				invoke: func(modelscli.InvokeConfig) error {
					called = true
					return nil
				},
			}}).NewCommand(nil, nil, nil)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(testCase.args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("execute %v error = %v, want %q", testCase.args, err, testCase.want)
			}
			if called {
				t.Fatal("invalid invoke input invoked Models service")
			}
		})
	}
}

func TestProductionModelsPullPreservesCancellationAndOperationFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := (CommandFactory{ModelsCLI: modelsCLIServiceFunctions{
		pull: func(cfg modelscli.PullConfig) error {
			if cfg.Context.Err() != context.Canceled {
				t.Fatalf("pull context error = %v, want canceled", cfg.Context.Err())
			}
			return context.Canceled
		},
	}}).NewCommand(nil, nil, nil)
	root.SetContext(ctx)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "pull", "model-a"})
	if err := root.Execute(); !errors.Is(err, context.Canceled) {
		t.Fatalf("execute canceled models pull error = %v, want context.Canceled", err)
	}
}

func TestProductionModelsCommandDefaultsNilOperatorDefaults(t *testing.T) {
	models, err := newProductionModelsCommand(&cliGlobalOptions{}, &cliDiagnosticsOptions{}, nil, CommandFactory{ModelsCLI: modelsCLIServiceFunctions{}})
	if err != nil {
		t.Fatalf("newProductionModelsCommand(nil operator defaults) error = %v", err)
	}
	invoke, _, err := models.Find([]string{"invoke"})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := invoke.Flags().GetString("operation")
	if err != nil || operation != "TTS" {
		t.Fatalf("invoke operation = %q, %v; want TTS", operation, err)
	}
}

func TestProductionDocsExecutesTopicWithVerboseDiagnostics(t *testing.T) {
	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--verbose", "docs", "models"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute verbose docs models: %v", err)
	}
	if !strings.Contains(stderr.String(), "docs request topic=models") {
		t.Fatalf("stderr = %q, want verbose docs diagnostics", stderr.String())
	}
}

func TestModelsListUsesInjectedService(t *testing.T) {
	called := false
	root := NewCommandFactory(CommandOperations{ModelsCLI: modelsCLIServiceFunctions{
		list: func(modelscli.ListConfig) error {
			called = true
			return nil
		},
	}}).NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute models list: %v", err)
	}
	if !called {
		t.Fatal("injected models list service was not invoked")
	}
}

func TestInjectedModelServicesRouteGeneratedCutoverCommands(t *testing.T) {
	var listed bool
	var inspected, pulled string
	var invocations []modelscli.InvokeConfig
	factory := withTestInjectedPlatformRoles(NewCommandFactory(CommandOperations{ModelsCLI: modelsCLIServiceFunctions{
		list: func(modelscli.ListConfig) error { listed = true; return nil },
		inspect: func(cfg modelscli.InspectConfig) error {
			inspected = cfg.ModelName
			return nil
		},
		pull: func(cfg modelscli.PullConfig) error {
			pulled = cfg.ModelName
			return nil
		},
		invoke: func(cfg modelscli.InvokeConfig) error {
			invocations = append(invocations, cfg)
			return nil
		},
	}}))
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	for _, args := range [][]string{
		{"models", "list"},
		{"models", "inspect", "OMNIVOICE_Q4_K_M"},
		{"models", "pull", "OMNIVOICE_Q4_K_M"},
		{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello"},
	} {
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
	}
	if !listed || inspected != "OMNIVOICE_Q4_K_M" || pulled != "OMNIVOICE_Q4_K_M" || len(invocations) != 1 {
		t.Fatalf("delegate routing = listed %t inspected %q pulled %q invocations %d", listed, inspected, pulled, len(invocations))
	}
	if invocations[0].Operation != "TTS" || invocations[0].Text != "hello" {
		t.Fatalf("invoke config = %#v, want operation/text bindings", invocations[0])
	}
}

func TestModelsCommandsPreserveBehaviorThroughProductionComposition(t *testing.T) {
	t.Parallel()

	defaults := operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CODEX",
		WorkerModel:         "gpt-test",
	}
	for _, test := range modelsCompositionCases() {
		t.Run(test.name+" success", func(t *testing.T) {
			service := modelsCompositionService(t, test.name, defaults, nil)
			stdout, stderr, err := executeModelsComposition(t, service, defaults, test.args)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if stdout != "models-ok\n" || stderr != "models-diagnostic\n" {
				t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
			}
		})
		t.Run(test.name+" failure", func(t *testing.T) {
			service := modelsCompositionService(t, test.name, defaults, test.wantError)
			stdout, stderr, err := executeModelsComposition(t, service, defaults, test.args)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Execute() error = %v, want %v", err, test.wantError)
			}
			if stdout != "" || stderr != fmt.Sprintf("Error: %v\n", test.wantError) {
				t.Fatalf("failure stdout = %q, stderr = %q", stdout, stderr)
			}
		})
	}
}

type modelsCompositionCase struct {
	name      string
	args      []string
	wantError error
}

func modelsCompositionCases() []modelsCompositionCase {
	operationFailure := errors.New("models operation failed")
	common := []string{"--verbose", "--json", "--server", "https://factory.example", "models"}
	return []modelsCompositionCase{
		{name: "list", args: append(append([]string(nil), common...), "list"), wantError: operationFailure},
		{
			name:      "inspect",
			args:      append(append([]string(nil), common...), "inspect", "model-alpha"),
			wantError: modelscli.ErrModelNotFound,
		},
		{
			name: "invoke",
			args: []string{
				"--verbose", "--json", "--server", "https://factory.example",
				"models", "invoke", "model-alpha", "--operation", "TTS",
				"--text", "hello", "--output", "speech.wav",
			},
			wantError: context.Canceled,
		},
		{
			name:      "pull",
			args:      append(append([]string(nil), common...), "pull", "model-alpha"),
			wantError: operationFailure,
		},
	}
}

func modelsCompositionService(
	t *testing.T,
	command string,
	defaults operatorconfig.ResolvedDefaults,
	result error,
) modelscli.Service {
	t.Helper()
	switch command {
	case "list":
		return modelsCLIServiceFunctions{list: func(cfg modelscli.ListConfig) error {
			assertModelsCommonConfig(t, cfg.Context, cfg.Server, cfg.JSON, cfg.Verbose)
			return writeModelsCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	case "inspect":
		return modelsCLIServiceFunctions{inspect: func(cfg modelscli.InspectConfig) error {
			assertModelsCommonConfig(t, cfg.Context, cfg.Server, cfg.JSON, cfg.Verbose)
			assertModelsName(t, cfg.ModelName)
			return writeModelsCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	case "invoke":
		return modelsCLIServiceFunctions{invoke: func(cfg modelscli.InvokeConfig) error {
			assertModelsCommonConfig(t, cfg.Context, cfg.Server, cfg.JSON, cfg.Verbose)
			if cfg.ModelName != "model-alpha" || cfg.Operation != "TTS" ||
				cfg.Text != "hello" || cfg.OutputPath != "speech.wav" ||
				cfg.HomeDir == "" || cfg.OperatorDefaults != defaults || cfg.Logger == nil {
				t.Fatalf("invoke config = %#v", cfg)
			}
			return writeModelsCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	case "pull":
		return modelsCLIServiceFunctions{pull: func(cfg modelscli.PullConfig) error {
			assertModelsCommonConfig(t, cfg.Context, cfg.Server, cfg.JSON, cfg.Verbose)
			assertModelsName(t, cfg.ModelName)
			return writeModelsCompositionOutput(cfg.Output, cfg.Diagnostics, result)
		}}
	default:
		t.Fatalf("unsupported models composition command %q", command)
		return nil
	}
}

func executeModelsComposition(
	t *testing.T,
	service modelscli.Service,
	defaults operatorconfig.ResolvedDefaults,
	args []string,
) (string, string, error) {
	t.Helper()
	factory := withTestInjectedPlatformRoles(NewCommandFactory(CommandOperations{ModelsCLI: service}))
	factory.resolveOperatorDefaults = func(
		_ string,
		_ operatorconfig.Defaults,
		flags operatorconfig.FlagOverrides,
	) (operatorconfig.ResolvedDefaults, error) {
		if flags.WorkerModelProvider != "" || flags.WorkerModel != "" {
			t.Fatalf("operator default flags = %#v", flags)
		}
		return defaults, nil
	}
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func assertModelsCommonConfig(
	t *testing.T,
	ctx context.Context,
	server string,
	jsonOutput, verbose bool,
) {
	t.Helper()
	if ctx == nil || server != "https://factory.example" || !jsonOutput || !verbose {
		t.Fatalf(
			"common config = context %v, server %q, JSON %t, verbose %t",
			ctx,
			server,
			jsonOutput,
			verbose,
		)
	}
}

func assertModelsName(t *testing.T, modelName string) {
	t.Helper()
	if modelName != "model-alpha" {
		t.Fatalf("ModelName = %q, want model-alpha", modelName)
	}
}

func writeModelsCompositionOutput(output, diagnostics io.Writer, result error) error {
	if result != nil {
		return result
	}
	if _, err := fmt.Fprintln(output, "models-ok"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(diagnostics, "models-diagnostic")
	return err
}

func TestProductionRootUsesGeneratedModelsDocsFamilyCutover(t *testing.T) {
	root := newLegacyTestRootCommand()
	docs, _, err := root.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("Find(docs) error = %v", err)
	}
	if docs.RunE == nil {
		t.Fatal("you docs must attach handwritten RunE through generated cutover")
	}

	models, _, err := root.Find([]string{"models"})
	if err != nil {
		t.Fatalf("Find(models) error = %v", err)
	}
	if models.RunE != nil {
		t.Fatal("you models must remain non-runnable")
	}
	list, _, err := root.Find([]string{"models", "list"})
	if err != nil {
		t.Fatalf("Find(models list) error = %v", err)
	}
	if list.RunE == nil {
		t.Fatal("you models list must attach handwritten RunE through generated cutover")
	}
}

func TestRootCommand_HelpDocumentsGlobalJSONFlag(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"--json", "structured JSON on stdout"} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}

func TestRootCommand_HelpDocumentsGlobalServerFlag(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{
		"--server",
		"factory API base URI",
	} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	if bytes.Contains([]byte(help), []byte("--port")) {
		t.Fatalf("root help must not advertise --port:\n%s", help)
	}
}

func TestFactoryShowCommand_HelpUsesGlobalFlags(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "show", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory show --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{
		"global --json",
		"global --server",
		"you --server http://localhost:9090 --json factory show",
		"you --json factory show",
	} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("factory show help missing %q:\n%s", want, help)
		}
	}
	if bytes.Contains([]byte(help), []byte("--port")) {
		t.Fatalf("factory show help must not advertise --port:\n%s", help)
	}
}

func TestSupportedCommands_DoNotRegisterLocalJSONFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	for _, path := range [][]string{
		{"factory", "show"},
		{"work", "list"},
		{"models", "list"},
		{"models", "inspect"},
		{"models", "invoke"},
		{"models", "pull"},
	} {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
		if cmd.LocalFlags().Lookup("json") != nil {
			t.Fatalf("%s registers a local --json flag; use root persistent --json", cmd.CommandPath())
		}
	}
}

func TestFactoryShowCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalQueryFactory := queryFactory
	defer func() {
		queryFactory = originalQueryFactory
	}()

	var got factorycli.QueryConfig
	queryFactory = func(cfg factorycli.QueryConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "factory", "show"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory show with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to QueryConfig.JSON")
	}
}

func TestFactoryReplaceCurrentCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalReplaceFactoryCurrent := replaceFactoryCurrent
	defer func() {
		replaceFactoryCurrent = originalReplaceFactoryCurrent
	}()

	var got factorycli.ReplaceCurrentConfig
	replaceFactoryCurrent = func(cfg factorycli.ReplaceCurrentConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "factory", "replace-current"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory replace-current with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to ReplaceCurrentConfig.JSON")
	}
}

func TestModelsInspectCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalModelsCLI := rootModelsCLI
	defer func() {
		rootModelsCLI = originalModelsCLI
	}()

	var got modelscli.InspectConfig
	rootModelsCLI = modelsCLIServiceFunctions{
		inspect: func(cfg modelscli.InspectConfig) error {
			got = cfg
			return nil
		},
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "models", "inspect", "OMNIVOICE_Q4_K_M"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models inspect with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to InspectConfig.JSON")
	}
}

func TestWorkListCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	var got workcli.ListConfig
	listWork = func(cfg workcli.ListConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "work", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to ListConfig.JSON")
	}
}

func TestSubmitCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json",
		"submit",
		"--name", "request-name",
		"--work-type-name", "tasks",
		"--payload", "request.md",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to SubmitConfig.JSON")
	}
}

func TestFactoryDeleteCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalDeleteFactory := deleteFactory
	defer func() {
		deleteFactory = originalDeleteFactory
	}()

	var got factorycli.DeleteConfig
	deleteFactory = func(cfg factorycli.DeleteConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "factory", "delete", "staging"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory delete with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to DeleteConfig.JSON")
	}
}

func TestInitCommand_GlobalJSONIsRejectedBeforeInitService(t *testing.T) {
	originalInitFactory := initFactory
	defer func() {
		initFactory = originalInitFactory
	}()

	called := false
	initFactory = func(factorydefinitions.ScaffoldConfig) error {
		called = true
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "init"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--json is not supported by you init") {
		t.Fatalf("execute init with global --json error = %v", err)
	}
	if called {
		t.Fatal("init service called after global --json rejection")
	}
}

func TestModelsListCommand_DefaultServerAndJSONFlagMapToConfig(t *testing.T) {
	originalModelsCLI := rootModelsCLI
	defer func() {
		rootModelsCLI = originalModelsCLI
	}()

	var got modelscli.ListConfig
	rootModelsCLI = modelsCLIServiceFunctions{
		list: func(cfg modelscli.ListConfig) error {
			got = cfg
			return nil
		},
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "models", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models list: %v", err)
	}
	if got.Server != "" {
		t.Fatalf("server = %q, want empty when --server is manifest-default so owned Models CLI routing stays local", got.Server)
	}
	if !got.JSON {
		t.Fatal("expected --json to map to ListConfig.JSON")
	}
}

func TestModelsListCommand_JSONVerboseKeepsStdoutParseableAndDiagnosticsOnStderr(t *testing.T) {
	originalModelsCLI := rootModelsCLI
	defer func() {
		rootModelsCLI = originalModelsCLI
	}()

	rootModelsCLI = modelsCLIServiceFunctions{
		list: func(cfg modelscli.ListConfig) error {
			if !cfg.Verbose {
				t.Fatal("expected verbose config")
			}
			if cfg.Diagnostics == nil {
				t.Fatal("expected diagnostics writer")
			}
			if _, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: models list"); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cfg.Output, `{"results":[]}`)
			return err
		},
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "models", "list", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models list --json --verbose: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if _, ok := payload["results"]; !ok {
		t.Fatalf("stdout JSON = %#v, want results key", payload)
	}
	if got := stderr.String(); !strings.Contains(got, "diagnostic: models list") {
		t.Fatalf("stderr = %q, want diagnostics", got)
	}
}

func TestModelsInspectCommand_MapsModelArgumentAndServer(t *testing.T) {
	originalModelsCLI := rootModelsCLI
	defer func() {
		rootModelsCLI = originalModelsCLI
	}()

	var got modelscli.InspectConfig
	rootModelsCLI = modelsCLIServiceFunctions{
		inspect: func(cfg modelscli.InspectConfig) error {
			got = cfg
			return nil
		},
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "inspect", "OMNIVOICE_Q4_K_M", "--server", "http://127.0.0.1:9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models inspect: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model name = %q, want OMNIVOICE_Q4_K_M", got.ModelName)
	}
	if got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("server = %q, want http://127.0.0.1:9090", got.Server)
	}
}

func TestModelsInvokeCommand_MapsArgumentsAndFlags(t *testing.T) {
	originalModelsCLI := rootModelsCLI
	defer func() {
		rootModelsCLI = originalModelsCLI
	}()

	var got modelscli.InvokeConfig
	rootModelsCLI = modelsCLIServiceFunctions{
		invoke: func(cfg modelscli.InvokeConfig) error {
			got = cfg
			return nil
		},
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello", "--output", "speech.wav", "--server", "http://127.0.0.1:9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models invoke: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" || got.Operation != "TTS" || got.Text != "hello" || got.OutputPath != "speech.wav" || got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("invoke config = %#v, want mapped invoke args and flags", got)
	}
}

func TestModelsPullCommand_MapsArgumentsAndFlags(t *testing.T) {
	originalModelsCLI := rootModelsCLI
	defer func() {
		rootModelsCLI = originalModelsCLI
	}()

	var got modelscli.PullConfig
	rootModelsCLI = modelsCLIServiceFunctions{
		pull: func(cfg modelscli.PullConfig) error {
			got = cfg
			return nil
		},
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "--server", "http://127.0.0.1:9090", "models", "pull", "OMNIVOICE_Q4_K_M"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models pull: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" || got.Server != "http://127.0.0.1:9090" || !got.JSON {
		t.Fatalf("pull config = %#v, want mapped pull args and flags", got)
	}
}

func TestModelsCommand_HelpMentionsDiscoverySurface(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"Run local inference", "speech", "voice", "embeddings", "list", "inspect", "invoke", "pull", "Current Factory", "./factory/factory.json", "--server"} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("models help missing %q:\n%s", want, help)
		}
	}
	for _, stale := range []string{"Inspect discovered models from a running service", "shared in-process bootstrap", "OMNIVOICE_Q4_K_M"} {
		if bytes.Contains([]byte(help), []byte(stale)) {
			t.Fatalf("models help contains stale wording %q:\n%s", stale, help)
		}
	}
}

func TestWorkersACPCommandsValidateAndRouteRequests(t *testing.T) {
	t.Parallel()

	options := CommandFactory{
		homeDir: func() (string, error) { return t.TempDir(), nil },
		acp:     acpcli.Operations{},
	}
	tests := []struct {
		name     string
		args     []string
		want     string
		canceled bool
	}{
		{name: "list is removed", args: []string{"acp", "list"}, want: "unknown command"},
		{name: "add validates transport", args: []string{"acp", "add", "--name", "custom-acp", "--transport", "http", "--argument", "agent acp"}, want: "transport must be stdio"},
		{name: "add routes to service", args: []string{"acp", "add", "--name", "custom-acp", "--argument", "agent acp"}, want: "ID generator is required"},
		{name: "delete routes to service", args: []string{"acp", "delete", "--name", "custom-acp"}, want: "context canceled", canceled: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := productionWorkersCommand(options)
			command.SetOut(io.Discard)
			command.SetErr(io.Discard)
			command.SetArgs(test.args)
			if test.canceled {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				command.SetContext(ctx)
			}
			if err := command.Execute(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%v) error = %v, want containing %q", test.args, err, test.want)
			}
		})
	}
}

// TestProductionModelsCLICharacterizationValidation pins the exact validation
// messages returned by the public command/handler composition. The operation
// double is only reached by the JSON success case, so missing-input assertions
// cannot accidentally depend on a runtime, HTTP server, or model asset.
func TestProductionModelsCLICharacterizationValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing text",
			args: []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--json"},
			want: "--text is required",
		},
		{
			name: "missing output without JSON",
			args: []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--text", "hello"},
			want: "--output is required unless --json is set",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			operation := &modelsCLICharacterizationInvocation{}
			root, _ := newModelsCLICharacterizationRoot(t, operation)
			root.SetArgs(testCase.args)

			err := root.Execute()
			if err == nil || err.Error() != testCase.want {
				t.Fatalf("execute %v error = %v, want exactly %q", testCase.args, err, testCase.want)
			}
			if operation.calls != 0 {
				t.Fatalf("validation invoked model operation %d time(s), want none", operation.calls)
			}
		})
	}
}

func TestProductionModelsCLICharacterizationJSONBypassesOutputRequirement(t *testing.T) {
	operation := &modelsCLICharacterizationInvocation{}
	root, stdout := newModelsCLICharacterizationRoot(t, operation)
	root.SetArgs([]string{
		"models", "invoke", "OMNIVOICE_Q4_K_M", "--json", "--text", "hello",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute JSON invoke without --output: %v", err)
	}
	var response map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON response %q: %v", stdout.String(), err)
	}
	if operation.calls != 0 {
		t.Fatalf("model operation calls = %d, want 0 for validation-only metadata", operation.calls)
	}
	for key, want := range map[string]any{
		"operation":         "TTS",
		"mode":              "VALIDATION_ONLY",
		"validationOnly":    true,
		"inferenceExecuted": false,
	} {
		if response[key] != want {
			t.Fatalf("JSON %s = %#v, want %#v", key, response[key], want)
		}
	}
}

// TestProductionModelsCLICharacterizationSuccessExit pins the successful
// command boundary for the audio projection. The current zero-error exit is
// characterized, not endorsed: artifact production is supplied by the
// deterministic Models service test and this test only proves Cobra preserves
// its success through the public command composition.
func TestProductionModelsCLICharacterizationSuccessExit(t *testing.T) {
	var got modelscli.InvokeConfig
	factory := withTestInjectedPlatformRoles(NewCommandFactory(CommandOperations{ModelsCLI: modelsCLIServiceFunctions{
		invoke: func(cfg modelscli.InvokeConfig) error {
			got = cfg
			_, err := io.WriteString(cfg.Output, "Wrote audio: speech.wav\n")
			return err
		},
	}}))
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS",
		"--text", "hello world", "--output", "speech.wav",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute successful Models invoke: %v", err)
	}
	if got.OutputPath != "speech.wav" || got.Operation != "TTS" || got.Text != "hello world" {
		t.Fatalf("successful invoke config = %#v, want output/operation/text bindings", got)
	}
	if gotOutput, wantOutput := stdout.String(), "Wrote audio: speech.wav\n"; gotOutput != wantOutput {
		t.Fatalf("successful invoke stdout = %q, want %q", gotOutput, wantOutput)
	}
}

func TestProductionModelsCLICharacterizationRejectsInvalidInputsBeforeService(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing model name",
			args: []string{"models", "invoke"},
			want: "accepts 1 arg(s), received 0",
		},
		{
			name: "extra model name",
			args: []string{"models", "invoke", "model-a", "model-b"},
			want: "accepts 1 arg(s), received 2",
		},
		{
			name: "unknown flag",
			args: []string{"models", "invoke", "model-a", "--unknown"},
			want: "unknown flag: --unknown",
		},
		{
			name: "unsupported operation",
			args: []string{"models", "invoke", "model-a", "--operation", "INVALID"},
			want: "not one of the declared choices",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			called := false
			root := (CommandFactory{ModelsCLI: modelsCLIServiceFunctions{
				invoke: func(modelscli.InvokeConfig) error {
					called = true
					return nil
				},
			}}).NewCommand(nil, nil, nil)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(testCase.args)

			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("execute %v error = %v, want %q", testCase.args, err, testCase.want)
			}
			if called {
				t.Fatal("invalid Models input invoked the service")
			}
		})
	}
}

func TestProductionModelsCLICharacterizationRejectsChangedLegacyPort(t *testing.T) {
	called := false
	root := (CommandFactory{ModelsCLI: modelsCLIServiceFunctions{
		list: func(modelscli.ListConfig) error {
			called = true
			return nil
		},
	}}).NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "list", "--port", "7437"})

	err := root.Execute()
	want := "--port is no longer supported; use --server instead (for example, --server http://localhost:7437)"
	if err == nil || err.Error() != want {
		t.Fatalf("execute changed --port error = %v, want exactly %q", err, want)
	}
	if called {
		t.Fatal("changed legacy --port invoked Models service")
	}
}

type modelsCLICharacterizationInvocation struct {
	calls   int
	request modelinference.Request
}

func (operation *modelsCLICharacterizationInvocation) ResolveModelInvocationFactoryDir(explicit string) (string, error) {
	return explicit, nil
}

func (*modelsCLICharacterizationInvocation) ExportModelInvocationArtifact(string, string) error {
	return nil
}

func (operation *modelsCLICharacterizationInvocation) InvokeModel(
	_ context.Context,
	_ modelscli.InvocationTarget,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	operation.calls++
	operation.request = request
	return modelinference.Result{
		ModelName: modelName,
		Operation: operation.request.Operation,
	}, nil
}

func newModelsCLICharacterizationRoot(
	t *testing.T,
	operation *modelsCLICharacterizationInvocation,
) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	service := modelscli.New(rootTestHTTPProtocol(), operation)
	factory := withTestInjectedPlatformRoles(NewCommandFactory(CommandOperations{ModelsCLI: service}))
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	return root, &stdout
}
