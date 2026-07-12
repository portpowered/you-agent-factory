package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configinitcmd "github.com/portpowered/infinite-you/pkg/cli/configinit"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
)

func TestConfigInitCommand_MapsGlobalJSONFlagToInitConfig(t *testing.T) {
	originalConfigInit := configInit
	defer func() {
		configInit = originalConfigInit
	}()

	var got configinitcmd.InitConfig
	configInit = func(cfg configinitcmd.InitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
