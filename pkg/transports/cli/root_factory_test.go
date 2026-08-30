package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	configcli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/config"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/spf13/cobra"
)

func TestLocalRunResolvesHomeOnceBeforeSystemInitialization(t *testing.T) {
	originalRunCLI := runCLI
	t.Cleanup(func() { runCLI = originalRunCLI })

	home := t.TempDir()
	workingFactory := t.TempDir()
	var stdout, stderr bytes.Buffer
	resolverCalls := 0
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		if cfg.HomeDir != home {
			t.Fatalf("run home = %q, want %q", cfg.HomeDir, home)
		}
		if cfg.StartupOutput == nil {
			t.Fatal("startup output is nil for a human local run")
		}
		_, err := fmt.Fprintln(cfg.StartupOutput, "Factory initiated: test")
		return err
	}

	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(func() (string, error) {
		resolverCalls++
		return home, nil
	}, func(string) (string, bool) { return "", false }, startupcli.Functions{
		InitializeSystemFunc: func(_ context.Context, initializedHome string) error {
			if initializedHome != home {
				t.Fatalf("initialized home = %q, want %q", initializedHome, home)
			}
			wantPrefix := "Home directory: " + home + "\n"
			if got := stdout.String(); got != wantPrefix {
				t.Fatalf("startup output before system initialization = %q, want %q", got, wantPrefix)
			}
			return nil
		},
		RunFunc: func(ctx context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
			cfg := testRunConfig(selection)
			if cfg.StartupPreparation == nil {
				return fmt.Errorf("run startup preparation is nil")
			}
			if err := cfg.StartupPreparation(ctx, true, cfg.StartupOutput); err != nil {
				return err
			}
			return runCLI(ctx, cfg)
		},
	})
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--dir", workingFactory, "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute local run: %v", err)
	}
	if resolverCalls != 1 {
		t.Fatalf("home resolver calls = %d, want one invocation-local resolution", resolverCalls)
	}
	if got, want := stdout.String(), "Home directory: "+home+"\nFactory initiated: test\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty human startup diagnostics", stderr.String())
	}
}

func TestPrepareRunSystemInitializationDefersOnlyProfileSelectedBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cfg          runcli.RunConfig
		changedFlag  string
		wantDeferred bool
	}{
		{
			name: "finite mock no-record batch",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				MockWorkersEnabled:      true,
				DisableDefaultRecording: true,
				Port:                    7437,
			},
			wantDeferred: true,
		},
		{
			name: "explicit recording",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				MockWorkersEnabled:      true,
				DisableDefaultRecording: false,
				RecordPath:              "recording.jsonl",
			},
		},
		{
			name: "replay",
			cfg: runcli.RunConfig{
				WorkFile:           "one-work.json",
				MockWorkersEnabled: true,
				ReplayPath:         "recording.jsonl",
			},
		},
		{
			name: "server",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				MockWorkersEnabled:      true,
				DisableDefaultRecording: true,
				WithServer:              true,
			},
		},
		{
			name: "continuous",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				MockWorkersEnabled:      true,
				DisableDefaultRecording: true,
				Continuously:            true,
			},
		},
		{
			name: "real workers",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				DisableDefaultRecording: true,
			},
		},
		{
			name: "bootstrap",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				MockWorkersEnabled:      true,
				DisableDefaultRecording: true,
				Bootstrap:               true,
			},
		},
		{
			name: "named Factory",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				MockWorkersEnabled:      true,
				DisableDefaultRecording: true,
				NamedFactoryName:        "@you/goal",
			},
		},
		{
			name: "listener",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				MockWorkersEnabled:      true,
				DisableDefaultRecording: true,
				ListenExplicit:          true,
			},
		},
		{
			name:        "explicit factory path",
			changedFlag: "factory",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				FactoryConfigPath:       "factory.json",
				MockWorkersEnabled:      true,
				DisableDefaultRecording: true,
			},
		},
		{
			name:        "explicit factory directory",
			changedFlag: "dir",
			cfg: runcli.RunConfig{
				WorkFile:                "one-work.json",
				Dir:                     "factory",
				MockWorkersEnabled:      true,
				DisableDefaultRecording: true,
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "run"}
			cmd.SetContext(context.Background())
			if test.changedFlag != "" {
				cmd.Flags().String(test.changedFlag, "", "")
				if err := cmd.Flags().Set(test.changedFlag, "selected"); err != nil {
					t.Fatalf("set changed flag %q: %v", test.changedFlag, err)
				}
			}
			options := CommandFactory{
				initializer: startupcli.Functions{
					InitializeSystemFunc: func(context.Context, string) error { return nil },
				},
			}
			allowed, err := prepareRunSystemInitialization(cmd, &test.cfg, options)
			if err != nil {
				t.Fatalf("prepareRunSystemInitialization() error = %v", err)
			}
			if got := !allowed; got != test.wantDeferred {
				t.Fatalf("deferred = %t, want %t", got, test.wantDeferred)
			}
		})
	}
}

