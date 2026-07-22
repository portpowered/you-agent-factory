package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

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
	root := CommandFactory{ModelsCLI: legacyModelsCLIService{}}.NewCommand(resolveHome, nil, startupcli.Functions{StdioFunc: initializeStdio})
	root.SetIn(stdin)
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"mcp", "serve", "--runtime", "--project-root", "project"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute generated mcp serve: %v", err)
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
		{"factory", "query"},
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
		"query",
		"list",
		"config",
		"create",
		"update",
		"replace-current",
		"delete",
		"global --server",
		"you factory query",
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

func TestFactoryQueryCommand_ServerFlagReachesHTTPTestServer(t *testing.T) {
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
	root.SetArgs([]string{"factory", "query", "--server", strings.TrimSuffix(srv.URL, "/")})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query --server: %v", err)
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

func TestFactoryQueryCommand_PortFlagRejected(t *testing.T) {
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query", "--port", "9090"})

	if execErr := root.Execute(); execErr == nil {
		t.Fatal("expected --port rejection")
	} else if !strings.Contains(execErr.Error(), "--server") {
		t.Fatalf("error = %v, want --server guidance", execErr)
	}
}

func TestNewFactoryConfigInitHandlerRegistryWiresContractedRunnableIDs(t *testing.T) {
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	state := factoryConfigInitBindingState{
		listDir:      defaultcmd.FactoryDir,
		createDir:    defaultcmd.FactoryDir,
		updateDir:    defaultcmd.FactoryDir,
		deleteDir:    defaultcmd.FactoryDir,
		initDir:      defaultcmd.FactoryDir,
		initType:     string(factorydefinitions.DefaultScaffoldType),
		initExecutor: factorydefinitions.DefaultStarterExecutor,
	}
	registry, err := newFactoryConfigInitHandlerRegistry(globals, diagnostics, CommandFactory{}, &state)
	if err != nil {
		t.Fatalf("newFactoryConfigInitHandlerRegistry() error = %v", err)
	}
	for _, commandID := range []string{
		"you.factory.query",
		"you.factory.list",
		"you.factory.create",
		"you.factory.update",
		"you.factory.delete",
		"you.factory.replace-current",
		"you.factory.config.validate",
		"you.factory.config.flatten",
		"you.factory.config.expand",
		"you.config.init",
		"you.init",
	} {
		if _, lookupErr := registry.Lookup(commandID); lookupErr != nil {
			t.Fatalf("Lookup(%q) error = %v", commandID, lookupErr)
		}
	}
}
func TestNewFactoryConfigInitWiringReturnsRegistryAndBindings(t *testing.T) {
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	registry, bindings, err := newFactoryConfigInitWiring(globals, diagnostics, CommandFactory{})
	if err != nil {
		t.Fatalf("newFactoryConfigInitWiring() error = %v", err)
	}
	if registry == nil {
		t.Fatal("registry = nil, want handler registry")
	}
	if bindings.FactoryListDir == nil || bindings.InitDir == nil {
		t.Fatalf("bindings = %#v, want populated flag pointers", bindings)
	}
}

func TestFactoryConfigInitFlagUsagesIncludesContractedKeys(t *testing.T) {
	usages := factoryConfigInitFlagUsages()
	for _, key := range []string{"dir", "from", "set-current", "session", "type", "executor"} {
		if usages[key] == "" {
			t.Fatalf("factoryConfigInitFlagUsages() missing %q", key)
		}
	}
}

func TestProductionFactoryConfigInitCommandsUsesGeneratedFamily(t *testing.T) {
	commands := productionFactoryConfigInitCommands(&cliGlobalOptions{}, &cliDiagnosticsOptions{}, CommandFactory{})
	if commands.Factory == nil || commands.Config == nil || commands.Init == nil {
		t.Fatalf("production commands = %#v, want factory/config/init", commands)
	}
	if _, _, err := commands.Factory.Find([]string{"query"}); err != nil {
		t.Fatalf("generated factory tree missing query: %v", err)
	}
}
