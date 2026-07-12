package configinitcmd_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli"
	configinitcmd "github.com/portpowered/infinite-you/pkg/cli/configinit"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
)

func TestConfigInitCommand_MapsGlobalJSONFlagToInitConfig(t *testing.T) {
	originalRunInit := configinitcmd.RunInit
	defer func() {
		configinitcmd.RunInit = originalRunInit
	}()

	var got configinitcmd.InitConfig
	configinitcmd.RunInit = func(cfg configinitcmd.InitConfig) error {
		got = cfg
		return nil
	}

	root := cli.NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "config", "init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute config init: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected --json to map to InitConfig.JSON")
	}
}

func TestConfigInitCommand_FreshIsolatedHomeCreatesSystemConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	var stdout bytes.Buffer
	root := cli.NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute config init: %v", err)
	}

	configPath := defaultpaths.OperatorConfigPath(homeDir)
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Stat(configPath): %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Created system config at") {
		t.Fatalf("stdout = %q, want created system config message", got)
	}
	for _, name := range factoryconfig.BuiltInNamedFactoryNames() {
		if !strings.Contains(got, "Created packaged factory "+name) {
			t.Fatalf("stdout = %q, want created packaged factory message for %q", got, name)
		}
	}

	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	for _, name := range factoryconfig.BuiltInNamedFactoryNames() {
		wantDir, err := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, name)
		if err != nil {
			t.Fatalf("MapNamedFactoryDir(%q): %v", name, err)
		}
		if _, err := os.Stat(wantDir); err != nil {
			t.Fatalf("Stat(%q): %v", wantDir, err)
		}
	}
}

func TestConfigInitCommand_JSONFreshHomeEmitsStructuredSummary(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	var stdout bytes.Buffer
	root := cli.NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "config", "init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute config init --json: %v", err)
	}

	var payload configinitcmd.InitResult
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal stdout: %v\n%s", err, stdout.String())
	}
	if payload.SystemConfigOutcome != "created" {
		t.Fatalf("systemConfigOutcome = %q, want created", payload.SystemConfigOutcome)
	}
	if payload.ConfigPath != filepath.Clean(defaultpaths.OperatorConfigPath(homeDir)) {
		t.Fatalf("configPath = %q, want %q", payload.ConfigPath, defaultpaths.OperatorConfigPath(homeDir))
	}
}

func TestConfigInitCommand_HelpDocumentsInitSubcommand(t *testing.T) {
	var out bytes.Buffer
	root := cli.NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "init", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute config init --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{"config init", "operator/system config"} {
		if !strings.Contains(help, want) {
			t.Fatalf("config init help missing %q:\n%s", want, help)
		}
	}
}

func TestSystemConfigCommand_HelpDistinguishesFactoryConfigTooling(t *testing.T) {
	var out bytes.Buffer
	root := cli.NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute config --help: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "you factory config") {
		t.Fatalf("config help missing factory config distinction:\n%s", got)
	}
}

func TestConfigInitCommand_DoubleRunIsSuccessfulNoOp(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	root := cli.NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("first execute config init: %v", err)
	}

	configPath := defaultpaths.OperatorConfigPath(homeDir)
	configBefore, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config before rerun): %v", err)
	}

	var stdout bytes.Buffer
	root = cli.NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("second execute config init: %v", err)
	}

	configAfter, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config after rerun): %v", err)
	}
	if string(configAfter) != string(configBefore) {
		t.Fatalf("config changed on rerun:\nbefore:\n%s\nafter:\n%s", configBefore, configAfter)
	}

	got := stdout.String()
	if !strings.Contains(got, "System config already present at") {
		t.Fatalf("stdout = %q, want already-present system config message", got)
	}
	for _, name := range factoryconfig.BuiltInNamedFactoryNames() {
		if !strings.Contains(got, "Packaged factory "+name+" already present at") {
			t.Fatalf("stdout = %q, want already-present packaged factory message for %q", got, name)
		}
	}
}

func TestConfigInitCommand_ConfigCreationFailureReportsActionableError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	configPath := defaultpaths.OperatorConfigPath(homeDir)
	parentDir := filepath.Dir(configPath)
	if err := os.WriteFile(parentDir, []byte("blocks config directory creation\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(parent blocker): %v", err)
	}

	root := cli.NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "init"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected config init command failure")
	}
	got := err.Error()
	for _, want := range []string{
		"create system config at",
		"config.json",
		"is not a directory",
		"remove or rename",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want substring %q", got, want)
		}
	}
}

func TestConfigInitCommand_FactoryMaterializationFailureReportsActionableError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	if err := os.MkdirAll(namedFactoriesRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(namedFactoriesRoot): %v", err)
	}
	blocker := filepath.Join(namedFactoriesRoot, "@you")
	if err := os.WriteFile(blocker, []byte("blocks hierarchical factory layout\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(factory scope blocker): %v", err)
	}

	root := cli.NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "init"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected config init command failure")
	}
	got := err.Error()
	for _, want := range []string{
		"materialize packaged default factory",
		"@you/fusion",
		namedFactoriesRoot,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want substring %q", got, want)
		}
	}
}