func TestDeferredBatchSystemInitializationDoesNotInvokeInitializer(t *testing.T) {
	t.Parallel()

	calls := 0
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(context.Background())
	options := CommandFactory{
		initializer: startupcli.Functions{
			InitializeSystemFunc: func(context.Context, string) error {
				calls++
				return nil
			},
		},
	}
	cfg := runcli.RunConfig{
		WorkFile:                "one-work.json",
		MockWorkersEnabled:      true,
		DisableDefaultRecording: true,
	}
	if err := prepareRunFactoryStartup(cmd, &cfg, options, false); err != nil {
		t.Fatalf("prepareRunFactoryStartup() error = %v", err)
	}
	if cfg.StartupPreparation == nil {
		t.Fatal("StartupPreparation is nil")
	}
	if err := cfg.StartupPreparation(context.Background(), false, nil); err != nil {
		t.Fatalf("deferred StartupPreparation() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("InitializeSystem calls = %d, want 0 for deferred batch", calls)
	}
}

func TestExactFiniteMockBatchCommandDefersSystemInitialization(t *testing.T) {
	var initialized int
	var got runcli.RunConfig
	workingDirectory := t.TempDir()
	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		startupcli.Functions{
			InitializeSystemFunc: func(context.Context, string) error {
				initialized++
				return nil
			},
			RunFunc: func(_ context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
				got = testRunConfig(selection)
				return nil
			},
		},
	)
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), workingDirectory))
	root.SetArgs([]string{
		"run", "--work", "one-work.json", "--with-mock-workers=accept.json", "--no-record",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute finite mock batch command: %v", err)
	}
	if initialized != 0 {
		t.Fatalf("InitializeSystem calls = %d, want 0 for exact finite mock batch command", initialized)
	}
	if got.WorkFile != "one-work.json" || !got.MockWorkersEnabled || got.MockWorkersConfigPath != "accept.json" || !got.DisableDefaultRecording {
		t.Fatalf("parsed batch config = %+v, want work, mock worker, config path, and no-record inputs", got)
	}
}

func TestDemandedBatchSystemInitializationPreservesFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("controlled hosted system initialization failure")
	calls := 0
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(context.Background())
	options := CommandFactory{
		initializer: startupcli.Functions{
			InitializeSystemFunc: func(context.Context, string) error {
				calls++
				return wantErr
			},
		},
	}
	cfg := runcli.RunConfig{
		WorkFile:                "one-work.json",
		MockWorkersEnabled:      true,
		DisableDefaultRecording: true,
		WithServer:              true,
	}
	if err := prepareRunFactoryStartup(cmd, &cfg, options, false); err != nil {
		t.Fatalf("prepareRunFactoryStartup() error = %v", err)
	}
	if cfg.StartupPreparation == nil {
		t.Fatal("StartupPreparation is nil")
	}
	err := cfg.StartupPreparation(context.Background(), false, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("demanded StartupPreparation() error = %v, want sentinel %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("InitializeSystem calls = %d, want one demanded initialization", calls)
	}
}

