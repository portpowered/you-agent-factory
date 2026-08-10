package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
)

func TestRunCommand_VerboseFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --verbose: %v", err)
	}

	if !got.Verbose {
		t.Fatal("expected --verbose to enable service verbose logging")
	}
	if got.Logger == nil {
		t.Fatal("expected run command to set logger")
	}
}

func TestRunCommand_VerboseDiagnosticsUseStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		if !cfg.Verbose {
			t.Fatal("expected verbose run config")
		}
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		_, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: run startup")
		return err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --verbose: %v", err)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no diagnostic output", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "diagnostic: run startup") {
		t.Fatalf("stderr = %q, want run diagnostics", got)
	}
}

func TestRunCommand_NamedFactoryResolutionMetadataFlowsIntoRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	projectFactoryDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(projectFactoryDir, interfaces.FactoryConfigFile),
		portableFactoryPayloadWithDefaultHandling(),
		0o600,
	); err != nil {
		t.Fatalf("write detached Factory fixture: %v", err)
	}

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithCatalog(rootNamedFactoryCatalogFake{
		resolve: func(projectRoot, globalRoot, name string) (*interfaces.NamedFactoryResolution, error) {
			return &interfaces.NamedFactoryResolution{
				Name:               name,
				FactoryDir:         projectFactoryDir,
				Source:             interfaces.NamedFactoryResolutionSourceProjectLocal,
				ProjectRoot:        projectRoot,
				GlobalRoot:         globalRoot,
				PrecedenceDecision: interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal,
			}, nil
		},
	})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "alpha", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named alpha: %v", err)
	}
	if got.NamedFactoryResolution == nil {
		t.Fatal("expected named-factory resolution metadata")
	}
	if got.Dir != got.NamedFactoryResolution.FactoryDir {
		t.Fatalf("run dir = %q, want resolved named-factory dir %q", got.Dir, got.NamedFactoryResolution.FactoryDir)
	}
	if got.NamedFactoryResolution.Source != interfaces.NamedFactoryResolutionSourceProjectLocal {
		t.Fatalf("resolution source = %q, want %q", got.NamedFactoryResolution.Source, interfaces.NamedFactoryResolutionSourceProjectLocal)
	}
	if got.NamedFactoryResolution.PrecedenceDecision != interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal {
		t.Fatalf("resolution precedence = %q, want %q", got.NamedFactoryResolution.PrecedenceDecision, interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal)
	}
}

func TestRunCommand_UnknownRunnerFlagReturnsCobraError(t *testing.T) {
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--runner", "codex"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unknown --runner flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "--runner") {
		t.Fatalf("error = %q, want unknown --runner flag error", err.Error())
	}
}

func TestRootAndRunHelp_HideRemovedDefaultWorkerModelFlagsAndRunner(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	for name, cmd := range map[string]*cobra.Command{"root": root, "run": runCmd} {
		if strings.Contains(cmd.Long, "--runner") {
			t.Fatalf("%s long help still mentions --runner:\n%s", name, cmd.Long)
		}
		if cmd.Root().PersistentFlags().Lookup("default-worker-model-provider") != nil {
			t.Fatalf("%s still exposes --default-worker-model-provider", name)
		}
		if cmd.Root().PersistentFlags().Lookup("default-worker-model") != nil {
			t.Fatalf("%s still exposes --default-worker-model", name)
		}
	}
}

func TestRootCommand_DefaultWorkerModelProviderFlagMapsToRunConfig(t *testing.T) {
	removedRoot := newLegacyTestRootCommand()
	removedRoot.SetArgs([]string{"run", "--default-worker-model-provider", "codex"})
	if err := removedRoot.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("removed provider flag error = %v, want unknown flag", err)
	}
}