func TestLocalRunHomeDisclosureKeepsJSONStdoutParseable(t *testing.T) {
	originalRunCLI := runCLI
	t.Cleanup(func() { runCLI = originalRunCLI })

	home := t.TempDir()
	workingFactory := t.TempDir()
	var stdout, stderr bytes.Buffer
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		output := cfg.Output
		if output == nil {
			output = cfg.StartupOutput
		}
		if output == nil {
			t.Fatal("JSON run output and startup output are nil")
		}
		_, err := fmt.Fprintln(output, `{"status":"started"}`)
		return err
	}

	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(func() (string, error) { return home, nil }, func(string) (string, bool) { return "", false }, startupcli.Functions{
		InitializeSystemFunc: func(context.Context, string) error { return nil },
		RunFunc: func(ctx context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
			return runCLI(ctx, testRunConfig(selection))
		},
	})
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "run", "--dir", workingFactory, "--with-server", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute JSON local run: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if result["status"] != "started" {
		t.Fatalf("JSON result = %#v, want started status", result)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty structured startup diagnostics", stderr.String())
	}
}

func TestLocalServerResolvesHomeOnceBeforeSystemInitialization(t *testing.T) {
	originalRunCLI := runCLI
	t.Cleanup(func() { runCLI = originalRunCLI })

	home := t.TempDir()
	workingDirectory := t.TempDir()
	var stdout, stderr bytes.Buffer
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		if cfg.HomeDir != home {
			t.Fatalf("server run home = %q, want %q", cfg.HomeDir, home)
		}
		if cfg.StartupOutput == nil {
			t.Fatal("startup output is nil for a human local server")
		}
		_, err := fmt.Fprintln(cfg.StartupOutput, "Factory initiated: test")
		return err
	}

	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(func() (string, error) { return home, nil }, os.LookupEnv, startupcli.Functions{
		InitializeSystemFunc: func(_ context.Context, initializedHome string) error {
			if initializedHome != home {
				t.Fatalf("initialized home = %q, want %q", initializedHome, home)
			}
			wantPrefix := "Home directory: " + home + "\n"
			if got := stdout.String(); got != wantPrefix {
				t.Fatalf("startup output before server system initialization = %q, want %q", got, wantPrefix)
			}
			return nil
		},
		RunFunc: func(ctx context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
			cfg := testRunConfig(selection)
			if cfg.StartupPreparation == nil {
				return fmt.Errorf("server startup preparation is nil")
			}
			if err := cfg.StartupPreparation(ctx, true, cfg.StartupOutput); err != nil {
				return err
			}
			return runCLI(ctx, cfg)
		},
	})
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), workingDirectory))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"server"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute local server: %v", err)
	}
	if got, want := stdout.String(), "Home directory: "+home+"\nFactory initiated: test\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty human startup diagnostics", stderr.String())
	}
}

func TestProductionMCPServeGeneratedMetadataDelegatesStdioInitializer(t *testing.T) {
	var got startupcli.MCPIntent
	initializeStdio := func(_ context.Context, intent startupcli.MCPIntent) error {
		got = intent
		return nil
	}
	stdin := strings.NewReader("protocol input")
	var stdout bytes.Buffer
	resolveHome := factorysessions.HomeDirectoryResolver(func() (string, error) {
		return t.TempDir(), nil
	})
	root := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI}).NewCommand(resolveHome, nil, startupcli.Functions{StdioFunc: initializeStdio})
	root.SetIn(stdin)
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"server", "mcp", "--runtime", "--project-root", "project"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute generated server mcp: %v", err)
	}
	if !got.RuntimeBacked || got.ProjectRoot != "project" {
		t.Fatalf("stdio intent = %#v, want generated flag values", got)
	}
	if got.Stdin != stdin || got.Stdout != &stdout {
		t.Fatalf("stdio = (%T, %T), want command streams", got.Stdin, got.Stdout)
	}

}

var removedFactoryConfigCommandPaths = []string{
	"you config flatten",
	"you config expand",
	"you factory validate",
}

var removedFactorySaveCommandPaths = []string{
	"you factory save",
}

func TestFactorySaveCommand_NotRegistered(t *testing.T) {
	root := newLegacyTestRootCommand()

	factoryCmd, _, err := root.Find([]string{"factory"})
	if err != nil {
		t.Fatalf("find factory: %v", err)
	}
	for _, child := range factoryCmd.Commands() {
		if child.Name() == "save" {
			t.Fatalf("factory must not register save as a direct child")
		}
	}
}

func TestFactorySaveCommand_DoesNotInvokeOwningPersistence(t *testing.T) {
	originalCreate := createFactoryFromFile
	originalReplace := replaceFactoryCurrent
	defer func() {
		createFactoryFromFile = originalCreate
		replaceFactoryCurrent = originalReplace
	}()

	createCalled := false
	replaceCalled := false
	createFactoryFromFile = func(factorycli.CreateFromFileConfig) error {
		createCalled = true
		return nil
	}
	replaceFactoryCurrent = func(factorycli.ReplaceCurrentConfig) error {
		replaceCalled = true
		return nil
	}

	cases := [][]string{
		{"factory", "save", "staging", "--from", "./factory.json"},
		{"factory", "save"},
		{"factory", "save", "staging"},
		{"factory", "nosuch"},
	}
	for _, args := range cases {
		root := newLegacyTestRootCommand()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(args)
		err := root.Execute()
		if err == nil {
			t.Fatalf("expected removed/unknown factory subcommand %v to fail", args)
		}
		if !strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("execute %v: got %v, want unknown-command error", args, err)
		}
	}
	if createCalled {
		t.Fatal("removed factory save must not invoke create persistence")
	}
	if replaceCalled {
		t.Fatal("removed factory save must not invoke replace-current persistence")
	}
}

func TestFactorySaveCommand_NoHiddenOrDeprecatedWrappers(t *testing.T) {
	root := newLegacyTestRootCommand()
	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("walk command tree: %v", err)
	}

	removed := make(map[string]struct{}, len(removedFactorySaveCommandPaths))
	for _, path := range removedFactorySaveCommandPaths {
		removed[path] = struct{}{}
	}

	for _, record := range inventory.Commands {
		if _, stillRegistered := removed[record.Path]; stillRegistered {
			t.Fatalf("removed path %q is still registered", record.Path)
		}
		if record.Visibility == "hidden" || record.Lifecycle == "deprecated" {
			for path := range removed {
				if record.Path == path {
					t.Fatalf("%s command %q reintroduces removed path", record.Visibility, record.Path)
				}
			}
		}
	}
}

func TestFactoryConfigCommand_OldPathsNotRegistered(t *testing.T) {
	root := newLegacyTestRootCommand()
	if _, _, err := root.Find([]string{"config", "init"}); err != nil {
		t.Fatalf("find config init: %v", err)
	}

	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("walk command tree: %v", err)
	}
	for _, path := range []string{"you config flatten", "you config expand"} {
		for _, record := range inventory.Commands {
			if record.Path == path {
				t.Fatalf("removed path %q is still registered", path)
			}
		}
	}

	factoryCmd, _, err := root.Find([]string{"factory"})
	if err != nil {
		t.Fatalf("find factory: %v", err)
	}
	for _, child := range factoryCmd.Commands() {
		if child.Name() == "validate" {
			t.Fatalf("factory must not register validate as a direct child")
		}
	}
}

func TestFactoryConfigCommand_OldPathsRejectAtRuntime(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "config flatten", args: []string{"config", "flatten", "./factory"}},
		{name: "config expand", args: []string{"config", "expand", "./factory.json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newLegacyTestRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(tc.args)
			if err := root.Execute(); err == nil {
				t.Fatal("expected removed command path to fail")
			}
		})
	}
}

func TestFactoryConfigCommand_DirectFactoryValidateDoesNotRun(t *testing.T) {
	originalValidateFactory := validateFactory
	defer func() {
		validateFactory = originalValidateFactory
	}()

	called := false
	validateFactory = func(factorycli.ValidateConfig) error {
		called = true
		return nil
	}

	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "validate", "./factory.json"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected removed factory validate to fail as unknown command")
	} else if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("factory validate error = %v, want unknown-command failure", err)
	}
	if called {
		t.Fatal("direct you factory validate must not invoke factory validation")
	}
	if out.Len() != 0 {
		t.Fatalf("factory validate should not write stdout, got:\n%s", out.String())
	}
}