func TestRootCommand_ExplicitEnvironmentIsIsolatedAndFlagsRetainPrecedence(t *testing.T) {
	homeDir := t.TempDir()

	originalRunCLI := runCLI
	defer func() { runCLI = originalRunCLI }()
	var got []runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = append(got, cfg)
		return nil
	}

	newCommand := func(
		environment map[string]string,
		wantEnvironment operatorconfig.Defaults,
		wantFlags operatorconfig.FlagOverrides,
		result operatorconfig.ResolvedDefaults,
	) *cobra.Command {
		factory := withTestInjectedPlatformRoles(CommandFactory{})
		factory.resolveOperatorDefaults = expectOperatorDefaultsResolution(t, wantEnvironment, wantFlags, result, nil)
		command := factory.NewCommand(
			func() (string, error) { return homeDir, nil },
			func(name string) (string, bool) {
				value, ok := environment[name]
				return value, ok
			},
			startupcli.Functions{
				RunFunc: func(ctx context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
					return runCLI(ctx, testRunConfig(selection))
				},
			},
		)
		command.SetContext(startupcli.WithWorkingDirectory(t.Context(), t.TempDir()))
		return command
	}

	first := newCommand(
		map[string]string{operatorconfig.EnvDefaultWorkerModelProvider: "codex"},
		operatorconfig.Defaults{WorkerModelProvider: "codex"},
		operatorconfig.FlagOverrides{},
		operatorconfig.ResolvedDefaults{WorkerModelProvider: "CODEX", WorkerModelProviderSource: operatorconfig.SourceEnv},
	)
	first.SetOut(io.Discard)
	first.SetErr(io.Discard)
	first.SetArgs([]string{"run", "--no-record"})
	if err := first.Execute(); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	second := newCommand(
		map[string]string{
			operatorconfig.EnvDefaultWorkerModelProvider: "codex",
			runcli.ModelCacheDirEnvironment:              "/customer/model-cache",
		},
		operatorconfig.Defaults{WorkerModelProvider: "codex"},
		operatorconfig.FlagOverrides{},
		operatorconfig.ResolvedDefaults{WorkerModelProvider: "CODEX", WorkerModelProviderSource: operatorconfig.SourceEnv},
	)
	second.SetOut(io.Discard)
	second.SetErr(io.Discard)
	second.SetArgs([]string{"run"})
	if err := second.Execute(); err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}

	third := newCommand(
		map[string]string{operatorconfig.EnvDefaultWorkerModelProvider: ""},
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{},
		operatorconfig.ResolvedDefaults{WorkerModelProvider: "CLAUDE", WorkerModelProviderSource: operatorconfig.SourceFile},
	)
	third.SetOut(io.Discard)
	third.SetErr(io.Discard)
	third.SetArgs([]string{"run"})
	if err := third.Execute(); err != nil {
		t.Fatalf("third Execute() error = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("run calls = %d, want 3", len(got))
	}
	if got[0].OperatorDefaults.WorkerModelProvider != "CODEX" || got[0].OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceEnv {
		t.Fatalf("first defaults = %+v, want CODEX from environment", got[0].OperatorDefaults)
	}
	if got[1].OperatorDefaults.WorkerModelProvider != "CODEX" || got[1].OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceEnv {
		t.Fatalf("second defaults = %+v, want CODEX from environment", got[1].OperatorDefaults)
	}
	if got[1].ModelCacheDir != "/customer/model-cache" {
		t.Fatalf("second model cache dir = %q, want environment value", got[1].ModelCacheDir)
	}
	if got[2].ModelCacheDir != "" {
		t.Fatalf("third model cache dir = %q, want isolated empty value", got[2].ModelCacheDir)
	}
	if got[2].OperatorDefaults.WorkerModelProvider != "CLAUDE" || got[2].OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFile {
		t.Fatalf("third defaults = %+v, want CLAUDE from file", got[2].OperatorDefaults)
	}
}

func TestRootCommand_DefaultWorkerModelFlagMapsToRunConfig(t *testing.T) {
	removedRoot := newLegacyTestRootCommand()
	removedRoot.SetArgs([]string{"run", "--default-worker-model", "gpt-5-codex"})
	if err := removedRoot.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("removed model flag error = %v, want unknown flag", err)
	}
}

func TestRootCommand_RunProviderAndModelOverrideOperatorDefaults(t *testing.T) {
	originalRunCLI := runCLI
	defer func() { runCLI = originalRunCLI }()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommandWithOperatorDefaults(expectOperatorDefaultsResolution(
		t,
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{WorkerModelProvider: "cursor-acp", WorkerModel: "auto"},
		operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "cursor-acp",
			WorkerModel:               "auto",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
			WorkerModelSource:         operatorconfig.SourceFlag,
		},
		nil,
	))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--provider", "cursor-acp", "--model", "auto", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with provider/model overrides: %v", err)
	}
	if got.OperatorDefaults.WorkerModelProvider != "cursor-acp" || got.OperatorDefaults.WorkerModel != "auto" {
		t.Fatalf("defaults = %+v, want cursor-acp/auto", got.OperatorDefaults)
	}
	if got.OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFlag || got.OperatorDefaults.WorkerModelSource != operatorconfig.SourceFlag {
		t.Fatalf("override sources = %+v, want flag", got.OperatorDefaults)
	}
}

func TestRootCommand_RunWorkerReasoningEffortMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() { runCLI = originalRunCLI }()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--worker-reasoning-effort", " XHIGH ", "--no-record"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with worker reasoning effort: %v", err)
	}
	if got.WorkerReasoningEffort != " XHIGH " {
		t.Fatalf("worker reasoning effort = %q, want raw CLI value preserved until run validation", got.WorkerReasoningEffort)
	}
}