func TestFactoryConfigCommand_ValidatePreservesSuccessAndFailureAtNewPath(t *testing.T) {
	dir := t.TempDir()
	validPath := writeRootFactoryConfigFixture(t, dir, "valid.json", rootFactoryConfigValidJSON())
	invalidPath := writeRootFactoryConfigFixture(t, dir, "invalid.json", rootFactoryConfigIncompatibleTaxonomyJSON())
	missingPath := filepath.Join(dir, "missing-factory.json")

	originalValidate := validateFactory
	t.Cleanup(func() { validateFactory = originalValidate })
	validateFactory = func(config factorycli.ValidateConfig) error {
		switch config.Path {
		case validPath:
			_, _ = fmt.Fprintln(config.Output, "Factory validation passed.")
			return nil
		case invalidPath:
			_, _ = fmt.Fprintln(
				config.Output,
				"Factory validation failed.\nworkstation-worker-behavior-compatibility",
			)
			return fmt.Errorf("factory validation found blocking issues")
		default:
			return fmt.Errorf("find factory config source")
		}
	}

	cases := []struct {
		name       string
		args       []string
		wantErr    bool
		errSubstr  string
		outSubstrs []string
	}{
		{
			name:       "valid fixture",
			args:       []string{"factory", "config", "validate", validPath},
			wantErr:    false,
			outSubstrs: []string{"Factory validation passed."},
		},
		{
			name:       "incompatible taxonomy",
			args:       []string{"factory", "config", "validate", invalidPath},
			wantErr:    true,
			errSubstr:  "factory validation found blocking issues",
			outSubstrs: []string{"Factory validation failed.", "workstation-worker-behavior-compatibility"},
		},
		{
			name:      "missing path",
			args:      []string{"factory", "config", "validate", missingPath},
			wantErr:   true,
			errSubstr: "find factory config source",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			root := newLegacyTestRootCommand()
			root.SetOut(&out)
			root.SetErr(io.Discard)
			root.SetArgs(tc.args)

			err := root.Execute()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected command to fail")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("error = %v, want substring %q", err, tc.errSubstr)
				}
			} else if err != nil {
				t.Fatalf("execute: %v", err)
			}
			for _, want := range tc.outSubstrs {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("output = %q, want substring %q", out.String(), want)
				}
			}
		})
	}
}

func TestFactoryConfigCommand_FlattenPreservesSuccessAndFailureAtNewPath(t *testing.T) {
	dir := t.TempDir()
	validPath := writeRootFactoryConfigFixture(t, dir, "factory.json", rootFactoryConfigValidJSON())
	invalidPath := writeRootFactoryConfigFixture(t, dir, "invalid.json", "{")
	missingPath := filepath.Join(dir, "missing-factory.json")
	originalFlatten := flattenFactoryConfig
	t.Cleanup(func() {
		flattenFactoryConfig = originalFlatten
	})
	flattenFactoryConfig = func(cfg configcli.FactoryConfigFlattenConfig) error {
		switch cfg.Path {
		case validPath:
			_, err := cfg.Output.Write([]byte(`{"name":"root-factory-config-valid"}`))
			return err
		case invalidPath:
			return fmt.Errorf("parse factory config: invalid JSON")
		default:
			return fmt.Errorf("find factory config source: %s", cfg.Path)
		}
	}

	cases := []struct {
		name      string
		args      []string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid fixture",
			args:    []string{"factory", "config", "flatten", validPath},
			wantErr: false,
		},
		{
			name:      "invalid json",
			args:      []string{"factory", "config", "flatten", invalidPath},
			wantErr:   true,
			errSubstr: "parse",
		},
		{
			name:      "missing path",
			args:      []string{"factory", "config", "flatten", missingPath},
			wantErr:   true,
			errSubstr: "find factory config source",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			root := newLegacyTestRootCommand()
			root.SetOut(&out)
			root.SetErr(io.Discard)
			root.SetArgs(tc.args)

			err := root.Execute()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected command to fail")
				}
				if tc.errSubstr != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errSubstr)) {
					t.Fatalf("error = %v, want substring %q", err, tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
				t.Fatalf("unmarshal flattened output: %v\n%s", err, out.String())
			}
			if payload["name"] != "root-factory-config-valid" {
				t.Fatalf("flattened name = %v, want root-factory-config-valid", payload["name"])
			}
		})
	}
}

func TestFactoryConfigCommand_ExpandPreservesSuccessAndFailureAtNewPath(t *testing.T) {
	dir := t.TempDir()
	validPath := writeRootFactoryConfigFixture(t, dir, "factory.json", rootFactoryConfigValidJSON())
	missingPath := filepath.Join(dir, "missing-factory.json")
	originalExpand := expandFactoryConfig
	t.Cleanup(func() {
		expandFactoryConfig = originalExpand
	})
	expandFactoryConfig = func(cfg configcli.FactoryConfigExpandConfig) error {
		if cfg.Path != validPath {
			return fmt.Errorf("find factory config source: %s", cfg.Path)
		}
		_, err := fmt.Fprintf(cfg.Output, "Expanded factory config into %s\n", dir)
		return err
	}

	cases := []struct {
		name       string
		args       []string
		wantErr    bool
		errSubstr  string
		outSubstrs []string
	}{
		{
			name:       "valid fixture",
			args:       []string{"factory", "config", "expand", validPath},
			wantErr:    false,
			outSubstrs: []string{"Expanded factory config into"},
		},
		{
			name:      "missing path",
			args:      []string{"factory", "config", "expand", missingPath},
			wantErr:   true,
			errSubstr: "find factory config source",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			root := newLegacyTestRootCommand()
			root.SetOut(&out)
			root.SetErr(io.Discard)
			root.SetArgs(tc.args)

			err := root.Execute()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected command to fail")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("error = %v, want substring %q", err, tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			for _, want := range tc.outSubstrs {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("output = %q, want substring %q", out.String(), want)
				}
			}
		})
	}
}

func TestFactoryConfigCommand_NoHiddenOrDeprecatedWrappers(t *testing.T) {
	root := newLegacyTestRootCommand()
	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("walk command tree: %v", err)
	}

	removed := make(map[string]struct{}, len(removedFactoryConfigCommandPaths))
	for _, path := range removedFactoryConfigCommandPaths {
		removed[path] = struct{}{}
	}

	for _, record := range inventory.Commands {
		if _, stillRegistered := removed[record.Path]; stillRegistered {
			t.Fatalf("removed path %q is still registered", record.Path)
		}
		if record.Visibility == "hidden" || record.Lifecycle == "deprecated" {
			for path := range removed {
				if record.Path == path {
					t.Fatalf("%s command %q reintroduces removed path", record.Visibility, record.Path)
				}
			}
		}
	}
}

func TestFactoryCommand_RegistersSubcommands(t *testing.T) {
	root := newLegacyTestRootCommand()
	for _, path := range [][]string{
		{"factory", "show"},
		{"factory", "list"},
		{"factory", "config"},
		{"factory", "config", "validate"},
		{"factory", "config", "flatten"},
		{"factory", "config", "expand"},
		{"factory", "create"},
		{"factory", "update"},
		{"factory", "replace-current"},
		{"factory", "delete"},
	} {
		if _, _, err := root.Find(path); err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
	}
}

func TestFactoryCommand_HelpDocumentsSubcommandsAndExamples(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"show",
		"list",
		"config",
		"create",
		"update",
		"replace-current",
		"delete",
		"global --server",
		"you factory show",
		"you factory config validate",
		"you factory list",
		"you factory create staging --from ./factory.json",
		"you factory update staging --from ./factory.json",
		"you factory delete staging",
		"you factory replace-current",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory help missing %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "--port") {
		t.Fatalf("factory help must not advertise --port:\n%s", help)
	}
	if strings.Contains(help, "you factory validate") {
		t.Fatalf("factory help must not advertise direct you factory validate:\n%s", help)
	}
	if strings.Contains(help, "you factory save") {
		t.Fatalf("factory help must not advertise removed you factory save:\n%s", help)
	}
}