func TestRootCommand_ExplicitRunHonorsDefaultWorkerModelFlags(t *testing.T) {
	removedRoot := newLegacyTestRootCommand()
	removedRoot.SetArgs([]string{"run", "--default-worker-model-provider", "codex"})
	if err := removedRoot.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("removed provider flag error = %v, want unknown flag", err)
	}
}

func TestRootCommand_DefaultProviderFlagRejectsUnresolvedSymbolicDefault(t *testing.T) {
	removedRoot := newLegacyTestRootCommand()
	removedRoot.SetArgs([]string{"run", "--default-worker-model-provider", "DEFAULT"})
	if err := removedRoot.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("removed provider flag error = %v, want unknown flag", err)
	}
}

func TestRootCommand_DefaultProviderFlagResolvesSymbolicDefaultFromFile(t *testing.T) {
	removedRoot := newLegacyTestRootCommand()
	removedRoot.SetArgs([]string{"run", "--default-worker-model-provider", "DEFAULT"})
	if err := removedRoot.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("removed provider flag error = %v, want unknown flag", err)
	}
}

type transportNamedFactoryCatalog map[string]string

func (catalog transportNamedFactoryCatalog) ListNamedFactories(string) ([]interfaces.NamedFactoryListEntry, error) {
	entries := make([]interfaces.NamedFactoryListEntry, 0, len(catalog))
	for name, factoryDir := range catalog {
		entries = append(entries, interfaces.NamedFactoryListEntry{Name: name, FactoryDir: factoryDir})
	}
	return entries, nil
}

func (transportNamedFactoryCatalog) DeleteNamedFactory(string, string) error {
	return nil
}

func (catalog transportNamedFactoryCatalog) ResolveNamedFactoryAcrossRoots(
	projectRoot string,
	globalRoot string,
	name string,
) (*interfaces.NamedFactoryResolution, error) {
	factoryDir, ok := catalog[name]
	if !ok {
		return nil, fmt.Errorf("named factory %q not found", name)
	}
	return &interfaces.NamedFactoryResolution{
		Name:               name,
		FactoryDir:         factoryDir,
		Source:             interfaces.NamedFactoryResolutionSourceGlobal,
		ProjectRoot:        projectRoot,
		GlobalRoot:         globalRoot,
		PrecedenceDecision: interfaces.NamedFactoryPrecedenceDecisionNone,
	}, nil
}

func newTransportNamedFactoryRoot(t *testing.T, names ...string) *cobra.Command {
	return newTransportNamedFactoryRootWithInvocation(t, rootInvocationInputScript{}, names...)
}

func newTransportNamedFactoryRootWithInvocation(
	t *testing.T,
	prepare rootInvocationInputScript,
	names ...string,
) *cobra.Command {
	t.Helper()

	workingDirectory := t.TempDir()
	homeDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	})
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	catalog := make(transportNamedFactoryCatalog, len(names))
	for _, name := range names {
		factoryDir := t.TempDir()
		payload := strings.Replace(goalFailureBaselineNamedFactoryJSON, "@you/goal", name, 1)
		if err := os.WriteFile(
			filepath.Join(factoryDir, interfaces.FactoryConfigFile),
			[]byte(payload),
			0o644,
		); err != nil {
			t.Fatalf("write named Factory fixture: %v", err)
		}
		catalog[name] = factoryDir
	}
	return newLegacyTestRootCommandWithCatalogAndInvocationInput(catalog, prepare)
}

func TestRunCommand_VerboseDiagnosticsIncludeOperatorDefaultPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var diagnostics bytes.Buffer
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		_, err := cfg.Diagnostics.Write([]byte(cfg.OperatorDefaults.DiagnosticsLine() + "\n"))
		return err
	}

	root := newLegacyTestRootCommandWithOperatorDefaults(expectOperatorDefaultsResolution(
		t,
		operatorconfig.Defaults{},
		operatorconfig.FlagOverrides{WorkerModelProvider: "codex", WorkerModel: "gpt-5-codex"},
		operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModel:               "gpt-5-codex",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
			WorkerModelSource:         operatorconfig.SourceFlag,
		},
		nil,
	))
	root.SetOut(io.Discard)
	root.SetErr(&diagnostics)
	root.SetArgs([]string{"run", "--verbose", "--provider", "codex", "--model", "gpt-5-codex", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with verbose operator defaults: %v", err)
	}

	got := diagnostics.String()
	for _, want := range []string{
		"operatorDefaults precedence=file < env < flag",
		"provider=CODEX",
		"providerSource=flag",
		"model=gpt-5-codex",
		"modelSource=flag",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
}