func TestFactoryConfigCommand_HelpDocumentsSubcommandsAndExamples(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "config", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory config --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"validate",
		"flatten",
		"expand",
		"you factory config validate ./factory.json",
		"you factory config flatten ./factory",
		"you factory config expand ./factory.json",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory config help missing %q:\n%s", want, help)
		}
	}
}

func TestFactoryListCommand_HelpDocumentsProjectAndGlobalRoots(t *testing.T) {
	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "list", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory list --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"project-local named factories from ./factory",
		"~/.you-agent-factory/factories",
		"never merges project-local and global entries",
		"you factory list --dir ~/.you-agent-factory/factories",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory list help missing %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "~/.you-agent-factory/you-agent-factories") {
		t.Fatalf("factory list help advertises retired global root:\n%s", help)
	}
}

func TestFactoryShowCommand_ServerFlagReachesHTTPTestServer(t *testing.T) {
	factoryDir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory-sessions/~default/factory" {
			t.Fatalf("path = %q, want /factory-sessions/~default/factory", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.Factory{
			Name:             apisurface.DefaultCurrentFactoryName,
			FactoryDirectory: &factoryDir,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

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
	root.SetArgs([]string{"factory", "show", "--server", strings.TrimSuffix(srv.URL, "/")})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory show --server: %v", err)
	}
	if got.Server != strings.TrimSuffix(srv.URL, "/") {
		t.Fatalf("server = %q, want %q", got.Server, strings.TrimSuffix(srv.URL, "/"))
	}
}

func writeRootFactoryConfigFixture(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func rootFactoryConfigValidJSON() string {
	return `{
  "name": "root-factory-config-valid",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "done", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "legacy",
    "type": "MODEL_WORKER",
    "operations": [{
      "name": "TTS",
      "inputs": [{"name": "text", "contentTypes": ["TEXT"]}],
      "outputs": [{"name": "audio", "contentTypes": ["AUDIO"]}]
    }]
  }],
  "workstations": [{
    "name": "legacy-run",
    "type": "MODEL_INVOKE",
    "operation": "TTS",
    "worker": "legacy",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "done"}]
  }]
}`
}

func rootFactoryConfigIncompatibleTaxonomyJSON() string {
	return `{
  "name": "root-factory-config-invalid",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "done", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "infer",
    "type": "INFERENCE_WORKER",
    "operations": [{
      "name": "TTS",
      "inputs": [{"name": "text", "contentTypes": ["TEXT"]}],
      "outputs": [{"name": "audio", "contentTypes": ["AUDIO"]}]
    }]
  }],
  "workstations": [{
    "name": "agent-with-infer",
    "type": "AGENT_RUN",
    "worker": "infer",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "done"}]
  }]
}`
}

func TestFactoryShowCommand_PortFlagRejected(t *testing.T) {
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "show", "--port", "9090"})

	if execErr := root.Execute(); execErr == nil {
		t.Fatal("expected --port rejection")
	} else if !strings.Contains(execErr.Error(), "--server") {
		t.Fatalf("error = %v, want --server guidance", execErr)
	}
}

func TestFactoryQueryCommand_IsUnknownAfterRename(t *testing.T) {
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "query"`) {
		t.Fatalf("execute retired factory query = %v, want unknown command", err)
	}
}

func TestProductionFactoryConfigInitCommandsUsesGeneratedFamily(t *testing.T) {
	commands := productionFactoryConfigInitCommands(&cliDiagnosticsOptions{}, CommandFactory{})
	if commands.Factory == nil || commands.Config == nil || commands.Init == nil {
		t.Fatalf("production commands = %#v, want factory/config/init", commands)
	}
	if _, _, err := commands.Factory.Find([]string{"show"}); err != nil {
		t.Fatalf("generated factory tree missing show: %v", err)
	}
}
